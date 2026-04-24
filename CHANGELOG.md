# Changelog

## v1.2 (2026-04-22)

### Bug 修复
- 修复 Docker 环境及自定义 entrypoint 场景下点击"保存设置"报 `config path not set` 错误（PR #1）
  - 新增 `SetDefaultConfigPath` 兜底路径，`Marshal()` / `GetFilePath()` 不再硬性要求 `Config.File` 已填充

### 新增功能

#### URL 解析（抖音）
- `GET /api/resolve-url` 升级为独立模块 `src/pkg/urlresolver`
- 支持抖音分享文案、无协议短链、`v.douyin.com` 跳转、`live.douyin.com` query/path 清洗
- GET 失败后自动降级 HEAD；页面 HTML 兜底提取 `web_rid` / `roomId`

#### iOS App 后端支持（第一阶段 · 仅抖音）

**API Key 鉴权**
- 新增 `Config.Security`：`enable_api_key` / `api_key` / `signed_url_ttl_seconds`
- 启用鉴权且 `api_key` 为空时自动生成 32 字节随机串并写回配置文件
- 中间件校验 `Authorization: Bearer <key>` 或 `X-API-Key` 请求头
- 本地开发可通过环境变量 `BILILIVE_DISABLE_API_AUTH=1` 跳过鉴权

**签名 URL**
- `GET /api/signed-url?kind=file|thumbnail|hls&path=<rel>&expires_in=<s>` — 为指定资源生成带 HMAC 签名的临时访问 URL
- 签名 URL 可绕过 Bearer Token 鉴权，适合 `AVPlayer` / `AsyncImage` 等无法附加自定义 Header 的场景

**录播文件 HLS 转封装**
- `GET /api/stream/hls/{path}` — 对 FLV / TS / MKV 等 AVPlayer 不支持的格式按需调用 ffmpeg 转封装为 HLS，缓存于 `.appdata/hls-cache/`
- `GET /api/stream/hls-segment/{cache_key}/{segment}` — HLS 分片下载

**OpenAPI 文档**
- 新增 `docs/openapi.yaml`（OpenAPI 3.1），覆盖抖音相关 API 子集，供 Swift OpenAPI Generator 生成 iOS 客户端

#### 更新管理
- 关闭前端更新管理入口（`/update` 路由和菜单项隐藏）
- `GET /api/update/check` 固定返回"无新版本"，不再访问旧上游更新源
- 启动时跳过后台自动更新检查

#### 外部工具下载
- 运行时为 `remote-tools-config.json` 自动注入 GitHub 镜像 fallback，提升国内网络下的工具下载成功率
- 工具下载和预置工具共享代理配置（`download_proxy`）

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
