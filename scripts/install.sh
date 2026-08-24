#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

# ============================================================
# install.sh — bililive-go 一键安装脚本（统一安装流程）
#
# 流程：检测系统 → 检测原数据 → 选择源（GitHub/自建源）→ 检测工具
#       → 选择安装方式（二进制/Docker）→ 确认目录端口 → 拉取启动 → doctor 检测
#
# 交互模式：
#   curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash
#   curl -fsSL https://<你的源站>/install.sh | bash     # 默认指向该源站
#
# 非交互模式（CI / 重装 / 全默认）：
#   curl -fsSL <url> | bash -s -- --yes
#
# 全部参数：
#   --dir PATH            安装目录（默认 ~/bililive-go）
#   --videos-dir PATH     视频目录（默认 <安装目录>/Videos；可指向原版本数据）
#   --port N              主机端口（默认 8080）
#   --source URL|github   安装源：github 或自建源地址（默认见脚本头部注入）
#   --version TAG         指定 GitHub Release tag
#   --image TAG           Docker 镜像 tag（默认 latest，仅 --docker）
#   --update              就地更新现有安装，不重新运行安装向导
#   --enable-api-key      自动启用 API Key 并随机生成
#   --api-key STR         指定 API Key（仅在 --enable-api-key 时生效）
#   --yes / -y            非交互，全部走默认值或命令行参数
#   --binary / --docker   安装方式（不指定时交互选择）
#   --help                显示帮助
# ============================================================

REPO="xuyuanzhang1122/bililive-go-UI"
DOCKER_IMAGE="xuniubi/bililive-go"
RAW_BASE="https://raw.githubusercontent.com/${REPO}/main"
# 由源站 /install.sh 动态注入为该源站地址；直接从 GitHub raw 获取时为空
DEFAULT_MIRROR=""

INSTALL_DIR=""
VIDEOS_DIR=""
HOST_PORT=""
TAG=""
SOURCE=""
ENABLE_API_KEY=""
API_KEY=""
ASSUME_YES="false"
MODE=""
UPDATE_MODE="false"
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
    sed -n '6,32p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
}

# ---- 交互输入 ----
# 当 stdin 是管道（curl | bash）时，从 /dev/tty 读取，让 read 在交互式 shell 里仍然有效
read_default() {
    # $1 prompt, $2 default
    local prompt="$1" default="$2" answer=""
    if [[ "$ASSUME_YES" == "true" ]]; then
        printf "%s [%s]: %s（自动）\n" "$prompt" "$default" "$default" >&2
        echo "$default"; return
    fi
    if [[ -t 0 ]]; then
        read -rp "$prompt [$default]: " answer || true
    elif [[ -e /dev/tty ]]; then
        read -rp "$prompt [$default]: " answer < /dev/tty || true
    else
        printf "%s [%s]: %s（无终端，自动）\n" "$prompt" "$default" "$default" >&2
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
        printf "%s [%s]: %s（自动）\n" "$prompt" "$hint" "$default" >&2
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
        printf "%s%s%s" "$(date +%s%N)" "$(hostname)" "$RANDOM$RANDOM" | sha256sum 2>/dev/null \
            | awk '{print $1}'
    fi
}

# ---- JSON 取值（优先 python3，缺省时用 grep 兜底只支持简单字段） ----
json_query() {
    # $1 json文件, $2 python表达式（以 data 为根）
    local file="$1" expr="$2"
    if command -v python3 &>/dev/null; then
        python3 - "$file" <<PYEOF 2>/dev/null || true
import json, sys
try:
    data = json.load(open(sys.argv[1]))
    result = ${expr}
    print(result if result is not None else "")
except Exception:
    pass
PYEOF
    fi
}

normalize_path() {
    local path="$1" base_dir="${2:-$PWD}"
    path="${path/#\~/$HOME}"
    if [[ "$path" != /* ]]; then
        path="${base_dir}/${path}"
    fi
    if command -v realpath >/dev/null 2>&1; then
        realpath -m "$path" 2>/dev/null || printf '%s\n' "$path"
    else
        printf '%s\n' "$path"
    fi
}

read_top_level_yaml_value() {
    local config_file="$1" key="$2"
    awk -v key="$key" '
        $0 ~ ("^" key ":[[:space:]]*") {
            sub("^" key ":[[:space:]]*", "")
            sub(/[[:space:]]+#.*/, "")
            gsub(/^[[:space:]"\047]+|[[:space:]"\047]+$/, "")
            print
            exit
        }
    ' "$config_file"
}

read_rpc_bind() {
    local config_file="$1"
    awk '
        /^rpc:[[:space:]]*$/ { in_rpc=1; next }
        /^[^[:space:]]/ { in_rpc=0 }
        in_rpc && /^[[:space:]]+bind:[[:space:]]*/ {
            sub(/^[[:space:]]+bind:[[:space:]]*/, "")
            sub(/[[:space:]]+#.*/, "")
            gsub(/^[[:space:]"\047]+|[[:space:]"\047]+$/, "")
            print
            exit
        }
    ' "$config_file"
}

resolve_binary_download() {
    : "${TAG:=latest}"
    ASSET="bililive-${OS}-${GOARCH}.tar.gz"
    DOWNLOAD_URL=""
    if [[ -n "$CATALOG_FILE" && "$TAG" == "latest" ]]; then
        local rel_url rel_ver
        rel_url=$(json_query "$CATALOG_FILE" "next((x['url'] for x in data.get('releases', []) if x.get('name')=='${ASSET}'), '')")
        rel_ver=$(json_query "$CATALOG_FILE" "next((x['version'] for x in data.get('releases', []) if x.get('name')=='${ASSET}'), '')")
        if [[ -n "$rel_url" ]]; then
            TAG="${rel_ver:-mirror}"
            case "$rel_url" in
                http*) DOWNLOAD_URL="$rel_url" ;;
                *)     DOWNLOAD_URL="${MIRROR_URL}${rel_url}" ;;
            esac
            log "自建源版本: $TAG"
        fi
    fi
    if [[ -z "$DOWNLOAD_URL" ]]; then
        if [[ "$TAG" == "latest" ]]; then
            log "查询 GitHub 最新版本…"
            local resolved
            resolved=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
                | grep -o '"tag_name": *"[^"]*"' | head -1 \
                | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
            [[ -n "$resolved" ]] || { err "无法查询最新版本，请用 --version 指定"; exit 1; }
            TAG="$resolved"
            log "最新版本: $TAG"
        fi
        DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
    fi
}

