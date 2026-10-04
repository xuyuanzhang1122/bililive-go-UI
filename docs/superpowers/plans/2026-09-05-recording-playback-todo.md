# 录播播放修复与架构落地：详细执行 TODO

> **给执行 agent：** 使用 `superpowers:executing-plans` 技能逐任务执行；本文件由用户交给其他 agent 实施，原审查 agent 负责复核。不要把本文件的创建视为已执行任何修复。步骤使用复选框跟踪，只有满足相应证据要求才能勾选。

**目标：** 修复 `261ad73` 的编译、JIT 生命周期和工程缺陷，再完成录后 MP4 主路径、能力回退与历史库治理。

**架构：** 后台作业统一拥有 ffmpeg、状态和资源配额，HTTP 读取原子发布的内容。新录播优先生成可直放 MP4；按媒体与设备能力选择播放版本；历史文件使用受控 JIT 兜底。

**技术栈：** Go、FFmpeg/ffprobe、HTTP Range、EVENT HLS、iOS 原生播放器、鸿蒙原生播放器。

**设计依据：** [正式架构报告](/Users/xumy/Desktop/bilive/bililive-录播播放架构与实施方案报告.md)、[方案复核](/Users/xumy/Desktop/bilive/bililive-录播播放最优方案复核.md)、本文件中的 `261ad73` 审查问题清单。

## 0. 执行范围和交付规则

- 审查基线：`261ad7328f3d0fc8c90f959ed4f56f5b5a23507f`。执行前记录实际 HEAD，若有后续修复，逐项确认，不重复覆盖。
- 工作目录：[服务端仓库](/Users/xumy/Desktop/bilive/bililive-go-UI)。后文代码路径相对于该目录；客户端路径用绝对链接。
- 阅读每个仓库的 AGENTS.md。中文交流和文档；不主动 commit、push、部署、删除源录播。用户另行授权提交时再提交。
- 用户未提交的 `docs/JIT-HLS-IMPLEMENTATION-PLAN.md` 保留；本 TODO 作为新执行依据。不要默认覆盖未提交文件。
- 不使用 `go build ./...`。服务端构建使用 `make dev`；交付检查使用 `make build-web dev`、`make lint`、`make test`。
- 修改 Config 结构必须添加配置迁移，并同步配置说明、读写接口及相关测试。
- 每批独立可审查。优先完成 A；A 未通过复核，不把 B/C 的大改混进修复补丁。
- 每项逻辑修复先写能复现缺陷的测试，再实现，再验证。测试失败原因必须是目标行为缺陷，而不是仅因新增符号不存在。
- 不以“编译通过”替代端到端验证，不以 resolve ready 替代首帧，不以 ENDLIST 替代源录播完整性。
- 默认不增加外部服务、商业依赖、SRS 或多码率系统。将复杂度限制在当前目标。
- 工具链缺失时先检查项目要求与可用环境；无法执行的检查写明原因，不能标 PASS。

### 批次总览

| 批次 | 内容 | 前置 | 审查重点 |
|---|---|---|---|
| A0 | 编译恢复与测试设施 | 无 | 可构建、测试能观察真实生命周期 |
| A1 | 全入口统一、锁与进程生命周期 | A0 | 首片后清单立即可读，进程仍运行 |
| A2 | 队列、失败恢复、缓存治理 | A1 | 有界运行、不会永久 queued/failed、不会误删 |
| A3 | API 失败语义、集成与双端验证 | A2 | partial 不冒充完整，协议兼容 |
| B | 新录播 MP4 主路径 | A 服务端复核通过 | 播放前准备、原子发布、进度不丢 |
| C | 能力分流与兼容版 | B | 能直放就直放，只转必要轨道 |
| D | 历史迁移与正式验收 | B/C 相应功能可用 | 迁移可恢复、资源可控、证据齐全 |

“通过复核”是工作分批交接点，不授权执行 agent 发布。每批交付后将 diff/commit 和测试证据交回原审查 agent。

## 1. 文件职责地图

