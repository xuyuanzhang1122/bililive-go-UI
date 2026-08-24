# Changelog

## v2.1.0 (2026-08-24)

### 修复
- **HLS 播放缓存无限堆积（生产现场已达 704 GB）**：根因是本地 FLV/TS/MKV 播放会按“路径 + mtime + size”同步转封装到 `hls-cache`，增长中文件不断产生新 Key，而缓存没有任何生命周期管理；客户端断连还会因 FFmpeg 绑定请求上下文而留下可被误命中的半成品。v2.1.0 同时完成四项修复：①启动时清空可重建缓存，并每 30 分钟按 TTL/总量 LRU 回收及释放失效锁条目；②仅把含 `#EXT-X-ENDLIST` 的 playlist 视为完整命中；③FFmpeg 改用独立 30 分钟超时上下文，客户端断连后继续完成转封装；④行为变更：正在录制的文件不再生成 HLS，接口返回 409 并提示录制结束后重试

### 新增
- 新增顶层 `hls_cache` 配置段：`max_age_hours` 默认 24，`max_total_size_gb` 默认 10；两项均可显式设为 0 关闭对应限制
- `scripts/install.sh` 新增 `--update` 就地更新：保留现有配置，支持二进制备份/重启/doctor，以及从现有 Docker 容器反查端口和挂载后原样重建
- 新增 `scripts/fix-hls-cache.sh` 存量修复脚本，支持自动探测数据目录、`--dry-run`、`--yes`、运行中进程提醒与删除空间报告；`install.sh --update` 会自动执行同等清理

### 说明
- 不默认开启 `convert_to_mp4`；如需浏览器友好的 MP4，可在确认存储和删源策略后自行开启 `on_record_finished.convert_to_mp4`

## v2.0.2 (2026-08-24)

### 修复
- **鸿蒙端 HLS 播放失败（错误码 5400106）**：HLS 播放列表端点兼容 `/playlist.m3u8` 后缀请求，鸿蒙 AVPlayer 以该形态 URL 拉流时不再报格式不支持（74e777e）
- Windows 本地 `make test/lint` 存量问题修复（ae4c031，开发体验）

### 文档
- README 重写为更友好的安装指引；补充官方备份源站（image.xumy.art）可选配置说明

> 本版为 v2.0.2-rc1 经鸿蒙真机端到端验证（播放/续播/历史同步）后的正式版。v2.0.1 及之前为 iOS 优先的 HLS 形态，鸿蒙端存在部分格式不可播问题，自本版起双端兼容。

## v1.3.1 (2026-04-28)

### 新增
- **引导式安装脚本**：`scripts/install.sh` 重写为交互式，自动询问安装目录 / 端口 / 是否启用 API Key / 镜像 tag，并自动下载 config 模板、创建 `Videos`/`Data` 目录、清理旧容器、启动并等待健康检查。`curl|bash` 通过 `/dev/tty` 读取输入，支持 `--yes` 全默认与 `--port`/`--dir`/`--enable-api-key` 命令行覆盖
- **Web UI 一键启用 API Key**：「设置 → API Key」标签未启用时新增「一键启用并生成 Key」按钮，调用 `PATCH /api/config` 写回 `security.enable_api_key: true`，后端 `Security.Normalize()` 自动生成随机 Key 并持久化到 config.yml，无需重启容器或 SSH 改 yaml

### 修复
- **README 首屏 Docker 命令缺 Data 挂载**：补上 `-v $(pwd)/Data:/var/lib/bililive`，避免重启后直播间和缩略图丢失（之前与 docker-compose.yml 行为不一致）
- **install.sh 不真正执行**：旧版只 `echo` 命令给用户复制，新版直接安装

### 文档
- **iOS 自编说明**：README iOS 段加醒目提示——iOS 端无 .ipa / TestFlight 分发，需自行用 Xcode 打开 `ios/Live OS/Live OS.xcodeproj` 真机编译；Docker 服务端升级不会影响已安装的 iOS App

### 改动文件
- `scripts/install.sh`、`README.md`、`docker-compose.yml`、`CHANGELOG.md`
- `src/webapp/src/component/config-info/index.tsx`（APIKeyPanel 一键启用按钮）

---

## P2 API Key + 跨端历史记录 (2026-04-27)

