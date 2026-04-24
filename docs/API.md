# Bililive-go API

## `GET /api/info` Get app info
- Request:
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/info
    ```
- Response:
    ```json
    {
      "app_name": "BiliLive-go",
      "app_version": "0.5.0-rc.3-3-g31ceeda",
      "build_time": "2020-05-05_01:07:16",
      "git_hash": "31ceeda8f508ba5546cfdefef5f3945828a87651",
      "pid": 33295,
      "platform": "darwin/amd64",
      "go_version": "go1.14.2"
    }
    ```
        
## `GET /api/lives` Get all live info 
- Request:  
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/lives
    ```
- Response:   
    ```json
    [
      {
        "id": "212d9c98c7b376b730d4336bb49f6d3f",
        "live_url": "https://live.bilibili.com/14917277",
        "platform_cn_name": "哔哩哔哩",
        "host_name": "湊-阿库娅Official",
        "room_name": "【B站限定】棉花糖＆唱歌！！！！",
        "status": false,
        "listening": true,
        "recording": false
      },
      {
        "id": "63dc965c77d3d81058c92c3e38822256",
        "live_url": "https://live.bilibili.com/11588230",
        "platform_cn_name": "哔哩哔哩",
        "host_name": "白上吹雪Official",
        "room_name": "古老niconico老人会with☆乐园",
        "status": false,
        "listening": true,
        "recording": false
      },
      {
        "id": "dfb964a56725bbad165cb9ea1ef8ac5b",
        "live_url": "https://live.bilibili.com/1030",
        "platform_cn_name": "哔哩哔哩",
        "host_name": "怕上火暴王老菊",
        "room_name": "直播做饭",
        "status": false,
        "listening": true,
        "recording": false
      }
    ]
    ```
        
## `GET /api/lives/{id}` Get live info by id
- Request:  
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/lives/212d9c98c7b376b730d4336bb49f6d3f
    ```
- Response:
    ```json
    {
      "id": "212d9c98c7b376b730d4336bb49f6d3f",
      "live_url": "https://live.bilibili.com/14917277",
      "platform_cn_name": "哔哩哔哩",
      "host_name": "湊-阿库娅Official",
      "room_name": "【B站限定】棉花糖＆唱歌！！！！",
      "status": false,
      "listening": true,
      "recording": false
    }
    ```
        
## `POST /api/lives` Add live
- Request:  
    ```text
    method: POST
    path: http://127.0.0.1:8080/api/lives
    body: 
        [
            {
                "url": "https://live.bilibili.com/14917277",
                "listen": true
            }
        ]
    ```
- Response:
    ```json
    [
        {
            "id": "212d9c98c7b376b730d4336bb49f6d3f",
            "live_url": "https://live.bilibili.com/14917277",
            "platform_cn_name": "哔哩哔哩",
            "host_name": "湊-阿库娅Official",
            "room_name": "【B站限定】棉花糖＆唱歌！！！！",
            "status": false,
            "listening": true,
            "recording": false
        }
    ]
    ```        
        
## `DELETE /api/lives/{id}` Delete live by id
- Request:  
    ```text
    method: DELETE
    path: http://127.0.0.1:8080/api/lives/212d9c98c7b376b730d4336bb49f6d3f
    ```
- Response:
    ```json
    {
        "err_no": 0,
        "err_msg": "",
        "data": "OK"
    }
    ```

## `GET /api/lives/{id}/start` Start listen live by id
- Request:  
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/lives/212d9c98c7b376b730d4336bb49f6d3f/start
    ```
- Response:
    ```json
    {
        "id": "212d9c98c7b376b730d4336bb49f6d3f",
        "live_url": "https://live.bilibili.com/14917277",
        "platform_cn_name": "哔哩哔哩",
        "host_name": "湊-阿库娅Official",
        "room_name": "【B站限定】棉花糖＆唱歌！！！！",
        "status": false,
        "listening": true,
        "recording": false
    }
    ```
        
## `GET /api/lives/{id}/stop` Stop listen and record live by id
- Request:  
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/lives/212d9c98c7b376b730d4336bb49f6d3f/stop
    ```
- Response:
    ```json
    {
        "id": "212d9c98c7b376b730d4336bb49f6d3f",
        "live_url": "https://live.bilibili.com/14917277",
        "platform_cn_name": "哔哩哔哩",
        "host_name": "湊-阿库娅Official",
        "room_name": "【B站限定】棉花糖＆唱歌！！！！",
        "status": false,
        "listening": false,
        "recording": false
    }
    ```
        
## `GET /api/config` Get config info
- Request:  
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/config
    ```
