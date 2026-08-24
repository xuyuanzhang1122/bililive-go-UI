# bililive-go-UI curl 命令速查

本文基于当前项目 `config.yml` / `config.docker.yml` 配置定制，覆盖所有主要 API。

> **使用前替换以下变量：**
> - `BASE=http://127.0.0.1:8080` → 改为实际服务地址
> - `KEY=your-api-key` → 改为实际 API Key（未启用鉴权时可省略 `-H "Authorization: Bearer $KEY"` 一行）

```bash
BASE=http://127.0.0.1:8080
KEY=your-api-key
AUTH='-H "Authorization: Bearer '"$KEY"'"'
```

---

## 一、安装与部署

### 交互式安装（推荐）
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh | bash
```

### 非交互安装 · 二进制模式（全默认）
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh \
  | bash -s -- --yes --dir ~/bililive-go --port 8080 --enable-api-key
```

### 非交互安装 · 指定目录和端口
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh \
  | bash -s -- --yes \
    --dir /opt/bililive \
    --port 9090 \
    --enable-api-key \
    --api-key "my-custom-key-here"
```

### 非交互安装 · Docker 模式
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh \
  | bash -s -- --docker --yes --dir ~/bililive-go --port 8080 --enable-api-key
```

### 非交互安装 · 指定版本
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh \
  | bash -s -- --yes --version v1.3.5 --dir ~/bililive-go --port 8080
```

### 就地更新现有安装
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/install.sh \
  | bash -s -- --update
```

更新脚本会保留现有配置：二进制模式会备份旧程序并重启已有 systemd 服务；Docker 模式会从现有容器反查端口和挂载后重建。也可显式加 `--binary`、`--docker`、`--dir PATH`、`--version TAG` 或 `--image TAG`。

### 修复 v2.0.2 及之前版本遗留的 HLS 缓存
```bash
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/fix-hls-cache.sh \
  | bash -s -- --dry-run

# 确认统计无误后执行清理
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/scripts/fix-hls-cache.sh \
  | bash -s -- --yes
```

### 手动下载 Docker 配置模板
```bash
# Docker Compose 配置
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/docker-compose.yml \
  -o docker-compose.yml

# Docker 运行配置（config.docker.yml）
curl -fsSL https://raw.githubusercontent.com/xuyuanzhang1122/bililive-go-UI/main/config.docker.yml \
  -o config.docker.yml
```

---

## 二、服务状态与认证

### 检查服务健康 / 鉴权状态
```bash
curl -s $BASE/api/auth-status | python3 -m json.tool
```

### 检查当前 API Key 对应的用户信息
```bash
curl -s $BASE/api/auth/me \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 获取服务器基本信息
```bash
curl -s $BASE/api/info | python3 -m json.tool
```

---

## 三、配置管理

### 获取当前配置
```bash
curl -s $BASE/api/config \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 修改配置（PATCH 局部更新）
```bash
# 修改端口和输出目录
curl -s -X PATCH $BASE/api/config \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "rpc": { "bind": ":8080" },
    "out_put_path": "/srv/bililive"
  }' | python3 -m json.tool
```

### 一键启用 API Key 并自动生成
```bash
curl -s -X PATCH $BASE/api/config \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"security": {"enable_api_key": true}}' | python3 -m json.tool
```

### 配置无头浏览器路径
```bash
# 查看当前无头浏览器配置
curl -s $BASE/api/config/headless-browser \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool

# 设置自定义路径（留空则自动检测）
curl -s -X PATCH $BASE/api/config/headless-browser \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/opt/bililive-tools/chromium",
    "auto_install": true,
    "timeout_seconds": 15
  }' | python3 -m json.tool

# 探测/检测无头浏览器是否可用
curl -s -X POST $BASE/api/tools/headless-browser/probe \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 配置抖音 Cookie
```bash
# 查看是否已配置（不返回完整 Cookie 内容）
curl -s $BASE/api/config/douyin-cookie \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool

