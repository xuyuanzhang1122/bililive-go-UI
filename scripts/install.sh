#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

# ============================================================
# install.sh — bililive-go 一键安装脚本
#
# 默认模式：下载 GitHub Release 预编译二进制，无需 Docker。
# 如需 Docker 部署，请加 --docker 参数。
#
# 交互模式：
#   curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash
#
# 非交互模式（CI / 重装 / 全默认）：
#   curl -fsSL <url> | bash -s -- --yes
#
# 全部参数：
#   --dir PATH            安装目录（默认 ~/bililive-go）
#   --port N              主机端口（默认 8080）
#   --version TAG         指定 GitHub Release tag
#   --image TAG           Docker 镜像 tag（默认 latest，仅 --docker）
#   --enable-api-key      自动启用 API Key 并随机生成
#   --api-key STR         指定 API Key（仅在 --enable-api-key 时生效）
#   --yes / -y            非交互，全部走默认值或命令行参数
#   --docker              使用 Docker 安装并启动容器
#   --help                显示帮助
# ============================================================

REPO="xuyuanzhang1122/bililive-go-UI"
DOCKER_IMAGE="xuniubi/bililive-go"
RAW_BASE="https://raw.githubusercontent.com/${REPO}/main"

INSTALL_DIR=""
HOST_PORT=""
TAG=""
ENABLE_API_KEY=""
API_KEY=""
ASSUME_YES="false"
MODE="binary"
CONTAINER_NAME="bililive-go"

# ---- 颜色 ----
if [[ -t 1 ]]; then
    C_GREEN=$'\033[1;32m'; C_YELLOW=$'\033[1;33m'; C_RED=$'\033[1;31m'
    C_BLUE=$'\033[1;34m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
    C_GREEN=""; C_YELLOW=""; C_RED=""; C_BLUE=""; C_DIM=""; C_RESET=""
fi
log()  { printf "%s→%s %s\n" "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf "%s✓%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf "%s⚠%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()  { printf "%s✗%s %s\n" "$C_RED" "$C_RESET" "$*" >&2; }

usage() {
    sed -n '6,24p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
}

# ---- 交互输入 ----
# 当 stdin 是管道（curl | bash）时，从 /dev/tty 读取，让 read 在交互式 shell 里仍然有效
read_default() {
    # $1 prompt, $2 default
    local prompt="$1" default="$2" answer="" tty_in=""
    if [[ "$ASSUME_YES" == "true" ]]; then
        printf "%s [%s]: %s（自动）\n" "$prompt" "$default" "$default"
        echo "$default"; return
    fi
    if [[ -t 0 ]]; then
        read -rp "$prompt [$default]: " answer || true
    elif [[ -e /dev/tty ]]; then
        read -rp "$prompt [$default]: " answer < /dev/tty || true
    else
        printf "%s [%s]: %s（无终端，自动）\n" "$prompt" "$default" "$default"
        echo "$default"; return
    fi
    echo "${answer:-$default}"
}

ask_yes_no() {
    # $1 prompt, $2 default(y|n)  → echoes "y" or "n"
    local prompt="$1" default="${2:-n}" answer=""
    local hint
    if [[ "$default" == "y" ]]; then hint="Y/n"; else hint="y/N"; fi
    if [[ "$ASSUME_YES" == "true" ]]; then
        printf "%s [%s]: %s（自动）\n" "$prompt" "$hint" "$default"
        echo "$default"; return
    fi
    if [[ -t 0 ]]; then
        read -rp "$prompt [$hint]: " answer || true
    elif [[ -e /dev/tty ]]; then
        read -rp "$prompt [$hint]: " answer < /dev/tty || true
    else
        echo "$default"; return
    fi
    case "${answer,,}" in
        y|yes) echo "y" ;;
        n|no)  echo "n" ;;
        "")    echo "$default" ;;
        *)     echo "$default" ;;
    esac
}