download_release_binary() {
    resolve_binary_download
    log "下载: $DOWNLOAD_URL"
    DOWNLOAD_WORK_DIR=$(mktemp -d)
    if ! curl -fsSL "$DOWNLOAD_URL" -o "${DOWNLOAD_WORK_DIR}/${ASSET}"; then
        err "下载失败"
        exit 1
    fi
    tar xzf "${DOWNLOAD_WORK_DIR}/${ASSET}" -C "$DOWNLOAD_WORK_DIR"
    DOWNLOADED_BIN=$(find "$DOWNLOAD_WORK_DIR" -type f -name "bililive-${OS}-${GOARCH}" -print -quit)
    if [[ -z "$DOWNLOADED_BIN" ]]; then
        DOWNLOADED_BIN=$(find "$DOWNLOAD_WORK_DIR" -type f -name 'bililive-*' ! -name '*.tar.gz' -print -quit)
    fi
    [[ -n "$DOWNLOADED_BIN" ]] || { err "压缩包内未找到二进制"; exit 1; }
}

cleanup_hls_cache_for_update() {
    local data_dir="$1" cache_dir cache_size cache_dirs process_running="false"
    [[ -n "$data_dir" ]] || { warn "未定位到数据目录，跳过 HLS 存量缓存修复"; return 0; }
    cache_dir="${data_dir%/}/hls-cache"
    if [[ ! -e "$cache_dir" ]]; then
        ok "无需修复：HLS 缓存目录不存在（${cache_dir}）"
        return 0
    fi
    if [[ "$(basename -- "$cache_dir")" != "hls-cache" ]]; then
        err "安全校验失败：待删除路径末级目录不是 hls-cache：$cache_dir"
        return 1
    fi
    if [[ ! -d "$cache_dir" && ! -L "$cache_dir" ]]; then
        err "安全校验失败：目标不是目录：$cache_dir"
        return 1
    fi

    cache_size=$(du -sh "$cache_dir" 2>/dev/null | awk '{print $1}' || true)
    cache_size=${cache_size:-未知}
    cache_dirs=$(find "$cache_dir" -mindepth 1 -maxdepth 1 -type d -print 2>/dev/null | awk 'END {print NR+0}' || true)
    log "HLS 缓存目录: $cache_dir"
    log "占用空间: ${cache_size}；缓存子目录: $cache_dirs"

    if command -v pgrep >/dev/null 2>&1 && pgrep -x bililive-go >/dev/null 2>&1; then
        process_running="true"
    fi
    if command -v docker >/dev/null 2>&1 \
        && docker ps --format '{{.Names}}' 2>/dev/null | grep -Fxq "$CONTAINER_NAME"; then
        process_running="true"
    fi
    if [[ "$process_running" == "true" ]]; then
        warn "检测到 bililive-go 正在运行；删除缓存本身安全，但正在进行的 HLS 转封装可能中断"
    fi

    rm -rf -- "$cache_dir"
    ok "HLS 缓存已删除，释放空间约 $cache_size"
}

wait_for_service() {
    local port="$1" url
    url="http://127.0.0.1:${port}/api/auth-status"
    SERVICE_READY=""
    for _ in $(seq 1 30); do
        if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
            SERVICE_READY="true"
            break
        fi
        sleep 1
    done
    if [[ "$SERVICE_READY" == "true" ]]; then
        ok "服务响应: $url"
    else
        warn "服务未在 30s 内就绪: $url"
    fi
}

