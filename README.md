# bililive-go 优化版
[![CI](https://github.com/xuyuanzhang1122/bililive-go/actions/workflows/tests.yaml/badge.svg?branch=master)](https://github.com/xuyuanzhang1122/bililive-go/actions/workflows/tests.yaml)
[![GitHub Release](https://img.shields.io/github/v/release/xuyuanzhang1122/bililive-go)](https://github.com/xuyuanzhang1122/bililive-go/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/xuniubi/bililive-go.svg)](https://hub.docker.com/r/xuniubi/bililive-go)

这是一个基于原版 `bililive-go` 深度优化的直播录制工具，保留多平台录播能力，并补齐了视频管理、播放器体验和 Docker 交付链路。

## 功能预览

### 视频库首页

![视频库首页](img/%E6%88%AA%E5%B1%8F2026-03-09%2017.13.50.png)

### 视频列表与缩略图

![视频列表与缩略图](img/%E6%88%AA%E5%B1%8F2026-03-09%2017.13.59.png)

### 内嵌播放器

![内嵌播放器](img/%E6%88%AA%E5%B1%8F2026-03-09%2017.14.21.png)

### 管理与配置界面

![管理与配置界面](img/%E6%88%AA%E5%B1%8F2026-03-09%2017.14.27.png)

## 当前版本重点

- 新增视频库首页，按平台和主播聚合展示录播内容
- 自动生成视频缩略图，缩略图按需缓存到 `.appdata/thumbnails/`
- 新增内嵌播放器，支持 FLV、TS、MP4、MKV、MOV
- 支持续播、移动端手势、全屏、快进快退和自定义控制栏
- 支持直接从视频库继续观看上次播放的视频
- 支持删除直播间时一并删除对应录播目录
- 支持文件列表单个删除和批量删除
- 优化抖音分享文案和短链解析，减少错误识别 `webcast` 地址的情况

详细改动见 [CHANGELOG.md](CHANGELOG.md)。

## 支持平台

常用平台包括：

- 抖音直播
- 哔哩哔哩直播
- 斗鱼直播
- 虎牙直播
- Acfun
- CC
- OPENREC
- 猫耳
- 微博直播
- YY

完整支持情况以代码实现为准。

## 界面与管理能力

### 视频库

- 首页展示每位主播的视频数量、总大小、最新录制时间
- 卡片封面直接使用自动生成的缩略图
- 支持从视频库页面直接添加直播间监控
- 顶部提供继续观看横幅

### 播放器

- 内嵌全屏播放器，无需跳转外部页面
- 自动保存播放进度，下次打开同一文件可直接续播
- 移动端支持滑动快进快退、长按倍速、单击显隐控制栏、双击播放暂停

### 删除与整理

- 删除直播间时可同步删除该主播的全部录制视频
- 文件列表支持单个删除和批量删除
- 转码、封面提取、上传后删除等录制后处理能力继续保留

## Docker 使用

### 直接拉取镜像

当前默认镜像仓库：

- Docker Hub: `xuniubi/bililive-go`
- 当前 compose 示例标签：`myfix-20260221`

```bash
docker run -d \
  --name bililive-go \
  --restart unless-stopped \
  -p 8080:8080 \
  -v $(pwd)/Videos:/srv/bililive \
  -v $(pwd)/config.docker.yml:/etc/bililive-go/config.yml \
  xuniubi/bililive-go:myfix-20260221
```

程序默认监听 `8080` 端口，录制文件输出到容器内 `/srv/bililive`。

### 使用 docker compose

仓库根目录已提供 [docker-compose.yml](docker-compose.yml)。默认使用：

- 镜像：`xuniubi/bililive-go:myfix-20260221`
- 配置文件：`config.docker.yml`
- 输出目录：`./Videos`

```bash
docker compose up -d
```

如需切换镜像标签，可以在启动前设置环境变量：

```bash
BILILIVE_IMAGE=xuniubi/bililive-go:myfix-20260221 docker compose up -d
```


## Docker 构建方式

### 方式一：基于当前源码构建镜像

主 [Dockerfile](Dockerfile) 已改为直接从当前仓库源码构建，不再下载 upstream 的 release 二进制。

```bash
docker build \
  --build-arg VERSION=myfix-local \
  -t xuniubi/bililive-go:myfix-local \
  .
```

### 方式二：基于本地预编译二进制构建镜像

如果你已经在仓库根目录准备好了 `bililive-linux-amd64`，可以使用 [Dockerfile.local](Dockerfile.local)：

```bash
docker build --platform linux/amd64 -f Dockerfile.local -t xuniubi/bililive-go:local .
```

这个方式默认面向你现在仓库里的 `bililive-linux-amd64` 成品二进制，适合已经单独编译好 Linux AMD64 版本、只想快速封装镜像的场景。

## 本地开发与构建

### 前置要求

| 工具 | 版本要求 |
|------|----------|
| Go | 1.25+ |
| Node.js | 20.x |
| GNU Make | 4.0+ |
| Git | 任意稳定版本 |

### 克隆仓库

```bash
git clone https://github.com/xuyuanzhang1122/bililive-go.git
cd bililive-go
```

### 常用命令

```bash
# 构建前端 + 后端开发版
make build-web dev

# 仅构建后端开发版
make dev

# 前端构建
make build-web

# 单元测试
make test

# 代码检查
make lint
```

如果要交叉编译 Linux 二进制：

```bash
make dev PLATFORM=linux ARCH=amd64
```

生成物默认位于 `bin/` 目录，例如 `bin/bililive-linux-amd64`。

## 配置说明

### Docker 默认配置

[config.docker.yml](config.docker.yml) 适用于容器内运行，默认内容包括：

- 服务监听地址：`:8080`
- 录制输出目录：`/srv/bililive`
- 应用数据目录：`/srv/bililive/.appdata`
- 内置工具目录：`/opt/bililive/tools`

### cookie 示例

```yaml
cookies:
  live.douyin.com: __ac_nonce=xxx;name=value
```

## 相关文档

- [常见问题](docs/FAQ.md)
- [通知服务说明](docs/notify.md)
- [API 文档](docs/API.md)
- [项目完整说明 / 改动文档](docs/PROJECT.md)
- [测试与 VSCode 调试](test/README.md)

## 参考项目

- [原版 bililive-go](https://github.com/bililive-go/bililive-go)
- [you-get](https://github.com/soimort/you-get)
- [ykdl](https://github.com/zhangn1985/ykdl)
- [youtube-dl](https://github.com/ytdl-org/youtube-dl)
