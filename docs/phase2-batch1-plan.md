# 二期 · 第一批快修 · 实施方案

> 本文是二期（phase-2）**第一批**工作的可执行方案。覆盖 5 项快修，跨 `bililive-go-UI`（Go 后端 + Web 前端）与 `bililive-ios`（SwiftUI）。
>
> **分工**：实现由实现者编写代码；方案作者负责出方案 + 每项完成后跑编译/测试并对照"验收"逐条核对（红 → 绿），**不以"能编译"作为放行标准**。
>
> 路径约定：`src/...` 为 `bililive-go-UI` 仓库内路径；iOS 文件标注 `bililive-ios/...`（同级仓库）。

## 验收契约（Definition of Done）

| 类型 | 真实验证手段 | 通过标准 |
|------|------------|---------|
| Go 后端 | `make dev` 后真跑二进制 + curl 打真实接口 | bug 先复现红、再修绿；逐条对照行为 |
| Web 前端 | 跑后端 + 浏览器/接口联调 | 行为符合计划，刷新后状态持久正确 |
| iOS | `xcodebuild` 编译 + 对照规格代码评审 + 真机测试脚本 | 编译过 + 评审过；真机运行由用户确认 |

---

## A. 修复 API Key 删除（任务 #2 · 纯后端）

**决策：方案二（真实删除 + 连带清理观看历史）。**

### 根因（已确认）
- `src/pkg/metadata/api_key_users.go:198` `RevokeAPIKeyUser` 是**软删**：`UPDATE ... SET enabled=0, revoked_at=CURRENT_TIMESTAMP`。
- `src/pkg/metadata/api_key_users.go:61` `ListAPIKeyUsers` 的 `SELECT` **没有 `WHERE revoked_at IS NULL`**。
- 结果：点"删除"后该行仍在列表里、只是 `enabled=0`，与"停用"无法区分 → 表现为"能停用、不能删除"。

### 改动文件与改法
1. `src/pkg/metadata/api_key_users.go`
   - 新增 `DeleteAPIKeyUser(ctx, id)`：在**一个事务**里执行
     ```sql
     DELETE FROM watch_history   WHERE api_key_user_id = ?;
     DELETE FROM api_key_users    WHERE id = ?;
     ```
     （`watch_history.api_key_user_id` 列名见 `src/pkg/metadata/store.go:439`。先删历史再删用户，避免孤儿数据。）
   - 若 `id` 不存在，`DELETE` 影响 0 行，视为成功（幂等）即可。
2. `src/servers/handler.go:3668` `deleteAPIKeyUser`
   - 改为调用 `store.DeleteAPIKeyUser(...)`（替换原 `RevokeAPIKeyUser`）。
3. 收尾检查
   - 全仓 grep `RevokeAPIKeyUser`，确认除该 handler 外**无其他调用方**；若无，可删除旧函数，或保留但不再被引用（实现者判断）。
   - `ListAPIKeyUsers` 在方案二下可不动（真删后行已消失）。但库里若已有历史软删行（`revoked_at` 非空）会继续显示——**建议**顺手给 `ListAPIKeyUsers` 也加 `WHERE revoked_at IS NULL`，清掉历史残留显示。

### 验收
- `make dev` → 跑二进制（带 `-c config.yml`）。
- 建 Key：`POST /api/api-keys {"name":"t1"}` → 记下 `id`。
- 删除：`DELETE /api/api-keys/{id}` 返回 ok。
- 复核：`GET /api/api-keys` **列表中不再出现该 id**（红→绿）。
- 数据库复核：该用户的 `watch_history` 行也已清空。
- 对照"停用"：另建 Key → `PATCH {"enabled":false}` → 仍在列表且标"已禁用"，证明删除与停用行为有别。

### 风险
- 真删不可恢复；这是用户明确选择。watch_history 连带清理是预期内（删用户=连同其观看进度一起清）。

---

## B. 修复 Cookie 不能修改（任务 #3 · Web 前端为主）

### 根因（已定位）
- `src/webapp/src/component/edit-cookie/index.tsx` 的 `handleOk`（约 :43-45）：
  先 `this.api.saveCookie({Host, Cookie})`（已经 `PUT /cookies` → `putLiveHostCookie` → `configs.SetCookie` 落库，见 `src/configs/config.go:565`），
  **紧接着又 `this.api.saveSettingsInBackground()`** → `saveSettings()` 把前端内存里**不含新 cookie** 的整份 config PUT 回服务端 → 覆盖刚保存的 cookie。
