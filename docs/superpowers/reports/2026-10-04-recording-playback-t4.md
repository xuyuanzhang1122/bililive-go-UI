# 录播播放 T4 实施记录（2026-10-04）

T4 核心实现、自动验证和独立审查修复已完成。T5 尚未开始；真机首帧、实际断电与生产容量验收仍待完成。仅修改 bililive-go-UI，沿用 feature/jit-hls-optimization；本轮基线为 `3cd117e846d89541dfd1b339769cc2a086997318`。

## 配置与录后行为

新增配置默认空，旧配置迁移保持关闭，仅显式列出的有限档位运行：

```yaml
on_record_finished:
  prepare_hls_profiles: [source, 720p, 1080p]
```

按录后入队时的配置快照先执行 T3 prepare_playback，再 prepare_hls；不兼容源先由 T3 准备兼容输入。source 复制视频/音频并保留 GOP；720p/1080p 最大分辨率分别为 1280×720/1920×1080，不放大小源，H.264 Main Level 4.1、yuv420p、30fps、AAC 48kHz 双声道。目标分片 6 秒；source 的实际分片长度受原 GOP 影响。禁止未知/重复档位，版本规则为 hls-<profile>-v1。启用准备时沿用保源保护，不并行执行旧删源转换与不受控自定义命令。

## 持久资产与恢复

资产位于 `AppData/media-assets/<recording_id>/<source_version>-hls-<profile>-v1/`，与 `hls-cache` 根目录分离，不受临时缓存 10GB/24h 回收策略影响。清单固定为 VOD 且必须有 ENDLIST；JIT 保持 EVENT 增长清单。

私有构建目录先落盘所有权标记，再写媒体；验证 codec/时长与全文件哈希后，先发布意图、再发布 index、最后发布完整 manifest 提交标记并同步目录。恢复只接受相同稳定来源与档位，拒绝覆盖已失效产物。版本目录中原画源路径只是历史提示，改名通过目录的 recording_id/source_version 找当前有效源。

T2 的 v2 验证表复用为清单/分片的集合证明，无新 SQLite schema 迁移。manifest 摘要绑定所有成员；统计将清单和 TS 记入 derived_size，保持主视频大小独立。资源只在发布、目录关联提交后可被选择。

重启清理仅触及已取得任务 claim 和预算的同版本有主未发布目录；当前意图/完成资产、未知目录与其他所有者保留。低空间暂停并保源；转换期间继续检查剩余空间。多档串行准备，与 T3/JIT 共用 mediawork 重任务并发预算，持久任务按来源/档位去重并持久化暂停、失败和就绪状态。

## HTTP 与旧入口

新增 GET/HEAD `/api/stream/hls-asset/{recording_id}/{asset_version}/{file}`，沿用正常 API 鉴权或短期签名。清单重写到受签名保护的固定版本分片；分片校验哈希后使用同一打开句柄支持 Range。请求只查所选目录资产，不扫描全录播库、不启动持久准备。

旧 resolver 可显式用 `hls_profile` 选择已准备档位；旧 HLS 清单入口优先选择持久资产。T3 可直放 faststart MP4 仍保留默认优先级。准备响应的 Retry-After 与 JSON 轮询间隔一致；队列满返回 429。JIT 已生成分片可读，未来分片持读租约最多等待 2 秒，可取消，失败明确返回并带重试信息；取消只结束请求。

## 自动验证

全部测试使用临时目录/数据库及生成样例，不修改或转换真实录播。配置、发布集合、目录证明、HTTP 与 resolver 均按失败→通过验证。真实 FFmpeg 的 source/720p/1080p 样例覆盖多分片、编码、时长、复用；恢复覆盖发布后任务未就绪、预算等待取消、低空间暂停/恢复、源变化、签名/Range/临时回收隔离、未来分片等待/租约。

独立审查两项 Important 均已 RED→GREEN 修复：源改名后资产失效；进程异常退出遗留未发布半成品。后者实际启动子进程并 os.Exit(72)，并验证未知/有主/意图/完成目录的选择性保护。完整反馈与裁定见 [审查记录](../evidence/2026-10-04-t4/review.md)。

| 最终命令 | 结果 | 证据 |
| --- | --- | --- |
| make build-web dev | 退出 0 | final-build-web-dev.log |
| PATH=/tmp/bilive-tools:$PATH make lint | 退出 0，0 issues | final-lint.log |
| make test | 退出 0 | final-test.log |
| go test -race -tags=dev ./src/pipeline/... ./src/pkg/hlsasset ./src/pkg/recordingcatalog ./src/servers | 退出 0 | final-race.log |
| make dev PLATFORM=windows ARCH=amd64 | 退出 0 | windows-build.log |
| make dev PLATFORM=linux ARCH=amd64 | 退出 0 | linux-build.log |

以上为本机 macOS 验证及交叉编译。校验清单、源码 SHA256 与阶段账本保存在 [T4 证据目录](../evidence/2026-10-04-t4/verification.json)。构建内嵌 GitHash 为构建时基线 HEAD，源码指纹覆盖实际受测改动；不能把二进制显示的旧 hash 当作未含 T4。

## 剩余验收与下一阶段

- 真机首帧、拖动与持续播放；Windows/Linux 实际运行及真正断电恢复；生产长视频、长 GOP、吞吐和哈希成本尚待验证。source 不保证严格 6 秒切片。
- 共享预算没有 FIFO/最大等待时长保证，列为审查延期 minor；并发限制与取消已有证据。
- 当前假定资产目录由服务管理；同 UID 恶意原位改写不在已验证防御范围。
- T3 HDR→SDR 正向验收仍需含 zscale 的 FFmpeg 环境，本轮不能据短样本扩大该结论。
- 下一阶段 T5 定义共享 OpenAPI、v2 录播/播放会话与短期 play_token，再接设备能力决策。自动质量选择尚未实现。

本轮按用户要求主动提交本地 commit，未 push、部署或转换真实历史库。阶段决策及风险成本详见 [账本](../evidence/2026-10-04-t4/progress.md)。

代码及验证证据提交：`fd83210e7e026e73b357f1162f6958781cca78a4`（feat: 实现 T4 持久 HLS 与资源调度）。随后文档提交仅补充本检查点引用；保留当前分支，不合并或推送。
