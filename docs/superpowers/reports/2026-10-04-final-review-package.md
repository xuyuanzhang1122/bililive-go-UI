# T7–T9 全范围审查包

使用 requesting-code-review/code-reviewer.md 的要求，独立只读检查，不修改工作树/索引/分支，不生成额外子agent。中文报告，Critical/Important/Minor 分级，给文件行号、触发、影响、修复建议；每项按使用者实际影响判定。显式列 Declined to judge，即考虑过但以超范围为由放弃判断的行为，空列表也说明。不得以规格未写为由降级。

## 范围
- /Users/xumy/Desktop/bilive/bililive-go-UI：b2dfa78..ddc35d0（T8/T9）
- /Users/xumy/Desktop/bilive/bililive-ios：ceb7b16..7ecd718（T7及T8历史身份，9fb7a72包括先前累积检查点，见基线报告）
- /Users/xumy/Desktop/bilive/Live-os-Harmony：a1321bb..c1bba1d（T8历史身份，T6原基线作为消费者背景）

## 计划/规格/记录
主计划 /Users/xumy/Desktop/bilive/docs/superpowers/plans/2026-10-03-recording-playback-mainline.md
跟踪副本及ledger /Users/xumy/Desktop/bilive/bililive-go-UI/docs/superpowers/{plans/2026-10-03-recording-playback-mainline.md,reports/2026-10-04-t7-t9-progress.md}
设计依据 /Users/xumy/Desktop/bilive/bililive-任务交接与残留问题-2026-10-03.md
报告：服务端reports/2026-10-04-t8-web.md、t9-history.md，iOS Docs/superpowers/reports/2026-10-04-t7-ios.md、t8-history-identity.md，鸿蒙TASKS/REPORT-T6-20261004.md、REPORT-T8-HISTORY-20261004.md。Ruling 行在上述ledger，逐条判断。

## 审查重点（原文）
1. 发布后源、目标或关联被替换：拒绝删除新文件，保留可信产物；T1 回归覆盖。
2. 中断和重启：发布/删源/准备任务幂等恢复，未知版本不猜身份；T1/T3 故障注入覆盖。
3. 活跃录制、低空间及资源争用：跳过录制中源，暂停重任务，不损伤源；T3/T4/T9 覆盖。
4. 旧路径、源替换与房间统计：别名和历史不串录播，不重复计算逻辑条目；T2/T8 覆盖。
5. 会话过期、设备退出、旧回调晚到：刷新授权或取消，错误真实可见；T5/T6/T7 覆盖。

## 实现和验证
T7 Codable DTO、严格fallback、native能力、会话模型和真实Pillarbox刷新位置/意图。样片iPhone首帧、暂停240秒刷新、超过原300秒token播放以及退出cancel已验证；指定大录播尚未获得，不将样片宣称完整验收。正式包已合法签名安装启动。
T8 OpenAPI运行时类型客户端、Go AST旧路由、受控Bearer fetch SSE、Web会话/能力/批删、SQL历史身份迁移与别名归一。三端历史POST捕获原身份与认证；目录写无身份409保护替换后的来源。
T9 默认预览，owner+摘要+confirm绑定持久SQL事务，只进原prepare_playback，60秒开始限速同时1个，暂停/恢复及重启，来源/规则变动拒绝，低空间pause保源。真实FFmpeg临时样片实际发布与原字节一致验证。真实历史库尚未执行。
全make build-web dev、lint0issues、test、go test -race ./...以及Web5Node契约脚本、yarn lint通过；iOS两个脚本与模拟器/签名设备build及覆盖启动通过；鸿蒙Node88/88、Hypium107/107及合法副本签名构建通过。

## 需刻意检查
Web 410 新会话是否充分核验 asset_version；SSE认证改变是否停止旧用户流；历史重试/清理后唯一约束和预览过期数据增长；历史任务pause/restart/低空间及全局普通队列公平性；DTO字段/能力证据/刷新URL取消竞态。以上是待审候选，按实际源码判定，不直接当作已知结论。

## 已知验收边界
真实大录播/库原先不可访问。用户2026-10-04最新已授权SSH访问192.168.1.189及尝试替换服务端统一测试；部署前正在检查远端并准备可回滚备份，等待此次审查修复后发布；真实历史批量转换仍需针对具体预览授权。凭据不入报告。T6曾意外覆盖本机默认配置，已披露，恢复材料未有；后续隔离环境变量防再写，远端配置原先未变。