# 设置抖音 Cookie
curl -s -X PUT $BASE/api/config/douyin-cookie \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"cookie": "sessionid=xxx; __ac_nonce=yyy"}' | python3 -m json.tool
```

---

## 四、API Key 多用户管理

### 列出所有 Key 用户
```bash
curl -s $BASE/api/api-keys \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 创建新 Key 用户（如为 iOS App 单独创建）
```bash
curl -s -X POST $BASE/api/api-keys \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "iPhone 15 Pro"}' | python3 -m json.tool
# 响应中包含完整 api_key，只展示一次，请立即复制
```

### 重命名 / 启用 / 禁用 Key 用户
```bash
# user_id 从列表接口获取
curl -s -X PATCH $BASE/api/api-keys/{user_id} \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "iPad", "enabled": true}' | python3 -m json.tool
```

### 吊销 Key 用户
```bash
curl -s -X DELETE $BASE/api/api-keys/{user_id} \
  -H "Authorization: Bearer $KEY"
```

---

## 五、直播间管理

### 获取直播间列表
```bash
curl -s $BASE/api/lives \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 添加直播间（粘贴直播间 URL 或抖音分享文案）
```bash
curl -s -X POST $BASE/api/lives \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://live.douyin.com/810339218646"}' | python3 -m json.tool
```

### 解析短链 / 分享文案为标准直播间 URL
```bash
curl -s "$BASE/api/resolve-url?url=https%3A%2F%2Fv.douyin.com%2FiXXXXXX%2F" \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 开始监听直播间
```bash
curl -s -X PUT $BASE/api/lives/{room_id}/start \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 停止监听直播间
```bash
curl -s -X PUT $BASE/api/lives/{room_id}/stop \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 删除直播间（同时删除录制目录）
```bash
curl -s -X DELETE "$BASE/api/lives/{room_id}?delete_files=true" \
  -H "Authorization: Bearer $KEY"
```

### 删除直播间（仅删除直播间记录，保留视频文件）
```bash
curl -s -X DELETE $BASE/api/lives/{room_id} \
  -H "Authorization: Bearer $KEY"
```

---

## 六、视频库与文件管理

### 获取视频库列表（按主播聚合）
```bash
curl -s $BASE/api/video-library \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 获取某主播的录制文件列表
```bash
# folder_path = 平台/主播名，需要 URL encode
curl -s "$BASE/api/video-files/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%E5%90%8D" \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 删除单个视频文件
```bash
# file_path = 相对路径，需要 URL encode
curl -s -X DELETE "$BASE/api/files/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%2Fxxx.flv" \
  -H "Authorization: Bearer $KEY"
```

### 获取视频缩略图
```bash
# 直接在浏览器打开或用 curl 下载
curl -s "$BASE/api/thumbnail/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%2Fxxx.flv" \
  -H "Authorization: Bearer $KEY" \
  -o thumbnail.jpg
```

### 获取 HLS 播放流地址（FLV/TS/MKV 转 HLS）
```bash
# 返回 m3u8 播放列表
curl -s "$BASE/api/stream/hls/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%2Fxxx.flv" \
  -H "Authorization: Bearer $KEY"
```

---

## 七、观看历史（服务端同步）

### 获取当前用户观看历史
```bash
curl -s $BASE/api/history \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 查询单个视频的续播进度
```bash
# video_path = 视频相对路径，逐段 URL encode
curl -s "$BASE/api/history/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%2Fxxx.flv" \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 保存 / 更新播放进度
```bash
curl -s -X POST $BASE/api/history \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "video_path": "抖音/主播/xxx.flv",
    "video_name": "xxx.flv",
    "position_seconds": 95.2,
    "duration_seconds": 600.0
  }' | python3 -m json.tool
```

### 删除单条历史记录
```bash
curl -s -X DELETE "$BASE/api/history/%E6%8A%96%E9%9F%B3%2F%E4%B8%BB%E6%92%AD%2Fxxx.flv" \
  -H "Authorization: Bearer $KEY"
```

