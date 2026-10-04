# T5 账本 — 主计划：docs/superpowers/plans/2026-10-03-recording-playback-mainline.md

BASE: 3c6240e。范围仅 T5，用户明确开始下一阶段；每次完成验证后主动 commit。
Ruling: 沿用已提交且干净的当前 feature 分支 — 与 T4 连续推进 — 成本是以提交区分阶段，整合另行授权。
Ruling: 细化现有授权计划，直接执行契约/测试，不重复设计审批 — 主计划 T5 已指定范围、接口和验收 — 成本是未指定的细节由账本明确供追溯。
Ruling: 会话消费 T3/T4 的有限准备资产与状态，HTTP 不创建任意转码档位 — 提前准备与受控历史治理是主架构 — 成本是无匹配已准备版本时需明确准备或增加后续档位。
Pre-flight: T2 ID/别名 → v2 ID 查询、分页和版本绑定；不允许按 basename 推断，不在请求中 reconcile。
Pre-flight: T3/T4 证明与准备表 → v2 校验真实选定资产、只读准备状态，不用 QueuePreparation 做查询。
Pre-flight: 旧媒体签名 → v2 独立短期 token 与管理 Bearer，无管理 Key 查询参数；旧响应 DTO 保持兼容。
Task 1: 能力决策 RED→GREEN（red-decision.log / green-decision.log），覆盖视频/音频完整参数、未知值、协议与容器，source 兼容才允许 direct。
Task 2: 目录 ID 查询/分页、改名/替换、活跃源 RED→GREEN（red-catalog.log / green-catalog.log）；只读准备状态 RED→GREEN（red-preparation-read.log / green-preparation-read.log），查询不插入 queued。
Task 3: 会话/媒体真实 FFmpeg 与 HTTP 测试 RED→GREEN（red-sessions.log / green-sessions.log），覆盖同句柄 Range、HLS 分片 token、刷新/取消/过期/篡改/跨会话/源替换、准备暂停失败恢复。
Task 4: 契约真实响应及逐项批删先 RED（red-contract.log / red-delete.log），实现后正在运行包含原有测试的相关全包 green-core.log。
Ruling: 会话内存保存 15 分钟，媒体 token 5 分钟，GET 会话刷新 — 会话是短期播放器上下文而非持久准备任务 — 成本是服务重启需重新建立会话，容量为 1024/每用户32。
Ruling: 媒体 token 使用服务实例随机密钥的 HMAC，绑定会话/来源/资产/过期，管理接口严格 Bearer — 不把管理 Key 置入媒体 URL，不沿用同源免认证 — 成本是旧 Web 必须显式走适配或增加 Bearer。
Ruling: 手动批删只删除该 ID/来源版本登记且逐个复核的普通媒体文件，不递归删除未知旁文件 — 不以目录名或 basename 猜关联 — 成本是派生目录的元数据旁文件保留待治理，I/O 故障可能部分删除，返回实际 deleted_files。
Task 3 补充: 源改名会话与旧接口适配 RED（red-rename-adapter.log）→修复；探测缓存同大小/时间换 inode RED（red-probe-identity.log）→来源/工具真实身份 cache v3；首轮 GREEN 套件发现测试复用 JSON 解码对象保留 omitempty 的旧 URL，已定位为测试对象问题，改用新 DTO，不改生产语义。
Task 3 补充: 授权用户/撤销、有限容量、录制中请求验证；429 轮询体字段 RED（red-retry-limit.log）→与 Header 一致的 error.retry_after_seconds；旧错误包装及 HLS MIME RED（red-legacy-errors.log）→ commonResp 保留与正确 MIME，green-legacy-errors.log 通过。
Task 3 补充: 扩大真实 AAC/MP3 和 MPEG4→H264 媒体矩阵；会话响应阶段过期回归 red-late-expiry-real-modes.log 正在执行。
Ruling: MP4 会话资产版本用来源、准备规则和文件快照构成稳定身份，当前路径通过同资产 Rebind 跟随已核验改名 — T2 主文件登记版本可能在 reconcile 后按原 faststart 规则重算 — 成本是 v2 MP4 资产标识不直接照抄过渡 media_assets.asset_version，必须固定 DTO 规则。
Ruling: 旧 resolver 仅显式 session_id 分支使用 v2，原调用保持原行为及包装 — 设备仍分阶段迁移，不能整体强拆旧 data — 成本是旧客户端需要先创建会话才获得能力决策。
Task 3 补充: 真实 audio-transcode/video-transcode 通过实际 T3 准备资产验证；响应时过期 RED→GREEN，410 不签发失效 URL（green-late-expiry-real-modes.log）。
Task 4: 核心全包 green-core.log 通过；新增样例 contract-samples.log 通过。初次完整检查 make dev/build-web/lint/test/race 均退出0，lint 0 issues；Windows/Linux amd64 make dev 交叉编译通过，实际运行仍未验。
Final review: 按 executing-plans 派发唯一 fresh-context gpt-6-astra 只读审查，基线 3c6240e 至 staged T5；报告待返回。
Final review: 独立审查返回 C1 删除竞态、I1 并发状态提交、I2 打开句柄身份、I3 MOV/WebM 容器识别；原始裁定为暂不可合并。审查只读，未复审，实施者一次修复并以回归/全套验证闭环。
Final: Ruling: 将 M1 普通文件 MIME 与 M2 取消配额从 Minor 提升 Important 并修复 — 实际旧播放器可能无法解封装，快速浏览可能在32次后停播15分钟，按用户影响评级 — 成本是取消记录与活跃配额分开，需维持2048条总记录上限。
Final: Ruling: 补登记 WebM 扩展名并按真实 BMFF 品牌/EBML DocType 识别容器，探测缓存升级 v4 — 声明的 WebM 能力必须能进入可信目录，不能通过扩展名推断 — 成本是既有目录统计纳入此前遗漏的 WebM，未知品牌继续保守拒绝 direct。
Final: fixed I1 并发刷新/旧适配覆盖 — TestV2LateCancelledRefreshCannotOverwriteReadyAsset 的 MP4/HLS × v2/旧入口四组合 RED→GREEN，修订号/CAS 与请求取消保护，final-test/final-race 均通过。
Final: fixed I2 同路径替换后恢复导致串文件 — TestV2OpenedHandleCannotServeReplacementAfterPathRestored RED→GREEN；实际句柄 identity/size/mtime 版本与绑定 FileVersion 比较，final-test/final-race 均通过。
Final: fixed I3 MOV/WebM 分类 — TestV2MOVCapabilityUsesActualContainer RED→GREEN；真实 WebM/MKV DocType、MOV改扩展名及仅MP4能力拒绝 MOV 测试通过，final-test/final-race 均通过。
Final: fixed C1 校验到 unlink 的竞态 — TestPinnedDeleteRejectsReplacementAndKeepsIt、TestPinnedDeleteStaysInOpenedParentAfterSymlinkReplacement、TestDeletionLeaseRejectsActiveAndBlocksNewRecording RED→GREEN；父目录锚定/实际文件版本绑定/录制租约，final-test/final-race 均通过。
Final: fixed 升级后的 M1/M2 — TestV2LegacyFileMIMEMatchesSelectedMOV、TestV2CancelledSessionsReleasePlaybackQuota RED→GREEN，33次创建取消可继续且旧 token 410，final-test/final-race 均通过。
Final: Ruling: Go 1.24 基线保持，Linux/macOS 用相对目录句柄捕获并核验后 unlink，Windows 用父句柄相对打开/版本复核/文件句柄删除，其他平台失败保源 — 不用 Go1.25 才有的 Root.Rename，也不回退路径 unlink — 成本是 Windows/Linux 本轮仅交叉编译、其他平台 v2 批删暂不可用；Unix 捕获中进程退出可能留下隐藏隔离文件，需人工恢复，不自动猜测删除。
Final: Ruling: 真机、生产、真实历史执行不以本地样本替代 — 审查 Declined to judge，与主计划阶段边界一致 — 成本是这些环境仍需独立验收。
Final: Ruling: 不带 session_id 的旧 resolver 保留旧鉴权/解析 — 审查 Declined to judge，显式迁移保证兼容 — 成本是旧流程的保证不能扩大为 v2。
Final: Ruling: 非默认用户同 ID 原地轮换 Key 没有现有 metadata 入口，保留当前撤销模型 — 审查 Declined to judge，未为不可达入口新增协议 — 成本是今后增加入口时必须同步绑定/撤销会话凭据。
Final: Ruling: 取消拒绝后续媒体请求，不保证中断已开始传输 — 审查 Declined to judge，当前契约明确该行为 — 成本是播放器退出也应主动取消本地传输。
Final: minor (deferred): 无；两项原 Minor 按实际播放影响升级并在同一次修复中闭环。
Task 1: complete — 能力决策及全部共享样例，最终 make test PASS。
Task 2: complete — ID查询/分页、改名/来源/句柄身份与只读准备状态，最终 make test PASS。
Task 3: complete — 会话/媒体/旧适配/授权/容器/并发/配额，最终 make test 与相关 race PASS。
Task 4: complete — 逐项批删、安全捕获及真实 T3/T4媒体集；唯一独立审查一次修复，所有最终检查退出0，等待本地提交记录。
Evidence: 两份失败的中间整套尝试改名为 attempt-rename-adapter-probe.log（测试对象复用）及 attempt-review-all-before-webm.log（目录漏登记WebM），保留原输出，不混作最终GREEN。最终七个命令日志均以 final- 命名且退出0。
