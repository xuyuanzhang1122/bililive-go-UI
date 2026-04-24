# Changelog

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
