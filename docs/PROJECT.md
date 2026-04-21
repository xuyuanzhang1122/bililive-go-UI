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

**更新管理（新增，目前指向原项目 release，需数据清洗，见 §7）**
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

## 7. 已知问题与开发路线图

> 该章节是本文档随项目演进持续维护的部分。每个 PR 完成后回填状态。

### 7.1 `config path not set`（第 2 项）— **待修复**

**现象**：Ubuntu 自构建镜像中点"保存设置"报错，但配置已应用。
**根因**：`src/configs/config.go:784, 816` —— 当 `Config.File == ""` 时 `Marshal()` 直接返回错误。`Config.File` 在 `cmd/bililive/bililive.go:56` 的 `GenConfigFromFlags()` 分支中不会被填充；`entrypoint.sh` 明确传了 `-c`，所以更可能的路径是：
- `servers/handler.go:1339` 对 `putRawConfig` 做了 `newConfig.File = oldConfig.File`，但 `putConfig`（`PUT /api/config`）以及其它 `updateConfig` / `updateRoomConfig` 分支可能没有继承；或
- 老配置被某条路径替换后 `File` 丢失。
**修复方向**：
1. 在 `SetCurrentConfig` 或 `Marshal` 内做兜底——若 `File == ""`，回落到启动时保存的 `flag.Conf` 或 exe 同目录 `config.yml`。
2. 所有 handler 的 "new 配置替换 old 配置" 路径统一走 helper，保证 `File` 继承。
3. 前端 toast 区分"应用成功"与"持久化失败"，避免误导。

### 7.2 更新管理入口关闭（第 4 项）— **待处理**

**目标**：本 fork 发布在 `xuyuanzhang1122/bililive-go`，而 `pkg/update` 当前仍指向上游 release API，数据无意义。
**方案**：
- **阶段一（最小改动）**：前端隐藏 `/update` 路由与菜单项；后端 `/api/update/*` 保留以免破坏 launcher。
- **阶段二（可选）**：改造 `pkg/update` 指向本 fork 的 GitHub Release 或关闭此能力。

### 7.3 外部工具下载（第 3 项）— **待处理**

**现象**：`/tools` 页面几乎无法下载；GitHub release 直连在国内常失败。
**方案**：
- `src/tools/remote-tools-config.json` 每个工具的 `downloadUrl` 已预留数组，用于多镜像 fallback；补充 ghproxy / 自建镜像。
- 服务端下载改为**流式代理**（减少超时），失败时按数组顺序重试。
- 不新增工具条目——URL 解析作为内置模块实现（§7.4）。

### 7.4 直播间 URL 转换（第 5 项）— **待处理**

**目标**：95%+ 转换率，将任意形态（分享文案 / 短链 / webcast / app 链接）规范化为后端可识别的房间 URL。
**方案**（**不引入无头浏览器**）：
- 新增 `src/pkg/urlresolver/`：
  - `resolver.go`：`Resolver` 接口 `func(ctx, raw string) (canonical string, err error)`
  - `registry.go`：按平台域名/特征注册
  - 每平台一个文件：`douyin.go` / `bilibili.go` / `huya.go` / `douyu.go` / `kuaishou.go` / `xhs.go` ...
  - 优先调用各平台**公开 room info 接口**；拿不到时用 HTTP + JSON/正则兜底。
- 原 `GET /api/resolve-url` 收编进此模块。
- 同步提供一个本地 skill（`.claude/skills/url-resolver`），用于批量测试和调试 URL。

### 7.5 iOS App 接入（第 6 项）— **待处理**

**范围（第一阶段）**：视频播放、海报页、增删直播间 URL、文件管理。

**需要补的后端能力**：

1. **鉴权**：API Key
   - 配置项 `Config.Security.ApiKey`（首次启动自动生成并写回 config.yml）。
   - Middleware 校验 `Authorization: Bearer <key>` 或 `X-API-Key`。
   - 对 `/files/*` 和 `/api/thumbnail/*` 使用签名 URL（HMAC + expires），避免视频 URL 直接泄露。

2. **WebSocket（或继续用 SSE）推送**：
   - 当前已有 SSE (`/api/sse`)。iOS 原生对 SSE 支持一般，但可用 `URLSession` 长连接。
   - 如决定引入 WS：`GET /api/ws` 带 Bearer 升级，消息格式同 SSE 事件。
   - 推荐**优先复用 SSE**，iOS 端实现一个轻量 SSE 客户端，省得双协议维护。

3. **视频流**：
   - 录制完成文件：HLS 不适用（MP4/FLV/TS 都是单文件），`AVPlayer` 原生支持 MP4；FLV/TS 需先转封装或走后端转流。
   - 建议新增 `GET /api/stream/hls/{path}` —— 对非 MP4 文件按需调 ffmpeg 转封装成 HLS m3u8 + ts 切片缓存，AVPlayer 直接拉。

4. **最小 OpenAPI 3.1**：
   - 放 `docs/openapi.yaml`，分组对应 §3.3。
   - 用于 Swift OpenAPI Generator 生成 client。

**Swift 侧建议**：
- MVVM + `@Observable`（iOS 17+）
- 网络层：`URLSession` + 生成的 OpenAPI client
- 播放器：`AVKit.VideoPlayer`
- 图片缓存：`Kingfisher` 或 `AsyncImage`

### 7.6 执行顺序

PR 建议顺序：
1. `config path not set` 修复（§7.1）
2. 更新管理入口隐藏（§7.2）
3. 外部工具下载修复（§7.3）
4. URL 转换 resolver（§7.4）
5. API Key + OpenAPI + HLS 转封装（§7.5）
6. Web UI 重构 — **已搁置**，优先 iOS

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
