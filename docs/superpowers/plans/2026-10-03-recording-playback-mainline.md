# 录播播放主基线 T0–T9 实施计划

> **给执行 agent：** 使用 `superpowers:executing-plans`，由主线程逐项实施。用户已要求继续任务，无需再次询问执行方式。本文是工作区唯一主执行计划；旧计划、交接和报告只作证据。已通过的局部修复不重复重写。依据用户 2026-10-04 的最新要求，每次改动完成并验证通过后主动提交本地 commit；push 与部署仍需对应授权。

**目标：** 服务端可靠保存和提前准备录播，按设备能力选择播放版本，使 iOS、鸿蒙和 Web 共用契约，消除长视频反复准备和播放失败。

**架构：** 主播放 faststart MP4 保存在原录播目录，验证、发布及关联提交后才按显式策略替代 FLV；兼容版与持久 HLS 放在 AppData/media-assets，临时 JIT 缓存单独管理。SQLite 录播目录负责稳定身份、资产状态、路径别名及房间统计；API 请求读取目录，不遍历全库、不启动同步全量转换。v2 第一期仅覆盖录播列表、播放会话、媒体授权和批量删除。

**技术栈：** Go、现有 modernc SQLite 与迁移机制、FFmpeg/ffprobe、HTTP Range/HLS、ArkTS/Hypium、Swift/SwiftUI、React/TypeScript。

**设计依据：** 用户 2026-10-03 在当前聊天提供的 T0–T9 核对与修正，以及 `/Users/xumy/Desktop/bilive/bililive-任务交接与残留问题-2026-10-03.md` 的最终目标和边界。聊天中未提供原计划全文；本文不冒充其逐字副本。

## 全局约束

- 工作区 `/Users/xumy/Desktop/bilive`；保留三仓已有工作成果，不 reset/clean。每次改动完成并验证通过后主动提交本地 commit；已有累积成果作为检查点提交时说明范围，不擅自 push。
- 不升级、重启、重配或覆盖正在使用的服务器软件；不擅自删除、覆盖或批量转换真实录播。
- 真机验收使用独立本地服务、目录、数据库、缓存和端口；目标 5,634,539,067 字节视频只读复制，安装修复版须合法签名。未安装修复版不得宣称真机完成。
- Go 改动运行 `make dev`；阶段交付运行 `make build-web dev`、`make lint`、`make test`；配置结构变更同步迁移。
- 默认保源。最终删源须显式策略、媒体验证、发布、关联提交、删除前身份复核及可恢复意图。客户端不得决定转换或清理真实文件。
- 统计由服务器核验并下发字节数和展示文本；大小统一十进制，一位小数。目录失败显式报告，不能以部分和或陈旧缓存冒充当前值。
- 记录代码完成、自动验证、真机验收、生产部署四种状态；HTTP 200/206 或单元测试不等于手机首帧。
- 不保存 SSH 密码、API Key、签名私钥或证书密码到计划和证据。

## 审查重点

1. 发布后源、目标或关联被替换：拒绝删除新文件，保留可信产物；T1 回归覆盖。
2. 中断和重启：发布/删源/准备任务幂等恢复，未知版本不猜身份；T1/T3 故障注入覆盖。
3. 活跃录制、低空间及资源争用：跳过录制中源，暂停重任务，不损伤源；T3/T4/T9 覆盖。
4. 旧路径、源替换与房间统计：别名和历史不串录播，不重复计算逻辑条目；T2/T8 覆盖。
5. 会话过期、设备退出、旧回调晚到：刷新授权或取消，错误真实可见；T5/T6/T7 覆盖。

## 当前基线与优先顺序

| 仓库 | HEAD（不能代表工作树全部修复） |
| --- | --- |
| bililive-go-UI | 4e8c935c7fb450cbb0b03b980f595d859c89a27b |
| Live-os-Harmony | e4be1f2aa5c87240631799c0ef553e43fa120b65 |
| bililive-ios | ceb7b166f9e7ec55f2427f6ea5d1b86595388c6b |

设备上读出的鸿蒙 versionName 2.0.1/versionCode 2 尚不能映射到修复源码；用户所称正式版 2.0.0 与之关系待确认。合法本机签名待提供。旧源码指纹是历史快照，不覆盖之后改动；每个里程碑冻结新指纹。

