#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

# 正式发布：版本写回、提交、标签与原子推送；sun 保留特殊标签名称。
# 用法：bash scripts/release.sh [--dry-run] <sun|x.y.z[-rc.N]>
DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then DRY_RUN=true; shift; fi
VERSION="${1:-}"
if [[ "$VERSION" == "sun" ]]; then
    TAG=sun
elif [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc\.?[0-9]+)?$ ]]; then
    TAG="v${VERSION}"
else
    echo "错误：版本格式应为 sun 或 x.y.z[-rc.N]" >&2
    exit 1
fi

update_versions() {
    local root_dir="$1"
    python3 - "$root_dir" "$TAG" "$VERSION" <<'PYVERSIONS'
from pathlib import Path
import json, re, sys
root, tag, version = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
for name in ['README.md', 'docker-compose.yml']:
    p = root / name
    text = re.sub(r'xuniubi/bililive-go:(?:v[0-9][A-Za-z0-9._-]*|sun)', f'xuniubi/bililive-go:{tag}', p.read_text())
    text = re.sub(r'--build-arg VERSION=(?:v[0-9][A-Za-z0-9._-]*|sun)', f'--build-arg VERSION={tag}', text)
    p.write_text(text)
p = root / 'src/webapp/public/version.json'
metadata = json.loads(p.read_text())
metadata['uiVersion'] = version
metadata['releaseVersion'] = '3.0.0' if version == 'sun' else version
metadata['minBackendVersion'] = tag
p.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
PYVERSIONS
}

if [[ "$DRY_RUN" == "true" ]]; then
    task_tmp=$(mktemp -d)
    trap 'rm -rf "$task_tmp"' EXIT
    mkdir -p "$task_tmp/src/webapp/public"
    cp README.md docker-compose.yml "$task_tmp/"
    cp src/webapp/public/version.json "$task_tmp/src/webapp/public/"
    update_versions "$task_tmp"
    echo "预演发布标签：${TAG}（不修改、不提交、不推送）"
    for file in README.md docker-compose.yml src/webapp/public/version.json; do
        diff -u "$file" "$task_tmp/$file" || true
    done
    exit 0
fi

if [[ "$(git branch --show-current)" != main ]]; then
    echo "错误：正式发布脚本须在 main 分支执行" >&2
    exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
    echo "错误：工作区须先验证并提交，不能混入未提交改动" >&2
    exit 1
fi
if git rev-parse --verify "refs/tags/$TAG" >/dev/null 2>&1; then
    echo "错误：标签已存在：$TAG" >&2
    exit 1
fi
update_versions .
git add README.md docker-compose.yml src/webapp/public/version.json
if ! git diff --cached --quiet; then git commit -m "release: 同步 ${TAG} 版本元数据"; fi
git tag -a "$TAG" -m "${TAG} 正式发布"
git push --atomic origin main "refs/tags/$TAG"
echo "已触发构建，请等待 GitHub Actions 产物与 Docker 全部成功后验收。"
echo "https://github.com/xuyuanzhang1122/bililive-go-UI/actions"
