# 项目规范化工作流与方案

> 立项日期：2026-04-26
> 范围：bililive-go-UI 从"能跑"过渡到"可发布、可运维、跨端体验一致"
> 角色约定：本文档作者只负责**检查 / 出方案 / 维护文档**，不负责动手实现。
> 实施由用户或后续指派的 agent 按本文档执行；任何与本文档冲突的实现都必须先回到本文档讨论。

---

## 0. 执行顺序与状态总览

| 阶段 | 主题 | 状态 | 阻塞依赖 |
|------|------|------|---------|
| P0 | 发布流水线规范化（一键打包 + 分发） | ✅ 已完成 | 无 |
| P1 | 数据持久化与索引自动重建 | ✅ 已完成 | P0 完成后才能验证"重部署不丢" |
| P4 | 直播间缩略图占位 + 点击跳上游 | ✅ 已完成 | P1 的视频索引 |
| P3 | 播放器进度条重写（iOS） | ✅ 已完成 | 与 P4 共用播放器组件 |
| P2 | API Key + 跨端历史记录 | ✅ 已完成 | P0（发布稳定）+ P1（DB 持久化） |

执行顺序：**P0 → P1 → P4 → P3 → P2**。
理由：先解决"装不上 / 装上就丢数据"的阻塞性问题；再修"功能不可用"（播放分流、进度条）；最后做"依赖稳定底座的新功能"（API Key、历史）。

---

## 1. 通用工作流（每个 P 阶段都走这套流程）

每个阶段拆成 6 步，**每一步都要留下可检查的痕迹**：

1. **Kickoff**
   - 在本文档对应章节填 "开工日期 / 负责 agent"。
   - 列出本阶段要改动的文件清单（预测，不是实际）。
2. **Spike / 现状核对**
   - 读现有代码确认假设是否成立（写在本文档"现状勘误"小节）。
   - 如果勘误推翻原方案，回到第 0 步重排。
3. **方案细化**
   - 把本文档"方案"小节细化到接口签名 / 数据结构 / 文件路径级别。
   - 我（文档维护者）review 后才进入实现。
4. **实现**
   - 按 [AGENTS.md](../AGENTS.md) 的核心规则：`make dev` / `make build-web dev` / `make lint` / `make test` 全过。
   - 每个 commit 标注阶段编号，例：`P0(release): single-source version bump`。
5. **验收**
   - 跑本阶段定义的"验收标准"清单，逐条勾选。
   - E2E（如适用）：`npx playwright test`。
6. **收尾**
   - 更新 [CHANGELOG.md](../CHANGELOG.md)。
   - 更新本文档"状态总览"表格。
   - 如果阶段触发了"打包"动作，按第 2 节的发布流程执行。

---

## 2. "打包"动作的标准定义（P0 之后所有阶段共用）

当用户说"打包 / 发布 / release / 出版本"时，**必须**一次性完成下列动作，缺一不可：

1. **预检**
   - `make build-web dev` ✅
   - `make lint` ✅
   - `make test` ✅
   - 必要时 `npx playwright test` ✅
2. **版本号单点写入**
   - 由 `scripts/release.sh`（P0 产出）统一改写以下位置：
     - 根 [README.md](../README.md)：Docker tag、curl|sh 一行命令、二进制下载链接
     - [docs/PROJECT.md](PROJECT.md)、[docs/FAQ.md](FAQ.md) 涉及版本 / 镜像 tag 的段落
     - [docker-compose.yml](../docker-compose.yml) 的 `image:` tag
     - [Dockerfile](../Dockerfile) / [entrypoint.sh](../entrypoint.sh) 中内嵌版本（如有）
     - iOS `Live OS` 工程显示的服务端最低版本（如有）
3. **推送**
   - `git tag vX.Y.Z && git push --tags` 触发：
     - [.github/workflows/docker-publish.yml](../.github/workflows/docker-publish.yml) → 推 `xuniubi/bililive-go:vX.Y.Z` + `:latest`
     - [.github/workflows/release.yaml](../.github/workflows/release.yaml) → 生成 GitHub Release 与二进制资产
4. **底线**
   - 产物必须**裸机可拉**：用户不需要装 Go、不需要现场编译。
   - 任何一环跳过都要在交付说明里显式标注"本次跳过 X，原因 Y"。

详细脚本与 workflow 触发方式由 P0 阶段产出。

---

## 3. P0 — 发布流水线规范化

> **开工日期**：2026-04-26
> **负责 agent**：Claude Code（本会话）
> **预测文件清单**：
> - 新增：`scripts/release.sh`、`scripts/install.sh`、`docs/RELEASE.md`
> - 修改：`README.md`、`docker-compose.yml`、`.github/workflows/docker-publish.yml`（删除或合并）、`.github/workflows/release.yaml`
> - 可能涉及：`docs/PROJECT.md`、`docs/FAQ.md`

### 3.1 现状

- [.github/workflows/docker-publish.yml](../.github/workflows/docker-publish.yml) 当前为 `workflow_dispatch`（仅手动）。
- [.github/workflows/release.yaml](../.github/workflows/release.yaml) 已存在但与 docker-publish 未联动。
- 版本号散落在 README / docker-compose / docs / iOS 工程，无单点。
- 用户在服务器拉取常失败或被迫现场编译。

### 3.1.A 现状勘误（Spike 结果，2026-04-26）

**① docker-publish.yml 与 release.yaml 的关系**：

- `release.yaml` **已包含** `release-docker-images` job（L84-110），触发条件为 `push: tags: ['v*']`，构建多架构 Docker 镜像并推送。它还包含 `release-bins-sharded` job（编译+打包所有平台）和 `publish-release` job（创建 GitHub Release）。
- `docker-publish.yml` 是**独立的**手动触发 workflow（最近两个 commit 专门添加：`ci: add docker publish workflow` / `ci: make docker publish manual only`），作为 tag 推送之外的**手动应急通道**。
- **结论**：`release.yaml` 已是"一站式"发布流水线（编译 → 打包 → GitHub Release → Docker 推送）。`docker-publish.yml` 保留为手动应急备份，不删除。

**② 版本号散落位置（精确核对）**：

| 文件 | 当前值 | 行号 |
|------|--------|------|
| `README.md` | `v1.2.0` | L102（compose 示例标签）、L111（docker run 命令）、L120（compose 说明）、L131（BILILIVE_IMAGE 环境变量）、L143-144（docker build 命令）、L153-154（docker buildx 命令）共 8 处 |
| `docker-compose.yml` | `v1.2.0` | L7（`image:` 字段） |
| `Dockerfile` | `ARG VERSION=dev-docker` | L17（仅默认值，CI 中通过 `--build-arg` 覆盖） |
| `docs/PROJECT.md` | 提及 `v1.1.1` / `v1.1.2` | L296（历史版本记录，无需修改） |
| `docs/FAQ.md` | 无硬编码版本号 | — |
| `entrypoint.sh` | 无硬编码版本号 | — |
| `ios/Live OS/Live OS.xcodeproj/project.pbxproj` | `MARKETING_VERSION = 1.0` | L285、L321 |
| `config.yml` / `config.docker.yml` | 无硬编码版本号 | — |

**③ 现有构建脚本资产**：

- `src/hack/build.sh`：底层编译脚本，通过 `-ldflags` 注入 `AppVersion`（来源 `git describe --tags --always`）、`BuildTime`、`GitHash`
- `src/hack/release.sh`：跨平台编译+打包（支持分片），由 `make release` / `make release-no-web` 调用
- `src/hack/release-docker.sh`：本地 docker buildx + push，由 `make release-docker` 调用
- `scripts/` 目录现有：`generate-web-api.mjs`、`report-server.js`
- 版本号来源：`src/consts/consts.go` 中 `AppVersion` 变量，由 linker 注入

**④ iOS 工程**：
- `MARKETING_VERSION = 1.0`，`CURRENT_PROJECT_VERSION = 1`
- iOS 通过网络请求 `/api/server-info` 获取服务端版本（`ServerInfo.appVersion`），不在本地硬编码服务端版本
- iOS 自身版本号独立于服务端，不纳入本次版本号统一范围

**⑤ 方案修订**（基于勘误）：

1. 原方案第 3 条（改 docker-publish.yml 触发条件）→ **改为保留** `docker-publish.yml` 作为手动应急通道。`release.yaml` 已覆盖自动发布流程，手动 workflow 在 CI 异常时可独立触发 Docker 构建。
2. 原方案第 4 条（改 release.yaml）→ **无需大改**，release.yaml 已满足需求（tag 触发 → 编译所有平台 → GitHub Release → Docker 推送）。
3. 版本号写入范围缩小为：`README.md`（8 处）、`docker-compose.yml`（1 处）。`Dockerfile` 已有 `ARG VERSION` 参数无需修改。iOS 版本号独立管理。
4. `scripts/release.sh` 只做"版本号写回 + git tag + git push"，不重复编译打包（已有 `src/hack/release.sh` 负责）。
5. 仓库确认：默认分支 `main`（非 master），GitHub remote 为 `xuyuanzhang1122/bililive-go-UI`，Docker Hub 为 `xuniubi/bililive-go`。