- Response:
    ```json
    {
      "RPC": {
        "Enable": true,
        "Bind": "127.0.0.1:8080"
      },
      "Debug": false,
      "Interval": 15,
      "OutPutPath": "/tmp",
      "Feature": {
        "UseNativeFlvParser": false
      },
      "LiveRooms": null
    }
    ```
        
## `PUT /api/config` Save lives info to config file
- Request:  
    ```text
    method: PUT
    path: http://127.0.0.1:8080/api/config
    ```
- Response:
    ```json
    {
        "err_no": 0,
        "err_msg": "",
        "data": "OK"
    }
    ```

## `GET /api/raw-config` Get raw config file
- Request:
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/raw-config
    ```
- Response:
    ```json
    {
        "config": "rpc:\n  enable: true\n  bind: 0.0.0.0:8080\ndebug: false\ninterval: 15\nout_put_path: ./\nfeature:\n  use_native_flv_parser: false\nlive_rooms:\n- url: https://www.huya.com/991111\n  is_listening: false\nout_put_tmpl: \"\"\nvideo_split_strategies:\n  on_room_name_changed: false\n  max_duration: 0s\ncookies:\n  live.douyin.com: name1=qwer;name2=asdf;aaaa\non_record_finished:\n  convert_to_mp4: true\n  delete_flv_after_convert: false\ntimeout_in_us: 50000000\n"
    }
    ```

## `PUT /api/raw-config` Save the whole config file
- Request:
    ```text
    method: PUT
    path: http://127.0.0.1:8080/api/raw-config
    body:
        {
            "config": "rpc:\n  enable: true\n  bind: 0.0.0.0:8080\ndebug: false\ninterval: 15\nout_put_path: ./\nfeature:\n  use_native_flv_parser: false\nlive_rooms:\n- url: https://www.huya.com/991111\n  is_listening: false\nout_put_tmpl: \"\"\nvideo_split_strategies:\n  on_room_name_changed: false\n  max_duration: 0s\ncookies:\n  live.douyin.com: name1=qwer;name2=asdf;aaaa\non_record_finished:\n  convert_to_mp4: true\n  delete_flv_after_convert: false\ntimeout_in_us: 50000000\n"
        }
    ```
- Response:
    ```json
    {
        "err_no": 0,
        "err_msg": "",
        "data": "OK"
    }
    ```
---

## 鉴权（API Key Auth）

所有 API 端点均支持 API Key 鉴权。启用后，每个请求需携带以下之一：

- `Authorization: Bearer <api_key>` 请求头
- `X-API-Key: <api_key>` 请求头

**配置**（`config.yml`）：

```yaml
security:
  enable_api_key: false      # 是否启用鉴权（默认关闭，保持 Web UI 兼容）
  api_key: ""                # 空时启动自动生成 32 字节随机串并写回配置
  signed_url_ttl_seconds: 3600
```

**跳过鉴权**（开发用）：设置环境变量 `BILILIVE_DISABLE_API_AUTH=1`。

---

## `GET /api/signed-url` 生成签名 URL

为 `/files/*`、`/api/thumbnail/*`、`/api/stream/hls/*` 生成带 HMAC 签名的临时 URL，供无法附加自定义 Header 的客户端（`AVPlayer`、`AsyncImage` 等）使用。

- Request:
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/signed-url
    query:
      kind    - 必填，枚举值：file | thumbnail | hls
      path    - 必填，相对于输出目录的路径
      expires_in - 可选，有效期秒数（默认 3600，最大 86400）
    ```
- Response:
    ```json
    {
        "err_no": 0,
        "err_msg": "",
        "data": {
            "url": "/files/some/video.mp4?sig=...&expires=1714000000",
            "expires": 1714000000,
            "expires_in": 3600
        }
    }
    ```

---

## `GET /api/stream/hls/{path}` 录播文件 HLS 转封装

将 FLV / TS / MKV 等格式的录播文件按需转封装为 HLS（`.m3u8` + `.ts` 分片），供 `AVPlayer` 播放。首次请求会调用 ffmpeg 生成缓存，后续请求秒开。

- Request:
    ```text
    method: GET
    path: http://127.0.0.1:8080/api/stream/hls/<相对路径>
    example: http://127.0.0.1:8080/api/stream/hls/主播名/2026-04-22_直播标题.flv
    ```
- Response: `Content-Type: application/vnd.apple.mpegurl`，返回 HLS playlist（`.m3u8`）。

---

## `GET /api/stream/hls-segment/{cache_key}/{segment}` HLS 分片

HLS playlist 内部引用的分片下载端点，由客户端自动调用，无需手动构造。