执行顺序：T0 → T1 → T2 → T3 → T4/T5 → T6/T7/T8 → T9。签名可用时立即插入指定视频真机验收，不等待 T6 全部完成。未拿到签名仍继续服务端主线。

## T0：基线、计划与隔离验收

**状态：** 旧证据存在；当前需重新冻结。计划本轮首次落盘。

**文件：** 本计划；`bililive-主线证据-2026-10-03/`；后续设备时间线记录。

- [x] 保存三仓 HEAD、status、已跟踪改动及未跟踪源码 SHA256；排除二进制、缓存、签名与凭据。
- [ ] 核对设备应用来源和合法本机签名；不凭版本号推断安装了修复。
- [ ] 只读复制指定 5.2GB 视频到独立本地验收目录，校验大小和摘要；本机服务使用独立配置，不覆盖生产。
- [ ] 记录冷/暖启动、resolver、prepared、首帧、50%/近尾拖动、退出重进和持续播放。失败进入证据驱动修复。

**门槛：** 当前源码可追溯；真机结果未有证据时明确待验收。

## T1：发布、删源与跨平台持久性

**状态：** 并发完整 manifest 判断、发布前身份复核、MP4 验证及 lint 已完成。本轮已补持久删源意图、删除前完整复核和中断收尾；平台目录同步已拆分；Windows 采用验证目录访问但不做 Unix 目录 Flush 的较弱策略。Windows 运行/断电持久性仍待验收。

**文件：** `bililive-go-UI/src/pkg/playbackasset/{publish.go,manifest.go,source_delete.go,source_delete_test.go,sync_unix.go,sync_windows.go}`；`src/pipeline/stages/convert_mp4.go` 及实际转换回归。

**接口：** 保持 `PublishValidated`/`RecoverPublication`；新增 `DeleteSourceValidated(source,target string,sourceInfo os.FileInfo) error`，删除前要求 Load 的完整来源关联匹配且目标仍有效。

- [x] 先加删除前目标消失/变化、不同来源关联、源被替换和合法成功的失败回归，再实现统一删除入口，接入转换阶段。
- [x] 把目录同步拆分为平台文件，Unix 真实目录 Sync；Windows 策略明确区分不支持能力和真实 I/O 错误，不把交叉编译当持久性证明。
- [x] 新增持久删源意图，记录完整来源/目标版本和显式删除策略；重启先核验已发布产物与源，再续做或安全停止。不得仅用 `.publishing.json` 代替删源授权。
- [x] 测试关联提交后/删源前、删源后/完成标记前重启，源或目标替换时不误删；恢复完成时清理意图。
- [x] 跑包测试/race、真实 MP4 pipeline、macOS `make dev` 与 Windows 交叉编译；Windows 真机和断电恢复另列待验。

**门槛：** 可信发布、删除恢复与平台持久性边界均有证据；当前不能勾选整个 T1。

## T2：持久录播目录、资产与权威统计

**状态：** 已实施 SQLite recording_id、版本、路径别名、资产登记和权威统计；启动/30 秒后台核验、录制完成、pipeline 完成、文件操作唤醒已接入。独立审查两项问题已按失败→通过回归修复；平台真实运行仍待验。

**文件：** 新增 `bililive-go-UI/src/pkg/recordingcatalog/{store.go,schema.go,types.go,store_test.go}`；录制关闭/pipeline 完成钩子；`src/servers/video_library_statistics.go` 适配；持久资产目录 `AppData/media-assets/<recording_id>/<asset_version>/`。

**接口：** `recording_id` 为生成一次并持久化的 UUID；`source_version` 独立于路径；`asset_version` 含来源版本和准备规则版本。表 `recordings`、`recording_aliases`、`media_assets`、`room_statistics` 分别存身份、旧路径、资产和核验统计。SQLite 事务同时提交目录与统计，不按 basename 猜关联。