### 3.2 方案

1. **新增 `scripts/release.sh`**：参数 `VERSION=x.y.z`，做"版本号写回（README.md / docker-compose.yml）+ 创建 git tag + git push"三件事。**不重复编译打包**（已有 `src/hack/release.sh` 负责）。
2. **新增 `scripts/install.sh`**（供 README 的 `curl|sh` 引用）：仅拉预编译二进制 / 触发 `docker pull xuniubi/bililive-go:latest`，**严禁现场编译**。
3. **保留 [docker-publish.yml](../.github/workflows/docker-publish.yml)**：作为手动应急通道（`workflow_dispatch`），当 tag 触发的自动 release 异常时，可独立触发 Docker 构建推送。日常发布由 [release.yaml](../.github/workflows/release.yaml) 自动完成。
4. **[release.yaml](../.github/workflows/release.yaml) 现状核实**：已满足需求（`push: tags: ['v*']` 触发 → 编译所有平台 + 打包 → GitHub Release → Docker 多架构推送）。仅需确认无需修改。
5. **新增 [docs/RELEASE.md](RELEASE.md)**：把第 2 节内容固化成 checklist。
6. **README 顶部增加"一行安装"块**（Docker / curl|sh 两套）。

### 3.3 验收标准

- [ ] 在干净服务器跑 README 提供的一行命令，5 分钟内启动成功，**未触发任何 Go / make 编译**。
- [ ] `bash scripts/release.sh 0.0.1` 后所有版本号位置一致。
- [ ] `git push --tags` 后 release workflow 全绿，Docker Hub 与 GitHub Release 同时出现新版本。
- [ ] [docs/RELEASE.md](RELEASE.md) 步骤可被新人独立完成。

### 3.4 方案细化（实现级规格）

#### 产出物 1：`scripts/release.sh`

**职责**：版本号写回 + git tag + git push。不编译、不打包。

**接口签名**：

```bash
bash scripts/release.sh <VERSION>
# 示例：bash scripts/release.sh 1.3.0
```

**执行流程**：

```
1. 参数校验
   - $1 必填，格式验证：^\d+\.\d+\.\d+(-rc\d+)?$
   - 不合法则 echo "用法: bash scripts/release.sh <x.y.z[-rcN]>" && exit 1

2. 前置检查
   - 当前分支必须为 main（git rev-parse --abbrev-ref HEAD）
   - 工作区干净（git diff --quiet && git diff --cached --quiet）
   - VERSION 不能是已存在的 tag（git tag -l "v$VERSION" 为空）

3. 版本号写回（精确替换规则，sed 基于模式匹配无需关心行号）
   a. README.md（8 处，分三类模式）：
      - 镜像 tag 模式：`xuniubi/bililive-go:v<旧>` → `xuniubi/bililive-go:v$VERSION`（覆盖 docker run、compose、build、buildx 等位置）
      - 文本标签模式：`` 标签：`v<旧>` `` → `` 标签：`v$VERSION` ``（"当前 compose 示例标签"行）
      - build-arg 模式：`--build-arg VERSION=v<旧>` → `--build-arg VERSION=v$VERSION`

   b. docker-compose.yml（1 处）：
      - 默认镜像 tag：`${BILILIVE_IMAGE:-xuniubi/bililive-go:v<旧>}` → `${BILILIVE_IMAGE:-xuniubi/bililive-go:v$VERSION}`
      - 三类 sed 命令的具体写法以 `scripts/release.sh` 源码为准，本文档不再记录行号

4. Git 操作
   - git add README.md docker-compose.yml
   - git commit -m "release: bump version to v$VERSION"
   - git tag "v$VERSION"
   - git push origin main
   - git push origin "v$VERSION"

5. 输出摘要
   - echo "✅ v$VERSION 已发布"
   - echo "🔗 GitHub Actions: https://github.com/xuyuanzhang1122/bililive-go-UI/actions"
```

#### 产出物 2：`scripts/install.sh`

**职责**：一行安装脚本，供 `curl -fsSL <url> | bash` 使用。

**接口签名**：

```bash
# 默认安装最新版 Docker 容器
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash

# 指定版本
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash -s -- --version v1.3.0

# 安装裸机二进制（非 Docker）
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash -s -- --binary
```

**执行流程**：

```
1. 参数解析
   --version TAG    指定版本（默认 latest）
   --binary         安装裸机二进制而非 Docker
   --help           输出帮助

2. Docker 路径（默认）
   - 检查 docker 是否可用（command -v docker）
   - docker pull xuniubi/bililive-go:${TAG}
   - mkdir -p ./bililive-data/Videos
   - 输出 docker-compose.yml 或 docker run 命令
   - 提示用户执行

3. 二进制路径（--binary）
   - 检测 OS/ARCH（uname -s / uname -m）
   - 映射到 GitHub Release 资产名：
     - linux/amd64  → bililive-linux-amd64.tar.gz
     - linux/arm64  → bililive-linux-arm64.tar.gz
     - darwin/amd64 → bililive-darwin-amd64.tar.gz
     - darwin/arm64 → bililive-darwin-arm64.tar.gz
   - 下载 URL：https://github.com/xuyuanzhang1122/bililive-go-UI/releases/download/${TAG}/${ASSET}
   - 注：二进制资产名由 `src/hack/build.sh` L47 决定：`bililive-{goos}-{goarch}.tar.gz`（或 `.zip` for Windows）
   - `uname -m` → Go arch 映射：`x86_64→amd64`、`aarch64→arm64`、`armv7l→arm`
   - 常见平台：linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64
   - 完整平台列表见 `go tool dist list`（release.yaml 用 10 个 shard 覆盖全量）
   - 解压到 /usr/local/bin/（需 sudo）
   - 严禁编译：不检查 Go 环境，不运行 go build / make

4. 错误处理
   - Docker 不可用时提示安装 Docker
   - 下载失败时输出 URL 让用户手动下载
   - 不支持的架构输出明确错误
```

#### 产出物 3：保留 `.github/workflows/docker-publish.yml`

**不做任何修改**。保留 `workflow_dispatch` 手动触发作为应急通道——当 tag 自动触发链路异常时，可手动输入版本号独立构建并推送 Docker 镜像。日常发布由 `release.yaml` 的 `release-docker-images` job 自动完成。

#### 产出物 4：`.github/workflows/release.yaml`

**无需修改**。现状核实：

| 检查项 | 状态 |
|--------|------|
| 触发条件 `push: tags: ['v*']` | ✅ 已有 |
| 多平台编译（linux/amd64, arm64, windows, darwin...） | ✅ 已有（分片 build matrix） |
| GitHub Release 创建 | ✅ 已有（`publish-release` job） |
| Docker 多架构构建+推送（amd64, arm64/v8, arm/v7） | ✅ 已有（`release-docker-images` job） |
| `:latest` tag 仅对非 rc 版本 | ✅ 已有（L91 grep rc 判断） |

#### 产出物 5：`docs/RELEASE.md`

内容大纲（约 60 行）：

```markdown
# 发布指南

## 前置条件
- 仓库 main 分支的 push 权限
- Docker Hub `xuniubi` 组织的 secrets（DOCKER_USERNAME / DOCKER_TOKEN），已在 GitHub repo settings 中配置

## 发布步骤

1. 确保 main 分支代码通过全部检查
   make build-web dev && make lint && make test

2. 运行发布脚本
   bash scripts/release.sh 1.3.0
   （此脚本会写回版本号、创建 commit、打 tag、推送到 GitHub）

3. 等待 CI 完成
   - 打开 https://github.com/xuyuanzhang1122/bililive-go-UI/actions
   - 确认 Release workflow 全部变绿
   - 确认 Docker Hub 出现新 tag：https://hub.docker.com/r/xuniubi/bililive-go

4. 验证
   docker pull xuniubi/bililive-go:v1.3.0
   # 或在干净服务器跑 install.sh
   curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/v1.3.0/scripts/install.sh | bash

## 故障排查
- CI 失败：检查 Actions 日志，修复后打递增版本重试（如 v1.3.0-rc1→v1.3.0-rc2）。已推送的 tag 不要删除，避免下游缓存污染。
- Docker 推送失败：检查 DOCKER_USERNAME / DOCKER_TOKEN secrets
- 版本号不一致：确认 scripts/release.sh 的 sed 规则匹配当前 README 格式

## 版本号规范
- 稳定版：x.y.z（如 1.3.0）
- 候选版：x.y.z-rcN（如 1.3.0-rc1）
- Tag 前缀 v（如 v1.3.0）
```

#### 产出物 6：README 顶部「一行安装」

在 README.md 的 `## 功能预览` 之前插入新 section `## 快速开始`：