run_binary_update() {
    TARGET_BIN="$INSTALL_DIR/bililive-go"
    CONFIG_FILE="$INSTALL_DIR/config.yml"
    if [[ ! -f "$TARGET_BIN" || ! -f "$CONFIG_FILE" ]]; then
        err "未在 $INSTALL_DIR 找到 bililive-go 二进制和 config.yml，请先完成安装或用 --dir 指定现有目录"
        exit 1
    fi

    local configured_data rpc_bind timestamp backup_bin service_found="false"
    configured_data=$(read_top_level_yaml_value "$CONFIG_FILE" "app_data_path")
    if [[ -n "$configured_data" ]]; then
        APP_DATA_DIR=$(normalize_path "$configured_data" "$INSTALL_DIR")
    else
        APP_DATA_DIR="$INSTALL_DIR/Data"
        warn "config.yml 未配置 app_data_path，按安装器默认值使用 $APP_DATA_DIR"
    fi
    rpc_bind=$(read_rpc_bind "$CONFIG_FILE")
    HOST_PORT="${rpc_bind##*:}"
    if ! [[ "$HOST_PORT" =~ ^[0-9]+$ ]] || (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
        err "无法从 config.yml 的 rpc.bind 解析有效端口：${rpc_bind:-未配置}"
        exit 1
    fi
    ok "现有配置: ${CONFIG_FILE}（保持原样）"
    ok "数据目录: ${APP_DATA_DIR}；Web UI 端口: $HOST_PORT"

    download_release_binary
    timestamp=$(date +%Y%m%d%H%M%S)
    backup_bin="${TARGET_BIN}.bak.${timestamp}"
    cp -p "$TARGET_BIN" "$backup_bin"
    if ! install -m755 "$DOWNLOADED_BIN" "$TARGET_BIN"; then
        cp -p "$backup_bin" "$TARGET_BIN"
        err "安装新二进制失败，已恢复旧版本"
        exit 1
    fi
    rm -rf -- "$DOWNLOAD_WORK_DIR"
    ok "旧二进制已备份: $backup_bin"
    ok "新版本已安装: ${TARGET_BIN}（${TAG}）"

    cleanup_hls_cache_for_update "$APP_DATA_DIR"

    if command -v systemctl >/dev/null 2>&1 && systemctl cat bililive-go.service >/dev/null 2>&1; then
        service_found="true"
        log "重启 systemd 服务 bililive-go …"
        if [[ "$(id -u)" -eq 0 ]]; then
            systemctl restart bililive-go.service
        elif command -v sudo >/dev/null 2>&1; then
            sudo systemctl restart bililive-go.service
        else
            warn "缺少重启 systemd 服务所需权限，请手动执行 systemctl restart bililive-go"
            service_found="false"
        fi
    fi
    if [[ "$service_found" != "true" ]]; then
        warn "未自动重启服务；请手动重启 bililive-go 使新版本生效"
    fi

    wait_for_service "$HOST_PORT"
    if ! "$TARGET_BIN" --doctor -c "$CONFIG_FILE"; then
        warn "doctor 检查存在未通过项（见上方输出）"
    fi

    echo
    ok "bililive-go 二进制更新完成: $TAG"
    echo "  配置文件: ${CONFIG_FILE}（沿用现有配置）"
    echo "  旧版备份: $backup_bin"
    echo "  Web UI  : http://127.0.0.1:${HOST_PORT}"
}

run_docker_update() {
    command -v docker >/dev/null 2>&1 || { err "未检测到 Docker"; exit 1; }
    docker info >/dev/null 2>&1 || { err "Docker 守护进程未运行或当前用户无权限访问"; exit 1; }
    if ! docker ps -a --format '{{.Names}}' | grep -Fxq "$CONTAINER_NAME"; then
        err "未找到容器 ${CONTAINER_NAME}，请先安装或用 --binary 更新二进制版本"
        exit 1
    fi

    local old_image new_image data_host_dir="" config_host_file="" host_port=""
    local mount_type mount_name mount_source mount_destination mount_rw mount_value
    local container_port host_ip mapped_port publish_spec
    local -a publish_args=() volume_args=()
    old_image=$(docker inspect --format '{{.Config.Image}}' "$CONTAINER_NAME")

    while IFS='|' read -r container_port host_ip mapped_port; do
        [[ -n "$container_port" && -n "$mapped_port" ]] || continue
        if [[ -z "$host_ip" || "$host_ip" == "0.0.0.0" ]]; then
            publish_spec="${mapped_port}:${container_port}"
        elif [[ "$host_ip" == *:* ]]; then
            publish_spec="[${host_ip}]:${mapped_port}:${container_port}"
        else
            publish_spec="${host_ip}:${mapped_port}:${container_port}"
        fi
        publish_args+=(--publish "$publish_spec")
        if [[ "$container_port" == "8080/tcp" ]]; then
            host_port="$mapped_port"
        fi
    done < <(docker inspect --format '{{range $port, $bindings := .HostConfig.PortBindings}}{{range $bindings}}{{printf "%s|%s|%s\n" $port .HostIp .HostPort}}{{end}}{{end}}' "$CONTAINER_NAME")

    while IFS='|' read -r mount_type mount_name mount_source mount_destination mount_rw; do
        [[ -n "$mount_type" && -n "$mount_destination" ]] || continue
        case "$mount_type" in
            bind) mount_value="${mount_source}:${mount_destination}" ;;
            volume) mount_value="${mount_name}:${mount_destination}" ;;
            tmpfs)
                volume_args+=(--tmpfs "$mount_destination")
                continue ;;
            *)
                warn "暂不支持自动还原的挂载类型: $mount_type ($mount_destination)"
                continue ;;
        esac
        if [[ "$mount_rw" != "true" ]]; then
            mount_value="${mount_value}:ro"
        fi
        volume_args+=(--volume "$mount_value")
        if [[ "$mount_destination" == "/var/lib/bililive" ]]; then
            data_host_dir="$mount_source"
        elif [[ "$mount_destination" == "/etc/bililive-go/config.yml" ]]; then
            config_host_file="$mount_source"
        fi
    done < <(docker inspect --format '{{range .Mounts}}{{printf "%s|%s|%s|%s|%t\n" .Type .Name .Source .Destination .RW}}{{end}}' "$CONTAINER_NAME")

    : "${TAG:=latest}"
    new_image="${DOCKER_IMAGE}:${TAG}"
    log "拉取镜像 $new_image …"
    docker pull "$new_image"
    log "按原端口与挂载参数重建容器 …"
    docker rm -f "$CONTAINER_NAME" >/dev/null
    cleanup_hls_cache_for_update "$data_host_dir"
    if ! docker run -d \
        --name "$CONTAINER_NAME" \
        --restart unless-stopped \
        "${publish_args[@]}" \
        "${volume_args[@]}" \
        "$new_image" >/dev/null; then
        err "新容器启动失败，尝试用原镜像回滚"
        docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
        docker run -d \
            --name "$CONTAINER_NAME" \
            --restart unless-stopped \
            "${publish_args[@]}" \
            "${volume_args[@]}" \
            "$old_image" >/dev/null
        exit 1
    fi

    HOST_PORT="${host_port:-8080}"
    wait_for_service "$HOST_PORT"
    if docker ps --format '{{.Names}}' | grep -Fxq "$CONTAINER_NAME"; then
        ok "容器运行正常"
    else
        err "容器未保持运行，请检查 docker logs $CONTAINER_NAME"
        exit 1
    fi

    echo
    ok "bililive-go Docker 更新完成: $new_image"
    [[ -n "$config_host_file" ]] && echo "  配置文件: ${config_host_file}（沿用现有配置）"
    [[ -n "$data_host_dir" ]] && echo "  数据目录: $data_host_dir"
    echo "  Web UI  : http://127.0.0.1:${HOST_PORT}"
}