- [x] 编写重开数据库 ID 稳定、旧路径别名、同路径换源不串身份、重复事件幂等及统计事务回滚测试。
- [x] 接入录制完成、可信发布、删除/移动事件；由后台受控 reconcile 校验外部文件变化，API 请求只读数据库。
- [x] 主 MP4 保持原房间目录；FLV→MP4 为同一逻辑录播，旧 URL/观看进度使用别名。兼容/HLS 资产存派生目录。
- [x] 统计明确定义：`video_count` 是逻辑录播数；`total_size` 是房间当前保留的主视频物理字节和，多个源/主版本仍同时存在时各占空间；派生大小单独返回 `derived_size`，总存储再单列。禁止隐式混用去重数量和空间口径。
- [x] 统计失败或未核验返回状态与最后核验时间；两端直接显示服务器文本。验证 176 GiB 对应 189.0 GB、增长、删源、派生资产不混入主大小及无 HTTP 全库扫描。

**过渡方案（已替换数据来源）：** 2026-10-03 上午曾在旧 `/api/video-library` 添加 `total_size_text/statistics_status/statistics_checked_at`，每次扫描房间普通视频文件，排除隐藏临时文件/目录、符号链接；失败不发布部分和。iOS/鸿蒙优先原样显示，旧服务端仅统一换算其字节值，鸿蒙 key 随统计变化。它不满足请求禁止全库扫描的最终原则，本轮 T2 已移除 HTTP 房间扫描，仅由后台扫描更新 SQLite，并保留 DTO 兼容；禁止恢复请求扫描。

**门槛：** 稳定身份、事件更新、持久统计和后台核验接入实际录制/发布路径；空目录包或仅 schema 不算 T2 完成。

## T3：录后自动准备

**状态：** 2026-10-04 已实施核心录后自动准备、持久去重/恢复/暂停、编码选择、T1 主/派生发布及 T2 v2 准备证明。真实三类 FFmpeg、Range、故障注入、全仓构建/test/lint/race 通过，独立审查两项已 RED→GREEN 修复。新增 `prepare_playback` 默认关闭，需显式启用；生产、真机及完整 HDR 正向验收仍待完成。

**文件：** `src/pipeline/{store.go,types.go}`、`src/pipeline/stages/prepare_playback.go` 与注册处、配置及迁移、recordingcatalog 资产提交。

**接口：** 准备任务以 `(recording_id,source_version,profile_version)` 去重，状态 queued/running/ready/failed/paused；输入为 T2 可信目录记录，输出验证完成的资产。

- [x] 测试录制完成入队、重复入队、重启恢复、录制中跳过、源变化、取消及低空间暂停。
- [x] 原画可兼容时 stream copy faststart MP4；音频不兼容仅转音频；视频不兼容生成明确 profile 的兼容资产。不盲目把所有源重新编码。
- [x] 共用 mediawork 预算、媒体验证及 T1 发布/删源协议；失败保源并记录原因。配置变更同步迁移，不能靠悄改用户配置开启。
- [x] 用真实 FFmpeg 样例验证 codec/时长/faststart/Range，恢复任务不得复用其他源版本。
- [ ] 在含 `zscale` 的 FFmpeg 环境完成 HDR→SDR 正向验收；当前本机仅验证缺滤镜失败保源与真实原因。Windows/Linux 真实运行与断电、手机验收/生产部署仍独立列待验。

## T4：持久 HLS 与资源调度

**状态：** 2026-10-04 已实现有限多档持久 VOD、版本化资产/验证证明、临时 JIT 隔离、暂停/恢复与 Retry-After。真实 FFmpeg、异常进程退出、源改名与替换回归、全仓构建/test/lint/race 和交叉编译通过；独立审查两项 Important 已 RED→GREEN 修复。配置默认空；真机、真正断电和生产长样本容量仍待验，共享预算 FIFO 公平性列延期 minor。

**文件：** `src/servers/{hls_job_manager.go,hls_cache_janitor.go,media_probe.go,hls_handler.go,hls_retry.go}`，新增持久 HLS pipeline 阶段及资产 manifest。