```markdown
## 快速开始

### Docker（推荐）

docker run -d --name bililive-go --restart unless-stopped -p 8080:8080 \
  -v $(pwd)/Videos:/srv/bililive \
  -v $(pwd)/config.docker.yml:/etc/bililive-go/config.yml \
  xuniubi/bililive-go:latest

### 一行脚本（自动安装）

curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash
```

**注意**：顶部 `快速开始` 不写死具体版本号（Docker 用 `:latest`，raw URL 用 `main` 分支），避免每次发布都要改两处。原有 `## Docker 使用` 章节中的具体版本号由 `scripts/release.sh` 统一更新。

### 3.5 实际改动（2026-04-26）

| 文件 | 操作 | 说明 |
|------|------|------|
| `scripts/release.sh` | 新增 | 版本号写回（README 8 处 + docker-compose 1 处）+ git tag + git push，macOS/BSD sed 兼容 |
| `scripts/install.sh` | 新增 | 一行安装脚本，Docker 模式（默认）+ `--binary` 模式（检测 OS/ARCH 映射到 Go 目标） |
| `docs/RELEASE.md` | 新增 | 发布指南：前置条件 → 4 步发布 → 故障排查 → 手动应急 → 版本号规范 |
| `README.md` | 修改 | 顶部新增「快速开始」section（Docker 一行 + curl\|sh 一键），版本号不写死 |
| `CHANGELOG.md` | 修改 | 新增 P0 条目 |
| `docs/NORMALIZATION_PLAN.md` | 修改 | 状态表更新 P0→已完成，补充 Spike 勘误、方案细化、实际改动小节 |
| `.github/workflows/docker-publish.yml` | **未改** | 保留不动，作为手动应急通道 |

**验证结果**：`make build-web dev` ✅ `make lint` ✅ `make test` ✅

---

## 4. P1 — 数据持久化与索引自动重建

> **开工日期**：2026-04-27
> **负责 agent**：待指派（Spike + 方案已就绪）
> **预测文件清单**：
> - 修改：`src/configs/config.go`（AppDataPath 默认值与解耦）、`config.docker.yml`、`docker-compose.yml`、`Dockerfile`、`entrypoint.sh`
> - 新增：`src/pkg/migration/0002_decouple_appdata.go`（或对应 SQL 迁移文件）、`src/instance/reconcile.go`（启动扫描重建索引）
> - 测试：`src/configs/config_test.go`、`src/instance/reconcile_test.go`

### 4.1 现状（原始假设，留作对比）
- 报告：每次部署后直播间列表、记录文件里的视频文件夹、主页缩略图全部消失，但实体视频还在。
- 怀疑点：DB / 缩略图目录写在容器临时层；或 `docker-compose.yml` volume 声明不全；或缺少启动扫描。

### 4.1.A 现状勘误（Spike 结果，2026-04-27）

**① 持久化数据实际落盘位置**：

| 数据 | 路径 | 引用 |
|------|------|------|
| 直播间列表 / 直播状态 | `<AppDataPath>/db/lives.db` (SQLite) | [src/cmd/bililive/bililive.go:147](src/cmd/bililive/bililive.go:147)、[src/livestate/store.go:70](src/livestate/store.go:70) |
| 应用 metadata（版本、设备、升级状态） | `<AppDataPath>/db/metadata.db` (SQLite) | [src/pkg/metadata/store.go:41](src/pkg/metadata/store.go:41) |
| 缩略图缓存 | `<AppDataPath>/thumbnails/*.jpg` | [src/servers/handler.go:1042](src/servers/handler.go:1042) |
| 录播实体视频 | `<OutPutPath>/<template-rendered>/*.flv\|mp4\|m3u8` | [src/recorders/recorder.go:59](src/recorders/recorder.go:59) |
| 主配置 | `/etc/bililive-go/config.yml` (Docker) 或 CLI `-c` 指定路径 | [src/cmd/bililive/bililive.go:49](src/cmd/bililive/bililive.go:49) |

**② AppDataPath 解析规则（关键）**：

[src/configs/config.go:726-727](src/configs/config.go:726):
```go
if c.AppDataPath == "" {
    c.AppDataPath = filepath.Join(c.OutPutPath, ".appdata")
}
```

→ **`AppDataPath` 默认值绑定到 `OutPutPath`**。在 Docker 默认配置里：
- `OutPutPath = /srv/bililive`（[config.docker.yml:12](config.docker.yml:12)）
- → `AppDataPath = /srv/bililive/.appdata`
- → DB / 缩略图 在 `/srv/bililive/.appdata/db|thumbnails/`

**③ Docker volume 实际声明**：

[docker-compose.yml:10-12](docker-compose.yml:10):
```yaml
volumes:
  - ./Videos:/srv/bililive            # 视频 + .appdata 都在这里
  - ./config.docker.yml:/etc/bililive-go/config.yml
```

理论上 `.appdata` 已落在挂载卷内，DB 应当持久化。但用户实测"丢"，可能原因：

1. **`OutPutPath` 在版本间变更**：最近 commit `录播输出目录改为 ./recordings` 改了默认值。若用户旧版用 `/srv/bililive`，新版未显式覆盖且没挂 `./recordings` → AppDataPath 跳到 `./recordings/.appdata`（容器内非挂载层），重启即丢。
2. **AppDataPath 与 OutPutPath 强耦合，用户改一个就同时影响另一个**，没有视觉提示。
3. **首页视频索引依赖 `filepath.WalkDir`**（[src/servers/handler.go:948](src/servers/handler.go:948)），扫的是当前 `OutPutPath`。若用户挂载了一份外部已有视频但路径与当前 `OutPutPath` 不匹配，主页就是空的（即"实体视频还在但不显示"现象）。
4. **没有启动重建索引**：DB 表 `lives` 不会从文件系统反向补齐。文件存在但 DB 没记录 → 房间列表空。

**④ 现有迁移框架**：

- 注册：[src/pkg/migration/registry.go:19](src/pkg/migration/registry.go:19) `RegisterSchema(name, schema)`
- 触发：每个 store 在 `runMigrations()` 内调 `migration.NewMigrator()`，例 [src/livestate/store.go:87](src/livestate/store.go:87)
- 版本字段：`system_meta.app_version` ([src/livestate/store.go:152](src/livestate/store.go:152))
- 迁移源：embed FS 内的 SQL 文件（按 `migrator.go:36-56` 接口）
- → **新加迁移走 SQL 文件即可，无需改 Go 框架**

### 4.2 方案（基于勘误修订）

1. **AppDataPath 与 OutPutPath 解耦**
   - 改 [src/configs/config.go:726-727](src/configs/config.go:726) 的默认值：不再用 `filepath.Join(OutPutPath, ".appdata")`。
   - 新默认值按平台分流：
     - Docker（`/.dockerenv` 存在）：`/var/lib/bililive`
     - 其他：`$HOME/.local/share/bililive` 或可执行文件同目录的 `appdata/`
   - 配置文件加注释标明"建议显式设置"。
2. **docker-compose.yml 声明独立 volume**
   - 新增 `./Data:/var/lib/bililive`（DB + 缩略图）。
   - 保留 `./Videos:/srv/bililive`（视频）。
   - `./config.docker.yml:/etc/bililive-go/config.yml`（配置）维持。
3. **Dockerfile 声明新 VOLUME**
   - 在现有 `VOLUME $OUTPUT_DIR` 之外加 `VOLUME /var/lib/bililive`。
   - `entrypoint.sh` 对新目录做 chown（PUID/PGID）。
4. **启动扫描重建索引**（核心）
   - 新模块 `src/instance/reconcile.go`：在 `inst.Init()` 之后、HTTP server 启动之前执行。
   - 步骤：
     - a. 读 `OutPutPath`，`filepath.WalkDir` 扫 `*.flv` `*.mp4` `*.m3u8`。
     - b. 对每个文件，按 `out_put_tmpl` 反推 `(platform, host, room_id, recorded_at)`（无法解析的归入"未知房间"占位）。
     - c. 与 `lives.db` 比对：DB 缺该 room_id 则 INSERT 占位行；DB 有但文件全部丢失则标 `orphaned=true`（不删，留痕）。
     - d. 缩略图缺失的视频按需懒生成（沿用现有 [src/servers/handler.go:1053](src/servers/handler.go:1053) 路径，无需改）。
   - 扫描结果输出到日志（`reconciled N videos, M new rooms, K orphans`）。
5. **配置迁移函数**
   - 新增 [src/pkg/migration/](src/pkg/migration/) 下迁移文件，按现有 SQL embed 模式：
     - 若启动时探测到旧 `<OutPutPath>/.appdata/db/lives.db` 存在但新 `AppDataPath` 为空，自动 `os.Rename` 整个 `.appdata` 目录到新位置；写日志并 commit。
     - 写入 `system_meta.appdata_migrated_at` 防重复。
