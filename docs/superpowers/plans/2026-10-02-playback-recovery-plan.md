# 录播播放任务恢复与收尾实施计划

> **给执行 agent：** 按 `superpowers:executing-plans` 逐项执行。先读本计划和引用的架构报告。默认由主线程串行实施，完成一个任务并保存检查点后再进入下一项；独立审查集中在交付前进行。已有用户授权直接修改代码，但本次请求是重新规划，当前只核查与制定计划。不要重复实现已经落盘的功能。

**目标：** 保留中断前的成果，修复 MP4 发布恢复中的数据删除风险，完成 A/B 服务端验证，再独立推进 C/D。

**架构：** HLS 使用统一后台任务拥有者；录后 MP4 复用持久化 pipeline，两者共用重媒体工作预算。可信来源关联保持旧录播路径和观看记录；发布意图用于恢复中断，不按同名文件猜来源。

**技术栈：** Go、FFmpeg/ffprobe、HTTP Range、EVENT HLS、SQLite 后处理队列与观看记录；后续涉及 iOS/鸿蒙客户端。

**设计依据：** 工作区上层 `bililive-录播播放架构与实施方案报告.md`、`bililive-录播播放最优方案复核.md`，以及 `docs/superpowers/plans/2026-09-05-recording-playback-todo.md`。原 TODO 是总体路线图；本计划覆盖当前恢复顺序，原 TODO 的未验收项不能自动勾选。

## 全局约束

- 工作目录：`/Users/xumy/Desktop/bilive/bililive-go-UI`；基线 HEAD：`261ad7328f3d0fc8c90f959ed4f56f5b5a23507f`。
- 保留当前所有未提交修改及用户原有 `docs/JIT-HLS-IMPLEMENTATION-PLAN.md`。未经明确授权不 commit、push、部署或对真实录播库执行删除/批量转换。
- 不使用 `go build ./...`；后端构建用 `make dev`；完整交付必须通过 `make build-web dev`、`make lint`、`make test`。
- Config 结构变更必须同步配置迁移；当前已新增 HLS 迁移，恢复任务不再新增配置。
- 不改写用户实际 `config.yml`。配置加载测试已使用临时副本。
- 视频源默认保留，仅执行原有显式删除策略，而且须验证、发布并提交关联成功后才允许删除。
- “代码已写”“测试曾通过”“当前交付验证通过”分别记录。HTTP 可读不等于播放器第一帧。
- 日志与恢复记录存入项目内 `docs/superpowers/evidence/`，不以 `/tmp` 或 agent 存活状态作为唯一证据。

## 审查重点

1. 并发恢复：一个流程已提交并删源，另一流程不得回滚已提交产物；由 R1 的现有失败回归覆盖。
2. 同名替换：验证或保存关联期间源/目标被替换，不得误关联或删除替换文件；由 R1 加强身份检查及测试。
3. Windows/文件系统能力：目录同步不支持不能令所有转换失败；媒体硬链接不支持时安全报错保源；由 R2 覆盖。
4. 来源删掉后的旧入口：resolve、HLS、缩略图、删改、观看进度必须按可信版本处理；由 R3 的真实接口集成覆盖。
5. 资源与退出：HLS 和 MP4 共用预算，等待/运行取消都回收进程，活跃读者不能被回收；由 R3/R4 的行为及 race 检查覆盖。

## 2026-10-02 实际核查

- 当前没有子 agent 在运行；以前的任务执行链已经中断，不能等待旧 agent 自动恢复。
- 工作区改动仍存在；HEAD 未变，代码未提交。这次不是从零重写。
- 已落盘：HLS 生命周期、队列、租约、失败恢复、资源配置/迁移、最低空间检查、探测缓存、耗时日志、公共媒体预算；MP4 验证、来源关联、列表去重、旧入口映射、签名 Range、发布意图恢复。
- 未完成：发布恢复最后一次审查反馈、Windows 目录同步处理、lint 修复、最终全仓验证和交付报告。
- 当前明确失败：`TestPublicationRecoveryKeepsConcurrentCommit`。另一流程 Save 成功后删除来源，当前流程 Save 失败，`finishPublication` 会删除已经提交的目标。视为 P0 数据删除风险，A/B 尚不可交付。
- 当前仍有待修 lint：`convert_mp4_validation.go` 中 `if size == 1 ... else if size == 0` 的 QF1003，旧 lint 记录指向该位置，当前代码仍未修改。
- 旧 `/tmp` 构建/测试日志已不可用。会话内曾有构建、HLS 多轮/race、真实 MP4/Range 通过记录；这些不替代当前最终验证。
- C 设备能力/兼容版、D 历史迁移尚未实施；双端真机和生产代理验收未完成。

新的基线证据：`docs/superpowers/evidence/2026-10-02/mp4-baseline-test.log`、`server-baseline-test.log`。日志末尾 `exit_code` 是该次命令的真实退出码。

---

## R1：修复 MP4 发布恢复中的提交/回滚边界（第一优先级）