### 新增
- **服务端观看历史**：`watch_history` 表 + `GET/POST/DELETE /api/history` 端点，替代 localStorage 方案
- **API Key 展示**：Web ConfigInfo 页新增「API Key」tab，展示/复制 key 供 iOS 使用
- **观看历史页面**：Web `/history` 路由 + iOS 底部「历史」tab，列表展示播放进度，支持续播跳转和删除
- `/api/auth-status` 端点：前端读取 API Key 鉴权状态

### 鉴权
- `/api/history` 路由受 `apiAuthMiddleware` 自动保护
- 未启用鉴权时（`security.enable_api_key: false`）所有请求直接放行（保持旧 Web 兼容）

### 改动文件
- 后端：`src/pkg/metadata/store.go`、`src/servers/handler.go`、`src/servers/server.go`
- Web：`src/webapp/src/component/history-page/index.tsx`、`config-info/index.tsx`、`App.tsx`、`layout/index.tsx`
- iOS：`ios/Live OS/Live OS/Views/HistoryView.swift`、`ContentView.swift`

---

## P3 播放器进度条重写 · iOS (2026-04-27)

### 修复
- **进度条拖动不再回弹/不跟手**：根因是 `GestureHandlerView`（UIKit touchesBegan/Moved/Ended）与 `PrecisionTimelineSlider`（SwiftUI DragGesture）触摸竞争。改为全 SwiftUI 手势 → 进度条拖动自然流畅

### 改进（iOS PlayerView 重写）

- **展开式进度条**：空闲 4pt 细条，拖动弹簧展开至 8pt + 白色圆形 thumb。渐变 active track（红→橙→金），拖动触觉反馈
- **Liquid Glass 风格控制栏**：全部按钮改 `.ultraThinMaterial` 毛玻璃圆形 pill，播放/暂停按钮白色实心突出，按钮按下回弹动画
- **手势全 SwiftUI 化**：双击播放暂停 / 单击显隐控制栏 / 横滑 seek / 竖滑音量亮度 / 长按 2x 加速，不再依赖 UIKit touch 拦截

### 改动文件
- `ios/Live OS/Live OS/Views/PlayerView.swift`（重写，1073→590 行）

---

## P4 直播间缩略图占位 + 点击跳上游 (2026-04-27)

### 改动
- **视频库卡片增加直播状态感知**：`/api/video-library` 返回新增 `recording`（是否直播中）和 `url`（上游平台原直播间链接）字段
- **直播中房间自动合并**：已添加但尚无录像的直播中房间也会出现在视频库主页，以占位卡形式展示
- **卡片点击分流**：直播中房间点击 → 浏览器新窗口打开上游平台直播间 URL（B 站/斗鱼/抖音等）；非直播房间保持进站内录像列表
- Web/iOS 卡片均显示红色"直播中"标签

### 范围
- 只改列表层 UX，不引入站内实时直播流播放（CORS/Referer/带宽约束）、不新建播放路由、不引入 hls.js

### 改动文件
- `src/servers/handler.go`：VideoRoomInfo 扩字段 + getVideoLibrary 合并 livestate
- `src/webapp/src/component/video-library/index.tsx`：卡片标签 + 点击分流
- `ios/Live OS/Live OS/Models/VideoLibrary.swift`：model 加 recording/url
- `ios/Live OS/Live OS/Views/VideoLibraryView.swift`：卡片标签 + 点击跳系统浏览器

---

## P0 发布流水线规范化 (2026-04-26)

### 新增
- `scripts/release.sh`：版本号写回 + git tag + git push 三合一脚本
- `scripts/install.sh`：一行安装脚本，支持 Docker（默认）和二进制两种模式
- `docs/RELEASE.md`：发布指南 checklist，含故障排查和手动应急发布步骤
- README 顶部新增「快速开始」section（Docker 一行命令 + curl|sh 一键安装）

### 维护
- `docker-publish.yml` 保留为手动应急通道，`release.yaml` 作为日常自动发布的主通道

## P1 数据持久化与索引自动重建 (2026-04-27)

