#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# ============================================================
# release.sh — 版本号写回 + git tag + git push
# 用法: bash scripts/release.sh [--dry-run] <VERSION>
# 示例: bash scripts/release.sh 1.3.0
#       bash scripts/release.sh --dry-run 1.3.0-rc1
# ============================================================

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    shift
fi

VERSION="${1:-}"
USAGE="用法: bash scripts/release.sh [--dry-run] <x.y.z[-rcN]>"

# ---- 1. 参数校验 ----
if [[ -z "$VERSION" ]]; then
    echo "错误：缺少版本号参数"
    echo "$USAGE"
    exit 1
fi

if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$'; then
    echo "错误：版本号格式不合法: $VERSION"
    echo "期望格式: x.y.z 或 x.y.z-rcN"
    echo "$USAGE"
    exit 1
fi

TAG="v${VERSION}"

# ---- 2. 前置检查 ----
if [[ "$DRY_RUN" == "true" ]]; then
    echo "→ 预演模式：仅展示将要替换的内容，不提交不推送"
else
    BRANCH=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$BRANCH" != "main" ]]; then
        echo "错误：当前分支为 '$BRANCH'，必须在 main 分支上发布"
        exit 1
    fi

    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo "错误：工作区不干净，请先提交或暂存所有更改"
        exit 1
    fi

    if git tag -l "$TAG" | grep -q .; then
        echo "错误：tag '$TAG' 已存在"
        exit 1
    fi
fi

# ---- 3. 版本号写回 ----
echo "→ 写回版本号 ${TAG} …"

# sed 双兼容：GNU sed 和 BSD sed（macOS）都接受 -i.bak（无空格）
# -E 在 BSD sed 上启用扩展正则，GNU sed 中 -E 等价于 -r（均支持）
sed_cmd() {
    local pattern="$1"
    local file="$2"
    sed -E -i.bak "$pattern" "$file" && rm -f "${file}.bak"
}

if [[ "$DRY_RUN" == "true" ]]; then
    # 预演：在临时副本上执行，然后 diff
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT
    for f in README.md docker-compose.yml; do
        cp "$f" "$tmpdir/$f"
    done
    sed_cmd "s/xuniubi\/bililive-go:v[0-9.]+(-rc[0-9]+)?/xuniubi\/bililive-go:${TAG}/g" "$tmpdir/README.md"
    sed_cmd "s/标签：\`v[0-9.]+(-rc[0-9]+)?\`/标签：\`${TAG}\`/g" "$tmpdir/README.md"
    sed_cmd "s/--build-arg VERSION=v[0-9.]+(-rc[0-9]+)?/--build-arg VERSION=${TAG}/g" "$tmpdir/README.md"
    sed_cmd "s/xuniubi\/bililive-go:v[0-9.]+(-rc[0-9]+)?/xuniubi\/bililive-go:${TAG}/g" "$tmpdir/docker-compose.yml"

    echo ""
    echo "--- README.md diff ---"
    diff -u README.md "$tmpdir/README.md" || true
    echo ""
    echo "--- docker-compose.yml diff ---"
    diff -u docker-compose.yml "$tmpdir/docker-compose.yml" || true
    echo ""
    echo "→ 预演完成。确认无误后执行："
    echo "  bash scripts/release.sh ${VERSION}"
    exit 0
fi

sed_cmd "s/xuniubi\/bililive-go:v[0-9.]+(-rc[0-9]+)?/xuniubi\/bililive-go:${TAG}/g" README.md
sed_cmd "s/标签：\`v[0-9.]+(-rc[0-9]+)?\`/标签：\`${TAG}\`/g" README.md
sed_cmd "s/--build-arg VERSION=v[0-9.]+(-rc[0-9]+)?/--build-arg VERSION=${TAG}/g" README.md
sed_cmd "s/xuniubi\/bililive-go:v[0-9.]+(-rc[0-9]+)?/xuniubi\/bililive-go:${TAG}/g" docker-compose.yml

echo "→ 版本号写回完成"

# ---- 4. Git 操作 ----
echo "→ 提交并推送 …"
git add README.md docker-compose.yml
git commit -m "release: bump version to ${TAG}"
git tag "$TAG"
git push origin main
git push origin "$TAG"

# ---- 5. 输出摘要 ----
echo ""
echo "✅ ${TAG} 已发布"
echo "🔗 GitHub Actions: https://github.com/xuyuanzhang1122/bililive-go-UI/actions"
echo "🔗 Docker Hub:      https://hub.docker.com/r/xuniubi/bililive-go"