- [x] 固定测试区分准备完成的持久清单和 JIT EVENT 增长清单；已有分片可读，未生成区间可等待/取消，失败明确。
- [x] 持久 HLS 随 T2/T3 资产版本管理，不能被 10 GB/24h 临时缓存策略回收；临时 JIT 继续读租约保护和空间准入。
- [x] 按配置准备需要的多档 profile，不默认无限档；队列和重任务共用预算，低空间暂停而非误删活跃资产。
- [x] processing/限流响应提供 `Retry-After`，与响应体轮询间隔一致；测试排队、重启、租约、取消与预算争用。

## T5：v2 第一期与播放决策

**状态：** 2026-10-04 核心 v2 列表/会话/短期 play_token/能力决策/逐项批删完成；真实 FFmpeg、HTTP契约、版本替换、并发与删除竞态、授权/容量回归及全仓构建/lint/test/race通过，一次独立审查修复闭环。设备消费和真机验收转入 T6/T7，生产未部署。

**文件：** `docs/contracts/openapi-v2.yaml`、`src/servers/server.go`、新增录播与会话 handler/DTO/测试；能力决策模块。

**接口：** `GET /api/v2/recordings`、`POST /api/v2/playback-sessions`、`GET/DELETE /api/v2/playback-sessions/{id}`、`POST /api/v2/recordings/batch-delete`；响应 `{data, error, request_id}`，错误 code/message/retryable；媒体使用短期 play_token，管理 API 保持 Bearer Key。

- [x] 先定义共享 OpenAPI/样例及服务端契约测试，再实现 handler；旧 `/api` 显式适配，不整体强拆 data。
- [x] 服务端根据容器、codec/profile/level、分辨率/帧率、位深/HDR、音频与协议能力决策 direct/remux/audio-transcode/video-transcode；输出所选资产、状态、过期及刷新信息。
- [x] 会话绑定 recording_id/source_version/asset_version；测试授权篡改、过期、源替换、无匹配能力、取消与失败可重试语义。
- [x] 第一期不迁移配置/直播控制/账号/备份等管理接口；第二期另立里程碑，不能阻塞播放交付。

## T6：鸿蒙统一消费与验收

**状态：** v2与prepared生命周期、取消、URL/批删及签名样片真机证据已有；指定大录播MatePad时间线仍待设备连接。

**文件：** `Live-os-Harmony/entry/src/main/ets/{net/APIClient.ets,player/PlayerController.ets,viewmodel/PlayerViewModel.ets,cache/ThumbnailCache.ets}` 及页面/测试。

- [x] APIClient 显式选择 v2/旧适配，共用 DTO 与样例，缩略图和 LAN 探测纳入统一鉴权和真实状态判断。
- [x] 能力上报与服务端会话同步；授权过期刷新、退出取消、旧回调隔离、HLS 时长增长均加回归。
- [ ] 合法签名可用时立即安装修复来源明确的 HAP，按 T0 指定视频验收，再扩展媒体矩阵。不等待本任务所有小项。

## T7：iOS 对齐

**状态：** v2、实际能力、会话及来源隔离已实施，指定5.63GB录播iPhone真机时间线通过；实际播放原T3准备资产。

**文件：** `bililive-ios/Live OS/Live OS/{Network/APIClient.swift,Models,ViewModels,Views/PlayerView.swift}` 与 `Tests/`。

- [x] 消费与鸿蒙相同 v2 样例、错误和统计文本；能力来自实际播放器支持，不能硬编码声称支持所有源。
- [x] 测试会话更新/过期、取消、未知状态、服务器变化和逐项失败；构建模拟器后执行同一大视频实机时间线。

## T8：Web、统一契约与交付

**状态：** OpenAPI类型化客户端、AST路由提取、受控Bearer SSE、跨端契约完成；独立审查修复及生产首次认证初始化已验证部署。

**文件：** `src/webapp/src/utils/generated-api.ts`、客户端生成脚本、SSE 消费处、OpenAPI 和共享样例。

- [x] 从 T5 OpenAPI 生成类型化客户端，显式裸/封装兼容；SSE 使用可携带鉴权的受控请求，不把 Key 暴露到无约束 URL。
- [x] 同样例在服务端、鸿蒙、iOS、Web 跑状态/错误/URL/统计/批删契约；录播身份与别名显示不重复。
- [x] 完成联合构建/lint/test/race 和独立审查，冻结当前指纹；审查工具不可用时如实记录，不能沿用旧结论覆盖新代码。