- `SetCookie` 本身正确；问题在前端这次多余的整体保存把它冲掉。

### 改动文件与改法
1. `src/webapp/src/component/edit-cookie/index.tsx`
   - 删除 `handleOk` 中 `saveCookie` 之后的 `saveSettingsInBackground()` 调用。cookie 已由 `PUT /cookies` 单独落库，无需再整体保存。
   - 保存成功后仅需 `this.props.refresh()` 刷新列表（保留现有刷新逻辑）。
2. 附带核对
   - 解析工具 tab 的 Douyin cookie 走独立路径 `PUT /api/config/douyin-cookie`（`saveDouyinCookie`，`src/webapp/src/utils/api.ts:406`）。复现确认它没有同样"保存后被整体 settings 覆盖"的问题（预期没有，因为它不触发 saveSettings）。

### 验收
- 跑后端 → Web 打开 cookie 编辑 → 改某 host 的 cookie → 保存。
- `GET /api/cookies`（或直接看 `config.yml` 的 `cookies:`）确认新值已写入。
- **刷新页面后再查**，确认 cookie 仍是新值（证明没有被后续 `saveSettings` 冲掉）——这是复现红→绿的关键一步。

### 风险
- 若别处依赖"编辑 cookie 顺带整体保存设置"的副作用（不太可能），需补一次精准的 config 刷新。实现者复现时留意。

---

## C. iOS 倍速选择菜单（任务 #4 · iOS）

### 根因（已确认）
- `bililive-ios/Live OS/Live OS/Views/PlayerView.swift:810-815`：右下角 `Text(String(format: "%.2gx", currentSpeed))` 是纯标签，无任何点击手势 → 点了没反应。
- `setSpeed(_:)` 已存在（`:621`）。

### 改动文件与改法
1. `PlayerView.swift` `PlayerBottomControls`
   - 把右下角那个倍速 `Text` 包进 `Menu`：
     ```swift
     Menu {
         ForEach([0.5, 1.0, 1.25, 1.5, 2.0], id: \.self) { s in
             Button("\(formatted(s))x") { setSpeed(Float(s)) }
         }
     } label: { /* 现有胶囊样式的 Text */ }
     ```
   - `setSpeed` 需从父视图传入（当前 `PlayerBottomControls` 只接收 `currentSpeed`，要把 `setSpeed` 闭包也传进来）。
2. **与长按 2x 的状态协调（重要）**
   - 现在 `handleLongPressEnded` 写死 `setSpeed(1.0)`（`:455`），会把菜单设定值也打回 1x。
   - 引入 `@State baseSpeed: Float = 1`，菜单选择时更新 `baseSpeed`；长按临时 2x 结束时恢复为 `baseSpeed` 而非写死 1.0。
   - 锁定 2x（长按下拉锁定）逻辑不变，但解锁时恢复到 `baseSpeed`。

### 验收
- 方案作者跑 `xcodebuild` 编译检查。
- 真机脚本（用户执行）：
  1. 点右下角倍速胶囊 → 弹出菜单 → 选 1.5x → 画面变 1.5x。
  2. 长按屏幕侧边触发临时 2x → 松手 → **回到 1.5x（不是 1x）**。
  3. 长按下拉锁定 2x → 再长按下拉解锁 → 回到 1.5x。

### 风险
- `Menu` 在全屏播放器上的弹出位置/层级需目视确认不被手势层遮挡。

---

## D. 续播竞态修复 + 直播标识自动刷新（任务 #5）

### D1. 续播从头播（iOS · point③）
**根因**：竞态。`preparePlayer`（`PlayerView.swift:513`）里 `resumeEntry = await loadResumeHistory()` 是异步；`applyResumeWhenReady`（`:574`）只在时长就绪时才 seek。若**时长先就绪、历史后返回**，`onChange(duration)`（`:328`）触发那次 `resumeEntry` 仍是 nil → 漏 seek；末尾补的 `await applyResumeWhenReady()` 若那一刻时长仍为 0 也直接返回 → 永远不再触发。
**改法**：让"历史就绪"也成为触发点。
- 将 `resumeEntry` 暴露为可观察状态，新增 `.onChange(of: resumeEntry)` 再调一次 `applyResumeWhenReady()`。
- `applyResumeWhenReady` 保留 `didApplyResume` 防重入。即"时长就绪"与"历史就绪"两个时机都尝试，谁后到谁触发。
**验收**：真机脚本——某视频看到中段 → 退出 → 从「观看历史」或「视频列表」再次进入 → 出现 "已从 mm:ss 继续播放" 并跳到该位置（不是从 0 开始）。方案作者只做编译检查。

