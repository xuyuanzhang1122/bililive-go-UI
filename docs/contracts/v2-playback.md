# T5 v2 录播与播放契约

权威结构在 `openapi-v2.yaml`（JSON 格式的合法 YAML），共享请求/响应样例在 `examples/v2-*.json`。服务端契约测试将真实 HTTP 响应与同一份 schema 比较；没有声称覆盖所有 OpenAPI 校验规则。iOS 与鸿蒙应在后续 T6/T7 消费这些样例。

管理请求使用 `Authorization: Bearer <Key>`。v2 不使用 `_key`、`X-API-Key` 或同源免认证。媒体只接受短期 `play_token`，URL 不包含管理 Key。令牌绑定会话、来源版本、资产版本和过期时间；撤销 Key、取消会话、来源或资产改变都会拒绝后续媒体请求。

1. `GET /api/v2/recordings?limit=50&after=<recording_id>` 读取已核验 SQLite 目录；每项包含稳定 ID、来源版本及十进制字节文本 `size_text`。limit 为 1–100，满页使用返回的 `next_cursor` 继续。
2. `POST /api/v2/playback-sessions` 提交录播 ID、来源版本和客户端视频/音频、容器、协议能力。未知视频 profile/level/位深等参数不会被当作兼容。
3. 会话决策为 `direct`、`remux`、`audio-transcode` 或 `video-transcode`。仅实际探测且版本复核通过的现有资产可以 `ready` 并携带 URL。请求不会启动同步转码，也不会生成任意新档位。
4. `GET /api/v2/playback-sessions/{id}` 查询状态、刷新播放授权。会话 15 分钟、媒体 token 最多 5 分钟，按 `refresh_after_seconds` 刷新。非 ready 没有 URL；存在准备任务时按 `retry_after_seconds` 与 `Retry-After` 轮询；未准备的资产明确 `asset_not_ready`。
5. `DELETE /api/v2/playback-sessions/{id}` 取消该会话。取消不停止其他播放器共享的后台准备任务。过期会话返回 410，需要重新创建；进程重启同样需要重建会话。每用户最多 32、服务实例最多 1024 个未取消且未过期会话；取消立即释放活跃配额，状态记录总数最多 2048，超限淘汰最早过期的取消记录。
6. 媒体地址支持 GET/HEAD；文件与 TS 分片支持 Range。HLS 清单的分片链接使用相同会话 token，不会暴露原始磁盘路径。已开始的传输不保证被取消操作即时中断。
7. `POST /api/v2/recordings/batch-delete` 最多 100 项，每项绑定 ID/来源版本。HTTP 200 可能包含部分失败，必须逐项检查 `status`、`deleted_files` 与 `error`。拒绝录制中、版本变化及越界路径；只删除有身份和证明的普通媒体文件，未知旁文件保留。Linux/macOS 在可信目录句柄内原子隔离、版本复核后删除，恢复冲突保留隔离对象且不覆盖新路径；Windows 使用实际文件句柄删除。其他平台失败保源。Unix 隔离中进程退出可能留下 `.bilive-delete-*/media`，需人工恢复，不自动推断已删完成。I/O 故障可能部分删除，计数报告实际完成数；目录重新核验期间新管理读取返回可重试错误。

管理响应为 `{data,error,request_id}`，错误包含 `code/message/retryable`，可重试的管理错误另含 `retry_after_seconds`。source_changed、no_matching_capabilities、过期和授权拒绝不能依靠同一个旧请求无限重试。

旧 `GET /api/playback/resolve/<旧路径>?session_id=<id>` 可显式适配 v2 会话，必须提供 Bearer Key，并保留旧 `commonResp` 包装。paused 映射 processing、cancelled 映射 failed；旧调用不带 session_id 时沿用原有路径解析。T5 未迁移配置、直播控制、账号或备份接口。