run_update() {
    echo
    log "开始就地更新（$MODE 模式）"
    case "$MODE" in
        binary) run_binary_update ;;
        docker) run_docker_update ;;
        *) err "无法识别更新模式: $MODE"; exit 1 ;;
    esac
}

# ---- 解析参数 ----
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir)            INSTALL_DIR="$2"; shift 2 ;;
        --videos-dir)     VIDEOS_DIR="$2"; shift 2 ;;
        --port)           HOST_PORT="$2"; shift 2 ;;
        --source)         SOURCE="$2"; shift 2 ;;
        --image)          TAG="$2"; shift 2 ;;
        --version)        TAG="$2"; shift 2 ;;
        --update)         UPDATE_MODE="true"; shift ;;
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
# 第 1 步：检测系统
# ============================================================
log "【1/7】检测系统环境"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
    x86_64)  GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    armv7l|armv6l) GOARCH="arm" ;;
    *) err "不支持的 CPU: $(uname -m)"; exit 1 ;;
esac
case "$OS" in
    linux|darwin) ok "系统: $OS / $GOARCH" ;;
    mingw*|msys*|cygwin*)
        err "Windows 请使用 PowerShell 安装脚本："
        echo "  irm https://<你的源站>/install.ps1 | iex"
        exit 1 ;;
    *) err "不支持的 OS: $OS"; exit 1 ;;
esac
command -v curl &>/dev/null || { err "缺少 curl"; exit 1; }
command -v tar  &>/dev/null || { err "缺少 tar"; exit 1; }

if [[ "$UPDATE_MODE" == "true" ]]; then
    if [[ -z "$INSTALL_DIR" ]]; then
        INSTALL_DIR="$HOME/bililive-go"
    fi
    INSTALL_DIR=$(normalize_path "$INSTALL_DIR" "$PWD")

    if [[ -z "$MODE" ]]; then
        binary_found="false"
        docker_found="false"
        if [[ -f "$INSTALL_DIR/bililive-go" && -f "$INSTALL_DIR/config.yml" ]]; then
            binary_found="true"
        fi
        if command -v docker >/dev/null 2>&1 \
            && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -Fxq "$CONTAINER_NAME"; then
            docker_found="true"
        fi
        if [[ "$binary_found" == "true" && "$docker_found" == "true" ]]; then
            err "同时检测到二进制安装与 Docker 容器，请用 --binary 或 --docker 指定更新对象"
            exit 1
        elif [[ "$binary_found" == "true" ]]; then
            MODE="binary"
        elif [[ "$docker_found" == "true" ]]; then
            MODE="docker"
        else
            err "未检测到可更新的现有安装；请确认 ${INSTALL_DIR}，或用 --dir/--binary/--docker 明确指定"
            exit 1
        fi
    fi

    if [[ -z "$SOURCE" ]]; then
        if [[ -n "$DEFAULT_MIRROR" ]]; then SOURCE="$DEFAULT_MIRROR"; else SOURCE="github"; fi
    fi
    ok "更新目标: $MODE"
fi

# ============================================================
# 第 2 步：选择安装源（GitHub / 自建源）
# ============================================================
echo
log "【2/7】选择安装源"
MIRROR_URL=""
if [[ -z "$SOURCE" ]]; then
    if [[ -n "$DEFAULT_MIRROR" ]]; then
        echo "  [1] 自建源 ${DEFAULT_MIRROR}（推荐，含工具分发）"
        echo "  [2] GitHub 官方"
        choice=$(read_default "选择源 (1=自建源 2=GitHub)" "1")
        if [[ "$choice" == "2" ]]; then SOURCE="github"; else SOURCE="$DEFAULT_MIRROR"; fi
    else
        echo "  [1] GitHub 官方"
        echo "  [2] 自建源（输入地址）"
        choice=$(read_default "选择源 (1=GitHub 2=自建源)" "1")
        if [[ "$choice" == "2" ]]; then
            SOURCE=$(read_default "自建源地址（如 https://update.example.com）" "")
            [[ -n "$SOURCE" ]] || { err "未输入源地址"; exit 1; }
        else
            SOURCE="github"
        fi
    fi
fi
if [[ "$SOURCE" != "github" ]]; then
    MIRROR_URL="${SOURCE%/}"
    log "校验自建源: $MIRROR_URL"
    if curl -fsS --max-time 5 "$MIRROR_URL/health" >/dev/null 2>&1; then
        ok "自建源可用"
    else
        warn "自建源无响应，回退到 GitHub"
        MIRROR_URL=""
        SOURCE="github"
    fi
