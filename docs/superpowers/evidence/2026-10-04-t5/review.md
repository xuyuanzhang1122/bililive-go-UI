# T5 独立审查与一次修复记录

审查者：fresh-context gpt-6-astra，主计划 executing-plans 的唯一最终审查席；只读审查基线 3c6240e 至阶段 staged 文件及补充真实批删测试。未修改工作区、index或HEAD。下面文件行号为审查时版本。

## 原始结论

优点：独立 Bearer 管理鉴权与版本绑定 token；请求只消费 T2/T3/T4 登记/证明；完整媒体能力字段；真实转码/HLS与逐项删除/未知旁文件保留测试。

- C1 Critical：delete.go:114 的最后校验与路径 unlink 分离，叶子替换、父目录链接或新录制可造成误删。需要可信目录句柄、捕获对象后复核及覆盖最终动作的录制互斥。依据可达调用顺序确认，原审查没有声称概率竞态测试已复现。
- I1 Important：v2_sessions.go:184、v2_legacy.go:30 的旧状态可覆盖新 ready/资产或 Invalidated；HLS 没有进入旧 File 判断。overlay 实际复现：A 在 ownedSession 时间检查阻塞，B 真正提交 ready，取消 A 后晚到结果将状态覆盖为 failed、资产清空。
- I2 Important：v2_media.go:134 仅 SameFile(before,opened) 与路径核验，未证明 opened 属于所选版本。overlay 按路径 A→B→打开B→恢复A 的顺序复现保护条件接受 B，非完整 HTTP 压力测试。
- I3 Important：v2_selection.go:25 把 mov,mp4 和 matroska,webm demuxer 名称当实际容器；真实 MOV＋mov 能力返回 failed/remux，可能向仅MP4客户端发布 MOV。需要品牌/DocType。
- M1 Minor：v2_legacy.go:60 所有普通文件 MIME 固定 MP4，旧播放器可能失败。
- M2 Minor：v2_sessions.go:153/192 取消仍占32配额，快速浏览会阻塞剩余有效期；按原“未过期会话”文字列产品建议。

原始 Assessment：No，需修复 C1 与 I1–I3。

## 重评级与闭环

M1/M2 按实际用户影响提升 Important：容器/MIME不一致可无法解封装；32次浏览导致15分钟停播不能因规格文字而降级。六项全部进入同一次修复，没有派发复审。

| 项目 | RED 证据 | GREEN/最终证据 |
| --- | --- | --- |
| C1 | red-review-delete-race.log：替换文件被删、父目录链接导致越界、活跃录制动作未拒绝 | green-review-delete-race.log；final-test.log、final-race.log |
| I1 | red-review-sessions-handle-container.log：file/HLS × v2/旧入口全部失败 | green-review-sessions-handle.log；final-test.log、final-race.log |
| I2 | red-review-sessions-handle-container.log：替换句柄被保护条件接受 | green-review-sessions-handle.log；final-test.log、final-race.log |
| I3 | red-review-sessions-handle-container.log：真实 MOV 非ready | green-review-container.log；新增WebM登记最初失败记录于 red-review-mime-quota.log，补登记后 final-test.log通过 |
| M1/M2 升级 | red-review-mime-quota.log：旧 MOV MIME错误、33次创建取消429 | final-test.log、final-race.log |

C1：os.OpenRoot（Go1.24）及父目录 SameFile 锚定；Linux/macOS 用 mkdirat/openat/renameat 原子捕获，按实际句柄版本复核，只在私有捕获目录 unlink；不匹配通过 Linkat 不覆盖恢复，冲突保留对象。Windows 用父句柄 NtCreateFile、实际句柄版本与 FileDispositionInformation；其他平台失败保源。最终操作与 BeginRecording 共用互斥。Unix 隔离中崩溃可能保留隐藏捕获对象，人工恢复，不自动删未知文件；Windows/Linux 只有交叉编译证据，不能扩大为实际运行。

I1：Revision CAS、取消请求不写回、两个入口共用 commitResolved，删除/取消后的旧状态不能复活。
I2：SnapshotOpenedFileVersion 按实际句柄取平台稳定身份、大小和时间并复核，与所选 FileVersion 比较。
I3：ffprobe major_brand 与 EBML DocType 区分容器；未知保持未知。缓存 v4，目录补登记 WebM。真实 MOV 改成 .mp4 仍按 MOV 识别，不向只支持MP4的客户端发布原 MOV。
M1：普通文件 MIME 从实际探测容器固定到会话，适配与传输一致。
M2：取消释放活跃配额；有界取消状态拒绝旧 token，总记录上限2048，超限淘汰最早过期取消记录。

## Declined to judge 与执行者裁定

1. 真实部署、历史批量和原生验收：本阶段未执行。本地HTTP/FFmpeg不代替这些证据；成本是仍需独立验收。
2. 不带 session_id 的旧resolver：明确保留旧鉴权/解析，不判为v2绕过；成本是旧流程不能享有v2保证。
3. 非默认用户同ID原地换Key：现有metadata无入口，仅创建/禁用/撤销；保留当前模型，成本是未来新增入口需同步凭据与会话撤销。
4. 取消即时中断已开始传输：契约不保证；后续请求拒绝，成本是客户端还需主动取消自身网络传输。

## 最终结果

一次修复后 make dev、make build-web dev、make lint（0issues）、make test、相关pipeline子包/catalog/decision/servers race、Windows/Linux amd64交叉编译均退出0。源码与日志 SHA256 在 verification.json。没有剩余延期minor；平台、真机、生产与隔离崩溃恢复边界作为明确裁定/待验保留。该结果是回归与全套检查验证，不冒充第二次独立审查。