**文件：**
- 修改：`src/pkg/playbackasset/publish.go`；必要时 `manifest.go`。
- 测试：`src/pkg/playbackasset/publish_test.go`、`src/pipeline/stages/convert_mp4_test.go`。

**接口：** 保留 `PublishValidated(source, target, temp string, sourceInfo os.FileInfo) error` 和 `RecoverPublication(source, target string, sourceInfo os.FileInfo, validate func(string) error) (bool, error)`。调用方仍在成功后执行原删除策略，不引入第二个任务队列。

- [ ] 运行 `go test ./src/pkg/playbackasset -run TestPublicationRecoveryKeepsConcurrentCommit -count=1`，保留现有失败证据。
- [ ] 定义提交判据：可信 manifest 的来源名、来源 stamp、目标名、目标 stamp 均匹配本次发布意图，且当前目标身份仍匹配已验证文件。只因源已按策略删除，不应撤销这一提交。
- [ ] 在 `finishPublication` 回滚前检查提交判据；若另一流程已经完成同一提交，幂等完成/清理自身意图，不删除目标。遇到不匹配但有效的关联或替换文件，报冲突并保留文件，不能猜测归属。
- [ ] 保存关联前和完成后重新校验目标身份/版本；不能把验证后替换的文件写入可信关联。来源变化仍拒绝提交。
- [ ] 补 `TestPublicationCommitRejectsTargetReplacement`：提交阶段目标被换，替换文件保留、不得被认作本次已验证产物。
- [ ] 补真实并发恢复测试：两个恢复调用交错，最终只保留有效的一份关联和媒体；已提交的目标不被另一流程撤销。
- [ ] 覆盖 intent-before-link、link-before-manifest、manifest-before-intent-cleanup 三个重启窗口；重复执行不重新转换，冲突保源且不覆盖。
- [ ] 执行 `go test -race -tags=dev ./src/pkg/playbackasset ./src/pipeline/stages -run 'TestPublication|TestManifest|TestConvertMP4' -count=1`；必须全部通过后记录 R1 完成。

**门槛：** 当前 P0 回归变绿；目标被替换和并发提交测试通过；没有扩大删除范围。

## R2：完成跨平台发布及静态检查

**文件：**
- 修改：`src/pkg/playbackasset/publish.go`、`src/pipeline/stages/convert_mp4_validation.go`。
- 新增：`src/pkg/playbackasset/publication_sync_unix.go`、`publication_sync_windows.go`；各文件保留清晰 build tags。
- 测试：`src/pkg/playbackasset/publish_test.go`、平台专用同步测试。

**接口：** `syncPublicationDir(path string) error` 名称不变，平台实现不能影响 R1 提交判据。

- [ ] 将目录同步实现拆到平台文件。Unix 继续同步目录并保留真实错误；Windows 采用平台适用方式，目录 fsync 不受支持时不误报所有发布失败，保留媒体和意图文件 Sync 与不覆盖的原子发布。
- [ ] 记录平台断电持久性边界，不能把 Windows 交叉编译视为真实断电验收。
- [ ] 补正常发布测试：关联可 Load、产物完整、重复执行幂等；目录同步真实 I/O 错误不能被一律吞掉。
- [ ] 将 box 长度分支改为 `switch size`，保留 0/1/普通长度、溢出和截断检查的语义。
- [ ] 运行 `make dev PLATFORM=windows ARCH=amd64` 和 macOS `make dev`；有 Windows runner 时运行该包测试，暂无则明确标记运行验证待补。
- [ ] 执行 `make lint`，退出码 0、0 issues。工具缺失时安装到项目之外并记录版本，不能删除/跳过规则以通过。

**门槛：** Windows/macOS 构建与 lint 通过；平台运行/断电验收状态单独记录。

## R3：固定 A/B 真实行为验收

**文件：** 现有 `src/servers/hls_lifecycle_test.go`、`hls_resource_limits_test.go`、`hls_prepared_asset_test.go`、`playback_asset_operations_test.go`、`playback_mp4_integration_test.go`；`src/pkg/mediawork/budget_test.go`。

**接口：** `mediawork.Shared.Acquire(ctx, limit) (release func(), err error)` 已接入 HLS 和 MP4；preparedPlayback、resolver 和现有 HTTP 路由不改变公共字段。

- [ ] 运行现有 HLS 测试：首片后进程未退出时读取清单/片；排队推进、同 key 去重、partial、失败重试、关闭、读租约和磁盘准入。
- [ ] 运行真实 H.264 copy / HEVC 转 H.264 的 HLS 集成，设置 `BILIVE_HLS_INTEGRATION=1`，不得因未安装 FFmpeg silently skip 后宣称通过。
- [ ] 运行真实 MP4 pipeline→来源关联→原路径 resolve→观看记录→签名 GET→本地代理 Range 集成。首、中、尾 Range 均 206 且字节正确；未签名/篡改签名 401。
- [ ] 运行来源删除后的单/批删改和旧 HLS 地址回归；正常活动旧 job 不另起转换；替换来源不串用旧版本。
- [ ] 增加/确认跨 HLS 与 MP4 的公共预算测试：占满预算时另一入口不得启动 ffprobe/ffmpeg；取消等待后能释放/复用名额。
- [ ] 跑上述包 race。只在仍有疑似偶发失败时重复受影响测试，不以反复重跑绿掩盖一次失败。