fi
[[ "$SOURCE" == "github" ]] && ok "使用 GitHub 官方源"

# 拉取自建源 catalog（用于二进制与工具下载）
CATALOG_FILE=""
if [[ -n "$MIRROR_URL" ]]; then
    CATALOG_FILE=$(mktemp)
    if ! curl -fsSL --max-time 10 "$MIRROR_URL/api/v1/catalog" -o "$CATALOG_FILE" 2>/dev/null; then
        warn "无法获取自建源 catalog，二进制将回退 GitHub 下载"
        rm -f "$CATALOG_FILE"; CATALOG_FILE=""
    fi
fi

if [[ "$UPDATE_MODE" == "true" ]]; then
    run_update
    [[ -n "$CATALOG_FILE" ]] && rm -f "$CATALOG_FILE"
    exit 0
fi

# ============================================================
# 第 3 步：选择安装方式（二进制 / Docker）
# ============================================================
echo
log "【3/7】选择安装方式"
if [[ -z "$MODE" ]]; then
    echo "  [1] 二进制 Release（默认，无需 Docker）"
    echo "  [2] Docker 容器"
    choice=$(read_default "选择安装方式 (1=二进制 2=Docker)" "1")
    if [[ "$choice" == "2" ]]; then MODE="docker"; else MODE="binary"; fi
fi
ok "安装方式: $MODE"

if [[ "$MODE" == "docker" ]]; then
    if ! command -v docker &>/dev/null; then
        err "未检测到 Docker。请先安装: https://docs.docker.com/engine/install/"
        exit 1
    fi
    if ! docker info >/dev/null 2>&1; then
        err "Docker 守护进程未运行或当前用户无权限访问 docker.sock。"
        echo "  - 启动 Docker：sudo systemctl start docker"
        echo "  - 加入 docker 组：sudo usermod -aG docker \$USER && newgrp docker"
        exit 1
    fi
fi

# ============================================================
# 第 4 步：确认目录与端口（含原版本数据检测）
# ============================================================
echo
log "【4/7】确认安装目录、视频目录与端口"

DEFAULT_DIR="${HOME}/bililive-go"
if [[ -z "$INSTALL_DIR" ]]; then
    INSTALL_DIR=$(read_default "安装目录（程序/配置/数据放这里）" "$DEFAULT_DIR")
fi
INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
INSTALL_DIR="$(realpath -m "$INSTALL_DIR" 2>/dev/null || echo "$INSTALL_DIR")"

# ---- 原版本视频数据检测 ----
if [[ -z "$VIDEOS_DIR" ]]; then
    found_old=""
    for candidate in "$INSTALL_DIR/Videos" "$HOME/bililive-go/Videos" "$HOME/bililive/Videos" "/srv/bililive"; do
        if [[ -d "$candidate" ]] && [[ -n "$(find "$candidate" -maxdepth 3 \( -name '*.flv' -o -name '*.ts' -o -name '*.mp4' \) -print -quit 2>/dev/null)" ]]; then
            found_old="$candidate"
            break
        fi
    done
    if [[ -n "$found_old" ]]; then
        warn "检测到原版本视频数据: $found_old"
        reuse=$(ask_yes_no "复用该视频目录？（新录制和旧视频都放这里）" "y")
        if [[ "$reuse" == "y" ]]; then
            VIDEOS_DIR="$found_old"
        fi
    fi
    if [[ -z "$VIDEOS_DIR" ]]; then
        VIDEOS_DIR=$(read_default "视频目录（可指向原版本数据位置）" "$INSTALL_DIR/Videos")
    fi
fi
VIDEOS_DIR="${VIDEOS_DIR/#\~/$HOME}"
VIDEOS_DIR="$(realpath -m "$VIDEOS_DIR" 2>/dev/null || echo "$VIDEOS_DIR")"

if [[ -z "$HOST_PORT" ]]; then
    HOST_PORT=$(read_default "Web UI 端口" "8080")
fi
if ! [[ "$HOST_PORT" =~ ^[0-9]+$ ]] || (( HOST_PORT < 1 || HOST_PORT > 65535 )); then
    err "端口非法: $HOST_PORT"; exit 1
fi

# 端口占用检测
port_in_use=""
if command -v ss &>/dev/null; then
    ss -tln 2>/dev/null | awk '{print $4}' | grep -E "[:.]${HOST_PORT}\$" >/dev/null && port_in_use="1"
elif command -v lsof &>/dev/null; then
    lsof -iTCP:"$HOST_PORT" -sTCP:LISTEN >/dev/null 2>&1 && port_in_use="1"
elif command -v netstat &>/dev/null; then
    netstat -tln 2>/dev/null | awk '{print $4}' | grep -E "[:.]${HOST_PORT}\$" >/dev/null && port_in_use="1"
fi
if [[ -n "$port_in_use" ]]; then
    warn "端口 ${HOST_PORT} 已被占用"
    cont=$(ask_yes_no "继续使用此端口？" "n")
    [[ "$cont" == "y" ]] || { err "已取消"; exit 1; }
fi

if [[ -z "$ENABLE_API_KEY" ]]; then
    ans=$(ask_yes_no "启用 API Key 鉴权？（公网部署建议开启）" "n")
    if [[ "$ans" == "y" ]]; then ENABLE_API_KEY="true"; else ENABLE_API_KEY="false"; fi