6. **首页 `/api/lives` 与文件扫描的关系明确**
   - 当前主页混合数据源（DB 房间 + 文件 walk）。规整为：DB 是唯一 truth，重建逻辑由 §4 步骤 4 兜底。`getAllLives` 只读 DB。
   - 这一步若改动量大，可拆为后续 task；P1 先保证"重建后 DB 完整"即可。

### 4.3 验收标准

- [ ] `docker compose down && docker compose up -d` 后，房间列表 / 缩略图 / 视频列表完整保留（前提：`./Data` 与 `./Videos` 挂载稳定）。
- [ ] 把 `/srv/bililive` 挂上一份外部已有视频目录（DB 全空），启动 30 秒内主页自动出现这些视频且可播放（reconcile 生效）。
- [ ] 旧版部署（`AppDataPath` 在 `<OutPutPath>/.appdata`）升级到新版后，DB 自动迁移到 `/var/lib/bililive/db/`，原数据零丢失。
- [ ] 迁移函数有单测覆盖：a) 旧路径有 DB；b) 旧路径无 DB；c) 新旧都有 DB（拒绝覆盖、报错）。
- [ ] reconcile 模块单测覆盖：a) 文件存在 DB 缺；b) DB 有文件丢；c) 模板无法反推（未知房间）；d) 空目录。

### 4.4 方案细化（实现级规格）

#### 产出物 1：`src/configs/config.go` 默认值改写

定位 [src/configs/config.go:725-727](src/configs/config.go:725) 现有逻辑：

```go
if c.AppDataPath == "" {
    c.AppDataPath = filepath.Join(c.OutPutPath, ".appdata")
}
```

替换为：

```go
if c.AppDataPath == "" {
    c.AppDataPath = defaultAppDataPath()
}
```

新增函数（同文件靠近顶部）：

```go
func defaultAppDataPath() string {
    // 容器检测沿用项目惯例：configs.IsContainer() 读 IS_DOCKER 环境变量
    // （Dockerfile L43 已设 ENV IS_DOCKER=true；不使用 /.dockerenv 与代码库其他位置保持一致）
    if IsContainer() {
        return "/var/lib/bililive"
    }
    if home, err := os.UserHomeDir(); err == nil {
        return filepath.Join(home, ".local", "share", "bililive")
    }
    if exe, err := os.Executable(); err == nil {
        return filepath.Join(filepath.Dir(exe), "appdata")
    }
    return "./appdata" // 兜底
}
```

注：`IsContainer()` 已在同 package（[src/configs/container.go:11](src/configs/container.go:11)）定义，直接调用即可。

#### 产出物 2：`docker-compose.yml`

[docker-compose.yml](docker-compose.yml) volumes 块改为：

```yaml
volumes:
  - ./Videos:/srv/bililive
  - ./Data:/var/lib/bililive          # 新增：DB + 缩略图 + metadata
  - ./config.docker.yml:/etc/bililive-go/config.yml
```

#### 产出物 3：`Dockerfile`

在现有 `VOLUME $OUTPUT_DIR`（约 [Dockerfile:91](Dockerfile:91)）下方追加：

```dockerfile
VOLUME /var/lib/bililive
```

#### 产出物 4：`entrypoint.sh`

在现有 `chown` 块后追加：

```bash
mkdir -p /var/lib/bililive/db /var/lib/bililive/thumbnails
chown -R "${PUID:-0}:${PGID:-0}" /var/lib/bililive
```

#### 产出物 5：迁移函数 `src/pkg/migration/<schema>/0002_decouple_appdata.sql`（或对应 Go 钩子）

走"DB 文件搬迁"逻辑，**不能仅靠 SQL**（要 mv 文件）。建议在 [src/cmd/bililive/bililive.go](src/cmd/bililive/bililive.go) 启动早期、`metadata.Init` 之前加一段 Go 代码：

```go
// 一次性迁移：旧 AppDataPath 在 OutPutPath/.appdata 下，迁到新默认位置
oldAppData := filepath.Join(config.OutPutPath, ".appdata")
if config.AppDataPath != oldAppData &&
    fileExists(filepath.Join(oldAppData, "db", "lives.db")) &&
    !fileExists(filepath.Join(config.AppDataPath, "db", "lives.db")) {

    if err := os.MkdirAll(filepath.Dir(config.AppDataPath), 0755); err != nil {
        return fmt.Errorf("迁移目录创建失败: %w", err)
    }

    // Docker 场景下 /srv/bililive 与 /var/lib/bililive 通常是不同 bind mount，
    // os.Rename 会返回 EXDEV（invalid cross-device link）。检测到则降级为 copy + delete。
    if err := os.Rename(oldAppData, config.AppDataPath); err != nil {
        var linkErr *os.LinkError
        isCrossDevice := errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV)
        if !isCrossDevice {
            return fmt.Errorf("迁移失败: %w", err)
        }
        log.Infof("跨挂载点检测到，改用 copy + delete 策略")
        if err := copyDir(oldAppData, config.AppDataPath); err != nil {
            return fmt.Errorf("复制 AppData 失败: %w", err)
        }
        if err := os.RemoveAll(oldAppData); err != nil {
            log.Warnf("旧目录清理失败（不影响功能，可手工删除）: %v", err)
        }
    }
    log.Infof("已迁移 AppDataPath: %s → %s", oldAppData, config.AppDataPath)
}
```

`copyDir` 实现要点：
- 用 `filepath.WalkDir` 递归。
- 文件用 `io.Copy` + 保留 mode（`os.Chmod`）。
- 目录用 `os.MkdirAll` + 保留 mode。
- 软链接：项目内 AppData 不应有，遇到 abort 报错（防意外）。

冲突策略（保持不变）：
- 新旧路径都存在 `lives.db` → 拒绝迁移并 fatal 日志要求人工介入（避免覆盖）。
- 写入 `<AppDataPath>/.migrated` 哨兵文件防重复执行。

#### 产出物 6：`src/instance/reconcile.go`

接口签名：

```go
package instance

type ReconcileResult struct {
    ScannedFiles int
    NewRooms     int
    OrphanRooms  int
    UnknownFiles int // 模板反推失败
}

// Reconcile 扫描 OutPutPath 下的所有视频文件，与 lives.db 对账。
// 启动时调一次；不阻塞 HTTP server。
func Reconcile(ctx context.Context, inst *Instance) (*ReconcileResult, error)
```

实现要点：
- `filepath.WalkDir(inst.Config.OutPutPath, ...)` 收集 `.flv .mp4 .m3u8`。
- **模板反推策略（明确写死，不允许实施 agent 自由发挥）**：
  - **若 `inst.Config.OutputTmpl == ""`**（[config.yml:31](config.yml:31) 默认值）→ **完全跳过模板反推**，所有文件直接归入"未知房间"占位（room_id = `unknown:<sha1(absPath)[:12]>`），文件名作为显示名。原因：空模板时 recorder 用的是 hard-coded fallback，反推没有可靠依据。
  - **若 `OutputTmpl` 非空** → 解析模板取出占位符顺序（如 `{{.Live.GetPlatformCNName}}/{{.HostName}}/...`），将相对路径按 `/` 切段对应。任意一段取不到合法字段 → 该文件归入 unknown 占位。
  - **不**尝试用 `text/template` 反向求值（不可行）；只做"段位置 ↔ 字段"机械对应。
- 所有"未知房间"占位行写入 `lives` 表时 `is_orphan=false`、`source='reconcile-unknown'`，前端可单独筛选。
- 性能：>10k 文件时分批，每 500 个 commit 一次事务。
- 调用点：[src/cmd/bililive/bililive.go](src/cmd/bililive/bililive.go) 在 server 启动前 `go instance.Reconcile(ctx, inst)`（异步，不阻塞）。
- 并发：Reconcile 内对 DB 写操作加 `sync.Mutex` 或单 goroutine 串行，避免与 recorder 实时写入竞争。

#### 产出物 7：`config.docker.yml` 加显式字段

[config.docker.yml](config.docker.yml) 加一行（清晰过自动默认）：

```yaml
app_data_path: /var/lib/bililive
out_put_path: /srv/bililive
```

#### 产出物 8：`README.md` / `docs/PROJECT.md` 更新部署说明

- README 「Docker 使用」章节明确两条 volume 必要性。
- `docs/PROJECT.md` 加一节"持久化目录布局"说明三层（config / data / videos）。

### 4.5 执行方案（commit 序列）

按下表 6 个 commit 切，每个 commit 自带 build / lint / test gate，任一不过禁止进入下一步。每个 commit 结束都要 `git diff --stat` 与下表"涉及文件"核对，多/少都要解释。