| 文件 | 责任与改动边界 |
|---|---|
| `src/servers/hls_job_manager.go` | 作业注册、排队、单次启动、进程拥有权、终态与重试 |
| `src/servers/hls_handler.go` | 清单/分片 HTTP 服务、签名、短时等待；移除旧进程构建入口 |
| `src/servers/playback_resolver.go` | 查询/确保作业，映射公共播放状态，不直接管理进程 |
| `src/servers/hls_cache_janitor.go` | 通过统一作业/租约机制申请回收，不能只看 mtime |
| `src/servers/media_probe.go` | 媒体探测和输出决策；命令参数不重复实现 |
| `src/servers/server.go` 及实际启动调用点 | 将管理器关闭接入既有服务生命周期 |
| 新增 `src/servers/hls_job_manager_test.go` | 调度、失败、取消、退出、短片等行为测试 |
| 现有 `hls_handler_test.go`、`playback_resolver_test.go`、`hls_cache_janitor_test.go` | HTTP、状态映射和回收竞态测试 |
| `src/pipeline/stages/convert_mp4.go` | 录后产物创建、验证和发布 |
| `src/pipeline/manager.go`、`executor.go`、`store.go` | 沿用已有任务持久化和执行机制，避免再造第二套离线队列 |
| `src/servers/handler.go` 及现有进度存储调用点 | 视频条目去重、文件关联、兼容旧路径 |
| `src/configs/config.go`、`src/pkg/migration/` | 仅新增确有必要的配置，并可迁移 |

客户端参考：[iOS 播放页](</Users/xumy/Desktop/bilive/bililive-ios/Live OS/Live OS/Views/PlayerView.swift>)、[鸿蒙播放页](/Users/xumy/Desktop/bilive/Live-os-Harmony/entry/src/main/ets/pages/PlayerPage.ets)、[鸿蒙 API 客户端](/Users/xumy/Desktop/bilive/Live-os-Harmony/entry/src/main/ets/net/APIClient.ets)。修改前通过 `rg` 定位对应模型和全部消费者，不能只修改页面。

## 2. A0：恢复编译和建立可重复测试

### A0-1 编译阻断

文件：`hls_job_manager.go`、`playback_resolver.go`。

- [ ] 记录 HEAD、工作区差异、Go/FFmpeg 版本和构建环境。
- [ ] 执行 `make dev`，保存原始失败结果；工具链缺失则先明确阻断，不虚构编译输出。
- [ ] 删除新实现的同包 `contains`，调用标准库 `bytes.Contains`；保留 `handler.go` 中原有字符串切片函数。
- [ ] 清理 resolver 未使用的 `sync` 导入，运行 gofmt。
- [ ] 再次执行 `make dev`；此项完成标准为真实退出码 0。

核心替换形式：

```go
import "bytes"

// 原 contains(content, []byte("#EXT-X-ENDLIST"))
bytes.Contains(content, []byte("#EXT-X-ENDLIST"))
```

### A0-2 可控子进程测试设施

文件：新增 `hls_job_manager_test.go`，必要的内部依赖注入放在 manager 内部，不增加公共生产 API。

- [ ] 用 Go 测试辅助进程或可控 runner 构造四种行为：首片后等待释放、单片正常结束、启动即非零退出、首片后非零退出。
- [ ] 辅助进程写出真实格式的最小清单和非空分片；测试程序负责创建与清理临时目录。
- [ ] 用事件/通道控制“片已发布”“允许退出”，不靠固定 sleep 猜进程状态。
- [ ] 注入可控时钟或短测试超时；所有等待均有上限；正常测试不得耗费真实三十分钟。
- [ ] 支持统计实际启动数、Wait 次数、最大同时运行数；测试结束确认所有辅助进程回收。
- [ ] 测试使用独立 manager；不得让全局 job map 污染其他用例。

**交付：** 编译恢复补丁、辅助进程使用说明和至少一个进程启动/退出自检测试。

## 3. A1：统一入口，真正实现边切边播

### A1-1 所有 HLS 请求使用同一作业拥有者

文件：manager、handler、resolver 及各自测试。

- [ ] 检索全部 `buildHLSCache`、`exec.CommandContext` 和 HLS 路由调用，列出真实入口。
- [ ] 合并“获取/创建并安排执行”为不可分割操作，防止创建 queued 后无人负责启动。
- [ ] resolve 和直接 GET m3u8 调用同一个确保作业入口；删除旧 `buildHLSCache` 执行逻辑及重复 ffmpeg 参数。
- [ ] 两种入口同时请求同一文件，仅启动一次 ffmpeg；热缓存不得重新删除并构建。
- [ ] 在作业登记前验证可复用的完整缓存；旧不完整缓存不直接当作健康活动作业，按恢复策略处理。
- [ ] 保留正在录制的拒绝逻辑、`.m3u8` 后缀和签名校验。