fi
if [[ "$ENABLE_API_KEY" == "true" && -z "$API_KEY" ]]; then
    API_KEY=$(generate_api_key)
    [[ -n "$API_KEY" ]] || { err "无法生成 API Key（缺少 openssl/xxd/hexdump）"; exit 1; }
fi

mkdir -p "$INSTALL_DIR" "$VIDEOS_DIR" "$INSTALL_DIR/Data"
ok "安装目录: $INSTALL_DIR"
ok "视频目录: $VIDEOS_DIR"
ok "端口: $HOST_PORT"

# ============================================================
# 第 5 步：检测/拉取依赖工具（ffmpeg、无头浏览器）
# ============================================================
echo
log "【5/7】检测依赖工具"
TOOLS_DIR="$INSTALL_DIR/tools"
FFMPEG_PATH=""
HEADLESS_PATH=""

detect_tool() {
    # $1 命令名 → echo 路径
    command -v "$1" 2>/dev/null || true
}

download_mirror_tool() {
    # $1 工具名（catalog 中的 name）→ echo 下载好的可执行路径
    local name="$1" url
    [[ -n "$CATALOG_FILE" ]] || return 0
    url=$(json_query "$CATALOG_FILE" "next((t['url'] for t in data.get('tools', []) if t.get('name')=='${name}' and t.get('os')=='${OS}' and t.get('arch')=='${GOARCH}' and t.get('url')), '')")
    [[ -n "$url" ]] || return 0
    mkdir -p "$TOOLS_DIR"
    local fname="${url##*/}"
    local dest="$TOOLS_DIR/$fname"
    log "从自建源拉取 ${name}: ${MIRROR_URL}${url}" >&2
    if ! curl -fsSL "${MIRROR_URL}${url}" -o "$dest"; then
        warn "拉取 ${name} 失败" >&2
        return 0
    fi
    case "$fname" in
        *.tar.gz|*.tgz)
            tar xzf "$dest" -C "$TOOLS_DIR" 2>/dev/null
            rm -f "$dest"
            locate_extracted_tool "$name"
            ;;
        *.tar.xz)
            tar xf "$dest" -C "$TOOLS_DIR" 2>/dev/null
            rm -f "$dest"
            locate_extracted_tool "$name"
            ;;
        *.zip)
            if command -v unzip &>/dev/null; then
                unzip -oq "$dest" -d "$TOOLS_DIR"; rm -f "$dest"
                locate_extracted_tool "$name"
            else
                warn "缺少 unzip，无法解压 ${fname}" >&2
            fi
            ;;
        *)
            chmod +x "$dest"
            echo "$dest"
            ;;
    esac
}

# locate_extracted_tool 在解压目录里定位工具可执行文件
# 工具压缩包内部命名不固定（如 chrome-headless-shell-linux64/chrome-headless-shell），
# 因此按已知可执行名匹配，而非按工具名匹配。
locate_extracted_tool() {
    local name="$1" candidates extracted
    case "$name" in
        ffmpeg)           candidates="ffmpeg" ;;
        headless-browser) candidates="chrome-headless-shell chrome chromium headless_shell" ;;
        *)                candidates="$name" ;;
    esac
    for exe in $candidates; do
        extracted=$(find "$TOOLS_DIR" -maxdepth 3 -type f -name "$exe" ! -name "*.zip" ! -name "*.tar.*" 2>/dev/null | head -1)
        if [[ -n "$extracted" ]]; then
            chmod +x "$extracted"
            echo "$extracted"
            return 0
        fi
    done
}

# ffmpeg（必需）
FFMPEG_PATH=$(detect_tool ffmpeg)
if [[ -z "$FFMPEG_PATH" ]]; then
    for candidate in /opt/homebrew/bin/ffmpeg /usr/local/bin/ffmpeg /usr/bin/ffmpeg; do
        [[ -x "$candidate" ]] && FFMPEG_PATH="$candidate" && break
    done
fi
if [[ -z "$FFMPEG_PATH" && -n "$MIRROR_URL" ]]; then
    FFMPEG_PATH=$(download_mirror_tool ffmpeg)
fi
if [[ -n "$FFMPEG_PATH" ]]; then
    ok "ffmpeg: $FFMPEG_PATH"
else
    if [[ "$MODE" == "docker" ]]; then
        ok "ffmpeg: 镜像内置，跳过"
    else
        warn "未找到 ffmpeg：录制、缩略图、HLS 转封装将不可用"
        echo "  安装方法: apt install ffmpeg / brew install ffmpeg"
    fi
fi

# 无头浏览器（可选）
for candidate in chromium chromium-browser google-chrome chrome; do
    HEADLESS_PATH=$(detect_tool "$candidate")
    [[ -n "$HEADLESS_PATH" ]] && break
done
if [[ -z "$HEADLESS_PATH" && "$OS" == "darwin" ]]; then
    for candidate in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
        [[ -x "$candidate" ]] && HEADLESS_PATH="$candidate" && break
    done
fi
if [[ -z "$HEADLESS_PATH" && -n "$MIRROR_URL" ]]; then
    HEADLESS_PATH=$(download_mirror_tool headless-browser)
fi
if [[ -n "$HEADLESS_PATH" ]]; then
    ok "无头浏览器: $HEADLESS_PATH"
else
    warn "未找到无头浏览器（可选）：非标准短链 JS 跳转解析会降级"
fi

# ============================================================
# 第 6 步：下载并部署
# ============================================================
echo
log "【6/7】下载并部署"