### 核心修复
- **AppDataPath 与 OutPutPath 解耦**：数据库和缩略图不再默认落在 `OutPutPath/.appdata`，改为容器 `/var/lib/bililive`、桌面 `~/.local/share/bililive`，解决「改 OutPutPath 就不小心丢 DB」的耦合 bug
- **Docker 独立数据卷**：新增 `./Data:/var/lib/bililive`，录播视频与 DB 物理隔离
- **启动自动迁移**：检测到旧版 `<OutPutPath>/.appdata` 存在时自动复制到新位置，迁移后删旧目录 + 写哨兵文件防重复
- **启动索引重建（reconcile）**：异步扫描 `OutPutPath` 下所有视频，与 `lives.db` 对账，缺失记录自动创建占位房间（`source='reconcile-unknown'`），根除「实体视频还在但首页空空」的问题

### 改动文件
- `src/configs/config.go`：新增 `defaultAppDataPath()`，解耦默认值
- `src/configs/migrate_appdata.go`：旧数据迁移 + 跨设备 fallback
- `src/reconcile/reconcile.go`：启动扫描重建索引
- `docker-compose.yml` / `Dockerfile` / `entrypoint.sh` / `config.docker.yml`：新 volume 声明
- `README.md` / `docs/PROJECT.md`：持久化说明

---

## v1.3.0 (2026-04-24)

### iOS 客户端架构重写与全效优化

#### 播放器交互升阶
- **告别回弹**：完全重写流媒体滑动进度条 (CustomSlider)，消除使用原生控件时经常出现的刷新跳动、与网络流时间线扯皮等 Bug，使指尖拖曳极致跟手
- **画中画** (Picture-in-Picture) ：应用新增 `Audio Background Modes` 权限并嵌入系统级画中画引擎，实现了点击即可将直播/录播变成小窗退回主屏幕继续看
- **双面手势操控**：引入经典流体滑动模式，屏幕左侧滑动精确控制系统亮度（带拟物感图标反馈），屏幕右半边继续接管音量调节 

#### 视觉与缓存系统
- **封面自适应升级**：竖向 9:16 (如抖音) 直播的封面在视频卡片中全面改用高斯模糊底图与核心保留（Fit）的叠层处理，从根本上解决人脸变形拉伸、强制粗暴裁剪
- **Offline-First 本地优先**：加入应用级别 CacheManager 及加强版 URLCache。列表进页瞬时返回本地数据渲染，背景暗中比对网络更新，网络环境差也能纵享丝滑极速展示
- **UI 精简与美化**：重置设置界面的项目信息并加入半透明渐变基酒衬托，移除视觉干扰

---

## v1.2.0 (2026-04-24)

### 新增功能

#### 前端 API 生成
- 新增 `make generate-web-api` / `npm run generate:web-api`，可从后端路由自动生成 `src/webapp/src/utils/generated-api.ts`
- `make build-web` 会先刷新生成的 API 调用表，减少前后端接口路径不一致

#### 配置自动镜像
- 保存配置时会同步一份到系统默认配置目录，避免覆盖项目目录或替换二进制后丢失已添加直播间
- 默认路径：Linux `~/.config/bililive-go/config.yml`，macOS `~/Library/Application Support/bililive-go/config.yml`，Windows `%AppData%\bililive-go\config.yml`
- 启动时如果指定配置缺失，会自动从系统默认配置镜像恢复；如果镜像较新，会在启动日志和 `/api/config/sync-status` 中提示

#### 抖音链接解析增强
- 抖音短链 HTTP 解析失败后，会自动尝试 Node + Playwright 无头浏览器兜底
- 支持通过 `BILILIVE_DOUYIN_HEADLESS=0` 关闭无头浏览器兜底

#### iOS 播放器手势优化
- 左右滑动：快进/快退（连续滑动，带 ±N 秒 HUD 反馈）
- 上下滑动：调节系统音量（带音量条 HUD 反馈）
- 双击任意区域：播放/暂停（不再限制于屏幕中央）
- 长按侧边：2x 加速播放（松手恢复原速）
- 倍速选择菜单：0.78x / 1x / 1.25x / 1.5x / 2x

#### 智能网络切换
- 支持同时配置局域网地址和公网地址
- 自动检测是否在局域网内（通过 HEAD 请求探测）
- 在店里时自动使用局域网地址（更快），在外面自动切换到公网地址
- 设置页显示当前网络连接状态

---

## v1.1.3 (2026-04-24)