### D2. 直播标识结束后不自动消失（point②）
**根因**：后端 `getVideoLibrary` 拉取时已自愈（`src/servers/handler.go:1032-1034` 把无活跃 recorder 的房间写回 `recording=false`），但客户端不主动刷新，所以要手动刷新才变。
**改法**：
- **后端先确认**：录制**停止**时是否派发 SSE `recorder_status`（`src/servers/sse.go:22-30` 已定义该事件类型）。查 `src/recorders/manager.go` / `src/listeners/manager.go` 的事件派发；若只在 start 派发，需补 stop 时派发。
- **Web**：video-library 组件订阅 SSE `recorder_status` / `list_change`，收到后重拉 `GET /api/video-library`。
- **iOS**：`VideoLibraryView` 目前**没有 SSE 客户端**（`APIClient` 无 EventSource）。最稳妥=加"页面可见时轮询"，仿 `bililive-ios/Live OS/Live OS/Views/RoomListView.swift:140` 的 `refreshRoomsWhileVisible`（10s 间隔，`silent` 重拉）。不引入 SSE，避免 iOS 端新协议维护。
**验收**：
- 后端：`curl -N` 连 `GET /api/sse`，开录 → 停录，确认真推 `recorder_status` 事件。
- iOS：真机脚本——房间直播中（红标）→ 直播结束 → **不手动刷新**，≤10s 内"直播中"标识自动消失。
- Web：同上，video-library 自动去红标。

### 风险
- iOS 轮询 10s 有最长 10s 延迟，可接受；若要求更快可缩短间隔，权衡耗电/请求量。

---

## E. iOS 导出按钮移到直播间页（任务 #6 · iOS）

### 现状
- 导出 + 恢复都在 `bililive-ios/.../Views/SettingsView.swift:64`（`BackupRestoreView`）。
- 需求：**导出/备份放直播间页**，**恢复留设置页**。

### 改动文件与改法
1. `bililive-ios/Live OS/Live OS/Views/RoomListView.swift`
   - 工具栏（`:24-33`，现有"添加""刷新"）增加入口"导出/备份"。
   - 点击 present 一个**仅含导出**的 sheet：复用 `BackupRestoreView` 的 `makeBackupPackage` / `createBackup` / `fileExporter` + `createRemoteBackup`（`BackupRestoreView.swift:108-155`）。
   - 建议把"导出"逻辑从 `BackupRestoreView` 抽成可复用组件/方法，供直播间页与（如保留的）设置页共用，避免两处重复。
2. `SettingsView.swift`
   - 备份与恢复入口**保留恢复部分**（按 ID / 选文件恢复）。导出部分可从设置页移除或保留（与用户确认；需求只明确"恢复放设置"，导出主入口移到直播间页）。

### 验收
- `xcodebuild` 编译检查。
- 真机脚本：直播间页点"导出/备份" → 生成可分享的本地文件 + 返回远端备份 ID；设置页仍可用 ID/文件恢复。

### 风险
- 导出逻辑抽离时注意 `AppConfig`/`client` 环境对象的传递。

---

## 编译/测试关卡（方案作者执行）

每项实现完成后运行并给出红/绿证据：
- 后端：`make dev` + `go test ./src/servers ./src/configs ./src/pkg/metadata`
- iOS：`xcodebuild`（编译检查）+ 对照本文"验收"逐条核对 + 输出真机测试脚本

## 建议落地顺序

```
A（纯后端、根因已确认、可全程验证）
  → B（前端小改、可验证）
  → D2 后端段（确认/补 SSE 派发）
  → C / E / D1（iOS 三项，编译 + 评审 + 真机脚本）
```

先清能在方案作者侧全程验证的后端/前端项，iOS 三项集中处理。
