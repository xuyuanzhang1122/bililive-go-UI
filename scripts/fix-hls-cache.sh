#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

DATA_DIR=""
DRY_RUN="false"
ASSUME_YES="false"

if [[ -t 1 ]]; then
    C_GREEN=$'\033[1;32m'; C_YELLOW=$'\033[1;33m'; C_RED=$'\033[1;31m'
    C_BLUE=$'\033[1;34m'; C_RESET=$'\033[0m'
else
    C_GREEN=""; C_YELLOW=""; C_RED=""; C_BLUE=""; C_RESET=""
fi
log()  { printf "%s→%s %s\n" "$C_BLUE" "$C_RESET" "$*"; }
ok()   { printf "%s✓%s %s\n" "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf "%s⚠%s %s\n" "$C_YELLOW" "$C_RESET" "$*"; }
err()  { printf "%s✗%s %s\n" "$C_RED" "$C_RESET" "$*" >&2; }

usage() {
    cat <<'EOF'
清理 bililive-go v2.0.2 及之前版本遗留的 HLS 播放缓存。

用法：
  bash scripts/fix-hls-cache.sh [--data-dir PATH] [--dry-run] [--yes]

参数：
  --data-dir PATH  bililive-go 数据目录；默认从同目录 config.yml 探测，
                   探测不到时使用 ~/bililive-go/Data
  --dry-run        只统计，不删除
  --yes, -y        跳过删除确认
  --help, -h       显示帮助
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --data-dir)
            [[ $# -ge 2 ]] || { err "--data-dir 缺少路径"; exit 1; }
            DATA_DIR="$2"; shift 2 ;;
        --dry-run) DRY_RUN="true"; shift ;;
        --yes|-y) ASSUME_YES="true"; shift ;;
        --help|-h) usage; exit 0 ;;
        *) err "未知参数: $1"; usage; exit 1 ;;
    esac
done

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

read_app_data_path() {
    local config_file="$1"
    awk '
        /^app_data_path:[[:space:]]*/ {
            sub(/^app_data_path:[[:space:]]*/, "")
            sub(/[[:space:]]+#.*/, "")
            gsub(/^[[:space:]"\047]+|[[:space:]"\047]+$/, "")
            print
            exit
        }
    ' "$config_file"
}

normalize_path() {
    local path="$1" base_dir="$2"
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

if [[ -z "$DATA_DIR" ]]; then
    for config_file in "$script_dir/config.yml" "$PWD/config.yml" "$HOME/bililive-go/config.yml"; do
        if [[ ! -f "$config_file" ]]; then
            continue
        fi
        configured_path=$(read_app_data_path "$config_file")
        if [[ -n "$configured_path" ]]; then
            DATA_DIR=$(normalize_path "$configured_path" "$(dirname -- "$config_file")")
            log "从配置文件探测数据目录: $config_file"
            break
        fi
    done
fi

if [[ -z "$DATA_DIR" ]]; then
    DATA_DIR="$HOME/bililive-go/Data"
fi
DATA_DIR=$(normalize_path "$DATA_DIR" "$PWD")
CACHE_DIR="${DATA_DIR%/}/hls-cache"

if [[ ! -e "$CACHE_DIR" ]]; then
    ok "无需修复：HLS 缓存目录不存在（${CACHE_DIR}）"
    exit 0
fi
if [[ "$(basename -- "$CACHE_DIR")" != "hls-cache" ]]; then
    err "安全校验失败：待删除路径末级目录不是 hls-cache：$CACHE_DIR"
    exit 1
fi
if [[ ! -d "$CACHE_DIR" && ! -L "$CACHE_DIR" ]]; then
    err "安全校验失败：目标不是目录：$CACHE_DIR"
    exit 1
fi

cache_size=$(du -sh "$CACHE_DIR" 2>/dev/null | awk '{print $1}' || true)
cache_size=${cache_size:-未知}
cache_dirs=$(find "$CACHE_DIR" -mindepth 1 -maxdepth 1 -type d -print 2>/dev/null | awk 'END {print NR+0}' || true)

log "HLS 缓存目录: $CACHE_DIR"
log "占用空间: ${cache_size}；缓存子目录: $cache_dirs"

process_running="false"
if command -v pgrep >/dev/null 2>&1 && pgrep -x bililive-go >/dev/null 2>&1; then
    process_running="true"
fi
if command -v docker >/dev/null 2>&1 \
    && docker ps --format '{{.Names}}' 2>/dev/null | grep -Fxq bililive-go; then
    process_running="true"
fi
if [[ "$process_running" == "true" ]]; then
    warn "检测到 bililive-go 正在运行；删除缓存本身安全，但正在进行的 HLS 转封装可能中断"
fi

if [[ "$DRY_RUN" == "true" ]]; then
    ok "dry-run 完成：未删除任何文件"
    exit 0
fi

if [[ "$ASSUME_YES" != "true" ]]; then
    answer=""
    if [[ -t 0 ]]; then
        read -rp "确认删除整个 hls-cache 目录？[y/N]: " answer || true
    elif [[ -e /dev/tty ]]; then
        read -rp "确认删除整个 hls-cache 目录？[y/N]: " answer < /dev/tty || true
    fi
    answer=$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')
    if [[ "$answer" != "y" && "$answer" != "yes" ]]; then
        warn "已取消，未删除任何文件"
        exit 0
    fi
fi

rm -rf -- "$CACHE_DIR"
ok "HLS 缓存已删除，释放空间约 $cache_size"
