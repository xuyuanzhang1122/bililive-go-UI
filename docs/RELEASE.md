# 发布指南

## 前置条件

- 仓库 main 分支的 push 权限
- Docker Hub `xuniubi` 组织的 secrets（DOCKER_USERNAME / DOCKER_TOKEN），已在 GitHub repo settings 中配置

## 发布步骤

### 1. 确保 main 分支代码通过全部检查

```bash
make build-web dev && make lint && make test
```

### 2. 运行发布脚本

```bash
bash scripts/release.sh 1.3.0
```

此脚本执行：
- 参数校验（格式、分支、工作区干净、tag 不重复）
- 版本号写回：`README.md`（8 处）和 `docker-compose.yml`（1 处）
- `git commit` + `git tag v1.3.0` + `git push origin main --tags`

### 3. 等待 CI 完成

- 打开 [GitHub Actions](https://github.com/xuyuanzhang1122/bililive-go-UI/actions)
- 确认 Release workflow 全部变绿（编译 → 打包 → GitHub Release → Docker 推送）
- 确认 [Docker Hub](https://hub.docker.com/r/xuniubi/bililive-go) 出现新 tag

`release.yaml` 的 `release-docker-images` job 仅在非 rc 版本打 `:latest` tag。

### 4. 验证

```bash
# Docker
docker pull xuniubi/bililive-go:v1.3.0

# 或在干净服务器跑 install.sh
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/v1.3.0/scripts/install.sh | bash
```

## 故障排查

- **CI 失败**：检查 Actions 日志，修复后打递增版本重试（如 v1.3.0-rc1 → v1.3.0-rc2）。已推送的 tag 不要删除，避免下游缓存污染。
- **Docker 推送失败**：检查 `DOCKER_USERNAME` / `DOCKER_TOKEN` secrets 是否有效。
- **版本号不一致**：确认 `scripts/release.sh` 的 sed 规则匹配当前 README / docker-compose.yml 格式。

## 手动应急发布

如果 tag 触发的自动 release 异常，可使用 `docker-publish.yml` 手动构建并推送 Docker 镜像：

1. 打开 GitHub Actions → Docker Publish → Run workflow
2. 输入镜像 tag（默认取 `git describe`）
3. 确认推送成功后，手动创建 GitHub Release 并上传二进制资产

## 版本号规范

- 稳定版：`x.y.z`（如 `1.3.0`）
- 候选版：`x.y.z-rcN`（如 `1.3.0-rc1`）
- Tag 前缀 `v`（如 `v1.3.0`）
- Docker 镜像：`xuniubi/bililive-go:v1.3.0`（`:latest` 仅指向最新非 rc 稳定版）
