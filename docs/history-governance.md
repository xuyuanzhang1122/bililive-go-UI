# 历史录播治理

默认操作是预览。API 只读取 T2 已核验目录及有限候选来源快照，保存预览审计记录，不启动转换或删除。管理接口要求 Bearer，禁止 query Key。每次最多100个逻辑录播，next_cursor继续分页；每页独立确认，不因翻页确认其他候选。

1. POST `/api/v2/history-previews`，JSON `{}` 或 `{"after":"上一页游标","limit":100}`。检查 data.items、skipped、预计空间文本、策略、profile_version、根目录、id 和 digest。预计空间是每候选3倍逻辑文件大小加1GB的保守预算，不承诺最终产物大小，实际执行另有磁盘核验。
2. 未授权时停在预览。预览30分钟有效，每用户最多32个有效待确认预览；不因读预览自动执行媒体探测、转换或删源。
3. 用户针对这一页候选授权后，POST `/api/v2/history-previews/{id}/confirm`，JSON `{"digest":"该预览原摘要","confirm":true}`。没有confirm=true、摘要不匹配、过期、用户不匹配、来源变化或规则变化都拒绝。返回task_ids可从原Pipeline任务接口跟踪。
4. POST `/api/v2/history-previews/{id}/control`，JSON `{"action":"pause"}` / `{"action":"resume"}`。暂停标志持久保存，取消当前转换并保源；进程重启仍暂停。恢复后继续遵守限速。GET `/api/v2/history-previews/{id}`读取结果。

确认与任务创建在同一SQLite事务中，重复确认返回原任务。另一个预览若抢先确认同一录播，会明确拒绝，不能通过新预览取得旧任务控制权。任务只包含既有prepare_playback阶段，版本绑定，不进入修复、上传或删除阶段。历史治理全局最多同时一个任务、开始间隔至少60秒；不占满普通任务队列，预算时间跨重启保存。录制中来源跳过/暂停，变化来源失效，低空间暂停。实际发布、验证和失败保源均使用原T1/T2/T3协议。

API Key只放Authorization请求头。不要将Key写入命令历史、报告或URL。生产执行前提交具体预览供用户审核；本轮只在临时样本执行，未对真实历史库确认或执行。
