# T5 v2 播放会话实施计划

> 执行方法：superpowers:executing-plans，主线程实施、测试先行，阶段末一次独立审查。用户已授权开始下一阶段并要求完成验证后主动本地 commit。

**目标：** 实现主计划 T5 的录播列表、能力决策、播放会话、短期媒体授权和逐项批删。
**架构：** T2 目录按 ID 查询，T3/T4 提供已准备资产及任务状态；会话保存选定来源/资产版本。管理接口 Bearer，媒体仅接受会话 play_token。沿用旧接口，不将管理 Key 暴露到媒体 URL。
**技术栈：** Go、现有 SQLite/FFmpeg、OpenAPI 3.0、JSON 样例。
**依据：** `2026-10-03-recording-playback-mainline.md` T5 及全局约束；本文件只细化已授权阶段。

## 全局约束

- 保留现有 feature/jit-hls-optimization 分支及其他仓库成果，本轮仅 Go；不 push、部署或转换真实历史录播。
- HTTP 不全库扫描，不同步转换。T5 消费 T3/T4 有限准备策略，不能为能力请求制造无限新档位。
- 默认保源；批删只执行显式请求、绑定 ID/来源版本、每项反馈实际结果，拒绝录制中与版本变化。
- 状态/错误不能将 processing、failed 或未知状态当 ready；轮询间隔与 Retry-After 一致。
- make dev、make build-web dev、make lint、make test；相关包 race；提交实际修改及证据。

## 审查重点

1. 旧 URL、同路径新源、改名及源被删除：ID/版本绑定与同句柄传输不得串文件。
2. token 过期、篡改、跨会话/跨资产、取消后的旧回调与不同管理用户不得越权。
3. 未知 codec/profile/level、10bit/HDR、分辨率/帧率与音频能力不得靠容器名冒充兼容。
4. 任务失败/暂停、目录不可用、会话容量耗尽：真实错误、可重试性与轮询一致。
5. 多项删除部分失败、活跃源、链接越界与来源变化：拒绝误删并报告逐项结果。

### Task 1: 契约与能力决策

文件：docs/contracts/openapi-v2.yaml、examples；src/pkg/playbackdecision/{decision.go,decision_test.go}。
接口：Capabilities/Media；Validate(Capabilities) error；Decide(Media,Capabilities) Decision（direct/remux/audio-transcode/video-transcode）。
- [x] 写 OpenAPI 与手工样例，先写兼容、未知/HDR/位深、音频及协议/容器回归。
- [x] go test ./src/pkg/playbackdecision：预期缺少实现 RED；实现后同命令 PASS。

### Task 2: 目录选择与准备状态

文件：src/pkg/recordingcatalog/playback.go、测试；src/pipeline/preparation.go 的只读查询；src/servers/media_probe.go/缓存字段。
接口：Manager.ListRecordings(ctx,after,limit)、PlaybackRecording(ctx,id)、PlaybackFiles(ctx,id,version)、VerifyPlaybackFile(ctx,file)；Manager.PreparationStatus(ctx,key)。
- [x] SQLite 分页、稳定身份、改名与来源替换、派生证明失效、未知路径及活跃录制 RED→GREEN。
- [x] 扩展 ffprobe 字段、缓存版本，真实媒体探测回归；请求只选择一个录播的有限资产。

### Task 3: 会话、授权与路由

文件：src/servers/v2_{dto,sessions,media,recordings}.go、handler.go 入口、server.go 路由、契约/HTTP 集成测试。
接口：GET recordings；POST playback-sessions；GET/DELETE playback-sessions/{id}；GET/HEAD media/{id}/{file}；统一 Envelope。
- [x] 写 HTTP 契约测试、身份/过期/取消/篡改/能力不匹配/准备状态 RED，注册路由并实现 GREEN。
- [x] 15 分钟会话、5 分钟 token；1024 全局/32 每用户活跃上限，取消记录不占配额，总记录不超过2048，GET 刷新媒体授权，取消独立会话不取消共享准备。
- [x] 已核验资产优先；无适合资产时报告所需模式与已有准备状态，未配置准备明确 asset_not_ready，不能无限假 processing。
- [x] MP4 同句柄 Range；HLS 清单与分片使用同会话授权，源/资产变化立即拒绝。

### Task 4: 逐项批删、交付与审查

文件：src/pkg/recordingcatalog/delete.go、src/servers/v2_recordings.go、测试及实施记录。
接口：POST recordings/batch-delete，最多 100 项 {recording_id,source_version}，返回逐项结果。
- [x] 先测成功、版本变化、录制中、不同录播、部分失败与越界保护，再实现最小安全删除。
- [x] 全仓构建/lint/test 与相关 race，独立审查全部阶段；Important 一次 RED→GREEN 修复并重跑检查。
- [x] 保存主计划/记录/源码指纹与验证证据，主动本地 commit，不询问已授权的提交。
