# Changelog

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
- 修复未配置任何直播间时，输出目录下**所有文件夹都被识别为视频库**的问题（移除 `len(knownPlatforms) > 0` 守卫条件，空 map 查询直接返回 false）
- 修复**观看历史记录**在原视频删除后仍继续显示的问题（加载时发 `HEAD` 请求校验文件是否存在，404 则自动清除记录）

#### 抖音短链解析
- 修复粘贴抖音分享文案时短链被解析为 `webcast.amemv.com/...` 长串地址的问题
  - 服务端解析短链时由 iPhone UA 改为**桌面 Chrome UA**，抖音服务器在桌面端会直接重定向到 `live.douyin.com/<room_id>` 格式
  - `normalizeLiveRoomURL` 不再对 `webcast.amemv.com` 地址强制提取 ID（webcast stream ID 与 room ID 不同，无法互换）
  - 若解析结果仍为 webcast 或超长 stream ID，前端会给出明确提示，引导用户手动输入正确地址

### 技术改进
- 视频缩略图通过 `/api/thumbnail/<path>` 接口按需生成，缓存于 `.appdata/thumbnails/`
- `getVideoLibrary` 后端接口跳过以 `.` 开头的隐藏目录（如 `.appdata`）
- 短链解析请求增加 `Accept` / `Accept-Language` / `Referer` 请求头，重定向时继承 UA，提高成功率
- 重定向上限从 10 次提升至 15 次，适应抖音多级跳转链路
