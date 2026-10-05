# sun 正式大版本

2026-10-05 发布。特殊版本名称及标签为 **sun**，无需加 `v`；内部升级排序对应 `3.0.0`。

本次将录制、录后准备和 Web / iOS / 鸿蒙播放连成完整流程：录制结束自动入队，核验并准备可播放资产，页面等待准备完成后自动播放。转换仍需要时间；准备失败保留原文件。

## 主要变化

- 统一 v2 录播身份、分页目录、存储统计、逐项删除和跨端续播记录。
- 根据设备实际解码能力选择文件或 HLS，提供短期播放授权、自动刷新、退出取消和失效会话重建。
- 修复零时长 FLV 和可诊断尾部损坏：依据连续包时间线恢复、转码并严格核验轨道、时长、格式和来源身份，不把残缺产物当完整录像发布。
- 修复准备队列尚未开工时错误显示资产失败；排队、准备和暂停恢复有明确状态，完成后自动继续播放。
- Web 显示清晰提示，底层 FFmpeg 诊断保留在任务记录；系统状态增加内置版本迭代详情。
- 历史库提供 dry-run 预览、摘要确认、限速和暂停恢复，失败或低空间保源；不会自动全库转换。
- 同步 iOS 与鸿蒙的设置和关于页，保留旧版本记录。
- 一键安装明确指定 `sun`，配置模板和二进制来自同一标签；新安装默认录后准备，升级保留旧配置和数据。

## 安装与升级

Linux / macOS / WSL 新安装：

```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/sun/scripts/install.sh | bash -s -- --source github --version sun
```

已有安装先备份并等待当前录制结束，将目录替换成实际路径：

```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/sun/scripts/install.sh | bash -s -- --update --source github --version sun --dir ~/bililive-go
```

Docker 镜像：`xuniubi/bililive-go:sun`（Linux amd64），Compose 与模板同步到该标签。新装需挂载 Videos、Data 和配置文件。其他架构使用对应二进制包。

Windows 固定版本安装：

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/sun/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -Version sun -Source github
```

Windows 更新时先停止程序并备份，再用脚本指定原目录；PowerShell 脚本没有 Linux 的 `--update` 参数。

已有配置需开启 `on_record_finished.prepare_playback: true`，保持 `custom_commandline` 为空。录后准备保留源文件并替代旧转换路径。未准备的历史录像仍需预览确认后准备，不因升级自动转换。

## 移动端

- [iOS sun](https://github.com/xuyuanzhang1122/bililive-ios/releases/tag/sun)：应用显示 sun（3.0.0），系统包版本 3.0.0、构建号 6。公开产物 LiveOS.ipa 未签名，需个人重签；不是 App Store / TestFlight 包。
- [鸿蒙 sun](https://github.com/xuyuanzhang1122/Live-os-Harmony/releases/tag/sun)：显示 sun，包内 versionName 3.0.0、versionCode 3。公开 HAP 未签名，需自己的合法签名或用 DevEco Studio 构建安装；API 12 兼容、API 21 目标。

三端建议一起升级，旧服务端适配保留。更改版本名不代表所有设备和媒体编码矩阵已经验证。

## 已有验收与边界

服务器三个故障录像（含四小时录播）已完成保源恢复，实际浏览器首段、中段、尾段播放及跳转通过，当前全部 8 条资产 ready。自动流程经过真实 FFmpeg、Native 下载器和损坏尾部 FLV 的完整集成测试。生产下一次真实下播尚未发生，未把集成测试说成新的生产直播验收。

先前指定大文件的 iPhone 时间线已验证；鸿蒙有 MatePad 样片测试，但指定大文件及完整编码、HDR、多设备矩阵仍有限制。具体记录见 [录播修复报告](../superpowers/reports/2026-10-05-flv-playback-repair.md)。本次新包及发布验收另记于 sun 发布报告。
