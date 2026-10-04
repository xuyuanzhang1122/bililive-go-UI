# 录播播放 T5 实施记录（2026-10-04）

范围仅 bililive-go-UI 的 T5 v2 第一期；基线 `3c6240ecfcae3e458da1d3969ffeb07fc6c1eacd`，沿用 feature/jit-hls-optimization。T5 核心实现、自动验证及一次独立审查修复完成；真机和生产状态另列待验。

## 接口与能力

已实现 v2 录播分页列表、创建/查询/取消播放会话、GET/HEAD 媒体与逐项批删。管理响应统一 `{data,error,request_id}`，只接受 Bearer；媒体使用实例随机密钥签发的 HMAC play_token，绑定会话、来源/资产版本及有效期，URL 不含管理 Key。

能力决策读取实际容器、视频 codec/profile/level、尺寸/帧率、位深/HDR及音频 profile/声道/采样率，返回 direct/remux/audio-transcode/video-transcode。T3/T4 的已准备资产经探测和证明复核才能 ready；未知参数不冒充兼容。没有适合资产时读取现有准备任务的 processing/paused/failed 状态，未准备时明确 asset_not_ready；HTTP 不新建任意转码档位。

会话为 15 分钟实例内状态，token 最多 5 分钟，GET 刷新并返回刷新间隔；服务重启需重建会话。全局/每用户未取消且未过期会话上限为 1024/32，取消立即释放配额；取消状态记录有界，总记录最多2048。取消、Key 撤销、来源/资产变化拒绝后续媒体请求，已开始传输不保证即时中断。

## 身份、旧接口与删除

列表按 SQLite 稳定 ID 分页，不扫描全录播库。文件通过相同资产的登记身份跟随已核验改名；同路径新源不能承接旧授权。会话刷新以修订号提交，两个入口都拒绝旧结果覆盖新状态；传输对象的实际句柄版本必须与所选 FileVersion 相同。MOV/MP4 按 BMFF 品牌、WebM/MKV 按 EBML DocType 区分，不能靠 demuxer 名称列表或扩展名冒充。探测缓存升级 v4，绑定来源/工具实际文件身份，避免同大小/时间换 inode 复用。

旧 resolver 显式 session_id 分支消费 v2，保留 commonResp，paused→processing、cancelled→failed；原调用保持原行为。第一期没有迁移配置、直播控制、账号和备份。

批删最多 100 个 ID/来源版本；每项报告状态、实际删除数及错误，HTTP 200 不表示全部成功。拒绝活跃录制、版本变化、符号链接与越界；仅删除登记且有证明的普通媒体文件，未知旁文件保留。Linux/macOS 通过已打开的父目录原子隔离、复核捕获对象后删除；恢复冲突保留隔离对象，不覆盖新路径。Windows 按实际文件句柄删除。最终操作与 BeginRecording 共用互斥。其他平台失败保源；Unix 隔离中进程退出可能保留 `.bilive-delete-*/media`，需人工恢复，未实现新的批删恢复日志。删除后目录 pending 并唤醒后台核验，I/O 故障允许部分成功并报告实际计数。

## 验证与交付

测试全部使用临时目录/SQLite/生成媒体；真实 FFmpeg 覆盖 MP4、FLV、HLS、MP3→AAC 和 MPEG4→H264，包含实际 T3/T4 准备、token/Range/刷新/取消/授权撤销/改名/换源、暂停与失败，以及登记资产集合的批删和未知旁文件保留。

独立审查发现 C1 删除竞态、I1 并发状态覆盖、I2 打开句柄身份、I3 容器识别；两个原 Minor（旧 MIME、取消配额）按实际影响提升 Important，一次修复全部 RED→GREEN。未派发复审，以实际回归与全套检查闭环。完整分级、取舍及证据见 [审查记录](../evidence/2026-10-04-t5/review.md)。

| 最终命令 | 结果 | 证据 |
| --- | --- | --- |
| make dev | 退出0 | final-dev.log |
| make build-web dev | 退出0 | final-build-web-dev.log |
| PATH=/tmp/bilive-tools:$PATH make lint | 退出0，0 issues | final-lint.log |
| make test | 退出0 | final-test.log |
| go test -race -tags=dev ./src/pkg/playbackdecision ./src/pkg/recordingcatalog ./src/pipeline/... ./src/servers | 退出0 | final-race.log |
| make dev PLATFORM=windows ARCH=amd64 | 退出0 | final-windows-build.log |
| make dev PLATFORM=linux ARCH=amd64 | 退出0 | final-linux-build.log |

[验证清单与源码 SHA256](../evidence/2026-10-04-t5/verification.json)覆盖实际受测代码；二进制内嵌 GitHash 是提交前基线。Go1.24 基线保持，没有新的配置或 SQLite schema 迁移。

契约 [openapi-v2.yaml](../../contracts/openapi-v2.yaml) 与 [接口说明](../../contracts/v2-playback.md) 供 T6/T7 消费。当前共享 schema 校验覆盖实际 DTO/样例约束，没有声称验证全部 OpenAPI 标准。手机首帧/拖动/长时间播放、生产容量、Windows/Linux 实际运行仍待验收；T3 HDR 正向验证的既有环境限制继续保留。

按用户要求主动本地 commit，保留当前分支；未 push、部署或转换真实历史库。主计划下一阶段为 T6 鸿蒙消费共享会话契约。
