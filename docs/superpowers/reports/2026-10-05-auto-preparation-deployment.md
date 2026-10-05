# 2026-10-05 新录播自动准备与录制竞态热修复记录

## 请求与范围

用户授权立即修改生产环境，要求今后的录播在结束后由服务端自动准备兼容资产。未授权对历史录播库进行批量准备或删除原始文件；本次未执行此类操作，也未推送代码或发布新标签。

## 根因与改动

- 生产配置此前 `on_record_finished.prepare_playback: false`、`prepare_hls_profiles: []`，完成录制后只执行旧 FLV 修复；网页和 iOS 端无原生 FLV 能力，故返回 `asset_not_ready`。
- 录制器下播时调用 `Stop()`，FFmpeg 可能以 `signal: killed` 返回；旧逻辑直接跳过后处理。提交 `7a44b76` 在已有非空输出时继续入队（也识别录播姬分段）。
- 首次重启中，同一房间短时间发生 `Record End` 与新 `Live Start`，两个 FFmpeg 同时录制。提交 `3069571` 防止录制器 `Close()` 后安装新 parser、缩短 Start 的 pending 竞态，并使 FFmpeg parser 在 `Stop()` 后拒绝延迟启动。
- 远端配置 `on_record_finished.prepare_playback: true`；`prepare_hls_profiles: []` 保持原值。新录播完成后准备兼容 MP4，保留来源，不改变既有上传策略。历史库不会因开启此项自动批量准备。

## 部署与验证

- 服务器：`192.168.1.189`，systemd `bililive-go.service`；生产配置和二进制在 `/home/xumy/bililive-go`。首次替换前备份 `backups/claude-20261005-prepare/`，竞态修复替换前备份 `backups/claude-20261005-stop-race/`。
- 最终二进制由提交 `30695712ce80a1bd9dcb8f70b465d06a75175f41` 构建；SHA256 `6832597e4ca8e7cbdd3376192465e5c515ec2ce515052d6c8e45030021e07803`，版本显示 `v2.1.2-rc.3&#43;hotfix.3069571`（构建参数中的 `+` 被转义为 `&#43;`，未冒称官方标签）。服务 `active`，本地 HTTP 首页返回 200，配置仍为 `prepare_playback: true`。
- 首次开启后，直播于 2026-10-05 05:02 UTC 结束。自动准备管道任务 `831` 完成，对应资产 `ready`，MP4 42,879,345 字节，`ffprobe` 时长 770.6 秒。另一路重复录制的任务 `830` 因 FLV 媒体读取错误失败，未删除源文件；旧重复录制问题已由第二次部署的竞态修复处理，但尚无**第二次部署之后的新直播结束**可验证真实自动准备闭环。
- 本地完成 `make build-web dev`、`make lint`（0 issues）、`make test`、`go test -race ./src/recorders ./src/pkg/parser/ffmpeg -count=1`、`make build VERSION=v2.1.2-rc.3+hotfix.3069571 PLATFORM=linux ARCH=amd64`。

## 未完成事项

- 原先未准备的历史 FLV，以及停服前切分留下的约 5.06 GB TUN训来源，仍需经历史治理预览和逐项确认；未擅自提交批量任务。
- 自动准备仍可能因损坏源、媒体参数、磁盘或任务执行错误失败，无法承诺每条文件无条件成功；应以后续实际下播和会话 `ready` 验收，发现失败按具体原因排查。
- 尚未核验生产浏览器或手机端真实播放本次任务 `831` 的画面；这里只验证了服务、任务状态、文件和媒体探测。
