# T4 账本 — 主计划：docs/superpowers/plans/2026-10-03-recording-playback-mainline.md

BASE: 3cd117e。用户要求开始下一步，并要求每次改完验证后主动本地 commit。

范围：仅执行主计划下一阶段 T4；T5 及之后仍待后续阶段。

Ruling: 沿用已有 feature/jit-hls-optimization 工作树 — T3 已提交且工作区干净，用户要求继续当前主线 — 成本是继续使用同一分支，提交边界区分阶段。
Ruling: 多档仅允许 source/720p/1080p，默认空，按录后配置快照执行 — 主计划要求有限档且不能悄改真实配置 — 成本是其他档位需另行扩展并升规则版本。
Ruling: 持久 HLS 使用独立发布意图及最后提交的完整 manifest，已生成不可变分片保存在版本目录的私有子目录，目录 proof 复用 T2 v2 表 — 文件集合不能靠单个 MP4 发布接口表达 — 成本是新增集合发布协议，需中断恢复和身份替换回归。
Pre-flight: T3 来源版本与准备任务 → T4 复用同一来源快照、去重状态及共享预算，HLS 在 prepare_playback 之后执行，原始来源仍保留。
Pre-flight: T2 目录 proof → T4 每个清单/分片的验证证明，HTTP 只查询选定资产和请求文件，不扫描全录播库、不启动持久准备。
Pre-flight: 临时缓存回收 → 持久资产根 media-assets，JIT 根 hls-cache，生命周期互不混用。

验证顺序：配置/迁移与阶段快照 RED→GREEN；真实 FFmpeg 多档 VOD/复用、意图恢复、空间/预算/取消、替换失效；HTTP 完整与增长清单、签名/Range、Retry-After 与未来分片等待；全仓检查、独立审查、修复和本地提交。

Task T4: 配置/迁移、入队快照与有限档位 RED→GREEN（red-initial.log / green-config.log）；集合发布验证 RED→GREEN（red-bundle.log / green-bundle.log）；目录 proof 与阶段 RED→GREEN（red-catalog.log / green-stage.log）；HTTP、Retry/未来分片与 resolver RED→GREEN（red-http.log、red-retry-wait.log、red-resolver.log → green-http.log、green-resolver.log）。
Task T4: 真实多档 FFmpeg、空间/预算/取消、数据库重开/发布复用通过（green-real-resources.log）。最初短样例不保证末尾 6 秒后关键帧，实测为测试样例问题；样例改为固定 GOP 形成多片，不改 source 的原 GOP 行为。
Ruling: 短样例测试设固定 GOP，但 source 保留原 GOP — 复制策略要求不盲目重编码 — 成本是 source 无严格 6 秒分片保证，长 GOP 的首帧/拖动待设备验收。
Ruling: 保留阶段账本和可提交证据，不删除计划工作成果 — 用户要求可追溯且父目录不是 Git，已有工作不可 reset/clean — 成本是仓库文档体积增加。
Ruling: 沿用当前分支并主动本地提交，不另询问 merge/push 菜单 — 用户已明确每次完成主动 commit、此轮只继续下一阶段 — 成本是整合分支仍需后续授权。
Final review: 独立上下文 t4_review，只读审查全部 T4，Critical 0 / Important 2 / Minor 1；记录于 review.md。
Final: fixed 源改名失效 — TestPersistentHLSSourceRenameKeepsIdentityAndRejectsReplacement 与 TestPersistentHLSReuseAfterSourceRename RED→GREEN；覆盖 T3 主 MP4 准备之后改名、旧路径复用、新源替换；修复后 make test 全仓通过。
Final: fixed 崩溃遗留半成品 — TestHLSRestartCleansOnlyOwnedUncommittedBuildsBeforeAdmission 与 TestAbandonedBuildCleanupProtectsIntentAndCommittedData RED→GREEN，os.Exit(72) 实际子进程留下半成品，清理发生在空间准入前且受 claim/预算保护；修复后 make test 全仓通过。
Final: minor (deferred): SharedBudget 无 FIFO/最长等待保证；继承预算逻辑，现已证明上限和可取消，公平性未证明。
Final: Ruling: 设备解码未判断 — 列独立真机待验，HTTP/媒体核验不足以证明首帧 — 成本是设备兼容问题可能仍在。
Final: Ruling: 断电与平台 Sync 未判断 — 进程退出及交叉编译已测，实际平台断电列待验 — 成本是持久性边界未实机证明。
Final: Ruling: 同 UID 原位恶意改写未判断 — 受管理资产存储保持信任边界，同句柄仅防路径替换 — 成本是防此类本地威胁需额外存储隔离。
Final: Ruling: 生产规模/长 GOP 性能未判断 — 保留 source GOP，独立长样本容量列待验 — 成本是小时级吞吐与延迟不能从短样例推断。
Final: Ruling: 能力/质量选择未判断 — 按主计划留在 T5，当前显式 profile 与旧入口兼容 — 成本是尚无统一会话自适应决策。
Final: Ruling: 审查未判断最终全仓验证 — 主线程修复后实际执行六组最终验证并读取退出码 — 成本是结果只证明当前源码与本机环境。
Task T4: complete（自动验证范围，BASE 3cd117e；make build-web dev / make lint / make test / 相关包 race / Windows与Linux make dev 全部退出 0）。真机、真正断电与生产容量不能据此标为完成；T5 未开始。