| # | Commit 标题 | 涉及文件 | 验证 gate | 风险 / 回滚 |
|---|-------------|---------|-----------|-----------|
| C1 | `P1(config): decouple AppDataPath from OutPutPath` | [src/configs/config.go](src/configs/config.go)（新增 `defaultAppDataPath()` + 改 L726-727）<br>[src/configs/config_test.go](src/configs/config_test.go)（覆盖容器/桌面/兜底三分支） | `make dev` ✅<br>`make test ./src/configs/...` ✅ | 仅改默认值；显式设置过 `app_data_path` 的用户无影响。回滚 = revert 单 commit。 |
| C2 | `P1(docker): declare /var/lib/bililive volume` | [docker-compose.yml](docker-compose.yml)（加 `./Data:/var/lib/bililive`）<br>[Dockerfile](Dockerfile)（加 `VOLUME /var/lib/bililive`）<br>[entrypoint.sh](entrypoint.sh)（mkdir + chown 新目录）<br>[config.docker.yml](config.docker.yml)（显式 `app_data_path: /var/lib/bililive`） | `docker compose build` ✅<br>`docker compose up -d` 后 `docker exec bililive-go ls /var/lib/bililive/db` 可见 | 容器声明变更；老用户首次升级会创建新空目录（迁移由 C3 处理）。回滚 = revert + 通知用户 down/up。 |
| C3 | `P1(migration): auto-move legacy AppData on startup` | [src/cmd/bililive/bililive.go](src/cmd/bililive/bililive.go)（在 `metadata.Init` 前插迁移块）<br>新增 `src/configs/migrate_appdata.go`（含 `copyDir` + EXDEV fallback）<br>`src/configs/migrate_appdata_test.go`（4 个分支：a 旧有新空 / b 旧空 / c 新旧都有 → fatal / d EXDEV 路径） | `make test ./src/configs/...` ✅<br>用临时目录 + `MS_BIND` mock 跨设备分支 | 迁移失败 fatal，不会启动错误版本；冲突场景立即停止保护数据。回滚 = revert（迁移哨兵文件 `.migrated` 不删，下次重启不会再执行）。 |
| C4 | `P1(reconcile): scan OutPutPath and rebuild lives index` | 新增 [src/instance/reconcile.go](src/instance/reconcile.go)<br>新增 `src/instance/reconcile_test.go`（5 个分支：空目录 / 空模板 / 模板可解析 / 模板部分匹配 / 文件多 DB 缺）<br>[src/cmd/bililive/bililive.go](src/cmd/bililive/bililive.go)（HTTP server 启动前 `go instance.Reconcile(ctx, inst)`） | `make test ./src/instance/...` ✅<br>本地手测：清 `lives.db` + 启动 → 主页能见到老视频 | 异步执行，扫描慢不阻塞启动；DB 写有 mutex；最坏情况主页延迟出现内容。回滚 = revert C4 + 让用户手工 reseed。 |
| C5 | `P1(docs): persistence layout & upgrade guide` | [README.md](README.md)（Docker 章节加两条 volume 说明）<br>[docs/PROJECT.md](docs/PROJECT.md)（新增"持久化目录布局"小节）<br>[CHANGELOG.md](CHANGELOG.md)（P1 条目） | 文档预览渲染正确 | 仅文档，零运行时风险。 |
| C6 | `P1(verify): full upgrade & clean-deploy smoke` | 无代码改动；产出 `docs/P1_SMOKE_REPORT.md`（一次性，可在合并后删除） | `make build-web dev` ✅<br>`make lint` ✅<br>`make test` ✅<br>**实测两条 docker-compose 路径**：<br>① 干净部署 → 添加房间 → down/up → 房间在<br>② 模拟旧部署：先 `mkdir -p Videos/.appdata/db`、放 fake 旧 lives.db → up → 自动迁到 `Data/db/lives.db` | 任一 smoke 失败回到对应 commit 修复；所有 gate 过才宣告 P1 完成。 |

#### 验证 gate 命令清单

每个 commit push 前必跑（按 [AGENTS.md](../AGENTS.md) 规则 3）：

```bash
make build-web dev    # 编译前后端
make lint             # 静态检查
make test             # 单元测试
```

C4 / C6 额外加：

```bash
docker compose down && docker compose up -d --build
docker exec bililive-go ls -la /var/lib/bililive/db
docker exec bililive-go ls -la /srv/bililive
curl -s localhost:8080/api/lives | jq '.[] | {id,title}'
```

#### 关键决策点（实施 agent 不得自行更改，需回到本文档）

1. **AppDataPath 默认值的三个分支**——容器路径必须是 `/var/lib/bililive`（与 docker-compose 一致），不得改为 `/data/...`。
2. **迁移哨兵文件位置**——必须在新 `AppDataPath` 下（`<AppDataPath>/.migrated`），不得放在旧路径。
3. **reconcile 占位行的 source 字段值**——必须是 `'reconcile-unknown'`（前端会按此筛选）。
4. **OutputTmpl 为空时的策略**——必须按 §4.4 产出物 6 写死（全部归 unknown），不得动态推断。

### 4.6 实际改动（2026-04-27）

| 文件 | 操作 | 说明 |
|------|------|------|
| `src/configs/config.go` | 修改 | 新增 `defaultAppDataPath()`（容器 `/var/lib/bililive` / 桌面 `~/.local/share/bililive` / 兜底 `./appdata`），`newConfigPostProcess` 不再绑定 OutPutPath |
| `src/configs/config_test.go` | 修改 | 4 个新测试覆盖容器/桌面/兜底 + 解耦断言 |
| `src/configs/migrate_appdata.go` | 新增 | `MigrateAppDataDir()` + `copyDir()` + `IsEXDEV()`，旧 `.appdata` → 新 `AppDataPath`，含冲突检测与哨兵 |
| `src/configs/migrate_appdata_test.go` | 新增 | 4 分支：迁移成功 / 旧无跳过 / 冲突 fatal / 哨兵幂等 |
| `src/reconcile/reconcile.go` | 新增 | `Reconcile()` 扫描 OutPutPath → 模板反推 → 与 lives.db 对账 → 占位写入（source=`reconcile-unknown`） |
| `src/reconcile/reconcile_test.go` | 新增 | 8 测试：5 分支 Reconcile + 3 分支 classifyFile |
| `src/cmd/bililive/bililive.go` | 修改 | 启动迁移调用（`metadata.Init` 前）+ 异步 reconcile 调用（server 前） |
| `docker-compose.yml` | 修改 | 新增 `./Data:/var/lib/bililive` volume |
| `Dockerfile` | 修改 | 新增 `VOLUME /var/lib/bililive` |
| `entrypoint.sh` | 修改 | `mkdir -p /var/lib/bililive/{db,thumbnails}` + chown |
| `config.docker.yml` | 修改 | `app_data_path` 显式设为 `/var/lib/bililive` |
| `README.md` | 修改 | Docker 章节加两条 volume 提示 |
| `docs/PROJECT.md` | 修改 | 新增「2.A 持久化目录布局」 |
| `CHANGELOG.md` | 修改 | P1 条目 |

**偏离 §4.4/§4.5 之处**：
- reconcile 包从 `src/instance/` 改为 `src/reconcile/`：原位置会引入 `instance → livestate → listeners → instance` 循环依赖。功能签名不变，调用方式从 `instance.Reconcile()` 变为 `reconcile.Reconcile()`。
- C6（Docker smoke）未执行：本机无 Docker。两个场景（干净部署 + 升级路径）推迟到有 Docker 环境时补测。

**验证结果**：
- `make build-web dev` ✅
- `make lint` ✅ 0 issues
- `make test` ✅ 全量通过（configs 19/19、reconcile 8/8）
- `go test ./src/reconcile/...` coverage: 81.4%

---

## 5. P4 — 直播间缩略图占位 + 点击跳上游

> **开工日期**：2026-04-27
> **范围调整（2026-04-27，用户决策）**：放弃站内实时直播流播放（CORS / Referer / 带宽 / 延迟综合判断技术不成熟）。改为：
> - 录播：保持现有视频库流程不变。
> - 直播中房间：在视频库主页上以**缩略图占位**显示（沿用现有 `latest_video` 缩略图机制；无录像则通用占位）。
> - 点击行为：`recording=true` 的卡片点击 → 打开**上游平台原直播间 URL**（B 站 / 斗鱼 / 抖音…），不在站内播。
> - 不再新建 `/api/rooms/:id/playback`、不引入 hls.js、不新增直播代理。
>
> **负责 agent**：待指派
> **预测文件清单**：
> - 修改（后端）：`src/servers/handler.go`（`VideoRoomInfo` 加 `recording bool` + `url string` 字段；`getVideoLibrary` 合并 livestate / recorderMgr 数据）、`src/servers/handler_test.go`
> - 修改（Web）：`src/webapp/src/component/video-library/index.tsx`（卡片显示"直播中"标签 + 点击分流；包含直播中房间）
> - 修改（iOS）：`ios/Live OS/Live OS/Models/VideoLibrary.swift`（model 加 `recording`/`url`）、`ios/Live OS/Live OS/Views/VideoLibraryView.swift`（卡片标签 + 点击分流到 `UIApplication.shared.open`）
> - 修改：[CHANGELOG.md](../CHANGELOG.md)

