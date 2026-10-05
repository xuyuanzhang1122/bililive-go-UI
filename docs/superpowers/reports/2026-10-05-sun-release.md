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

等待正式标签推送和实际 CI 产物完成后，追加运行状态、下载版本与摘要。