建议内部接口契约，允许沿用现有命名但必须一次性更新全部调用方：

```text
EnsureJob(cfg, key, sourcePath, cacheDir, playlistPath) → job
  原子去重；确保新任务已进入可调度队列；HTTP 调用不等待完整转换。
Snapshot(job) → status, error, retryAt
  返回一致快照；不能通过读取多个未同步字段拼出状态。
WaitPlayable(requestContext, job) → snapshot, error
  只等待首批发布、终态或请求取消；请求取消不终止共享任务。
```

**必需测试：** `TestHLSResolveAndPlaylistShareOneJob`、`TestHLSCompleteCacheDoesNotRebuild`、`TestHLSDirectPlaylistUsesManagedJob`。逐个证明旧行为失败、修复后通过。

### A1-2 拆开作业互斥与清单读取

- [ ] 移除跨越 `cmd.Wait()` 的清单读取共用锁。
- [ ] 清单采用原子发布，reader 一次读取稳定版本；不让 HTTP reader 排在整个转码任务之后。
- [ ] 只在目录初始化/替换、作业登记和回收决定等必要阶段使用短临界区。
- [ ] 所有锁记录顺序，禁止 manager mutex 和 job mutex 反向嵌套。
- [ ] 持引用计数时先 unlock 再减 users，避免旧锁仍持有时被替换为另一把同 key 锁。

关键测试的事件顺序必须是：

```text
启动作业 → 辅助进程发布首批分片 → 进程等待 release
→ resolve 返回可播 → GET m3u8 返回 200 → GET 引用分片成功
→ 断言进程此时尚未退出 → release → 正常 completed
```

**必需测试：** `TestHLSPlaylistReadableBeforeProcessExit`。若只断言 ready 而没读取清单，视为未完成。

### A1-3 进程退出、短片、超时

- [ ] `Start` 成功后立即安排唯一的 `Wait`，通过结果通道同时观察退出与首片，禁止首片等待阻止退出检测。
- [ ] context 由 worker/manager 持有，正常终态后 cancel；HTTP 生命周期不控制共享进程。
- [ ] 运行期间首批条件采用已验证的清单引用分片；正常退出的合法单片视频无需等待两片。
- [ ] 进程无输出立即失败时尽快返回 failed，保留有限长度 stderr 和真实退出原因，不统一误报首片超时。
- [ ] 首片超时、无进展超时、总体运行预算分开；总体运行预算从实际开始执行计算，不占用排队时间。
- [ ] 超时与关闭保证进程被回收且 Wait 恰好执行一次。
- [ ] 不随意改小分片时长作为性能修复；copy 模式不能创造源关键帧。

**必需测试：** `TestHLSSingleSegmentCompletes`、`TestHLSImmediateExitFailsPromptly`、`TestHLSRequestCancelKeepsSharedJob`、`TestHLSJobTimeoutReapsProcess`。

## 4. A2：排队、恢复与缓存资源闭环

### A2-1 真正的有限并发队列

- [ ] 使用 FIFO 待执行队列和固定 worker 数，或等价的释放名额后自动调度机制；不能仅保留 queued 状态。
- [ ] worker 开始任务时再创建运行时限；排队取消/服务关闭能移除队列项。
- [ ] 防止同一 job 二次启动，成功、失败、超时、panic 均释放运行名额。
- [ ] 给待执行队列设置容量上限；超过上限返回明确可重试的繁忙结果，不能无限持有 job/context。
- [ ] 保持现有并发默认值作为初始行为；若提供可配置参数，同步迁移和读写验证。

**可复现测试：** 并发设 3，阻塞前三个辅助进程，提交第四个；释放任意一个后第四个自动进入 running。全程最大运行数不超过 3。同 key 并发提交 20 次仍只启动一次。

**必需测试：** `TestHLSQueuedJobStartsWhenSlotReleased`、`TestHLSSameKeyStartsOnce`、`TestHLSQueueCapacityBounded`。

### A2-2 失败重试和过期记录

