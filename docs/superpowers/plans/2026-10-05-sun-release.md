# sun 正式大版本发布实施计划

> 使用 superpowers:executing-plans 在当前会话逐项实施；用户已授权正式 Release，包含必要的推送、打包及发布。

**目标：** 将已验收的录播修复作为特殊版本 `sun` 正式发布，同步 Web、iOS、鸿蒙版本展示、内置迭代说明及一键安装文档。

**架构：** 服务端、Web 和三端 GitHub 标签为 `sun`；鸿蒙包内 versionName 为 `3.0.0`、应用展示 sun；版本比较将其映射到 `3.0.0`。iOS 的系统包版本必须为数字，设为 `3.0.0`、构建号 `6`，设置和关于页展示 `sun（3.0.0）`。三个仓库分别提交、验证并发布，不改写既有历史。

**技术栈：** Go / React / Bash / PowerShell、SwiftUI / Xcode、ArkTS / Hvigor。

**规格：** 本会话用户明确要求正式大版本 `sun`、同步移动端与内置迭代详情、完善一键安装命令。

## 全局约束

- 保留已有录播修复与原文件；不执行全库历史转换。
- 本地提交必须先验证；正式发布包含三仓库标签、源码和实际可下载产物。
- 不提交签名材料或管理凭据；iOS 发布未签名 IPA，鸿蒙发布未签名 HAP 与本地构建指引。
- 新安装模板启用录后播放准备并保留源文件；升级沿用用户配置。

## 审查重点

- `sun` 与旧 RC、未来数字版比较不能触发降级或反复更新。
- 安装命令锁定 `sun`，不因镜像滞后装回旧版。
- 三端显示、包内版本和 Release 标签一致；iOS 数字约束明确说明。
- 产物需要实际构建、下载及校验，空 Release 不算交付。
- 发布不改变生产录制、鉴权及观看历史配置。

## 任务 1：版本、更新和安装发布链

**文件：** `src/pkg/update/checker.go`、新增更新回归测试、`src/servers/handler.go`、`src/webapp/public/version.json`、`scripts/release.sh`、安装脚本、配置模板、README / CHANGELOG / `docs/releases/sun.md`、Release 工作流。

**接口：** `CompareVersions` / `Checker.CheckForUpdate` 保持现有签名；将 `sun` 归一化为 `3.0.0`。GitHub 下载标签始终保留 `sun`。

- [x] 先写并运行版本比较、真实 HTTP 更新选择、发布预演、指定版本安装回归，确认必要的失败。
- [x] 实现特殊版本识别、内置版本说明、录后准备模板与固定版本安装文档。
- [x] 运行 `make build-web dev VERSION=sun`、`make lint`、`make test` 及 Web / 安装契约检查，检查暂存后提交。

## 任务 2：iOS 与鸿蒙版本和包

**文件：** iOS 工程版本、Info.plist、SettingsView、RELEASE_NOTES / README；鸿蒙 AppScope/app.json5、版本展示常量、SettingsPage / AboutPage、包元数据、README 和发布报告。

**接口：** 播放会话及服务端契约不变，页面新增当前版本说明并保留旧迭代详情。

- [x] 同步显示 `sun`，iOS 数字版本为 `3.0.0` / `6`，鸿蒙 versionCode 从 `2` 增到 `3`。
- [x] 构建 iOS Release iphoneos 并核对 IPA 包内版本。
- [x] 运行鸿蒙源码测试、Hypium 和 assembleHap，核对 HAP 包内版本；签名仅由本地私有配置提供。
- [x] 分别检查暂存范围并提交两个仓库。

## 任务 3：正式发布与交付验收

**文件：** 三仓库发布记录与资产证据；GitHub 三个 `sun` 标签及正式 Release。

**接口：** 服务端保留现有资产命名，iOS `LiveOS.ipa`，鸿蒙 `LiveOS-sun-unsigned.hap`。

- [x] 核对远端分支仅作快进更新，推送已验证提交及标签，触发现有 CI。
- [x] 等待服务端跨平台包 / Docker 和 GitHub IPA 构建，核对状态并下载关键资产检查版本与摘要。
- [x] 发布鸿蒙 HAP、三端安装说明；检查 `latest`、固定版本一键安装地址及下载可用性。
- [x] 将真实发布 URL、构建结果、限制和摘要写入报告，提交证据。

## 实施裁定与进度

- 用户正式发布指令包含三端推送和公开 Release，按已授权范围推进，无需重复确认。
- 鸿蒙 SDK 实际 schema 拒绝纯字母 versionName，使用数字 3.0.0，应用显示和标签仍为 sun；不绕过工具校验。
- 特殊版本排序和真实 HTTP 更新检查已完成 RED→GREEN，避免镜像旧版降级及未来数字版本反复更新。
- 新安装模板开启录后准备；迁移保持旧配置 opt-in，升级不覆盖用户策略。
- 内置版本详情读取实际 version.json；Web 构建要求不从 public 跨 src 导入，改为运行时读取。
- 最终独立审查发现 Windows 保留旧配置仍重写目录/端口的 Important；真实 PowerShell 测试先复现配置字节改变，再验证保留原字节、9090 端口及 D:/Data / D:/Videos。问题已修复。

- 正式三端 Release、CI 和下载验收完成；生产运行实例有活动录制，保持此前已修复版本，版本切换未执行。独立审查无剩余 Critical / Important，Windows 发现已真实 RED→GREEN 修复。
