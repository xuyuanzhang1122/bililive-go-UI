# 执行账本 — /Users/xumy/Desktop/bilive/docs/superpowers/plans/2026-10-03-recording-playback-mainline.md

Ruling: 在用户指定的现有工作树实施，不创建新 worktree、不提交；证据放工作区并保留 — 三仓已有未提交成果且用户要求保留 — 错误成本为需要依靠源码指纹而非提交恢复。
Ruling: 主计划跨三仓，根目录非 Git 仓库，使用本账本替代按单仓提交生成的 SDD 脚本 — 不虚构提交或完成记录 — 错误成本为人工维护状态，每阶段指纹补足。
Pre-flight: T1 发布/删除 → T2 可信资产导入；仅 Load 完整有效 manifest 可合并逻辑录播。
Pre-flight: T2 目录 → T3 准备队列/T5 会话；ID 与版本分别持久化，不用路径作为录播身份。
Pre-flight: T2 房间统计 → 双端旧 DTO；保留文本/status 字段，仅替换服务端数据来源。
T1: 开始平台策略拆分；Windows 明确较弱持久性，不吞父目录访问错误，运行/断电验收另列。

T1: 平台策略已拆分，Mac测试与Windows交叉编译通过；真实Windows与断电测试待验，不能整体声称持久性已验收。
T2: 持久身份、事务统计、主资产来源合并、派生登记、后台生命周期与HTTP纯查询已接入；自动回归/race通过。
Ruling: 新独立 catalog DB 第一版用事务性 user_version=1 初始化并拒绝更高版本，不改 lives/pipeline 现有DB — 数据目录尚无已发布旧版需迁移 — 以后schema变化必须显式迁移，不可修改已交付v1。
Ruling: Windows目录策略明确弱化为父目录访问验证，保留媒体/意图Sync和不可覆盖硬链接 — 普通目录句柄不提供Unix等价保证 — 成功不保证断电目录项持久性，真实Windows待验。
Ruling: Linux身份包含statx创建时间，不支持该字段时报核验不可用，不猜inode重用 — 保证不串录播优先 — 旧内核/不支持btime的文件系统需后续明确兼容策略（https://man7.org/linux/man-pages/man2/statx.2.html）。
Review fixed: 交换路径身份丢失、旧源派生误标verified，两项真实RED→GREEN；追加派生目录位于输出目录的混计回归RED→GREEN；SQLite子进程退出72验证未提交事务恢复。

T2: 已增加resolver旧别名读取（SQL+单候选身份复核），真实路径优先。别名独立只读审查无新阻断；补同大小同时间替换/父链接越界拒绝回归。T3未启动，保持待办。

T3: 2026-10-04 开始，仅实施 T3；沿用现有工作树和证据账本，无提交、推送或部署。
Pre-flight: T2 单候选身份核验 → T3 源快照；录制结束捕获身份/大小/mtime，后台目录已核验后才接受，同路径替换失效。
Pre-flight: T1 发布 → T3 原画主 MP4；兼容资产跨目录需显式派生发布入口，保持源文件，不扩展删源授权。
Ruling: 新增 prepare_playback 显式开关，默认关闭，旧配置迁移保持关闭 — 避免安装修复后自动转换真实库 — 用户启用后才为新录播准备，历史库仍属 T9。
T3 RED: 新队列/状态、配置迁移、源快照及录制保护测试先运行，缺失接口编译失败，证据 red-queue.log。
T3: 第一组新增接口缺失 RED→队列/配置/目录 GREEN；三类真实 FFmpeg 样例 GREEN（copy / audio-transcode / video-transcode），已有包回归全部通过。
Ruling: prepare_playback 启用后替代旧 fix_flv_at_first/convert_to_mp4/delete_flv_after_convert，上传后删除也关闭；自定义命令与新准备组合明确拒绝 — 旧修复会改写源版本，新准备独立探测且保源 — 启用用户需理解该配置优先级，配置注释与实施记录明确说明。
Ruling: 跨目录兼容产物采用 manifest v2 绝对来源路径并复用 T1 同目标目录持久意图/硬链接/恢复；v2 关联明确禁止删源 — 兼容资产必须放 AppData/media-assets — 数据目录搬迁后旧绝对来源路径不自动猜测恢复，需要重新准备。
T3: macOS /var→/private/var 系统父目录链接导致新派生检查误拒绝，已从根及 ID/版本目录的 Lstat 检查修复；仍拒绝可控资产目录链接。
T3: 已补主文件本身可播时的资产登记 RED→GREEN，并验证30秒核验后仍保持可信主资产；暂停任务按待执行优先及上次尝试时间轮转，避免低空间老任务饿死新任务。
T3: 调度取消与运行状态提交失败两项故障注入均 RED→GREEN；进程退出72验证派生已发布而外层任务未提交完成标记时恢复复用，不重新转换。
T3: 完成 make build-web dev / make test、lint 0 issues（复用 /tmp/bilive-tools/golangci-lint）及相关包 race；后续边界回归后仍需最终完整检查。
Final review: 独立只读审查发现两项阻断：ready未核验关联与目录归属；探测后目标同大小同mtime替换被记作ready。两项复现测试纳入仓库，red-review.log 两项失败 → green-review-final.log 通过。
Final fixed: 绑定媒体验证前后完整目标身份，发布/目录提交后仍需相同身份及manifest；ready必须同时匹配目录、来源、规则证明与目标。
Final fixed: 增补后台核验回归 red-review-catalog-proof.log 失败 → 持久验证证明与失效标记通过，避免后台按相同size/mtime恢复信任。
Ruling: 录播目录新增事务性 v1→v2 迁移，仅增加 prepared_verifications 表，v1原表不改；保持已保存录播ID及统计 — 准备输出需跨重启保持完整身份/规则证明 — v1旧二进制不能打开v2目录DB，升级及回滚需保留旧DB备份。
Final declined-to-judge: 审查未评估Windows真实断电、手机首帧、生产部署、HDR正向映射；这些需要独立环境/设备证据，继续列待验，不据代码审查宣称通过。旧T1/T2/HLS并非本轮完整复审范围，既有回归随全仓运行。

T3: 核心代码与自动验证完成；最终 make test / make build-web dev / make dev、lint 0 issues、Web ESLint、相关包 race、git diff --check 通过。Mac构建及Windows/Linux交叉编译通过；独立审查两项+目录证明补充回归RED→GREEN，无延期minor。保留28文件指纹、源码快照、三仓HEAD/status及所有日志，不提交、不推送、不部署。HDR正向/WindowsLinux真实运行断电/手机验收保持待办。