- [ ] failed 记录携带下一次允许重试时间；用户后续请求到期后可启动一个新尝试。
- [ ] 多个同时重试请求只能产生一个新尝试；活动作业不能被失败清理误删。
- [ ] `RetryAfter` 与实际退避时间对应；不要返回五秒但永不重试。
- [ ] 失败记录清理不依赖缓存目录是否存在，覆盖 mkdir/寻找 ffmpeg 失败。
- [ ] 重试按新的有效配置构建命令，修复路径配置后可恢复。
- [ ] 使用尝试标识或对象身份保护：旧尝试的迟到完成/清理不能覆盖新尝试状态。

**必需测试：** `TestHLSFailedJobRetriesAfterBackoff`、`TestHLSRetryRequestsDeduplicate`、`TestHLSFailedJobWithoutDirectoryExpires`。用虚拟时钟推进退避，不等待真实 TTL。

### A2-3 缓存回收与服务关闭

- [ ] 将“判断可回收”和“标记正在回收”放进统一同步协议，关闭检查后立即启动任务的竞态窗口。
- [ ] queued/starting/playable 构建不得回收；清单和分片请求获得短读租约。
- [ ] 已完成但仍被观看的缓存，使用可配置或内部明确的闲置保护时间；访问更新时间必须进入回收判断，不能只在扫描前碰一次 mtime。
- [ ] 容量不足且全部条目受保护时，限制新任务并返回明确原因，不能强删正在使用的缓存。
- [ ] 删除前保证没有活跃拥有者；删除后按作业身份移除记录，不能取消刚创建的新尝试。
- [ ] 将 manager 关闭接入既有服务退出流程：停止接收、取消排队、取消运行、回收进程。
- [ ] 明确重启策略：若保持启动清缓存，必须早于任何请求和任务；不要同时声称跨重启命中缓存。

**必需测试：** `TestHLSJanitorSkipsRunningJob`、`TestHLSJanitorCannotRaceNewAttempt`、`TestHLSJanitorProtectsActiveRead`、`TestHLSShutdownReapsJobs`。

## 5. A3：产物、公共协议和端到端验收

### A3-1 验证清单引用，区分 partial

- [ ] 解析 EXTINF 与后续本地 URI 的配对，不只搜索 EXTM3U 或统计目录中 ts 数量。
- [ ] 清单引用文件必须存在、非空、不是临时文件，路径必须限制在缓存内；不接受遍历路径或未授权外部 URI。
- [ ] 已完成检查覆盖所有引用分片；没有引用的多余 ts 不算成功。
- [ ] 非零退出且有有效前缀时内部记 partial；无有效前缀则 failed。
- [ ] 不因存在分片就补 ENDLIST 冒充完整；有 ENDLIST 也不能忽略进程失败。
- [ ] 第一批只修服务端时，保守将 partial 映射为公共 failed 并提供错误说明，避免旧客户端当成完整 ready。
- [ ] 若要允许播放部分内容，作为独立客户端协议任务增加可识别字段和界面提示；未完成客户端支持前不得宣称“部分内容体验已支持”。

**必需测试：** `TestHLSPlaylistMissingReferencedSegment`、`TestHLSPartialIsNotReportedAsNormalReady`、`TestHLSPlaylistRejectsTraversal`。

### A3-2 探测与日志

- [ ] 探测失败不回退未知 TS copy；返回可识别失败，或使用经过验证的明确兼容策略。
- [ ] 保留源版本对应的探测缓存；格式输出字段减少不作为“禁止全文件扫描”的证明。
- [ ] 日志包含 job ID、状态转换、排队时长、探测耗时、首批耗时、退出原因；不输出 API key 或完整签名 URL。
- [ ] ffmpeg stderr 有界保留，失败报告能区分缺工具、源损坏、超时和磁盘不足。

### A3-3 自动化与真机

- [ ] 执行 `make build-web dev`、`make lint`、`make test` 并保存退出码和摘要。
- [ ] 执行 `go test -race -tags=dev ./src/servers -run 'TestHLS|TestResolvePlayback' -count=1`；若当前项目标签不同，以构建脚本为准并记录实际命令。
- [ ] 在真实 FFmpeg 环境分别验证 H.264/AAC copy 与 HEVC→兼容转码，证明首帧前无需完整生成。
- [ ] 测试客户端实际使用的鉴权入口；签名 GET URL 使用 GET 验证，不能因 HEAD 签名不匹配误判不可访问。
- [ ] URL 解析按相对 URL 规则基于清单地址合成，不直接把相对路径交给 curl。
- [ ] iOS/鸿蒙验证：从头播放、时长增长、已生成范围内 seek、未生成区域恢复行为、结束事件、失败提示。
- [ ] 记录设备型号和实际系统版本，不沿用旧计划未经确认的最低系统版本。