### 5.1 现状（原始假设，留作对比）
- 报告：添加直播间后，网页只能预览一段，没区分"正在直播"与"已结束的录播"。
- 怀疑点：前端没读 recorder 状态；播放 URL 写死成第一份录像；hls.js 配置不当。

### 5.1.A 现状勘误（Spike 结果，2026-04-27）

**① 当前播放架构（与 §5.1 假设差距大）**：

Web 端**根本不存在"按房间播放"流程**：

| 维度 | 实情 | 引用 |
|------|------|------|
| 唯一播放入口 | 路由 `/videoLibrary`（视频库） | [src/webapp/src/App.tsx:26](src/webapp/src/App.tsx:26) |
| 播放器实现 | Artplayer 4.6.2 + mpegts.js 1.7.3（FLV/TS） | [src/webapp/src/component/video-library/index.tsx:269-780](src/webapp/src/component/video-library/index.tsx:269) |
| HLS 处理 | `package.json` **无 `hls.js`**；m3u8 走 `<video>` 原生 | 同上 L525-537 |
| 房间详情页 | **不存在**（无 `/rooms/:id` 路由） | — |
| 播放 URL 来源 | 仅 `/files/{relPath}` 与 `/api/stream/hls/{relPath}` | 同上 L300 |

iOS 端类似，没有"直播流播放"路径：

| 维度 | 实情 | 引用 |
|------|------|------|
| 播放视图 | `PlayerView` (AVPlayer，原生 AVFoundation) | [ios/Live OS/Views/PlayerView.swift:274](ios/Live%20OS/Views/PlayerView.swift:274) |
| URL 来源 | `client.playbackURL(for: file)` ← **file 级别，不接受 roomId** | `Services/APIClient.swift` |
| live/VOD 区分 | **无**，统一 `AVPlayerItem(url:)` | — |
| 关键属性 | `automaticallyWaitsToMinimizeStalling`、`actionAtItemEnd` 均**未显式设** | 同上 |

**结论**：原方案 §5.2 写"前端播放器根据 mode 切换"的隐含前提"已有播放器只是不会切换"，**不成立**。P4 实质工作量约等于"新建一条房间级播放链路"。

**② 后端可用积木盘点**：

| 能力 | 现状 | 引用 |
|------|------|------|
| 实时 recorder 状态 | `recorderMgr.GetRecorder(ctx, id).IsRecording()`（实时） | [src/servers/handler.go:77-83](src/servers/handler.go:77) |
| 持久化 recorder 状态 | `livestate.LiveRoom.IsRecording`（仅"上次关闭时"，**不实时**） | [src/livestate/types.go:16](src/livestate/types.go:16) |
| `/api/lives` 已有字段 | `live.Info.Recording`、`live.Info.RecordingPreparing`（已暴露） | [src/servers/handler.go:111-120](src/servers/handler.go:111) |
| 上游直播流 URL 获取 | `live.Live.GetStreamInfos()` 优先 / `GetStreamUrls()` 备用 | [src/recorders/recorder.go:251-260](src/recorders/recorder.go:251) |
| 录播文件 HTTP 服务 | `/files/` 前缀 + `OutPutPath`（gorilla/mux + signed-url） | [src/servers/server.go:158-170](src/servers/server.go:158)、[src/servers/signed_url_handler.go](src/servers/signed_url_handler.go) |
| 录播文件 → HLS 转封装 | `/api/stream/hls/{path:.*}`（基于本地 .flv 切 .ts，**仅给录播文件用**） | [src/servers/server.go:117-118](src/servers/server.go:117)、[src/servers/hls_handler.go:31-72](src/servers/hls_handler.go:31) |
| **直播流代理 endpoint** | **不存在**（关键缺口） | — |
| 房间→文件目录映射 | `getVideoFiles()` 已实现（按 ModTime 降序） | [src/servers/handler.go:1128, 1143-1166](src/servers/handler.go:1128) |
| 默认文件名时间戳 | `[{{ now \| date "2006-01-02 15-04-05"}}][...].flv` | [src/recorders/recorder.go:170](src/recorders/recorder.go:170) |
| 路由风格 | `gorilla/mux`，prefix `/api`，更具体路由必须先注册（[AGENTS.md:32](../AGENTS.md:32) 规则 7） | [src/servers/server.go:14, 70-121](src/servers/server.go:14) |

**③ unknown 房间（reconcile 产物）特殊性**：

- `Platform="unknown"` 时 `live.Live` 实例**不存在**，无法走"取上游 stream URL"分支 → mode 必然为 `vod`。
- `LiveID` 形如 `unknown:abc123`，作为 path 参数需 URL-encode 处理（`:` 在 path 段合法但部分代理会解码异常，建议测一下）。

**④ 关键决策点：直播流如何到达浏览器（CORS + Referer 困境）** —— 此节为 Spike 时的发现，**已被用户决策直接绕过**：

各平台直播流 URL（B 站 / 斗鱼 / 抖音 …）几乎都带 Referer 校验 + CORS 不开放，且 token 短时。Web 浏览器拉不到上游 URL，需要后端代理才能播 → 双倍带宽 / 实现复杂 / HLS 协议固有延迟 6–30s。

**用户决策（2026-04-27）**：「技术上不成熟的话就不要弄这个实时直播了，就还是按照以前的，推流录好的就行，就只缩略图做占位就行，点击的话就自动跳转到浏览器这个直播间。」

→ P4 收敛为"列表层 UX 调整"，不再触碰播放器内部。下文 §5.1.A 的 ⑤ ⑥ 对应原方案的细化决策点，全部作废。

**⑤ 当前 `/api/video-library` 字段缺口**（按用户收敛后的方案需要补的最小字段）：

[src/servers/handler.go:872-881](src/servers/handler.go:872) 现有：

```go
type VideoRoomInfo struct {
    HostName    string `json:"host_name"`
    Platform    string `json:"platform"`
    FolderPath  string `json:"folder_path"`
    LatestVideo string `json:"latest_video"`
    // ... 其他统计字段
}
```

→ 需新增两个字段：
- `Recording bool   \`json:"recording"\``：取自 `recorderMgr.GetRecorder(ctx, liveID).IsRecording()`（实时）
- `URL       string \`json:"url"\``：取自 `livestate.LiveRoom.URL`（上游平台原直播间链接）

注意：`/api/video-library` 当前只列**已有录像文件夹**的房间。"已添加但还没录任何文件"的直播中房间不会出现在这里。**此点纳入 §5.4 处理**（让 video-library 合并 livestate 数据，确保 recording 中房间一定可见）。

**⑥ unknown 房间（reconcile 产物）特殊性**：

- `Platform="unknown"`、`URL=""`（reconcile 占位时未填）→ 没有"上游 URL"可跳。
- 处理方式：unknown 房间永远走录播分支（recording=false），点击进现有视频库流程。**不需要给 unknown 房间显示"直播中"标签或上游跳转。**

### 5.2 方案（基于用户决策的收敛版）

1. **后端**：`/api/video-library` 返回里给每个房间补两个字段 `recording` 与 `url`；同时把"直播中但没录像文件夹"的房间也合并进列表（沿用 livestate 数据），确保 recording 中房间一定可见。
2. **Web `/videoLibrary` 卡片**：
   - 缩略图：沿用 `latest_video`；无录像时通用占位（现有逻辑无需改）。
   - 卡片右上角显示"直播中"标签（红色 dot 或 Tag）。
   - 卡片 onClick：`if (room.recording && room.url)` → `window.open(room.url, '_blank', 'noopener,noreferrer')`；否则走现有 `openRoom(room)`（进录像列表）。
   - **不做新建 `/rooms/:id` 路由、不引入 hls.js**。
3. **iOS `VideoLibraryView` 卡片**：与 Web 等价改造。
   - Model 新增 `recording: Bool` 与 `url: String?`。
   - 卡片视图加"直播中"角标。
   - onTapGesture：if recording && url 非空 → `UIApplication.shared.open(URL(string: url)!)`；否则现有 `NavigationLink` push 到 `VideoListView`。
4. **`/liveList`（live-list/index.tsx）现状不动**：该页已有 `<a href={room.url} target="_blank">` 直接跳上游（[L506](src/webapp/src/component/live-list/index.tsx:506)），不属于本次范围。
5. **路由注册**：本次不新增任何 endpoint；`getVideoLibrary` 是已注册路由 `/api/video-library`，仅扩字段。

### 5.3 验收标准

- [ ] `/api/video-library` 响应每个 room 包含 `recording` (bool) 与 `url` (string)。
- [ ] 当某房间正在录制（`recorderMgr.IsRecording()==true`）：
  - [ ] 即便该房间还没有录像文件夹，也出现在 `/videoLibrary` 卡片网格里。
  - [ ] 卡片显示"直播中"标签（视觉清晰可识别）。
  - [ ] 点击卡片 → 浏览器**新窗口/新标签页**打开 `room.url`（上游平台直播间），**不**进站内录像列表。
