#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# ============================================================
# install.sh — 一行安装 bililive-go
# 用法:
#   curl -fsSL <raw-url> | bash                     # Docker（默认）
#   curl -fsSL <raw-url> | bash -s -- --binary       # 裸机二进制
#   curl -fsSL <raw-url> | bash -s -- --version v1.3.0  # 指定版本
# ============================================================

REPO="xuyuanzhang1122/bililive-go-UI"
DOCKER_IMAGE="xuniubi/bililive-go"
MODE="docker"
TAG="latest"

usage() {
    echo "用法: bash install.sh [--binary] [--version TAG] [--help]"
    echo ""
    echo "选项:"
    echo "  --binary         安装裸机二进制（默认安装 Docker 容器）"
    echo "  --version TAG    指定版本（默认 latest，示例: --version v1.3.0）"
    echo "  --help           输出此帮助信息"
    exit 0
}

# ---- 参数解析 ----
while [[ $# -gt 0 ]]; do
    case "$1" in
        --binary) MODE="binary"; shift ;;
        --version) TAG="$2"; shift 2 ;;
        --help) usage ;;
        *) echo "未知参数: $1"; usage ;;
    esac
done

# ============================================================
# Docker 路径
# ============================================================
if [[ "$MODE" == "docker" ]]; then
    echo "→ bililive-go 安装脚本（Docker 模式）"

    if ! command -v docker &>/dev/null; then
        echo "错误：未检测到 Docker。请先安装 Docker："
        echo "  https://docs.docker.com/engine/install/"
        exit 1
    fi

    echo "→ 拉取镜像: ${DOCKER_IMAGE}:${TAG} ..."
    docker pull "${DOCKER_IMAGE}:${TAG}"

    echo ""
    echo "✅ 镜像已就绪。启动容器："
    echo ""
    echo "  docker run -d \\"
    echo "    --name bililive-go \\"
    echo "    --restart unless-stopped \\"
    echo "    -p 8080:8080 \\"
    echo "    -v \$(pwd)/Videos:/srv/bililive \\"
    echo "    -v \$(pwd)/config.docker.yml:/etc/bililive-go/config.yml \\"
    echo "    ${DOCKER_IMAGE}:${TAG}"
    exit 0
fi

# ============================================================
# 二进制路径
# ============================================================
echo "→ bililive-go 安装脚本（二进制模式）"

# GitHub /releases/download/<tag>/ 不支持 "latest" 占位符，需先解析真实 tag
if [[ "$TAG" == "latest" ]]; then
    echo "→ 查询最新版本 …"
    RESOLVED=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep -o '"tag_name": *"[^"]*"' \
        | head -1 \
        | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    if [[ -z "$RESOLVED" ]]; then
        echo "错误：无法查询最新版本，请用 --version 指定具体版本"
        echo "示例: bash install.sh --binary --version v1.3.0"
        exit 1
    fi
    TAG="$RESOLVED"
    echo "→ 最新版本: ${TAG}"
fi

# 检测 OS/ARCH
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
RAW_ARCH=$(uname -m)

case "$RAW_ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    arm64)   GOARCH="arm64" ;;
    armv7l)  GOARCH="arm" ;;
    armv6l)  GOARCH="arm" ;;
    *)
        echo "错误：不支持的 CPU 架构: $RAW_ARCH"
        echo "请手动从以下地址下载："
        echo "  https://github.com/${REPO}/releases"
        exit 1
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    *)
        echo "错误：不支持的操作系统: $OS"
        echo "请使用 Docker 模式安装："
        echo "  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash"
        exit 1
        ;;
esac

ASSET="bililive-${OS}-${GOARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

echo "→ 检测平台: ${OS}/${GOARCH}"
echo "→ 下载: ${URL}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if ! curl -fsSL "$URL" -o "${TMPDIR}/${ASSET}"; then
    echo "错误：下载失败（URL: ${URL}）"
    echo "请确认版本 '${TAG}' 已发布："
    echo "  https://github.com/${REPO}/releases"
    exit 1
fi

echo "→ 解压 …"
tar xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"
BINARY=$(find "$TMPDIR" -name 'bililive-*' -type f | head -1)
if [[ -z "$BINARY" ]]; then
    echo "错误：压缩包中未找到二进制文件"
    exit 1
fi

# 安装路径选择：sudo → 无密码写 → ~/.local/bin
install_binary() {
    local src="$1"
    local dest_dir

    if command -v sudo &>/dev/null && sudo -n true 2>/dev/null; then
        dest_dir="/usr/local/bin"
        sudo mv "$src" "${dest_dir}/bililive-go"
        sudo chmod +x "${dest_dir}/bililive-go"
    elif [[ -w /usr/local/bin ]]; then
        dest_dir="/usr/local/bin"
        mv "$src" "${dest_dir}/bililive-go"
        chmod +x "${dest_dir}/bililive-go"
    else
        dest_dir="${HOME}/.local/bin"
        mkdir -p "$dest_dir"
        mv "$src" "${dest_dir}/bililive-go"
        chmod +x "${dest_dir}/bililive-go"
        echo ""
        echo "⚠ 无 sudo 权限，已安装到 ${dest_dir}/bililive-go"
        echo "  请确保该目录在 PATH 中，或将下行加入 ~/.bashrc / ~/.zshrc："
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        return
    fi
    echo ""
    echo "✅ bililive-go 已安装到 ${dest_dir}/bililive-go"
    echo "   运行: bililive-go -c /path/to/config.yml"
}

install_binary "$BINARY"
