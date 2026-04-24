# bililive-go 优化版 · 项目总览文档

> 本文档是本 fork 相对上游 [hr3lxphr6j/bililive-go](https://github.com/hr3lxphr6j/bililive-go) 的**全量改版说明**，面向后续维护者以及与 GPT / Claude 协同工作时作为上下文注入。
> 它不是单纯的 API 文档（API 单独见 [`API.md`](API.md)），而是覆盖：架构、新增模块、配置、部署、已知问题、开发路线图和协同协议。

---

## 1. 项目定位

原项目 `bililive-go` 是一个轻量级多平台直播录制服务。本 fork 在"稳定录制"之上追加了**视频管理 + 观看 + 远程交付**三条能力线，目标是：

1. **录制后不必再手动管理文件** — 视频库 / 缩略图 / 续播 / 删除。
2. **开箱即用的 Docker 部署** — 支持 amd64 / arm64 / armv7，附带 launcher 自更新。
3. **面向移动端开放接入** — 后续将引入 iOS App，通过 REST + WebSocket + HLS 复用现有服务。

上游聚焦"录"，本 fork 聚焦"录 + 看 + 管 + 扩"。

---

## 2. 顶层目录结构

```
bililive-go-UI/
├── src/                    # Go 后端
│   ├── cmd/bililive/       # 主入口（含 launcher 热更逻辑）
│   ├── configs/            # 配置加载 / 序列化 / 迁移
│   ├── servers/            # HTTP API + SSE + 反向代理
│   ├── live/               # 各直播平台 parser / extractor
│   ├── recorders/          # 录制引擎（FLV/HLS/TS 写入）
│   ├── listeners/          # 房间状态监听
│   ├── pipeline/           # 录制后处理（转码 / 回调等）
│   ├── tools/              # 外部工具管理（ffmpeg/BililiveRecorder 等）
│   ├── pkg/
│   │   ├── events/         # 全局事件总线 (→ SSE)
│   │   ├── iostats/        # 系统 IO / 内存监控
│   │   ├── kliveproxy/     # K-live 反代
│   │   ├── launcher/       # 自更新启动器
│   │   ├── metadata/       # 视频元数据 / 缩略图
│   │   ├── openlist/       # 云端上传 (OpenList)
│   │   ├── update/         # 更新检查 / 下载 / 应用 / 回滚
│   │   └── ...
│   └── webapp/             # 打包后的前端 (embed.FS)
├── src/webapp/src/         # 前端源码 (React + Vite)
├── docs/                   # 用户文档
├── Dockerfile(.local)      # 多架构镜像构建
├── entrypoint.sh           # 容器启动脚本
├── config.yml              # 配置样例（本地）
└── config.docker.yml       # 配置样例（容器）
```

---

## 3. 相对上游的全部改动

### 3.1 新增模块（src/pkg/ 下）

| 模块 | 作用 | 关键文件 |
|---|---|---|
| `events` | 全局事件总线，SSE 推送源 | `events/*.go` |
| `iostats` | CPU / 磁盘 / 内存 / 网络统计 | `iostats/*.go` |
| `launcher` | 下载新版本后由 launcher 拉起目标二进制，失败自动回滚 | `launcher/check.go`, `runner.go` |
| `metadata` | 视频文件元数据扫描、ffprobe 调用 | `metadata/*.go` |
| `migration` | 旧配置结构 → 新结构的迁移 | `migration/*.go` |
| `openlist` | 云端上传 OpenList 集成 | `openlist/*.go` |
| `streamprobe` | 录制前探测流格式 / 编码 | `streamprobe/*.go` |
| `telemetry` | 匿名使用统计 | `telemetry/*.go` |
| `update` | 版本检查 + 下载 + 校验 + 应用 | `update/*.go` |
| `kliveproxy` | 特定平台反向代理 | `kliveproxy/*.go` |
| `ratelimit` | 请求速率控制 | `ratelimit/*.go` |
| `sentry` | 错误上报 | `sentry/*.go` |
| `livelogger` | 每房间独立滚动日志 | `livelogger/*.go` |

### 3.2 新增前端页面

| 页面 | 路由 | 功能 |
|---|---|---|
| 视频库首页 | `/library` | 按主播聚合视频，展示缩略图 / 数量 / 大小 / 最新时间 |
| 视频列表 | `/library/:platform/:uploader` | 单主播全部视频 + 批量删除 |
| 内嵌播放器 | `/player/...` | FLV / TS / MP4 / MKV / MOV，续播、手势、全屏 |
| 文件管理 | `/files` | 目录树 + 单/批量删除、重命名 |
| 设置 | `/settings` | 直播间增删、配置持久化、导入导出 |
| 外部工具 | `/tools` (反代到 tools WebUI) | 管理 ffmpeg / BililiveRecorder 等 |
| IO 统计 | `/iostats` | CPU / 磁盘 / 内存实时图 |
| 更新管理 | `/update` | 检查 / 下载 / 应用 / 回滚 |
| Pipeline | `/pipeline` | 录制后处理任务队列 |

### 3.3 新增 HTTP API（相对上游）

**完整列表见 `servers/server.go`，以下按功能分组：**

**配置管理（扩展）**
- `GET/PUT/PATCH /api/config` — 整体 / 部分更新
- `GET /api/config/effective` — 实际生效配置（合并后）
- `GET /api/config/platforms` — 平台统计
- `PUT/PATCH/DELETE /api/config/platforms/{platform}`
- `PUT/PATCH /api/config/rooms/id/{id}`
- `PUT/PATCH /api/config/rooms/{url}`
- `POST /api/config/preview-template` — 输出文件名模板预览
- `GET/PUT /api/raw-config` — 原始 YAML 读写

**房间运行时（扩展）**
- `GET /api/lives/{id}/logs` — 每房间日志
- `GET /api/lives/{id}/sessions` — 录制会话历史
- `GET /api/lives/{id}/name-history` — 主播名变更历史
- `GET /api/lives/{id}/history` — 统一事件分页
- `POST /api/lives/{id}/switchStream` — 切换流

**文件管理（新增）**
- `GET/PUT/DELETE /api/file/{path}` — 单文件
- `PUT /api/batch/file/rename`
- `POST /api/batch/file/delete`

**视频库（新增）**
- `GET /api/video-library`
- `GET /api/thumbnail/{path}`
- `GET /api/video-files/{path}`

**抖音 / Bilibili 辅助（新增）**
- `GET /api/resolve-url` — 解析抖音短链
- `GET /api/bilibili/qrcode` + `/poll` + `POST /api/bilibili/cookie/verify`

**Cookie 管理（新增）**
- `GET/PUT /api/cookies`

**更新管理（新增，入口已临时关闭，后续需指向本 fork release，见 §7）**
- `GET /api/update/check`、`/latest`、`/status`、`/launcher`、`/rollback`
- `POST /api/update/download`、`/apply`、`/cancel`、`/rollback`
- `PUT /api/update/channel`

**IO 统计（新增）**
- `GET /api/iostats`、`/requests`、`/filters`、`/disk`、`/devices`、`/memory`、`/memory/categories`

**OpenList 云上传（新增）**
- `GET /api/openlist/status`、`/check-storage`

**Pipeline（新增）**
- `RegisterPipelineHandlers` 下一组路由

**SSE（新增）**
- `GET /api/sse` — Server-Sent Events 实时推送房间状态、录制进度、系统指标

**文件静态服务**
- `/files/*` — 录制输出目录静态访问
- `/tools/*` — 反向代理到 tools WebUI 端口（动态发现）

### 3.4 配置字段新增

上游 `Config` 结构被扩展为包含以下字段（详见 `src/configs/config.go`）：

- `AppDataPath` — launcher / 缩略图 / 数据库存储根
- `VideoSplitStrategies` — 视频分片策略
- `OnRecordFinished` — 录制结束后的 shell 命令钩子
- `Notify` — 通知渠道（邮件/Telegram/企业微信…）
- `Feature` — 功能开关
- `OpenList` — 云上传配置
- `Telemetry` — 遥测开关
- 每房间（`LiveRoom`）新增 `LiveId` 持久化、`PlatformName` / `Uploader` 等元信息

### 3.5 Bug 修复要点（详见 [CHANGELOG.md](../CHANGELOG.md)）

- iOS 播放器白屏、全屏、长按加速放大镜等
- 空 `knownPlatforms` 时所有目录被识别为视频库
- 观看历史指向已删除文件仍显示
- 抖音 `webcast.amemv.com` 短链解析错误（改用桌面 UA）

---

## 4. 部署

### 4.1 二进制

```bash
./bililive-go -c config.yml
```

启动时 `-c` 必须给定；否则 `config.File == ""`，后续通过 UI 保存配置会失败（见 §7）。

### 4.2 Docker（官方镜像）

```yaml
# docker-compose.yml 节选
services:
  bililive:
    image: xuniubi/bililive-go
    volumes:
      - ./config:/etc/bililive-go
      - ./videos:/srv/bililive
    ports:
      - "8080:8080"
    environment:
      - PUID=1000
      - PGID=1000
      - UMASK=022
```

启动命令由 `entrypoint.sh` 固定为：
```
/usr/bin/bililive-go -c /etc/bililive-go/config.yml
```

### 4.3 Docker（自己构建）

```bash
docker build -f Dockerfile.local -t bililive-go:dev .
```

**已知问题**：在 Ubuntu 上用自构建镜像运行，点击"保存设置"会弹出 `config path not set`，但配置实际已应用。详见 §7.1。

### 4.4 Launcher 自更新

`launcher/check.go` 在进程启动时读取 `${AppDataPath}/.launcher-state.json`，如果 `target_version > current`，launcher 进程会作为父进程 `fork/exec` 新版本二进制，失败时自动回滚。`entrypoint.sh` 不感知此机制。

---

## 5. 运行时架构

```
                                ┌─────────────────────────┐
                                │  前端 React (embed.FS)  │
                                └───────────┬─────────────┘
                                            │ HTTPS
                                            ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                    servers/server.go (gorilla/mux)           │
  │  ┌──────────┐ ┌──────────┐ ┌───────┐ ┌─────────┐ ┌────────┐│
  │  │ REST API │ │  SSE     │ │ /files│ │ /tools/ │ │ OSRP   ││
  │  └────┬─────┘ └────┬─────┘ └───┬───┘ └────┬────┘ └───┬────┘│
  └───────┼────────────┼───────────┼──────────┼──────────┼─────┘
          │            │           │          │          │
          ▼            ▼           ▼          ▼          ▼
  ┌─────────────┐ ┌─────────┐ ┌────────┐ ┌────────┐ ┌────────┐
  │  instance   │ │ events  │ │  fs    │ │ tools  │ │ OSRP   │
  │  (单例容器) │ │ 总线    │ │ Output │ │ WebUI  │ │        │
  └──────┬──────┘ └────┬────┘ └────────┘ └────────┘ └────────┘
         │             │
         ▼             ▼
  ┌─────────────────────────────────────┐
  │ live parsers → recorders → pipeline │
  └─────────────────────────────────────┘
```

- **instance**：进程级单例，持有 `Lives`, `Config`, `Server`, `Logger` 等
- **events**：发布 / 订阅，SSE 端点订阅后推流给前端
- **live.Parser**：按平台解析流 URL → `recorders.Recorder` 写入本地

---

## 6. 前端技术栈

- React 18 + TypeScript + Vite
- 路由：React Router
- 状态：Redux Toolkit + RTK Query
- UI：Ant Design 5
- 播放器：自研（`video` 原生 + `flv.js` + `mpegts.js`）
- 构建产物：`src/webapp/src/dist` → Go `embed.FS`

---

## 7. 开发路线图 · 进度追踪

> **本章节是协同者（GPT / Claude / 人类）接手时的唯一真相源**。每个 PR 合并后更新状态标记。
> 状态：`✅ 已完成` / `🔧 进行中` / `⏳ 待处理` / `⏸ 已搁置`

### 7.0 用户原始需求（存档，不修改）

以下是用户在项目重启讨论时提出的全部需求原文，用于协同者理解意图：

> 1. 重构视频播放页面以及 UI
> 2. 在 ubuntu 使用自己构建的 docker 镜像部署，在点击设置时会出现（保存设置失败: Error: config path not set）报错，但是配置会实际应用
> 3. 外部工具管理页面基本不能下载使用
> 4. 更新管理，这个项目里的更新管理应该还是原项目的更新获取，这是不行的，做一下数据清洗
> 5. 这个直播间的链接转换还是不稳定，是否能开发一个 skill 或者工具，在使用时能够转换（比如装一个无头浏览器），能基本做到 95% 以上的转换率
> 6. 最后，我打算使用原生 swift 语言开发一个前端 iOS APP 接入这个项目，能做到视频播放、海报页、增删直播间 URL，以及文件的删除管理（第一阶段），那么是否需要把 web 项目做出来相关的 websocket 或者其他相关的握手协议，视频流传播协议等
>
> 附加：最后做好项目文档，和 GPT 协同工作。
>
> 后续澄清（2026-04-21）：
> - 第 1 项 Web UI 暂不改，优先 iOS
> - 第 3 项不新增外部工具条目（URL 转换走 §7.4 内置实现）
> - 第 4 项先关闭入口
> - iOS 鉴权使用 API Key

---

### 7.A 已完成

#### ✅ 修复 `config path not set`（2026-04-21，commit `caf7736`，对应用户需求 #2）
- 根因：`src/configs/config.go` 的 `Marshal()` 在 `Config.File == ""` 时直接报错；某些启动场景（自定义 entrypoint / flag 分支 / launcher 重启）路径未填充
- 修复：新增 `SetDefaultConfigPath` / `GetDefaultConfigPath` 兜底路径，启动时 `cmd` 登记
- 波及：`Marshal()` / `GetFilePath()` / `updateImpl` / `updateCASImpl` 四处持久化门槛放宽
- 验证：`make dev` 通过，`go test ./src/configs/` 通过

#### ✅ 项目文档与清洗（2026-04-21，commit `8992b81`）
- 新建本文档 `docs/PROJECT.md`（架构 / 全部改动 / API / 路线图 / 协同上下文包）
- 删除 19 个上游残留文件：`bililive-linux-amd64`（46MB 二进制）、`.travis.yml`、`Procfile`、群晖教程、grafana/prometheus 配置、wechat 群截图等
- `CHANGELOG.md` 补全 v1.1.1 / v1.1.2
- `AGENTS.md` 加入路线图和架构引用，`make sync-agents` 同步到 copilot/gemini/antigravity 三处镜像
- `package.json` 修正 name/version/description
- `README.md` 移除失效链接，指向 `docs/PROJECT.md`

#### ✅ 关闭更新管理入口（2026-04-21，PR #1，对应用户需求 #4）
- 用户原文：*"更新管理，这个项目里的更新管理应该还是原项目的更新获取，这是不行的，做一下数据清洗"*
- 用户澄清：*"先关闭这个入口"*
- 前端隐藏 `/update` 路由与菜单项
- 后端 `/api/update/*` 保留（launcher 依赖），但 `check` 返回"无新版本"
- 启动时跳过后台自动更新检查，避免继续访问旧更新源
- 首页不再弹升级提示
- 验证：`make build-web dev`、`make lint`、`make test` 通过

#### ✅ 修复外部工具下载（2026-04-21，PR #1，对应用户需求 #3）
- 用户原文：*"外部工具管理页面基本不能下载使用"*
- 用户澄清：*"按你的来"*（不新增工具条目，URL 转换走 §7.4）
- 不新增工具条目；现有 `remote-tools-config.json` 在运行时自动扩展 GitHub 镜像 fallback
- remotetools 初始化前注入项目 `download_proxy` 环境变量，工具页安装和预置工具同步可复用代理
- 进度反馈继续复用 remotetools 自带 SSE / WebUI 进度
- 验证：`go test ./src/tools` 通过，`go test ./src/pkg/proxy` 通过

#### ✅ URL 转换 resolver · 抖音第一阶段（2026-04-21，PR #1，对应用户需求 #5）
- 用户原文：*"直播间的链接转换还是不稳定，是否能开发一个 skill 或者工具，在使用时能够转换（比如装一个无头浏览器），能基本做到 95% 以上的转换率"*
- 范围收窄：配合 iOS 第一阶段，当前只做抖音；其他平台 resolver 推后到下一版本
- 新增 `src/pkg/urlresolver/`，原 `GET /api/resolve-url` 已收编进该模块
- 支持抖音分享文案、无协议短链、`v.douyin.com` 跳转、`live.douyin.com` query/path 清洗
- 使用桌面 UA + 信息代理配置请求短链，GET 失败后降级 HEAD；页面 HTML 中兜底提取 `web_rid` / `roomId`
- 对 `webcast.amemv.com` 不做错误互转，无法得到稳定 `live.douyin.com/<room_id>` 时返回明确错误
- 验证：`go test ./src/pkg/urlresolver` 通过，`go test ./src/servers` 通过

---

### 7.B iOS 第一阶段（已完成，PR #1）

#### ✅ 7.5 iOS App 后端支持 · 第一阶段仅抖音（2026-04-22，PR #1，对应用户需求 #6）

**用户原文**：*"使用原生 swift 语言开发一个前端 iOS APP 接入这个项目，能做到视频播放、海报页、增删直播间 URL，以及文件的删除管理（第一阶段），那么是否需要把 web 项目做出来相关的 websocket 或者其他相关的握手协议，视频流传播协议等"*
**用户澄清**：
- iOS 鉴权使用 API Key
- **第一阶段仅保留抖音直播间服务**，其他平台（B站/斗鱼/虎牙/快手/YY 等）和 **OpenList 云上传** 均推后到下一版本

**第一阶段范围（scope 收窄）**：
- 平台：**仅抖音**（`live.douyin.com` / 抖音分享文案短链）
- 功能：视频播放、海报页、增删抖音直播间 URL、录制文件删除
- **不做**：其他直播平台接入、OpenList 云上传、Pipeline 后处理管理、外部工具管理

**需补后端能力**：

1. **API Key 鉴权**（已实现，PR #1）
   - `Config.Security.ApiKey`（启用鉴权且为空时自动生成 32 字节随机串并写回 config.yml）
   - Middleware 校验 `Authorization: Bearer <key>` 或 `X-API-Key`
   - `/files/*` 和 `/api/thumbnail/*` 使用签名 URL（HMAC + expires）
   - 本地开发可用 env 跳过鉴权

2. **实时事件**
   - 复用已有 SSE (`/api/sse`)，iOS `URLSession` 可承载
   - 不引入 WebSocket（避免双协议维护）

3. **视频流**（本地已实现录播文件 HLS，直播中视频流待下一版本）
   - MP4：`AVPlayer` 原生支持，直接给 `/files/<path>` 签名 URL
   - FLV / TS：新增 `GET /api/stream/hls/{path}`，ffmpeg 按需转封装为 HLS，缓存 `.appdata/hls-cache/`
   - 直播流（进行中的录制）：第一阶段不做

4. **最小 OpenAPI 3.1 文档**（已实现，PR #1）
   - 新建 `docs/openapi.yaml`，**只覆盖抖音相关的 endpoint 子集**：
     - `POST /api/lives`（添加直播间，URL 限定 `live.douyin.com`）
     - `GET /api/lives` / `DELETE /api/lives/{id}`
     - `GET /api/video-library` / `GET /api/thumbnail/{path}` / `GET /api/video-files/{path}`
     - `GET/DELETE /api/file/{path}` + `POST /api/batch/file/delete`
     - `GET /api/sse`、`GET /api/resolve-url`、鉴权端点
   - Swift 侧用 [Swift OpenAPI Generator](https://github.com/apple/swift-openapi-generator) 生成 client
   - 其他平台 endpoint 留在文档 §3.3 但不进入 openapi.yaml，避免 Swift 生成无用 client

**验证**：
- `make build-web dev`
- `make lint`
- `make test`

**iOS 侧建议**（供 Swift 协同者参考）：
- MVVM + `@Observable`（iOS 17+）
- 网络：`URLSession` + OpenAPI 生成的 client
- 播放器：`AVKit.VideoPlayer`
- 图片：`Kingfisher` 或原生 `AsyncImage`

**预估**：后端 2~3 个 PR（鉴权 / HLS / OpenAPI 抖音子集分别做）。iOS App 独立仓库。

---

#### ⏸ 7.7 下一版本范围（暂定）

由 §7.5 推后至本节，便于协同者知晓已讨论过、但本期不做：

- **多平台 iOS 接入**：B站、斗鱼、虎牙、快手、YY、小红书等（需扩展多平台 resolver）
- **OpenList 云上传 iOS 端**：目前后端已有 `/api/openlist/*`，iOS 端暂不接
- **Pipeline 任务管理**：录制后处理任务在 iOS 端的查看/触发/取消
- **直播中视频流播放**：正在录制的 FLV 实时推给 iOS（需要 live HLS 转封装流水线）

---

#### ⏸ 7.6 Web UI + 播放页重构（对应用户需求 #1）— 已搁置

**用户原文**：*"重构视频播放页面以及 UI"*
**用户澄清**：*"web 先不改了，现已 iOS 端"*

待 iOS 第一阶段稳定后再议。

---

### 7.C 执行顺序

```
[✅ 完成] 项目文档与清洗
[✅ 完成] 7.1 修复 config path not set
   ↓
[✅ 完成] 7.2 关闭更新管理入口
   ↓
[✅ 完成] 7.3 修复外部工具下载
   ↓
[✅ 完成] 7.4 URL 转换 resolver（抖音第一阶段）
   ↓
[✅ 完成] 7.5 iOS 后端支持（API Key / 录播 HLS / OpenAPI 抖音子集）
   ↓
[⏸] 7.6 Web UI 重构（搁置）
```

每步独立 PR；完成后将对应项改为 ✅ 并标注 PR 编号与日期。

---

## 8. 与 GPT / Claude 协同的上下文包

当你把任务丢给另一个 LLM 时，建议附带的最小上下文：

```
我在维护 github.com/xuyuanzhang1122/bililive-go（一个直播录制服务）。
技术栈：Go 后端（src/）+ React 前端（src/webapp/src/）+ Docker 部署。
架构和改动详见 docs/PROJECT.md。
API 详见 docs/API.md（或 docs/openapi.yaml）。
我要做的事：<具体描述>
相关文件：<路径>
```

对频繁问答的问题，固化到本文档 §7 或单独 `docs/DECISIONS.md`，避免每次重新解释。

---

## 9. 约定与规范

- **提交信息**：`<类型>: <范围> <说明>` — 类型 `feat/fix/docs/refactor/chore/test`
- **分支**：`main` 为发布分支，功能分支形如 `feature/url-resolver`、`fix/config-path`
- **配置兼容**：任何 `Config` 结构变更必须配套 `src/pkg/migration` 的升级函数
- **平台新增**：`src/live/<platform>/` 下一个 Parser 即可；不要改主循环
- **事件新增**：在 `src/pkg/events/` 定义常量 + 类型；SSE 自动转发
- **前端 API 调用**：统一走 `src/webapp/src/store` 下的 RTK Query slice，不在组件里直接 fetch

---

## 10. 文档维护

- 本文档（`docs/PROJECT.md`）：**架构 / 改动 / 路线图** —— 每个 PR 合并后更新
- `docs/API.md`：接口说明
- `docs/openapi.yaml`：机器可读 API spec（待引入）
- `CHANGELOG.md`：面向用户的版本日志
- `README.md`：对外介绍 + 快速开始
- `AGENTS.md`：LLM 协作约定

当你发现本文档过时，直接改；不要在别处开新文档。