**门槛：** 服务端与本地代理行为通过。真机第一帧、生产代理/CDN、完整 seek/签名刷新仍单列待验收。

## R4：全仓交付检查与证据冻结

**文件：** `CHANGELOG.md`、`docs/superpowers/plans/2026-09-21-playback-delivery.md`，新增最终交付记录和 evidence 目录。

- [ ] R1–R3 完成后停止并行修改，记录 HEAD、`git status --short` 和待交付文件的 SHA256；记录 Go、FFmpeg、ffprobe、lint 版本。
- [ ] 按序执行 `make build-web dev`、`make lint`、`BILIVE_HLS_INTEGRATION=1 make test`；每个命令的 stdout/stderr、退出码写入项目 evidence 目录。
- [ ] 再运行改动包的 race 检查并保留日志。只有源码改变、出现新失败或未解决疑点才重跑已通过的检查。
- [ ] 核对前端生成 API 中 resolve 路由是否匹配实际路由；恢复本次构建产生的无关 lockfile 改动之前，先确认它在开始时无用户修改。
- [ ] 对最终 diff 进行一次独立审查，包含未跟踪新增文件。发现阻断项先修复，再重跑受影响验证。
- [ ] 更新交付记录：明确 A/B 服务端完成项、平台/设备限制、配置迁移、回退边界。保留过去记录但注明被本次证据替代。
- [ ] 输出最终可审查 diff 和证据链接。未经用户另行要求，不提交、不部署。

**门槛：** 最终源码版本对应的构建/lint/test/race 和审查均有证据。R4 完成前不说“全部完成”。

## R5：双端与生产环境验收（接在服务端交付后）

**文件：** 独立设备验收记录；需要客户端修改时先读各仓库 AGENTS.md。

- [ ] 记录实际可用设备、系统、播放器和生产代理配置，不填未经确认的型号/版本。
- [ ] iOS/鸿蒙分别验证 MP4 从头、50%、接近末尾、结束、续播与签名刷新；HLS 验证时长增长、已生成区间 seek、未生成区间提示/恢复和失败提示。
- [ ] 同一批固定样本分别测 HTTP ready、首片可读、播放器第一帧；记录样本数、网络和码率，达到足够样本后才计算 P95。
- [ ] 不满足时按设备/协议建可复现缺陷单，先修当前路径再评价性能。

**门槛：** 有真实设备/生产环境结果；没有设备时保持待验收，不能用服务端测试替代。

## R6：后续 C/D 分批规划，保持原总体目标

这两批没有因额度中断而自动取消，也没有在 A/B 中隐式完成。为控制恢复成本，进入各批前制定独立计划，不与 R1 的数据安全修复混写。

**C：能力分流与兼容产物**
- [ ] 建立 H.264/AAC、H.264/非 AAC、HEVC Main、Main10/HDR、无音轨固定样本及客户端能力矩阵。
- [ ] 明确 codec/profile/level/位深/HDR/音频能力协议及两个客户端全部消费者。
- [ ] 视频兼容只转音频；视频不兼容生成独立 H.264/AAC 兼容版；HDR 使用经验证的显式色调映射。
- [ ] 缓存 key 区分规格，失败回退有上限且保留原观看位置；完成客户端构建与设备验收。

**D：历史库治理**
- [ ] 先做扫描/预览，给出来源版本、候选任务和空间预算；不默认批量转换真实库。
- [ ] 执行复用持久化队列、预算与可靠发布；处理中断恢复、源变化、录制中跳过、失败重试和低空间暂停。
- [ ] 小样本先验证身份/时长/画质/进度，再交付显式执行方式及回退说明；工具完成不等于授权对真实库批量运行。

## 中断恢复规则与持久检查点

每项只使用这些状态：`未开始 / 执行中 / 待验证 / 已验证 / 阻塞`。每完成一个测试周期或准备长命令前更新 `docs/superpowers/evidence/<日期>/checkpoint.md`：

```text
当前任务：R1
状态：执行中
最后已完成动作：复现 TestPublicationRecoveryKeepsConcurrentCommit
最新源码指纹文件：source-manifest.json
最后测试：命令、退出码、日志相对路径
仍失败的具体场景：另一流程已提交删源，本流程误回滚目标
下一步唯一动作：修复 finishPublication 的提交判据
禁止重复：重写已落盘的 HLS 队列、来源映射、配置迁移
```

恢复时：先读 checkpoint 与日志，再比源码指纹；源码未变可沿用对应的有效证据，源码改变则重跑受影响检查。agent 状态只用于观察，不代表完成。额度中断时无需重建全部 agent 或重做前期研究。

**推荐执行方式：** 主线程串行 R1→R2→R3→R4；在 R4 使用一次独立审查。这样减少重复上下文和并行协调成本，每一步都有文件可恢复。R5 由实际设备条件决定，C/D 按各自独立计划推进。
