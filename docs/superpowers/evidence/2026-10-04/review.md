# T3 独立只读审查与修复

审查代理仅审查 T3 相关模块，未修改工作树；不把旧 T1/T2/HLS 全部改动算作本轮实现。审查通过临时 Go overlay 复现两项阻断。

1. P1：产物已发布、准备终态提交失败后，重启恢复中的 ffprobe 返回后用同大小同 mtime 的损坏文件替换目标，原实现把最后读取到的新版本写入 ready。
2. P2：保持 ready 目标媒体不变，将 manifest 改指另一同大小同 mtime 来源，后台目录已把目标归到另一 recording_id，原实现仍成功复用。

两项测试已纳入 `src/pipeline/stages/prepare_playback_regression_test.go`。`red-review.log` 先复现两项失败；修复后 `green-review.log` / `green-review-final.log` 通过。

进一步同一修复范围：失败后后台可能按相同 size/mtime 再次标可信。`red-review-catalog-proof.log` 复现；v1→v2 准备证明表保存录播/来源/规则/完整目标身份及持久失效标记，`green-review-catalog-proof.log` 通过。新 v2 迁移另经 ID 保留回归验证。最终全仓检查见 final-* 日志。

修复后的完成条件：本次验证的目标身份在探测、发布及提交后保持一致；manifest 来源匹配；目录资产属于当前录播和来源；持久规则证明与目标版本匹配；失败撤销可信标记，后台不能自动重新信任失效文件。

审查未评估 Windows 断电、手机首帧、生产部署、HDR 正向色调映射。它们需要独立环境/设备证据，实施记录继续列待验，不因代码审查宣称通过。没有报告需要延期的 minor。