**A 批次交付门槛：** 编译无阻断；队列可推进；失败可恢复；首批后真实 HTTP 可读；单片正常；无旧裸启动入口；缓存和退出测试通过。真机不可用时区分“服务端验证完成”和“双端发布验收未完成”。

## 6. B：录后 MP4 主路径

### B1 录后转换与安全发布

文件：`convert_mp4.go`、现有 pipeline 配置/执行/存储，新增对应阶段测试。

- [ ] 查明实际有效 `convert_to_mp4` 配置、自定义命令覆盖关系及任务触发点；文档记录，不能仅改示例 YAML。
- [ ] 录制文件关闭后入既有处理队列；服务重启后从持久化状态恢复未完成任务。
- [ ] H.264/AAC 兼容输入执行视频音频 copy + faststart；输出唯一临时路径。
- [ ] 已是 MP4 也检查索引位置和编码信息，不能按后缀无条件跳过。
- [ ] 验证轨道、时长合理性和产物可读取后原子发布；失败保持源文件及错误状态。
- [ ] 默认保留源文件；仅执行用户已有删除策略，并确保验证成功后才删。
- [ ] 检查目标路径冲突、磁盘满和发布时失败；不覆盖其他录播的已有 MP4。

命令基线，仅用于已确认兼容的输入：

```sh
ffmpeg -i input.flv -map 0:v:0 -map '0:a:0?' \
  -c copy -movflags +faststart output.partial.mp4
```

**必需测试：** `TestConvertMP4PublishesOnlyValidatedOutput`、`TestConvertMP4FailureKeepsSource`、`TestConvertMP4DoesNotOverwriteExistingAsset`；真实 ffprobe 验证 codec 未被无谓重编码。

### B2 播放版本关联与完整录播体验

- [ ] 定位现有录播身份、列表和观看进度存储；优先复用已有稳定 ID。
- [ ] 一条录播关联源文件与 MP4，列表默认不重复显示两条。
- [ ] 旧 relPath 请求仍能解析到同一录播的可用版本，原有进度可恢复。
- [ ] resolve 优先返回已验证且兼容的 MP4 file URL；准备中保留正确状态或走 A 的受控兜底。
- [ ] 同一源的离线转封装与 JIT 共用资源预算，避免同时无约束读写。

**必需测试：** `TestRecordingConversionKeepsIdentityAndProgress`、`TestPreparedMP4ResolveDoesNotStartHLS`。

### B3 实际 Range 验收

- [ ] 经真实反向代理请求首部和中间字节范围，验证 206、Content-Range 和返回字节。
- [ ] 双端从头、50% 和接近结尾处播放；验证签名刷新不丢进度。
- [ ] 记录已准备 MP4 的首帧；明确网络、码率与样本数量。P95 ≤ 2 秒是目标，不是未经测量的完成结论。

## 7. C：能力分流与兼容版

### C1 先建立媒体和设备矩阵

- [ ] 媒体元数据包含 codec、profile、level、位深、分辨率、帧率、HDR、音频编码/声道。
- [ ] 客户端上报或由已验证配置提供能力；不只依赖 User-Agent 或“HEVC”单个布尔值。
- [ ] 建立固定测试样本：H.264/AAC、H.264/非 AAC、HEVC Main、Main10/HDR、无音轨。
- [ ] 已验证 HEVC MP4 原画直放；能力未知先选择保守兼容路径，不连续黑屏重试。

### C2 只转必要内容

- [ ] 视频兼容而音频不兼容：`-c:v copy`，仅音频转 AAC。
- [ ] 视频不兼容：后台生成 H.264/AAC 兼容版，保留原画关联。
- [ ] HDR 转 SDR 采用显式色调映射并验证画面，不把简单改 pix_fmt 当作正确 HDR 转换。
- [ ] 不同输出规格使用不同缓存/资产 key，避免原画与兼容版互相覆盖。
- [ ] 补决策表测试，每个样本断言选择的路径和实际输出 codec。