## T9：历史库补齐

**状态：** 治理预览/确认/暂停恢复、原准备队列、失败保源及重启恢复完成；真实库16项dry-run预览已提交，批量执行待用户审核授权。

**文件：** 新增历史治理命令/预览 API、持久任务记录与测试、操作报告。

- [x] 默认仅 dry-run：扫描预览、候选身份、预计空间和策略；不自动转换/删源。
- [x] 显式授权后持久入队，限速/暂停/恢复，录制中跳过，源变化失效，低空间暂停；复用 T1 原子发布和删源协议。
- [x] 临时样本验证中断恢复和失败保源，再提交真实库可审查预览；真实批量执行需要用户针对该预览授权。

## 当前检查点

- 2026-10-03：视频库统计过渡修复自动验证通过；服务端 make test/lint/dev、iOS 模拟器构建、鸿蒙 Node 47 项及 Hypium 构建通过。鸿蒙 CompileArkTS/PackageHap 通过但 SignHap 失败，未安装。
- 本轮已补 T1 删除前完整复核、持久删源意图与恢复，临时文件回归/race/真实 FFmpeg 通过；未有证据的条目保持未勾选。签名、Windows 运行和主架构其余任务不因局部修复自动完成。

- 本轮 T1 平台策略与 T2 目录/统计已实施。新证据持续保存在 `bililive-mainline-work/`，旧 83 项指纹为上轮快照，不覆盖本轮新源码。下一项 T3 录后自动准备。

- T1/T2 本轮自动验证与审查修复记录：[实施记录](../../../bililive-T1-T2-实施记录-2026-10-03.md)。T2 resolver 别名仍保持旧请求入口，公共 v2 媒体会话与历史 ID 合并在 T5/T8。Windows/Linux 交叉编译通过但暂无真实运行证据。

- 2026-10-04：T3 核心实现与自动验证、独立审查修复完成；[T3 实施记录](../../../bililive-T3-实施记录-2026-10-04.md)记录配置优先级、目录 v2 迁移、代码指纹与仍待验项目。本轮未提交、部署或转换真实历史库。

- 2026-10-04 最新用户要求覆盖此前不自动提交约定：每次改动完成并验证后主动本地 commit。服务端已补交检查点 `94312d7907d23a25679ba1ee1750ce7a6984baec`，包含此前 T1/T2、HLS 修复依赖及本轮 T3；未 push 或部署。

- 2026-10-04：T4 核心实现与自动验证、独立审查修复完成；[T4 实施记录](../../../bililive-T4-实施记录-2026-10-04.md)记录配置、发布集合协议、源改名恢复、异常退出半成品清理及剩余验收。下一阶段 T5；本轮主动本地提交，不 push 或部署。

- T4 本地检查点：`fd83210e7e026e73b357f1162f6958781cca78a4`，包含核心实现、两项 Important 修复及最终验证证据；当前分支保留，T5 未开始。

- 2026-10-04：T5 核心实现、共享契约及自动验证/独立审查修复完成；[T5 实施记录](../../../bililive-T5-实施记录-2026-10-04.md)记录并发、句柄与安全删除修复和平台边界。下一阶段 T6，按要求主动本地提交，未 push 或部署。

- T5 本地检查点：`c2d09342ee83092ec6e3fd7b36c31b20ba977bd3`，包含共享契约、核心实现、六项审查修复和最终验证证据；下一阶段 T6，真机/生产状态保持独立待验。

- 2026-10-04最终更新：T7指定5.63GB录播副本iPhone时间线通过；T8/T9实现、联合检查与一次跨仓独立审查修复完成。服务器已部署v2.1.2-rc.3，GitHub多平台包/Docker RC发布成功；最近3条真实录播经用户授权保源准备完成，实际媒体/Range/刷新/取消与Web帧/跳转/暂停通过。鸿蒙功能分支已推，iOS v2.0.3-rc.5的GitHub IPA构建成功并下载校验。T6指定MatePad时间线、旧本机配置恢复来源和审计预览清理仍待补齐；此前“不push/不部署/待文件”是历史检查点状态。