generate_api_key() {
    if command -v openssl &>/dev/null; then
        openssl rand -hex 32
    elif [[ -r /dev/urandom ]] && command -v xxd &>/dev/null; then
        head -c 32 /dev/urandom | xxd -p -c 64 | tr -d '\n'
    elif [[ -r /dev/urandom ]] && command -v hexdump &>/dev/null; then
        head -c 32 /dev/urandom | hexdump -ve '/1 "%02x"'
    else
        # 兜底，不强：时间戳 + hostname + random
        printf "%s%s%s" "$(date +%s%N)" "$(hostname)" "$RANDOM$RANDOM" | sha256sum 2>/dev/null \
            | awk '{print $1}'
    fi
}


# ---- 解析参数 ----
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir)            INSTALL_DIR="$2"; shift 2 ;;
        --port)           HOST_PORT="$2"; shift 2 ;;
        --image)          TAG="$2"; shift 2 ;;
        --version)        TAG="$2"; shift 2 ;;
        --enable-api-key) ENABLE_API_KEY="true"; shift ;;
        --api-key)        API_KEY="$2"; shift 2 ;;
        --yes|-y)         ASSUME_YES="true"; shift ;;
        --binary)         MODE="binary"; shift ;;
        --docker)         MODE="docker"; shift ;;
        --help|-h)        usage ;;
        *)  err "未知参数: $1"; usage ;;
    esac
done