### C3 客户端回退与进度

- [ ] 更新两个客户端的返回模型和所有消费处，区分准备中、可播、失败及部分内容。
- [ ] 播放失败最多进行有界的兼容回退，保留录播 ID 和观看位置。
- [ ] 未生成时间区域不能假装可 seek；提供准备提示或切换已完整产物。
- [ ] 按各客户端 AGENTS.md 执行构建与测试，交付设备矩阵和结果。

**不在 C 的必做范围：** HEVC fMP4 HLS、多码率 HLS、按任意时间点重新启动转码。这些只有在 MP4 + 兼容版仍不满足已证实需求时单独立项。

## 8. D：历史库整理与正式验收

### D1 可恢复的离线整理工具

- [ ] 创建扫描/预览模式，输出候选文件、源版本、预计操作和磁盘预算，不默认开始批量转换。
- [ ] 执行模式必须显式启用，遵守已有处理队列和并发上限。
- [ ] 已验证产物跳过；源 mtime/size 变化则重新核对，录制中跳过。
- [ ] 持久化每项成功/失败，支持中断恢复和失败重试。
- [ ] 空闲空间不足时暂停新任务；保留当前错误证据和源文件。
- [ ] 先小样本验证关联、画质、时长和进度，再交付批量执行说明。工具交付不等于授权对真实库执行。

### D2 最终报告与回退准备

- [ ] 更新 CHANGELOG 和实际配置说明，删去已证伪的性能保证。
- [ ] 回退文档记录已验证的版本/镜像标识、配置/数据兼容边界及操作步骤；不能把 `git checkout main` 当作已验证回退。
- [ ] 回退或停用转换不能删除录播，不要求为暂停处理而停止正在进行的录制。
- [ ] 完成故障矩阵：长 GOP、长录播、磁盘满、进程失败、并发、重启、签名过期、活跃回收。
- [ ] 将 HTTP ready、首片可读、播放器首帧分别统计，不混为同一指标。

## 9. 旧计划中禁止直接继承的结论

| 旧说法 | 本 TODO 的要求 |
|---|---|
| 去掉 duration 就不会扫描文件 | 实测探测耗时和读取行为，设置预算与缓存 |
| 两三个轮询后 ready 就证明首帧快 | 必须读到清单和分片，并记录播放器第一帧 |
| first segment 存在即可视为完整可播 | 检查清单引用、有效分片、进程与完成状态 |
| 所有客户端不用改 | 核查状态协议、增长时长、恢复进度与失败回退 |
| 鸿蒙不兼容就改成增长 VOD | VOD 不可增长；替代路径必须符合规范并真机验证 |
| 失败有分片就补 ENDLIST | 标 partial，不伪装完整；公共状态按 A3 保守处理 |
| 既有锁已覆盖并发 | 用真实入口测试确认去重和读取不阻塞 |
| 性能只会提升 | 同时验证录制吞吐、磁盘、CPU、并发和首帧 |

## 10. 每批交给审查 agent 的材料模板

复制下列模板填写，缺失证据写“未执行 + 原因”，不要留空或笼统写“全部通过”。

```text
批次与任务 ID：
基线 SHA / 最终 SHA（未提交则提供 diff）：
改动文件与行为变化：
原问题的复现方式：
修复前失败证据：
修复后测试命令、退出码、结果摘要：
构建 / lint / 全量测试 / race：
真实 FFmpeg 集成结果：
双端设备、系统与首帧/续播结果：
配置与迁移影响：
未完成项及原因：
当前风险和回退边界：
```

原审查 agent 检查：实际 diff、调用链、重复入口、锁顺序、状态可达性、测试是否验证用户行为，以及报告与证据是否一致。不能只按执行 agent 的“已修复”摘要批准。

## 11. 可直接发给执行 agent 的起始指令

> 请读取本 TODO 和仓库 AGENTS.md，先执行 A0–A1，基线参考 `261ad73`。先确认现有 HEAD 是否已有修复，不覆盖用户未提交工作。目标是恢复构建、统一 HLS 入口并证明 ffmpeg 未退出时清单和分片已经可以读取。每项逻辑修复附回归测试，按第 10 节交付证据供另一位 agent 审查。第一批不混入 MP4、客户端能力或历史迁移改造；未经另行授权不提交、不推送、不部署、不删除录播。