---

## 八、配置备份与恢复（iOS 协作接口）

### 上传配置备份到主服务（本机）
```bash
curl -s -X POST $BASE/api/backups \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "schemaVersion": 1,
    "exportedAt": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",
    "iosConfig": {
      "serverURL": "http://192.168.1.10:8080",
      "lanURL": "http://192.168.1.10:8080",
      "publicURL": "https://your-domain.com",
      "autoSwitchNetwork": true
    },
    "server": {
      "rpc_bind": ":8080",
      "out_put_path": "/srv/bililive",
      "app_data_path": "/var/lib/bililive",
      "live_rooms": []
    }
  }' | python3 -m json.tool
```

### 根据备份 ID 查找备份
```bash
curl -s $BASE/api/backups/{backup_id} \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

### 触发本机配置恢复
```bash
curl -s -X POST $BASE/api/backups/restore \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "backup_id": "bgo_20260505_abcd1234",
    "restart": true
  }' | python3 -m json.tool
```

### 查询恢复任务状态
```bash
curl -s "$BASE/api/backups/restore/status/{job_id}" \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

---

## 九、本机 Doctor / 重启

### 本机环境检测（Doctor）
```bash
curl -s -X POST $BASE/api/local/doctor \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "os": "linux",
    "arch": "amd64",
    "install_mode": "binary",
    "port": 8080,
    "paths": {
      "output_path": "/srv/bililive",
      "app_data_path": "/var/lib/bililive"
    },
    "tools": [
      {"id": "ffmpeg", "path": "/usr/bin/ffmpeg", "version": "6.1", "ok": true},
      {"id": "headless-browser", "path": "", "ok": false}
    ]
  }' | python3 -m json.tool
```

### 重启服务（配置恢复后使用）
```bash
curl -s -X POST $BASE/api/local/restart \
  -H "Authorization: Bearer $KEY" | python3 -m json.tool
```

---

## 十、Docker 容器常用操作

```bash
# 查看容器状态
docker ps | grep bililive-go

# 查看实时日志
docker logs -f bililive-go

# 查看最后 100 行日志
docker logs --tail 100 bililive-go

# 重启容器
docker restart bililive-go

# 停止容器
docker stop bililive-go

# 升级到最新版本
docker pull xuniubi/bililive-go:latest
docker rm -f bililive-go
# 然后重新执行 docker run 命令（参数与初次安装相同）

# 指定版本升级
docker pull xuniubi/bililive-go:v1.3.5
BILILIVE_IMAGE=xuniubi/bililive-go:v1.3.5 docker compose up -d

# 进入容器内部排查
docker exec -it bililive-go sh

# 查看容器内配置文件
docker exec bililive-go cat /etc/bililive-go/config.yml
```

---

## 十一、systemd 服务管理（二进制模式 Linux）

```bash
# 查看服务状态
systemctl status bililive-go

# 启动
systemctl start bililive-go

# 停止
systemctl stop bililive-go

# 重启
systemctl restart bililive-go

# 开机自启
systemctl enable bililive-go

# 查看实时日志
journalctl -u bililive-go -f

# 查看最近 100 条日志
journalctl -u bililive-go -n 100
```

---

## 十二、常用调试

### 测试服务是否可访问
```bash
curl -fsS --max-time 5 $BASE/api/auth-status && echo "服务正常" || echo "服务不可达"
```

### 格式化任意接口响应（需要 jq）
```bash
curl -s $BASE/api/lives -H "Authorization: Bearer $KEY" | jq .
```

### 批量检测录制状态
```bash
curl -s $BASE/api/lives -H "Authorization: Bearer $KEY" \
  | python3 -c "
import sys, json
data = json.load(sys.stdin)
for room in data:
    print(f\"{room.get('platform','?')} | {room.get('host_name','?')} | recording={room.get('recording',False)}\")"
```
