# 远端更新与统一验收

## 最新交付结果

服务器实际运行 `v2.1.2-rc.3` / `b8acf09a9a42e608668cf3b6684483bb3afd9c21`，二进制SHA256 `ca2a79448535cdfa3bdef70f49576d0ee5ecdee0f92bdabc2d5a0e1c6263dd91`。部署前等待最近3条准备全部完成，再备份和替换，启动健康与16项v2目录通过，原录制/鉴权策略保持。回退备份 `/home/xumy/bililive-go/backups/codex-t7-t9-20261004-rc3`。

用户后来明确授权只触发最近2–3条，按mtime选最近3条，826–828均completed/100%。三个来源最终SHA256与处理前一致；三个ready会话、实际Range206逐字节读取、刷新保持资产及取消通过，资产合计2,106,725,199字节。其余13条未确认，也未删除旧文件。完整任务为816完成/12旧失败，只有3条治理执行关联。

实际Web首个ready资产暴露ArtPlayer拒绝undefined type，修复后真实依赖校验RED→GREEN，全套检查通过；RC.3部署后浏览器显示真实视频帧、readyState4、时钟推进，前进10秒至32.301秒与暂停正常。截图 `2026-10-04-web-rc3-playback.jpg`。前RC.2发布说明已标记Web已知问题、推荐RC.3。

服务端GitHub运行37202268294成功，12个平台包与校验和、Docker RC镜像发布完成；Linux包实际下载并核对GitHubdigest、包内配置、版本和源码commit。Release：`https://github.com/xuyuanzhang1122/bililive-go-UI/releases/tag/v2.1.2-rc.3`。

鸿蒙功能分支已推送（代码11a4468，后续计划文档另提交）。iOS功能分支及v2.0.3-rc.5已推，GitHub Xcode26.4.1运行37202297849成功，Actions artifact和Release IPA下载后摘要一致，ZIP/Mach-O/版本检查通过，bundle build5；未签名IPA需重签安装。Release：`https://github.com/xuyuanzhang1122/bililive-ios/releases/tag/v2.0.3-rc.5`。rc.3/rc.4编译失败标签保留，空Release转草稿；本机Xcode27通过未冒充CI通过。

T7指定5.63GB副本iPhone时间线完成；T6指定视频MatePad时间线仍缺连接设备，旧本机默认配置恢复来源仍待提供。历史审计预览清理为独立审查遗留Minor。下面是首轮更新历史记录，状态以本节和对应JSON证据为准。

## 首轮更新记录（历史）

用户2026-10-04明确授权SSH访问及尝试替换192.168.1.189服务端。运行方式：Ubuntu x86_64 systemd bililive-go.service，用户xumy，工作目录/home/xumy/bililive-go，端口8686；旧版本v2.1.1/8a266a1。读取配置只提取必要非敏感字段，API Key保留远端且不打印。

预检：无活跃录制、无FFmpeg，现有825个旧管道为完成/失败状态；磁盘751GB可用。二进制、配置、start.sh、systemd单元及4个在线SQLite backup已保存/home/xumy/bililive-go/backups/codex-t7-t9-20261004。替换前再查活跃工作并在停服后冻结最终DB备份；保留旧配置（prepare_playback未启用），不擅自开启录后准备/删源。systemd显式BILILIVE_CONFIG_FILE指向原配置，防默认路径镜像副作用。审查修复后编译Linux release，上传为独立新文件，校验摘要并原子更名；启动失败或健康检查失败即回退原二进制/配置/DB。

指定录播已只读复制到本机隔离目录，大小5634539067、SHA256 8f0e44067ec903637f68547c0e61a82ec459d6827d8e396b6167cc040c44cf4c。真实原文件不转换、不删除；真机播放只用副本。远端真实历史只生成可审查dry-run预览，confirm需针对具体预览另获授权。

最终状态：独立审查的5个Important已修复；服务端已更新到 `v2.1.2-rc.1-14-g3279645`，代码指纹 `327964582355049d5f3c5b646f878f43e8e17360`，Linux二进制SHA256 `da39d82a6c012afb1113d80bdbbc67950099fc81f8e65f7f0129d681e517917c`。健康检查、16录播v2清单、无Bearer返回401均通过；录制/认证策略保持原值。最后更新前的回退备份位于 `/home/xumy/bililive-go/backups/codex-t7-t9-20261004-final`；最初旧版及配置/最终DB快照仍保存在前述首个备份目录。

真实网页验收：3个房间、目标主播10个录播可读；首次认证自动初始化修复与缺失日期显示已验证。未准备的指定原FLV明确显示“没有匹配的已准备资产；需按服务端准备策略完成准备”，没有偷偷退回旧接口或触发真实转换。当前真实库全部16项均为FLV，生产原件播放/Range成功路径尚未执行；iPhone大录播播放成功路径来自SHA256一致的本机只读副本准备资产。

最新真实预览 `2efb5ce2-87c6-42a7-b5a6-f40369f6fb62`：16候选、0跳过，保守预计新增227,996,609,065字节（约228GB），保留原文件，不上传/不删源，task_ids为空、history_tasks关联为0。预览快照见相邻 `2026-10-04-t9-real-preview.json`，30分钟失效；用户授权时应重新验证或重新生成并核对候选身份及摘要。未确认任何远端真实历史转换，管道813完成/12失败保持不变。指定原文件最终大小和SHA256再次验证一致。

iPhone指定文件真实时间线PASS，正式入口包已重新覆盖安装并启动；隔离本机服务已正常停止。MatePad HDC当前仍为空，T6指定文件设备验收待连接；旧本机默认配置恢复来源仍未获得。发布事实与证据见 `2026-10-04-deployment-final-evidence.json`，网页截图及跨端报告与计划同仓提交，无push。