### 改动
- 修复 Web 文件页在 `out_put_path` 指向项目目录时展示源码文件夹的问题，现在只显示录制相关目录和文件
- 修复 iOS 停止/监听按钮成功后仍提示数据解析失败的问题
- 修复 iOS 播放 FLV/TS/MKV 时 HLS 分片地址在未启用 API Key 场景下不可访问的问题
- 重构 iOS 播放页面，改为自定义 AVPlayerLayer 播放器，支持全屏沉浸、双击播放暂停、侧边长按 2x 加速
- 更新 Docker 默认镜像标签、容器配置说明和多架构构建命令

---

## v1.1.2 (2026-03-09)

### 改动
- 修复 Docker 镜像标签和 compose 配置
- 精简视频库组件，移除冗余逻辑

---

## v1.1.1 (2026-03-01)

### 改动
- 新增 `Dockerfile.local`，支持基于本地预编译二进制快速封装 Docker 镜像
- 修正 Docker 默认端口配置（`config.docker.yml`）
- 清理 Go 模块依赖

---

## v1.1 (2026-02-21)

### 新增功能

#### 视频库 (Video Library)
- 新增**视频库**页面，自动扫描录播输出目录，按平台/主播展示所有视频文件
- 每位主播的视频以卡片网格形式展示，包含封面缩略图（由 ffmpeg 自动提取）、视频数量、总大小、最新录制时间
- 支持从视频库页面直接**添加直播间**监控（粘贴抖音分享文案或直播间地址即可自动识别）
- 新增**继续观看**横幅：上次观看的视频会在视频库首页顶部展示进度条，点击直接续播

#### 内嵌视频播放器
- 嵌入式全屏播放器，支持 **FLV / TS / MP4 / MKV / MOV** 格式
- 支持**续播**：每 10 秒自动保存播放进度，下次打开同一文件从上次位置继续
- 支持**手势操作**（移动端）：
  - 单指左右滑动：快进/快退（每 8px ≈ 1 秒）
  - 长按：2× 加速播放，松手恢复
  - 单击：显示/隐藏控制栏
  - 双击：切换播放/暂停
- 自定义控制栏：播放/暂停、±10 秒跳转、进度滑块、时间标签、全屏按钮
- 悬浮返回按钮，始终可见

### Bug 修复

#### iOS 移动端播放器
- 修复拉动进度条导致**白屏**的问题（`touch-action` 父子继承冲突导致 range slider 失效）
- 修复 iOS Safari **全屏按钮无效**的问题（iOS 不支持标准 Fullscreen API，改用 `webkitEnterFullscreen()`）
- 修复**长按加速**会触发 iOS 系统文本选择放大镜的问题（`touchstart` 增加 `preventDefault()`）
- 修复控制栏**隐藏后点击无法唤出**的问题（增加 `onClick` 备用 handler + 修复 `touchmove` 防滚动逻辑）
- 修复 FLV/TS 格式 seek 后可能白屏的问题（改为直接操作 `video.currentTime`）

#### 视频库
- 修复未配置任何直播间时，输出目录下**所有文件夹都被识别为视频库**的问题
- 修复**观看历史记录**在原视频删除后仍继续显示的问题（加载时发 `HEAD` 请求校验文件是否存在，404 则自动清除记录）

#### 抖音短链解析
- 修复粘贴抖音分享文案时短链被解析为 `webcast.amemv.com/...` 长串地址的问题
  - 服务端解析短链时由 iPhone UA 改为**桌面 Chrome UA**，抖音服务器在桌面端会直接重定向到 `live.douyin.com/<room_id>` 格式
  - `normalizeLiveRoomURL` 不再对 `webcast.amemv.com` 地址强制提取 ID
  - 若解析结果仍为 webcast 或超长 stream ID，前端会给出明确提示

### 技术改进
- 视频缩略图通过 `/api/thumbnail/<path>` 接口按需生成，缓存于 `.appdata/thumbnails/`
- `getVideoLibrary` 后端接口跳过以 `.` 开头的隐藏目录（如 `.appdata`）
- 短链解析请求增加 `Accept` / `Accept-Language` / `Referer` 请求头，提高成功率
- 重定向上限从 10 次提升至 15 次，适应抖音多级跳转链路