write_config_common() {
    # 替换 out_put_path / app_data_path / 端口 / ffmpeg / 无头浏览器 / 更新源 / API Key
    local cfg_file="$1"
    [[ -f "$cfg_file" ]] || return 0
    local tmp_cfg
    tmp_cfg="$(mktemp)"
    awk -v out="$VIDEOS_DIR" -v data="$INSTALL_DIR/Data" -v port="$HOST_PORT" -v ffm="$FFMPEG_PATH" -v hb="$HEADLESS_PATH" -v mirror="$MIRROR_URL" '
        /^out_put_path:/  { print "out_put_path: " out; next }
        /^app_data_path:/ { print "app_data_path: " data; next }
        /^ffmpeg_path:/   { if (ffm != "") { print "ffmpeg_path: \"" ffm "\"" } else { print } ; next }
        /^[[:space:]]*bind:/ { sub(/bind:.*/, "bind: :" port); print; next }
        /^headless_browser:[[:space:]]*$/ { in_hb=1; in_up=0; print; next }
        /^update:[[:space:]]*$/ { in_up=1; in_hb=0; print; next }
        /^[A-Za-z]/ && !/^(headless_browser|update):/ { in_hb=0; in_up=0 }
        in_hb==1 && /^[[:space:]]*path:/ {
            if (hb != "") { sub(/path:.*/, "path: \"" hb "\"") }
            print; next
        }
        in_up==1 && /^[[:space:]]*source_url:/ {
            # 安装用什么源，应用内更新就用什么源（源站不可用时程序会回退本仓库 GitHub）
            if (mirror != "") { sub(/source_url:.*/, "source_url: \"" mirror "\"") }
            print; next
        }
        { print }
    ' "$cfg_file" > "$tmp_cfg"
    mv "$tmp_cfg" "$cfg_file"

    if [[ "$ENABLE_API_KEY" == "true" ]]; then
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
        ' "$cfg_file" > "$tmp_cfg"
        mv "$tmp_cfg" "$cfg_file"
    fi
}

fetch_config_template() {
    # $1 模板名, $2 目标路径
    local tpl="$1" dest="$2"
    if [[ -f "$dest" ]]; then
        warn "配置文件已存在: $dest"
        overwrite=$(ask_yes_no "用最新模板覆盖？（旧文件会备份为 .bak）" "n")
        if [[ "$overwrite" == "y" ]]; then
            cp -f "$dest" "${dest}.bak.$(date +%Y%m%d%H%M%S)"
        else
            log "保留现有配置文件"
            return 0
        fi
    fi
    log "下载配置模板 → $dest"
    curl -fsSL "${RAW_BASE}/${tpl}" -o "$dest"
}

SYSTEMD_INSTALLED=""
if [[ "$MODE" == "binary" ]]; then
    download_release_binary

    TARGET_BIN="$INSTALL_DIR/bililive-go"
    install -m755 "$DOWNLOADED_BIN" "$TARGET_BIN"
    rm -rf -- "$DOWNLOAD_WORK_DIR"
    ok "已安装到 $TARGET_BIN"

    CONFIG_FILE="$INSTALL_DIR/config.yml"
    fetch_config_template "config.yml" "$CONFIG_FILE"
    write_config_common "$CONFIG_FILE"

    # ---- 启动脚本 ----
    START_SCRIPT="$INSTALL_DIR/start.sh"
    cat > "$START_SCRIPT" <<STARTEOF
#!/usr/bin/env sh
cd "$INSTALL_DIR"
exec "$TARGET_BIN" -c "$CONFIG_FILE"
STARTEOF
    chmod +x "$START_SCRIPT"

    # ---- systemd 服务（仅 Linux 且有 systemctl） ----
    if [[ "$OS" == "linux" ]] && command -v systemctl &>/dev/null; then
        SERVICE_FILE="/etc/systemd/system/bililive-go.service"
        log "创建 systemd 服务: $SERVICE_FILE"
        service_unit() {
            cat <<SVCEOF
[Unit]
Description=bililive-go 直播录制服务
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
${1:+User=$1}
WorkingDirectory=$INSTALL_DIR
ExecStart=$TARGET_BIN -c $CONFIG_FILE
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF
        }
        if [[ "$(id -u)" -eq 0 ]]; then
            service_unit "" > "$SERVICE_FILE"
            systemctl daemon-reload
            systemctl enable bililive-go.service >/dev/null 2>&1
            systemctl restart bililive-go.service
            SYSTEMD_INSTALLED="true"
            ok "systemd 服务已创建并启动"
        elif command -v sudo &>/dev/null; then
            service_unit "$(whoami)" | sudo tee "$SERVICE_FILE" >/dev/null
            sudo systemctl daemon-reload
            sudo systemctl enable bililive-go.service >/dev/null 2>&1
            sudo systemctl restart bililive-go.service
            SYSTEMD_INSTALLED="true"
            ok "systemd 服务已创建并启动"
        else
            warn "需要 root 权限创建 systemd 服务，跳过"
        fi
    fi

    if [[ -z "$SYSTEMD_INSTALLED" ]]; then
        log "启动 bililive-go …"
        nohup "$START_SCRIPT" > "$INSTALL_DIR/bililive-go.log" 2>&1 &
        ok "已后台启动 (PID: $!)"
    fi
