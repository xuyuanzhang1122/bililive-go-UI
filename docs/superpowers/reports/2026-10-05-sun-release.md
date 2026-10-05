# sun 正式发布验收记录

## 授权与范围

用户明确授权正式大版本 sun，包含三仓库版本、内置迭代说明、一键安装与正式 Release。保留此前全部录播修复，不执行历史库批量转换，不改变生产录制和鉴权配置。

## 实施结果

- 服务端与 Web 显示 sun；内部升级排序归一化为 3.0.0，下载标签仍为 sun。真实 HTTP 更新和版本比较回归先失败再通过，避免反复更新或降级。
- Web 内置元数据、系统状态详情和真实 UI 版本读取同步更新。安装模板默认录后准备、保留源文件；旧安装升级沿用配置。
- 发布流程支持 sun，上传跨平台包、安装脚本、配置与 SHA256SUMS。固定版本安装同时固定模板标签。
- iOS 3.0.0 / build 6，应用显示 sun（3.0.0）；鸿蒙 3.0.0 / versionCode 3，应用显示 sun。鸿蒙 SDK 实际拒绝纯字母 versionName，因此不绕过校验。公开包均未签名。
- 独立代码审查发现 Windows 保留配置仍覆盖路径/端口的 Important，已真实 PowerShell RED→GREEN；保持旧配置字节、数据目录、端口和 Key 不变。

## 本地验证

- make build-web dev VERSION=sun：通过；make lint：0 issues；make test：通过。
- 全部 Web 契约、发布预演和模板检查：通过；更新比较 race：通过。
- Windows PowerShell 语法与旧配置保护回归：通过。
- Xcode 27 Release iphoneos：BUILD SUCCEEDED；本地 IPA ZIP、3.0.0 / 6 / sun 元数据通过。
- 鸿蒙源码回归：87 pass、0 fail；Hypium 107 pass、Failure 0、Error 0（读取实际 test_result.txt）；Release assembleHap：BUILD SUCCESSFUL，HAP ZIP、3.0.0 / 3、arm64-v8a 与 x86_64 的 libavcap.so 通过。

## 决策与边界

- 用户已授权正式发布，直接完成推送/发布；不重复请求许可。
- iOS 的数字包版本依据 Apple CFBundleShortVersionString 要求：https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleshortversionstring 。鸿蒙数字约束依据本地 SDK 实际 schema 构建结果。
- 新装默认准备；旧配置迁移维持 opt-in，升级不替用户改变录后处理策略。
- 前一轮生产三段录像与全部 8 条 ready、浏览器实播证据见相邻 FLV 修复报告；不把本次版本同步说成新的生产直播下播验收。
- 未签名公开移动包需要用户自行签名；鸿蒙指定大文件及多设备/编码/HDR 矩阵仍未完整覆盖。

## 公开发布

三个仓库主分支已快进推送，sun 标签分别指向：服务端 ff71bc923b6692a97f28b753149381e50aee0532、iOS aca28eafda65b701dfff892dc9b6d57c42554c4d、鸿蒙 c304933。未移动或覆盖既有标签。

- 服务端正式 Release：https://github.com/xuyuanzhang1122/bililive-go-UI/releases/tag/sun ，18 个资产（12 个平台包、5 个安装/配置文件、SHA256SUMS）。GitHub 运行 37300318446 全部 12 个 job success（10 分片、发布、Docker）。sun 为非草稿、非预发布，并已设为 latest。
- 下载 Linux amd64、macOS arm64、Windows amd64 及安装/配置文件共 9 个资产；全部匹配 GitHub digest 与 SHA256SUMS，摘要清单覆盖所有发布资产。macOS / Linux 二进制实际运行 --version 返回 sun；Windows ZIP / PE 和模板录后准备配置通过。
- 公开固定版本安装脚本实际下载、解包 sun，与手动下载二进制逐字节一致；安装脚本和模板与标签源码一致。Raw 地址可达，帮助和 Bash / PowerShell 语法通过。
- 实际公开 Linux 包在远端隔离目录和回环 18687 端口运行，API 版本 sun / GitHash ff71bc9，Web 版本 sun，浏览器系统状态显示 6 条内置迭代详情，page_errors 为空；截图及运行证据已保存。测试实例已关闭。
- Docker xuniubi/bililive-go:sun 发布成功，注册表实际核对 linux/amd64 与入口。sun 和 latest 镜像 digest 一致，详见 sun-docker-evidence.json。
- iOS GitHub 运行 37300317425（Xcode26.4.1）success，Actions IPA 与公开 Release IPA 逐字节相同，ZIP / Mach-O / 3.0.0 / build6 / sun 和 GitHub digest 通过。大小 8,699,764 字节，SHA256 e15416859de9c9eb734e2779f8f4aa16f99bab97f9caf54943fbfe2ab2aaf663；已附 SHA256SUMS。正式地址：https://github.com/xuyuanzhang1122/bililive-ios/releases/tag/sun 。
- 鸿蒙公开 HAP 与本地 Release 构建逐字节一致，ZIP / 3.0.0 / code3 / release 模式及 GitHub digest 通过。大小 4,324,855 字节，SHA256 35284254a270a7ff84223527e1cf437ff9ebf2469fbaf2f52fe786b77ed3c220。正式地址：https://github.com/xuyuanzhang1122/Live-os-Harmony/releases/tag/sun 。

## 生产运行状态

验收结束时服务器仍有 1 个直播录制任务，运行此前已修复的 v2.1.2-rc.3-playback-repair-queue-20261005 / a51c5d8；没有为版本名称同步中断录制，因此生产运行实例尚未切换 sun。on_record_finished.prepare_playback 已开启，录播修复代码与本次正式版一致；本次新增主要是版本、安装保护和展示。正式包已实际在该服务器隔离运行通过。

生产 config.yml SHA256 a909eab989a7e2a76455d156d432e5538f0c16e85a3007ee8620458a5e723a0d 与事前一致。未修改生产配置、管理 Key 或观看进度；未执行历史批量转换。
