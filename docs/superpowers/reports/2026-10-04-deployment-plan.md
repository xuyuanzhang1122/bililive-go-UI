# 远端更新与统一验收

用户2026-10-04明确授权SSH访问及尝试替换192.168.1.189服务端。运行方式：Ubuntu x86_64 systemd bililive-go.service，用户xumy，工作目录/home/xumy/bililive-go，端口8686；旧版本v2.1.1/8a266a1。读取配置只提取必要非敏感字段，API Key保留远端且不打印。

预检：无活跃录制、无FFmpeg，现有825个旧管道为完成/失败状态；磁盘751GB可用。二进制、配置、start.sh、systemd单元及4个在线SQLite backup已保存/home/xumy/bililive-go/backups/codex-t7-t9-20261004。替换前再查活跃工作并在停服后冻结最终DB备份；保留旧配置（prepare_playback未启用），不擅自开启录后准备/删源。systemd显式BILILIVE_CONFIG_FILE指向原配置，防默认路径镜像副作用。审查修复后编译Linux release，上传为独立新文件，校验摘要并原子更名；启动失败或健康检查失败即回退原二进制/配置/DB。

指定录播已只读复制到本机隔离目录，大小5634539067、SHA256 8f0e44067ec903637f68547c0e61a82ec459d6827d8e396b6167cc040c44cf4c。真实原文件不转换、不删除；真机播放只用副本。远端真实历史只生成可审查dry-run预览，confirm需针对具体预览另获授权。

当前状态：备份/检查/副本已取得；替换尚未执行，待本次跨仓审查。