else
    # ---- Docker 模式 ----
    : "${TAG:=latest}"
    existing=$(docker ps -a --format '{{.Names}}' | grep -Fx "$CONTAINER_NAME" || true)
    if [[ -n "$existing" ]]; then
        warn "已存在容器 '$CONTAINER_NAME'"
        rm_old=$(ask_yes_no "删除旧容器并重建？（数据保留在 ${INSTALL_DIR}）" "y")
        [[ "$rm_old" == "y" ]] || { err "已取消，请手动处理后重跑"; exit 1; }
        log "删除旧容器 $CONTAINER_NAME …"
        docker rm -f "$CONTAINER_NAME" >/dev/null
    fi

    CONFIG_FILE="$INSTALL_DIR/config.docker.yml"
    fetch_config_template "config.docker.yml" "$CONFIG_FILE"
    # Docker 内路径固定，宿主机只改 API Key
    if [[ "$ENABLE_API_KEY" == "true" && -f "$CONFIG_FILE" ]]; then
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

    log "拉取镜像 ${DOCKER_IMAGE}:${TAG} …"
    docker pull "${DOCKER_IMAGE}:${TAG}"

    log "启动容器 …"
    docker run -d \
        --name "$CONTAINER_NAME" \
        --restart unless-stopped \
        -p "${HOST_PORT}:8080" \
        -v "${VIDEOS_DIR}:/srv/bililive" \
        -v "${INSTALL_DIR}/Data:/var/lib/bililive" \
        -v "${CONFIG_FILE}:/etc/bililive-go/config.yml" \
        "${DOCKER_IMAGE}:${TAG}" >/dev/null
fi

# ============================================================
# 第 7 步：doctor 检测
# ============================================================
echo
log "【7/7】doctor 检测"

# 等待服务就绪
url="http://127.0.0.1:${HOST_PORT}/api/auth-status"
ready=""
for _ in $(seq 1 30); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
        ready="true"; break
    fi
    sleep 1
done

doctor_check() {
    # $1 名称, $2 ok(true/false), $3 提示
    if [[ "$2" == "true" ]]; then
        ok "$1"
    else
        warn "$1 — $3"
    fi
}

doctor_check "服务响应 (${url})" "${ready:-false}" "未在 30s 内就绪，请检查日志"
if [[ "$MODE" == "binary" ]]; then
    # 程序自带 doctor 做完整检查（配置/ffmpeg/无头浏览器/目录/端口/磁盘）
    if "$INSTALL_DIR/bililive-go" --doctor -c "$CONFIG_FILE" 2>/dev/null; then
        :
    else
        warn "doctor 检查存在未通过项（见上方输出）"
    fi
else
    doctor_check "容器运行" "$(docker ps --format '{{.Names}}' | grep -Fxq "$CONTAINER_NAME" && echo true || echo false)" "容器未运行"
    doctor_check "视频目录可写" "$([[ -w "$VIDEOS_DIR" ]] && echo true || echo false)" "无写权限"
    doctor_check "无头浏览器（可选）" "$([[ -n "$HEADLESS_PATH" ]] && echo true || echo false)" "短链 JS 跳转解析降级"
fi

# 上报 doctor 结果到自建源（尽力而为，不阻塞安装）
if [[ -n "$MIRROR_URL" ]]; then
    curl -fsS --max-time 5 -X POST "$MIRROR_URL/api/v1/doctor" \
        -H "Content-Type: application/json" \
        -d "{\"os\":\"$OS\",\"arch\":\"$GOARCH\",\"install_mode\":\"$MODE\",\"port\":$HOST_PORT,\"paths\":{\"output_path\":\"$VIDEOS_DIR\"},\"tools\":[{\"id\":\"ffmpeg\",\"path\":\"$FFMPEG_PATH\",\"ok\":$([[ -n "$FFMPEG_PATH" ]] && echo true || echo false)},{\"id\":\"headless-browser\",\"path\":\"$HEADLESS_PATH\",\"ok\":$([[ -n "$HEADLESS_PATH" ]] && echo true || echo false)}]}" \
        >/dev/null 2>&1 || true
fi

[[ -n "$CATALOG_FILE" ]] && rm -f "$CATALOG_FILE"

# ============================================================
# 完成
# ============================================================
echo
if [[ "$ready" == "true" ]]; then
    ok "bililive-go 已启动并就绪"
else
    warn "服务未就绪，请检查日志"
    if [[ "$MODE" == "docker" ]]; then
        echo "  查看日志: docker logs -f $CONTAINER_NAME"
    elif [[ -n "$SYSTEMD_INSTALLED" ]]; then
        echo "  查看日志: journalctl -u bililive-go -f"
    else
        echo "  查看日志: tail -f $INSTALL_DIR/bililive-go.log"
    fi
fi

cat <<EOF

${C_GREEN}=== 安装完成 ===${C_RESET}

  Web UI       : http://<服务器 IP>:${HOST_PORT}
  本机访问     : http://127.0.0.1:${HOST_PORT}
  配置文件     : ${CONFIG_FILE}
  数据目录     : ${INSTALL_DIR}/Data
  视频目录     : ${VIDEOS_DIR}
EOF
[[ "$MODE" == "binary" ]] && echo "  程序文件     : ${INSTALL_DIR}/bililive-go"

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

echo
if [[ "$MODE" == "docker" ]]; then
    cat <<EOF
  常用命令     :
    docker logs -f $CONTAINER_NAME       # 看日志
    docker restart $CONTAINER_NAME       # 重启
    docker rm -f $CONTAINER_NAME         # 移除（数据保留在 ${INSTALL_DIR}）

EOF
elif [[ -n "$SYSTEMD_INSTALLED" ]]; then
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
    $INSTALL_DIR/start.sh                # 启动
    kill \$(pgrep bililive-go)            # 停止
    nohup $INSTALL_DIR/start.sh &        # 后台运行

EOF
fi