# ============================================================
# 二进制模式（默认）
# ============================================================
if [[ "$MODE" == "binary" ]]; then
    log "bililive-go 一键安装（二进制模式，无需 Docker）"
    : "${TAG:=latest}"
    if [[ "$TAG" == "latest" ]]; then
        log "查询最新版本…"
        RESOLVED=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
            | grep -o '"tag_name": *"[^"]*"' | head -1 \
            | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
        [[ -n "$RESOLVED" ]] || { err "无法查询最新版本，请用 --version 指定"; exit 1; }
        TAG="$RESOLVED"; log "最新版本: $TAG"
    fi
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$(uname -m)" in
        x86_64)  GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        armv7l|armv6l) GOARCH="arm" ;;
        *) err "不支持的 CPU: $(uname -m)"; exit 1 ;;
    esac
    case "$OS" in linux|darwin) ;; *) err "不支持的 OS: $OS，请用 --docker 模式"; exit 1 ;; esac

    # ---- 交互输入 ----
    echo
    log "请确认以下安装参数（回车使用默认值）："
    echo

    DEFAULT_DIR="${HOME}/bililive-go"
    if [[ -z "$INSTALL_DIR" ]]; then
        INSTALL_DIR=$(read_default "安装目录（数据/视频/配置都放这里）" "$DEFAULT_DIR")
    fi
    INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
    INSTALL_DIR="$(realpath -m "$INSTALL_DIR")"

    if [[ -z "$HOST_PORT" ]]; then
        HOST_PORT=$(read_default "Web UI 端口" "8080")
    fi
    if ! [[ "$HOST_PORT" =~ ^[0-9]+$ ]] || (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
        err "端口非法: $HOST_PORT"; exit 1
    fi

    if [[ -z "$ENABLE_API_KEY" ]]; then
        ans=$(ask_yes_no "启用 API Key 鉴权？（公网部署建议开启；可稍后在 Web UI 一键启用）" "n")
        if [[ "$ans" == "y" ]]; then ENABLE_API_KEY="true"; else ENABLE_API_KEY="false"; fi
    fi

    if [[ "$ENABLE_API_KEY" == "true" && -z "$API_KEY" ]]; then
        API_KEY=$(generate_api_key)
        if [[ -z "$API_KEY" ]]; then
            err "无法生成 API Key（缺少 openssl/xxd/hexdump）"; exit 1
        fi
    fi

    # ---- 下载二进制 ----
    ASSET="bililive-${OS}-${GOARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
    log "下载: $URL"
    TMPDIR=$(mktemp -d); trap 'rm -rf "$TMPDIR"' EXIT
    curl -fsSL "$URL" -o "${TMPDIR}/${ASSET}" || { err "下载失败"; exit 1; }
    tar xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"
    BIN=$(find "$TMPDIR" -name 'bililive-*' -type f | head -1)
    [[ -n "$BIN" ]] || { err "压缩包内未找到二进制"; exit 1; }

    # ---- 创建目录 ----
    log "创建安装目录: $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR/Videos" "$INSTALL_DIR/Data"

    # ---- 安装二进制 ----
    TARGET_BIN="$INSTALL_DIR/bililive-go"
    install -m755 "$BIN" "$TARGET_BIN"
    ok "已安装到 $TARGET_BIN"

    # ---- 下载 / 生成配置 ----
    CONFIG_FILE="$INSTALL_DIR/config.yml"
    if [[ -f "$CONFIG_FILE" ]]; then
        warn "配置文件已存在: $CONFIG_FILE"
        overwrite=$(ask_yes_no "用最新模板覆盖？（旧文件会备份为 .bak）" "n")
        if [[ "$overwrite" == "y" ]]; then
            cp -f "$CONFIG_FILE" "${CONFIG_FILE}.bak.$(date +%Y%m%d%H%M%S)"
            log "下载配置模板 → $CONFIG_FILE"
            curl -fsSL "${RAW_BASE}/config.yml" -o "$CONFIG_FILE"
        else
            log "保留现有配置文件"
        fi
    else
        log "下载配置模板 → $CONFIG_FILE"
        curl -fsSL "${RAW_BASE}/config.yml" -o "$CONFIG_FILE"
    fi

    # ---- 替换配置路径和端口 ----
    if [[ -f "$CONFIG_FILE" ]]; then
        tmp_cfg="$(mktemp)"
        awk -v out="$INSTALL_DIR/Videos" -v data="$INSTALL_DIR/Data" -v port="$HOST_PORT" '
            /^out_put_path:/ { print "out_put_path: " out; next }
            /^app_data_path:/ { print "app_data_path: " data; next }
            /^[[:space:]]*bind:/ { sub(/bind:.*/, "bind: :" port); print; next }
            { print }
        ' "$CONFIG_FILE" > "$tmp_cfg"
        mv "$tmp_cfg" "$CONFIG_FILE"
    fi

    # ---- 写入 API Key ----
    if [[ "$ENABLE_API_KEY" == "true" && -f "$CONFIG_FILE" ]]; then
        log "写入 API Key 到配置"
        tmp_cfg="$(mktemp)"
        awk -v key="$API_KEY" '
            BEGIN { in_security=0 }
            /^security:[[:space:]]*$/ { in_security=1; print; next }
            /^[A-Za-z]/ && !/^security:/ { in_security=0 }
            in_security==1 && /^[[:space:]]*enable_api_key:/ {
                sub(/enable_api_key:.*/, "enable_api_key: true"); print; next
            }
            in_security==1 && /^[[:space:]]*api_key:/ {
                sub(/api_key:.*/, "api_key: \"" key "\""); print; next
            }
            { print }
        ' "$CONFIG_FILE" > "$tmp_cfg"
        mv "$tmp_cfg" "$CONFIG_FILE"
    fi


    # ---- 生成启动脚本 ----
    START_SCRIPT="$INSTALL_DIR/start.sh"
    cat > "$START_SCRIPT" <<STARTEOF
#!/usr/bin/env sh
cd "$INSTALL_DIR"
exec "$TARGET_BIN" -c "$CONFIG_FILE"
STARTEOF
    chmod +x "$START_SCRIPT"

    # ---- systemd 服务（仅 Linux 且有 systemctl） ----
    SYSTEMD_INSTALLED=""
    if [[ "$OS" == "linux" ]] && command -v systemctl &>/dev/null; then
        SERVICE_FILE="/etc/systemd/system/bililive-go.service"
        log "创建 systemd 服务: $SERVICE_FILE"
        # 需要 root 权限写 /etc/systemd/system
        if [[ "$(id -u)" -eq 0 ]]; then
            cat > "$SERVICE_FILE" <<SVCEOF
[Unit]
Description=bililive-go 直播录制服务
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$TARGET_BIN -c $CONFIG_FILE
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF
            systemctl daemon-reload
            systemctl enable bililive-go.service >/dev/null 2>&1
            systemctl start bililive-go.service
            SYSTEMD_INSTALLED="true"
            ok "systemd 服务已创建并启动"
        elif command -v sudo &>/dev/null; then
            sudo tee "$SERVICE_FILE" >/dev/null <<SVCEOF
[Unit]
Description=bililive-go 直播录制服务
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$INSTALL_DIR
ExecStart=$TARGET_BIN -c $CONFIG_FILE
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF
            sudo systemctl daemon-reload
            sudo systemctl enable bililive-go.service >/dev/null 2>&1
            sudo systemctl start bililive-go.service
            SYSTEMD_INSTALLED="true"
            ok "systemd 服务已创建并启动"
        else
            warn "需要 root 权限创建 systemd 服务，跳过"
        fi
    fi

    # 非 systemd 环境：直接后台启动
    if [[ -z "$SYSTEMD_INSTALLED" ]]; then
        log "启动 bililive-go …"
        nohup "$START_SCRIPT" > "$INSTALL_DIR/bililive-go.log" 2>&1 &
        ok "已后台启动 (PID: $!)"
    fi

    # ---- 等待服务就绪 ----
    log "等待服务就绪 …"
    url="http://127.0.0.1:${HOST_PORT}/api/auth-status"
    ready=""
    for _ in $(seq 1 15); do
        if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
            ready="true"; break
        fi
        sleep 1
    done

    echo
    if [[ "$ready" == "true" ]]; then
        ok "bililive-go 已启动并就绪"
    else
        warn "服务未在 15s 内响应，请检查日志"
        if [[ -n "$SYSTEMD_INSTALLED" ]]; then
            echo "  查看日志: journalctl -u bililive-go -f"
        else
            echo "  查看日志: tail -f $INSTALL_DIR/bililive-go.log"
        fi
    fi

    cat <<EOF

${C_GREEN}=== 安装完成 ===${C_RESET}

  Web UI       : http://<服务器 IP>:${HOST_PORT}
  本机访问     : http://127.0.0.1:${HOST_PORT}
  程序文件     : ${TARGET_BIN}
  配置文件     : ${CONFIG_FILE}
  数据目录     : ${INSTALL_DIR}/Data
  视频目录     : ${INSTALL_DIR}/Videos

EOF

    if [[ "$ENABLE_API_KEY" == "true" ]]; then
        cat <<EOF
  ${C_YELLOW}API Key（请妥善保存）${C_RESET}: ${API_KEY}

  iOS App / 外部客户端请把上面这串粘贴进设置页。
  如需更换：编辑 ${CONFIG_FILE} 的 api_key 字段后重启即可。

EOF
    else
        cat <<EOF
  API Key      : 未启用（公网访问建议开启）
  ${C_DIM}稍后可在 Web UI "设置 → API Key" 页一键启用并生成。${C_RESET}

EOF
    fi

    if [[ -n "$SYSTEMD_INSTALLED" ]]; then
        cat <<EOF
  服务管理     :
    systemctl status bililive-go     # 状态
    systemctl restart bililive-go    # 重启
    systemctl stop bililive-go       # 停止
    journalctl -u bililive-go -f     # 日志

EOF
    else
        cat <<EOF
  常用命令     :
    ${START_SCRIPT}               # 启动
    kill \$(pgrep bililive-go)     # 停止
    nohup ${START_SCRIPT} &       # 后台运行

EOF
    fi
    exit 0
fi

# ============================================================
# Docker 模式
# ============================================================
log "bililive-go 引导式安装（Docker 模式）"

# ---- Docker 可用性检查 ----
if ! command -v docker &>/dev/null; then
    err "未检测到 Docker。请先安装："
    echo "  https://docs.docker.com/engine/install/"
    exit 1
fi
if ! docker info >/dev/null 2>&1; then
    err "Docker 守护进程未运行或当前用户无权限访问 docker.sock。"
    echo "  - 启动 Docker：sudo systemctl start docker"
    echo "  - 加入 docker 组：sudo usermod -aG docker \$USER && newgrp docker"
    exit 1
fi

# ---- 交互输入 ----
echo
log "请确认以下安装参数（回车使用默认值）："
echo

# 默认安装目录：~/bililive-go
DEFAULT_DIR="${HOME}/bililive-go"
if [[ -z "$INSTALL_DIR" ]]; then
    INSTALL_DIR=$(read_default "安装目录（数据/视频/配置都放这里）" "$DEFAULT_DIR")
fi
# 展开 ~
INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
INSTALL_DIR="$(realpath -m "$INSTALL_DIR")"

if [[ -z "$HOST_PORT" ]]; then
    HOST_PORT=$(read_default "Web UI 端口（主机侧）" "8080")
fi
if ! [[ "$HOST_PORT" =~ ^[0-9]+$ ]] || (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
    err "端口非法: $HOST_PORT"; exit 1
fi

if [[ -z "$TAG" ]]; then
    TAG=$(read_default "Docker 镜像 tag" "latest")
fi

if [[ -z "$ENABLE_API_KEY" ]]; then
    ans=$(ask_yes_no "启用 API Key 鉴权？（建议公网部署时开启；可稍后在 Web UI 一键启用）" "n")
    if [[ "$ans" == "y" ]]; then ENABLE_API_KEY="true"; else ENABLE_API_KEY="false"; fi
fi

if [[ "$ENABLE_API_KEY" == "true" && -z "$API_KEY" ]]; then
    API_KEY=$(generate_api_key)
    if [[ -z "$API_KEY" ]]; then
        err "无法生成 API Key（缺少 openssl/xxd/hexdump）"; exit 1
    fi
fi

# ---- 端口冲突检查 ----
port_in_use=""
if command -v ss &>/dev/null; then
    ss -tln 2>/dev/null | awk '{print $4}' | grep -E "[:.]${HOST_PORT}\$" >/dev/null && port_in_use="ss"
elif command -v netstat &>/dev/null; then
    netstat -tln 2>/dev/null | awk '{print $4}' | grep -E "[:.]${HOST_PORT}\$" >/dev/null && port_in_use="netstat"
fi
if [[ -n "$port_in_use" ]]; then
    warn "端口 ${HOST_PORT} 已被占用（${port_in_use} 检测）"
    cont=$(ask_yes_no "继续使用此端口？" "n")
    if [[ "$cont" != "y" ]]; then err "已取消"; exit 1; fi
fi

# ---- 容器冲突 ----
existing=$(docker ps -a --format '{{.Names}}' | grep -Fx "$CONTAINER_NAME" || true)
if [[ -n "$existing" ]]; then
    warn "已存在容器 '$CONTAINER_NAME'"
    rm_old=$(ask_yes_no "删除旧容器并重建？（数据保留在 ${INSTALL_DIR}）" "y")
    if [[ "$rm_old" != "y" ]]; then err "已取消，请手动处理后重跑"; exit 1; fi
    log "删除旧容器 $CONTAINER_NAME …"
    docker rm -f "$CONTAINER_NAME" >/dev/null
fi

# ---- 创建目录 ----
log "创建安装目录: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR/Videos" "$INSTALL_DIR/Data"

# ---- 下载 / 准备 config.docker.yml ----
CONFIG_FILE="$INSTALL_DIR/config.docker.yml"
if [[ -f "$CONFIG_FILE" ]]; then
    warn "配置文件已存在: $CONFIG_FILE"
    overwrite=$(ask_yes_no "用最新模板覆盖？（旧文件会备份为 .bak）" "n")
    if [[ "$overwrite" == "y" ]]; then
        cp -f "$CONFIG_FILE" "${CONFIG_FILE}.bak.$(date +%Y%m%d%H%M%S)"
        log "下载配置模板 → $CONFIG_FILE"
        curl -fsSL "${RAW_BASE}/config.docker.yml" -o "$CONFIG_FILE"
    else
        log "保留现有配置文件"
    fi
else
    log "下载配置模板 → $CONFIG_FILE"
    curl -fsSL "${RAW_BASE}/config.docker.yml" -o "$CONFIG_FILE"
fi

# ---- 写入 API Key（如果启用） ----
if [[ "$ENABLE_API_KEY" == "true" ]]; then
    log "写入 API Key 到配置"
    # 用 awk 替换两个键（兼容 macOS / Linux，不依赖 sed -i 的差异行为）
    tmp_cfg="$(mktemp)"
    awk -v key="$API_KEY" '
        BEGIN { in_security=0 }
        /^security:[[:space:]]*$/ { in_security=1; print; next }
        /^[A-Za-z]/ && !/^security:/ { in_security=0 }
        in_security==1 && /^[[:space:]]*enable_api_key:/ {
            sub(/enable_api_key:.*/, "enable_api_key: true"); print; next
        }
        in_security==1 && /^[[:space:]]*api_key:/ {
            sub(/api_key:.*/, "api_key: \"" key "\""); print; next
        }
        { print }
    ' "$CONFIG_FILE" > "$tmp_cfg"
    mv "$tmp_cfg" "$CONFIG_FILE"
fi

# ---- 拉取镜像 ----
log "拉取镜像 ${DOCKER_IMAGE}:${TAG} …"
docker pull "${DOCKER_IMAGE}:${TAG}"

# ---- 启动 ----
log "启动容器 …"
docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    -p "${HOST_PORT}:8080" \
    -v "${INSTALL_DIR}/Videos:/srv/bililive" \
    -v "${INSTALL_DIR}/Data:/var/lib/bililive" \
    -v "${CONFIG_FILE}:/etc/bililive-go/config.yml" \
    "${DOCKER_IMAGE}:${TAG}" >/dev/null

# ---- 等待健康 ----
log "等待服务就绪 …"
url="http://127.0.0.1:${HOST_PORT}/api/auth-status"
ready=""
for _ in $(seq 1 30); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
        ready="true"; break
    fi
    sleep 1
done

echo
if [[ "$ready" == "true" ]]; then
    ok "bililive-go 已启动"
else
    warn "服务未在 30s 内响应，请用 'docker logs $CONTAINER_NAME' 排查"
fi

cat <<EOF

${C_GREEN}=== 安装完成 ===${C_RESET}

  Web UI       : http://<服务器 IP>:${HOST_PORT}
  本机访问     : http://127.0.0.1:${HOST_PORT}
  数据目录     : ${INSTALL_DIR}/Data
  视频目录     : ${INSTALL_DIR}/Videos
  配置文件     : ${CONFIG_FILE}

EOF

if [[ "$ENABLE_API_KEY" == "true" ]]; then
    cat <<EOF
  ${C_YELLOW}API Key（请妥善保存）${C_RESET}: ${API_KEY}

  iOS App / 外部客户端请把上面这串粘贴进设置页。
  如需更换：编辑 ${CONFIG_FILE} 的 api_key 字段后 docker restart $CONTAINER_NAME 即可。

EOF
else
    cat <<EOF
  API Key      : 未启用（公网访问建议开启）
  ${C_DIM}稍后可在 Web UI "设置 → API Key" 页一键启用并生成。${C_RESET}

EOF
fi

cat <<EOF
  常用命令     :
    docker logs -f $CONTAINER_NAME       # 看日志
    docker restart $CONTAINER_NAME       # 重启
    docker rm -f $CONTAINER_NAME         # 移除（数据保留在 ${INSTALL_DIR}）

EOF