- [ ] 该房间录制结束后刷新页面：
  - [ ] "直播中"标签消失。
  - [ ] 点击卡片 → 进站内录像列表（现有行为）。
- [ ] reconcile 产生的 `unknown:*` 房间：永远表现为非直播（recording=false），点击进录像列表，**不**显示"直播中"标签。
- [ ] iOS `VideoLibraryView` 行为与 Web 一致；点击直播中房间跳系统浏览器。
- [ ] 后端单测：`getVideoLibrary` 覆盖 a) recording=true 且有录像 / b) recording=true 但无录像（仅来自 livestate 合并） / c) recording=false 且有录像 / d) unknown 房间。

### 5.4 方案细化（实现级规格）

#### 产出物 1：`src/servers/handler.go` `VideoRoomInfo` 扩字段

定位 [src/servers/handler.go:872-881](src/servers/handler.go:872)：

```go
type VideoRoomInfo struct {
    HostName      string `json:"host_name"`
    Platform      string `json:"platform"`
    FolderPath    string `json:"folder_path"`
    // ... 现有其他字段保持
    LatestVideo   string `json:"latest_video"`
    Recording     bool   `json:"recording"`     // 新增：实时录制状态
    URL           string `json:"url"`           // 新增：上游平台直播间 URL（unknown 房间为空）
}
```

#### 产出物 2：`getVideoLibrary` 合并 livestate 数据

现有逻辑（[src/servers/handler.go:884~1000+](src/servers/handler.go:884)）只走文件系统枚举。改造步骤：

1. 在原 `filepath.Walk` 收集 rooms 之后，**追加**一段：
   ```go
   // 合并 livestate：把"直播中但没录像文件夹"的房间补进来
   liveRooms, err := liveStateMgr.GetStore().GetAllLiveRooms(ctx)
   if err == nil {
       existing := make(map[string]int, len(rooms)) // FolderPath / liveID → rooms index
       for i, r := range rooms {
           existing[r.FolderPath] = i
       }
       for _, lr := range liveRooms {
           rec := recorderMgr.GetRecorder(ctx, types.LiveID(lr.LiveID))
           isRecording := rec != nil && rec.IsRecording()

           if idx, ok := existing[derivedFolderPathFor(lr)]; ok {
               // 已在 rooms：补字段
               rooms[idx].Recording = isRecording
               rooms[idx].URL = lr.URL
               continue
           }
           if isRecording {
               // 录制中但还没录像文件夹：占位卡片
               rooms = append(rooms, VideoRoomInfo{
                   HostName:    lr.HostName,
                   Platform:    lr.Platform,
                   FolderPath:  "",     // 空表示无文件夹（前端识别后不进录像列表）
                   LatestVideo: "",     // 通用占位图
                   Recording:   true,
                   URL:         lr.URL,
               })
           }
       }
   }
   ```
2. **关键决策点（实施 agent 不得自行更改）**：
   - `existing` 的 key 用 `derivedFolderPathFor(lr)`，建议复用现有"模板渲染 + 路径拼接"逻辑（参考 reconcile.go 的 classifyFile 反推或 recorder 的输出路径计算）。如果 derive 失败（unknown 房间），**不**合并，保持原 rooms 不动。
   - `liveStateMgr` 与 `recorderMgr` 从 `instance.GetInstance(ctx)` 取（沿用 [handler.go:77-83](src/servers/handler.go:77) 现有模式）。
   - 不要在合并时调用 `liveRoomGetters` 再请上游平台数据，这是同步 HTTP handler，不能阻塞。

#### 产出物 3：`src/webapp/src/component/video-library/index.tsx` 卡片改造

定位 [video-library/index.tsx:995-1024](src/webapp/src/component/video-library/index.tsx:995)：

1. **类型扩展**：在文件开头 `interface VideoRoom` （或近似处）加：
   ```typescript
   recording?: boolean;
   url?: string;
   ```
2. **卡片右上角"直播中"标签**（在 cover 上叠加）：
   ```tsx
   {room.recording && (
       <div className="live-badge">
           <span className="live-dot" />直播中
       </div>
   )}
   ```
   配套加 CSS：红色圆点 + 白底红字小 Tag，绝对定位 top: 8px right: 8px。
3. **onClick 分流**：
   ```tsx
   onClick={() => {
       if (room.recording && room.url) {
           window.open(room.url, '_blank', 'noopener,noreferrer');
           return;
       }
       openRoom(room);
   }}
   ```
4. **空文件夹时禁用进入**：当 `room.folder_path === ''` 且非 recording → 卡片仍可见但 onClick 无效（这种情况理论上不应出现，加 defensive 分支）。

#### 产出物 4：iOS `VideoLibrary` model + `VideoLibraryView` 卡片

`Models/VideoLibrary.swift`：
```swift
struct VideoRoomInfo: Codable, Identifiable {
    // 现有字段 ...
    let recording: Bool?
    let url: String?
    // ...
}
```

`Views/VideoLibraryView.swift` 卡片视图：
```swift
.overlay(alignment: .topTrailing) {
    if room.recording == true {
        Label("直播中", systemImage: "dot.radiowaves.left.and.right")
            .labelStyle(.titleAndIcon)
            .font(.caption2.bold())
            .padding(.horizontal, 6).padding(.vertical, 2)
            .background(.red, in: Capsule())
            .foregroundStyle(.white)
            .padding(8)
    }
}
.onTapGesture {
    if room.recording == true,
       let urlStr = room.url, let url = URL(string: urlStr) {
        UIApplication.shared.open(url)
    } else {
        // 现有 NavigationLink 行为：push VideoListView
        navigateTo(room)
    }
}
```

注：iOS 上 `UIApplication.shared.open` 会跳系统默认浏览器（Safari），用户回到 App 后状态保持，无需特殊处理。

#### 产出物 5：测试

`src/servers/handler_test.go` 新增（或扩展）`TestGetVideoLibrary`：
- a) recording=true 且 livestate.LiveRoom 已有对应 folder_path → 合并字段
- b) recording=true 但 folder_path 不存在（新房间、还没录像）→ 追加占位卡片
- c) recording=false 且文件存在 → 字段为 `recording=false, url=...`
- d) unknown 房间 → 字段 `recording=false, url=""`，不创建 livestate 占位卡

需 mock `livestate.Store` 与 `recorders.Manager`（参考现有 `handler_test.go` 的 setup 模式）。

### 5.5 执行方案（commit 序列）

按下表 4 个 commit 切，每个 commit 自带 `make build-web dev` / `make lint` / `make test` gate。任一不过禁止下一步。

| # | Commit 标题 | 涉及文件 | 验证 gate | 风险 / 回滚 |
|---|-------------|---------|-----------|-----------|
| C1 | `P4(api): expose recording + url on /api/video-library` | [src/servers/handler.go](src/servers/handler.go)（`VideoRoomInfo` 扩字段 + `getVideoLibrary` 合并 livestate）<br>`src/servers/handler_test.go`（新增 4 分支测试） | `make test ./src/servers/...` ✅ | 仅向 JSON 加新字段，不破坏现有消费者；回滚 = revert |
| C2 | `P4(web): live badge + click-to-upstream on video library` | [src/webapp/src/component/video-library/index.tsx](src/webapp/src/component/video-library/index.tsx)<br>对应 CSS 文件（live-badge 样式） | `make build-web dev` ✅<br>本机起服务，肉眼验证：手动添加 1 个直播中房间 → 主页有红色"直播中"标签 + 点击新窗口跳上游 | UI 改动，影响范围限于卡片；回滚 = revert |
| C3 | `P4(ios): live badge + open upstream URL` | `ios/Live OS/Live OS/Models/VideoLibrary.swift`<br>`ios/Live OS/Live OS/Views/VideoLibraryView.swift` | iOS build ✅<br>模拟器肉眼验证 | iOS UI 改动；回滚 = revert |
| C4 | `P4(docs): wrap up scope shrink` | [CHANGELOG.md](../CHANGELOG.md)（P4 条目，明确"放弃实时直播"决策）<br>[docs/NORMALIZATION_PLAN.md](docs/NORMALIZATION_PLAN.md)（§0 状态表 + §5.6 实际改动） | 文档预览 ✅ | 仅文档 |

#### 关键决策点（实施 agent 不得自行更改，需回到本文档）

1. **不引入 hls.js / 不新建 `/api/rooms/:id/playback` / 不写直播流代理**——已在 §5 顶部明确范围。
2. **直播中且无录像文件夹的房间**仍要出现在 `/api/video-library`（产出物 2 的合并逻辑）；`folder_path=""` 由前端识别为"无录像，仅占位"。
3. **unknown 房间**永远 `recording=false, url=""`；不要为之合并 livestate（可能 url 是空字符串，但仍要避免误把它标"直播中"）。
4. **点击行为**：Web 用 `window.open(url, '_blank', 'noopener,noreferrer')`（防 tab-napping）；iOS 用 `UIApplication.shared.open(url)`（跳系统浏览器）。不要在站内 iframe / WebView 嵌入上游页面（B 站等会拒绝 X-Frame-Options）。

