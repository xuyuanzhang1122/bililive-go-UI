#!/bin/sh

HOME=/srv/bililive

CONFIG_PATH=/etc/bililive-go/config.yml
CONFIG_DEFAULT=/opt/bililive/config.docker.yml

# Docker bind-mount 陷阱：宿主机不存在 config.docker.yml 文件时，
# docker 会自动创建同名空目录而非报错，导致容器读到目录而非 YAML 文件。
if [ -d "$CONFIG_PATH" ]; then
    echo "[entrypoint] $CONFIG_PATH 是目录（bind-mount 陷阱），恢复默认配置"
    rm -rf "$CONFIG_PATH"
    cp "$CONFIG_DEFAULT" "$CONFIG_PATH"
elif [ ! -f "$CONFIG_PATH" ]; then
    echo "[entrypoint] $CONFIG_PATH 不存在，复制默认配置"
    cp "$CONFIG_DEFAULT" "$CONFIG_PATH"
fi

mkdir -p /var/lib/bililive/db /var/lib/bililive/thumbnails

chown -R ${PUID}:${PGID} ${HOME}
chown -R ${PUID}:${PGID} /etc/bililive-go
chown -R ${PUID}:${PGID} /opt/bililive
chown -R ${PUID}:${PGID} /var/lib/bililive

umask ${UMASK}

# Detect runtime architecture; on armv7/armhf skip gosu and run directly
ARCH_UNAME="$(uname -m 2>/dev/null || echo unknown)"
ARCH_DPKG="$(dpkg --print-architecture 2>/dev/null || echo unknown)"

case "${ARCH_UNAME}:${ARCH_DPKG}" in
	armv7l:armhf|armv6l:armhf|armv7l:unknown|armv6l:unknown)
		echo "[entrypoint] armv7/armhf detected (${ARCH_UNAME}/${ARCH_DPKG}), starting without gosu"
		exec /usr/bin/bililive-go -c "$CONFIG_PATH"
		;;
	*)
		exec gosu ${PUID}:${PGID} /usr/bin/bililive-go -c "$CONFIG_PATH"
		;;
esac