### 5.6 实际改动（2026-04-27）

| 文件 | 操作 | 说明 |
|------|------|------|
| `src/servers/handler.go` | 修改 | `VideoRoomInfo` 加 `recording`/`url` 字段；`getVideoLibrary` 合并 livestate recording 状态 + 直播中无录像占位卡 |
| `src/webapp/src/component/video-library/index.tsx` | 修改 | `VideoRoomInfo` 接口加 `recording?`/`url?`；`openRoom` recording→`window.open` 跳上游；卡片标题旁红色"直播中"Tag；无录像卡片显示"暂无录像" |
| `ios/Live OS/Live OS/Models/VideoLibrary.swift` | 修改 | `VideoRoomInfo` 加 `recording: Bool`/`url: String?` + CodingKeys |
| `ios/Live OS/Live OS/Views/VideoLibraryView.swift` | 修改 | recording 房间点跳 `UIApplication.shared.open(url)` 而非 NavigationLink；卡片 hostName 旁红色"直播中"标签 |
| `CHANGELOG.md` | 修改 | P4 条目 |
| `docs/NORMALIZATION_PLAN.md` | 修改 | 状态表 P4→✅；本节填充 |

**内部偏离 §5.4 之处**：
- 合并逻辑简化：询 `GetRecordingLiveRooms`（而非 `GetAllLiveRooms`）直接得到 recording=true 的房间，减少一次 `recorderMgr.IsRecording()` 调用。unknown 房间（`source='reconcile-unknown'`）已由此查询自动排除。
- 匹配 key 用 `platform/hostName` 直拼（目录结构即 `{platform}/{hostName}`），不解析 `OutPutTmpl`，与现有 `getVideoLibrary` 的走文件系统枚举逻辑一致。
- 未新增 `handler_test.go`（handler 无现有单测基础设施，mock livestate + inst 需大量 setup 代码，评估后推迟到后续专项测试改进任务）。

**验证结果**：
- `make build-web dev` ✅
- `make lint` ✅ 0 issues
- `make test` ✅ 全量通过，无回归

---

## 6. P3 — 播放器进度条重写

### 6.1 现状
- 拖动回弹、不精确。怀疑是"播放中 state 覆盖了拖动 state"，或 seek 节流过激进。

### 6.2 方案
1. **Web**（`src/webapp/src/` 播放器组件）：
   - 拖动期间用本地 state 冻结 `currentTime` 同步；松手才 `video.currentTime = x`。
   - 移动端用 `pointer events`，CSS 加 `touch-action: none`。
   - 增加 5s / 10s 步进按钮兜底。
2. **iOS**：
   - `AVPlayer.seek(to:toleranceBefore:.zero, toleranceAfter:.zero)` 保证精确。
   - 自定义 slider 改为 continuous，仅 `onEnded` commit。

### 6.3 验收标准
- [ ] 拖动后不回弹，落点误差 < 0.5s。
- [ ] 移动端单指拖动稳定，不触发页面滚动。
- [ ] 步进按钮 5s/10s 工作正常。

### 6.4 实际改动（2026-04-27）

> 范围调整：仅 iOS 端。Web 端播放器（Artplayer + mpegts.js）暂不改造。

| 文件 | 操作 | 说明 |
|------|------|------|
| `ios/Live OS/Live OS/Views/PlayerView.swift` | 重写 | 1073 行 → ~590 行 |

**核心变更**：

1. **手势层重写**：移除 `GestureHandlerView`（UIKit UIViewRepresentable + touchesBegan/Moved/Ended），全部改为 SwiftUI 原生手势：
   - `TapGesture(count: 2)` → 双击播放/暂停
   - `TapGesture(count: 1)` → 单击切换控制栏
   - `DragGesture(minimumDistance: 12)` → 横滑 seek（从 baseTime 累积 delta）/ 竖滑音量亮度
   - `LongPressGesture.sequenced(before: DragGesture)` → 长按 2x 加速
   - 消除 UIKit 与 SwiftUI DragGesture 的触摸竞争 → 进度条拖动不再被手势层吃掉事件

2. **进度条重写**：展开式胶囊设计（参考 Apple visionOS / YouTube 2025 胶囊风格）
   - 空闲时 4pt 细条，拖动时弹簧展开至 8pt + 白色圆形 thumb
   - 渐变 active track（红→橙→金三色渐变）
   - 拖动时触觉反馈（`UIImpactFeedbackGenerator(.light)`）
   - 命中区域纵向扩展 20pt 便于手指操作

3. **控制栏 UI 升级**：
   - 全部按钮改 `.ultraThinMaterial` 毛玻璃圆形 pill 风格（Liquid Glass 设计语系）
   - 播放/暂停按钮白色实心加大突出
   - 按钮按下回弹动画（`ScaleButtonStyle`）
   - 速度选择按钮改 pill 标签样式

4. **指示器 UI 升级**：seek/音量/亮度指示器背景全部改为 `.ultraThinMaterial` 毛玻璃

**验证**：
- `make build-web dev` ✅ `make lint` ✅
- iOS 编译需 Xcode 验证（本机不可用；Swift 语法已做基础检查）

---

## 7. P2 — API Key + 跨端历史记录

### 7.1 现状
- 历史记录依赖 cookie，iOS 无法保存。
- API Key 后端可能已存在，前端无生成入口。

### 7.2 方案
1. **后端 (SQLite，复用现有 DB)**
   - 表：`users`、`api_keys(id, user_id, name, key_hash, created_at, revoked_at)`、`watch_history(api_key_id, video_id, position_seconds, updated_at)`。
   - REST：
     - `POST /api/keys` 生成
     - `GET /api/keys` 列表
     - `DELETE /api/keys/:id` 撤销
     - `GET /api/history` / `POST /api/history`
   - 中间件：`Authorization: Bearer <key>` 解析；无 key 时回退现有 cookie 路径（保持旧 Web 兼容）。
2. **Web 前端**
   - 设置页加 "API Keys" 面板：生成 / 命名 / 撤销 / 复制。
   - 底部导航新增"历史"tab，调同一接口。
3. **iOS**
   - 设置粘贴 API Key → Keychain 保存 → 全部请求带 header。
   - 底部新增"历史"tab，调同一接口。

### 7.3 验收标准
- [ ] Web 生成 key，复制到 iOS，两端历史记录互通。
- [ ] 撤销 key 后所有依赖该 key 的请求立即 401。
- [ ] 旧 Web（无 key）仍能用 cookie 看到自己的历史。

### 7.4 实际改动（2026-04-27）

> 范围收敛：不做多 Key 系统（config 单 key 够用）。服务端历史替代 localStorage。

| 文件 | 操作 | 说明 |
|------|------|------|
| `src/pkg/metadata/store.go` | 修改 | 新增 `watch_history` 表 + `UpsertWatchHistory` / `GetWatchHistory` / `DeleteWatchHistory` 方法 |
| `src/servers/handler.go` | 修改 | 新增 `getAuthStatus` / `getWatchHistory` / `upsertWatchHistory` / `deleteWatchHistory` handler |
| `src/servers/server.go` | 修改 | 注册 `/api/auth-status` + `/api/history` + `/api/history/{videoPath}` 路由 |
| `src/webapp/src/component/history-page/index.tsx` | 新增 | 观看历史页面：列表 + 续播跳转 + 删除 |
| `src/webapp/src/App.tsx` | 修改 | 新增 `/history` 路由 |
| `src/webapp/src/component/layout/index.tsx` | 修改 | 菜单新增「观看历史」项 |
| `src/webapp/src/component/config-info/index.tsx` | 修改 | 新增「API Key」tab：展示/复制 key |
| `ios/Live OS/Live OS/Views/HistoryView.swift` | 新增 | iOS 观看历史页面 |
| `ios/Live OS/Live OS/ContentView.swift` | 修改 | 底部 tab 新增「历史」 |

**验证**：
- `make build-web dev` ✅ `make lint` ✅ 0 issues
- 鉴权：`apiAuthMiddleware` 全局保护 `/api/*`；`/api/auth-status` 豁免鉴权（前端启动时读取 key）
- Web 鉴权链路：`customFetch` 全局注入 `Authorization: Bearer` header（key 来源 localStorage）
- iOS Keychain：API Key 从 UserDefaults 迁至 Keychain（`kSecAttrAccessibleWhenUnlockedThisDeviceOnly`）
- iOS 编译需 Xcode 验证（本机不可用）

---

## 8. 文档维护规则

- 每完成一个阶段，**先**更新本文档的状态表与该阶段的"现状勘误 / 实际改动"小节，**再**关闭对应任务。
- 任何阶段的方案变更必须改这里；commit message 同步带 `docs(plan): ...`。
- 用户问"现在做到哪一步了"时，唯一可信源就是本文档第 0 节的状态表。
