# JIT HLS 优化实施方案

## 概述

本方案旨在解决录播首帧加载慢的问题。当前用户点播放时需要等待整个文件转封装完成（可能 30 分钟），改为首个分片就绪即可开始播放（预计 6-12 秒）。

**目标**：
- 首帧加载从"分钟级"降到"秒级"
- 双端（iOS + 鸿蒙）兼容性不降低
- 磁盘占用更可控
- 不引入新的外部依赖

**改动范围**：
- 服务端 Go 代码：4 个文件，~76 行增改
- 配置文件：1 处默认值修改
- 客户端：无需改动（协议保持兼容）

---

## 一、改动内容

### 1.1 已实现的改动（feature/jit-hls-optimization 分支）

| 文件 | 改动 | 影响 |
|---|---|---|
| `media_probe.go` | 移除 ffprobe 的 `format=duration` 请求 | FLV 探测时不再全文件扫描，节省 30s-2min |
| `hls_handler.go` | 新增 `isPlaylistReady()` 函数 | 检查是否有至少一个分片（`#EXTINF`） |
| `playback_resolver.go` | ready 条件从 `isPlaylistComplete` 改为 `isPlaylistReady` | 有首个分片即返回 ready |
| `hls_handler.go` | `buildHLSCache()` 从 `CombinedOutput()` 改为 `Start()` | ffmpeg 后台运行，不阻塞 |
| `hls_handler.go` | `-hls_playlist_type` 从 `vod` 改为 `event` | 播放列表可增长 |
| `hls_handler.go` | 新增 `-probesize 1M -analyzeduration 1s` | 限制 ffmpeg 转封装前的探测时间 |
| `media_probe.go` | 转码参数同步改为 `event` + 快速探测 | 保持两种模式一致 |
| `config.yml` | `convert_to_mp4: true` | 新录播默认转 MP4 |

### 1.2 核心逻辑变化

**改动前**：
```
用户点播放
  ↓
服务端检查 HLS 缓存是否完整（有 #EXT-X-ENDLIST）
  ↓ 否
启动 ffmpeg，用 CombinedOutput() 等待进程退出
  ↓
30 分钟后转完，写入 ENDLIST
  ↓
返回 ready 给客户端
  ↓
客户端开始播放
```

**改动后**：
```
用户点播放
  ↓
服务端检查 HLS 缓存是否有分片（有 #EXTINF）
  ↓ 否
启动 ffmpeg（Start() 立即返回）
  ↓
6-12 秒后首个分片落地
  ↓
返回 ready 给客户端
  ↓
客户端开始播放（ffmpeg 在后台继续切片）
  ↓
播放器定期重新请求 m3u8，获取新分片
  ↓
ffmpeg 结束时追加 #EXT-X-ENDLIST
```

---

## 二、测试计划

### 2.1 单元测试

#### Test 1: `isPlaylistReady()` 函数
**文件**: `src/servers/hls_handler_test.go`

```go
func TestIsPlaylistReady(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        expected bool
    }{
        {
            name: "empty file",
            content: "",
            expected: false,
        },
        {
            name: "only header",
            content: "#EXTM3U\n#EXT-X-VERSION:3\n",
            expected: false,
        },
        {
            name: "with one segment",
            content: "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:6.0,\nseg_00000.ts\n",
            expected: true,
        },
        {
            name: "complete with ENDLIST",
            content: "#EXTM3U\n#EXTINF:6.0,\nseg_00000.ts\n#EXT-X-ENDLIST\n",
            expected: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tmpfile, err := os.CreateTemp("", "playlist*.m3u8")
            require.NoError(t, err)
            defer os.Remove(tmpfile.Name())

            _, err = tmpfile.WriteString(tt.content)
            require.NoError(t, err)
            tmpfile.Close()

            result := isPlaylistReady(tmpfile.Name())
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

#### Test 2: `isPlaylistComplete()` 保持向后兼容
验证 `isPlaylistComplete()` 只在有 ENDLIST 时返回 true

#### Test 3: ffprobe 不再请求 duration
Mock `exec.Command` 验证参数列表中没有 `format=duration`

### 2.2 集成测试

#### Test 4: 端到端播放流程
**测试环境**: 本地 Docker + 测试 FLV 文件

**准备**：
1. 准备 3 个测试文件：
   - `test-h264-aac.flv` (100MB, H.264+AAC, 标准 B 站录播)
   - `test-hevc.ts` (50MB, HEVC, B 站原画 PRO)
   - `test-large.flv` (2GB, 模拟长时录播)

2. 启动服务：`docker-compose up`

**测试步骤**：
```bash
# 1. 上传测试文件到 recordings/
cp test-h264-aac.flv recordings/test/

# 2. 首次播放，记录首帧时间
START=$(date +%s)
curl -s "http://localhost:8080/api/playback/resolve/test/test-h264-aac.flv" | jq .
# 轮询直到 status=ready
while [ "$(curl -s http://localhost:8080/api/playback/resolve/test/test-h264-aac.flv | jq -r .data.status)" != "ready" ]; do
    sleep 2
done
END=$(date +%s)
echo "首帧就绪耗时: $((END - START)) 秒"

# 3. 验证播放列表可访问
HLS_URL=$(curl -s http://localhost:8080/api/playback/resolve/test/test-h264-aac.flv | jq -r .data.url)
curl -s "$HLS_URL" | head -20

# 4. 验证第一个分片可下载
SEG_URL=$(curl -s "$HLS_URL" | grep -v "^#" | head -1)
curl -I "$SEG_URL"

# 5. 等待转换完成，验证 ENDLIST 出现
sleep 60
curl -s "$HLS_URL" | grep "EXT-X-ENDLIST"
```

**预期结果**：
- 首帧就绪耗时 < 15 秒（改动前可能数分钟）
- 第一次轮询返回 `processing`
- 第二或第三次轮询返回 `ready`
- m3u8 中有 `#EXTINF` 和至少一个 `.ts` 文件
- 最终 m3u8 包含 `#EXT-X-ENDLIST`

#### Test 5: 并发播放同一文件
```bash
# 启动 5 个并发请求
for i in {1..5}; do
    (time curl -s http://localhost:8080/api/playback/resolve/test/test-h264-aac.flv > /tmp/response-$i.json) &
done
wait

# 验证所有请求最终都得到 ready
for i in {1..5}; do
    jq .data.status /tmp/response-$i.json
done
```

**预期**：5 个请求都能成功，且只启动一次 ffmpeg

#### Test 6: HEVC 转码路径
```bash
cp test-hevc.ts recordings/test/
curl -s "http://localhost:8080/api/playback/resolve/test/test-hevc.ts"
# 检查日志，应该看到 "改用 H.264/AAC 转码"
docker logs bililive-go-ui | grep "改用 H.264/AAC 转码"
```

### 2.3 客户端兼容性测试

#### Test 7: iOS 播放 event 播放列表
**设备**: iPhone (iOS 15+)
**步骤**:
1. 在 iOS 客户端点开测试录播
2. 观察首帧出现时间
3. 拖动进度条，验证 seek 正常
4. 播放到结尾，验证无异常

**预期**: 播放流畅，无卡顿或错误

#### Test 8: 鸿蒙播放 event 播放列表（关键）
**设备**: 鸿蒙真机（HarmonyOS 4.0+）
**步骤**: 同 Test 7

**预期**: 播放流畅。如果失败：
- 检查错误码（5400106 = CONTAINER_ERR）
- 回退方案：不写 `#EXT-X-PLAYLIST-TYPE` 标签（兼容性最好）

### 2.4 性能基准测试

#### Test 9: 首帧时间对比
使用相同的 3 个测试文件，分别在 main 分支和 feature 分支测试：

| 文件 | main 分支 | feature 分支 | 改善 |
|---|---|---|---|
| test-h264-aac.flv (100MB) | ___ 秒 | ___ 秒 | ___ % |
| test-hevc.ts (50MB) | ___ 秒 | ___ 秒 | ___ % |
| test-large.flv (2GB) | ___ 秒 | ___ 秒 | ___ % |

#### Test 10: CPU / 内存占用
```bash
# 播放过程中监控资源
docker stats bililive-go-ui --no-stream

# 对比改动前后的资源占用峰值
```

### 2.5 回归测试

#### Test 11: MP4 文件仍然走 Range 请求
```bash
# 录制结束后应该生成 MP4
ls -lh recordings/test/*.mp4

# MP4 应该返回 file_url 而不是 hls_url
curl -s http://localhost:8080/api/playback/resolve/test/xxx.mp4 | jq .data.protocol
# 预期: "file"
```

#### Test 12: 正在录制的文件拒绝生成 HLS
```bash
# 模拟录制中：启动一个直播间录制
# 立即请求播放
curl -s http://localhost:8080/api/playback/resolve/xxx/recording.flv | jq .data.status
# 预期: "recording"
```

#### Test 13: HLS 缓存清理仍然工作
```bash
# 检查 hls-cache 目录大小
du -sh .appdata/hls-cache/

# 等待 24 小时或手动触发清理
# 验证旧缓存被删除
```

---

## 三、风险评估与缓解

### 3.1 高风险项

| 风险 | 概率 | 影响 | 缓解措施 |
|---|---|---|---|
| 鸿蒙不支持 event 播放列表 | 中 | 高 | 真机测试；准备回退到不写 PLAYLIST-TYPE |
| ffmpeg 后台运行导致僵尸进程 | 低 | 中 | 监控进程数；context 超时仍然有效 |
| 首个分片损坏导致播放失败 | 低 | 中 | temp_file flag 保证原子写入；客户端重试机制 |
| 并发请求导致多个 ffmpeg 进程 | 低 | 中 | 现有锁机制已覆盖；增加集成测试验证 |

### 3.2 中风险项

| 风险 | 概率 | 影响 | 缓解措施 |
|---|---|---|---|
| event 播放列表 duration 不稳定 | 中 | 低 | 文档说明这是预期行为；ENDLIST 后稳定 |
| probesize 限制导致某些文件探测失败 | 低 | 低 | 降级到全探测；日志记录 |
| 默认转 MP4 导致磁盘占用翻倍 | 高 | 低 | 文档说明可删除源 FLV；对比 HLS 缓存更省 |

### 3.3 低风险项

- 播放列表格式不兼容：HLS 是标准协议，event 类型是规范的一部分
- 性能下降：改动减少了阻塞，理论上性能只会提升
- 向后兼容：旧的完整播放列表仍然优先使用

---

## 四、部署方案

### 4.1 灰度策略

#### 阶段 1: 开发环境验证（2-3 天）
- 在本地 Docker 完成所有单元测试和集成测试
- 修复发现的 bug
- 确认性能指标达标

#### 阶段 2: 单个直播间灰度（3-5 天）
- 选择一个活跃但非核心的直播间
- 只对该直播间的新录播应用改动（通过配置或代码判断）
- 监控指标：
  - 首帧时间（P50, P95, P99）
  - 播放成功率
  - 错误日志
  - 用户反馈

#### 阶段 3: 部分用户灰度（1 周）
- 扩大到 10-20% 的直播间
- 双端（iOS + 鸿蒙）真实用户测试
- 收集 Sentry 错误上报

#### 阶段 4: 全量上线
- 确认无重大问题后全量
- 保持 1-2 周的密切监控

### 4.2 回滚方案

**如果出现重大问题（播放失败率 > 5%）**：

#### 快速回滚（5 分钟内）
```bash
git checkout main
docker-compose down
docker-compose up -d
```

#### 代码级回滚
如果只是部分问题，可以通过功能开关回退：

```go
// 在 config 中新增开关
type HLSConfig struct {
    EnableJIT bool `yaml:"enable_jit" json:"enable_jit"`
}

// 在 playback_resolver.go 中
if cfg.HLS.EnableJIT && isPlaylistReady(playlistPath) {
    // 新逻辑
} else if isPlaylistComplete(playlistPath) {
    // 旧逻辑（完全兼容）
}
```

**配置回滚**：
```yaml
hls_cache:
  enable_jit: false  # 关闭 JIT，恢复等待完整播放列表
```

### 4.3 监控指标

部署后需要持续监控：

| 指标 | 目标 | 告警阈值 |
|---|---|---|
| 首帧就绪时间 P95 | < 15 秒 | > 30 秒 |
| 播放成功率 | > 98% | < 95% |
| HLS 转换失败率 | < 2% | > 5% |
| ffmpeg 僵尸进程数 | 0 | > 10 |
| HLS 缓存目录大小 | < 10GB | > 20GB |
| 5xx 错误率 | < 0.1% | > 1% |

**监控方法**：
1. 日志聚合：`grep "HLS" /var/log/bililive.log | grep "ERROR"`
2. Prometheus + Grafana（如果已部署）
3. 定期检查：`docker logs bililive-go-ui | tail -100`

---

## 五、上线检查清单

### 5.1 代码审查
- [ ] 所有单元测试通过
- [ ] 集成测试通过
- [ ] 代码 review 完成
- [ ] 无明显的代码坏味道（重复代码、复杂度过高）

### 5.2 测试验证
- [ ] iOS 真机测试通过
- [ ] 鸿蒙真机测试通过（关键）
- [ ] 性能基准测试达标
- [ ] 并发测试通过
- [ ] 回归测试通过

### 5.3 文档更新
- [ ] CHANGELOG.md 记录改动
- [ ] README.md 更新性能说明
- [ ] 运维文档更新（新的配置项）
- [ ] 故障排查文档（常见问题）

### 5.4 部署准备
- [ ] 数据库备份（如有）
- [ ] 配置文件备份
- [ ] 回滚脚本准备完成
- [ ] 监控告警配置完成
- [ ] 值班人员到位

### 5.5 用户沟通
- [ ] 提前通知用户可能的短暂中断
- [ ] 发布更新说明
- [ ] 准备用户反馈渠道（Issue / Telegram）

---

## 六、后续优化方向

### 6.1 短期（1-2 周内）
1. **HEVC 直接放进 MP4**
   - 当前对 HEVC 会转码为 H.264
   - 现代系统都支持 HEVC-in-MP4
   - 节省转码时间和 CPU

2. **存量 FLV 批量转换**
   - 写一个脚本扫描 recordings/
   - 对所有 FLV 执行 `ffmpeg -c copy -movflags +faststart`
   - 避免用户首次打开时等待

3. **优化 HLS 分片大小**
   - 当前 6 秒一片
   - 可以改为 4 秒（更快首帧）或 10 秒（更少请求）
   - 需要权衡

### 6.2 中期（1-2 个月内）
4. **播放预热**
   - 录制结束后自动生成第一个分片
   - 用户打开时已经 ready
   - 类似 CDN 的预热机制

5. **智能降级**
   - 检测客户端类型（User-Agent）
   - 鸿蒙如果不支持 event，自动回退 vod
   - iOS 继续用 event

6. **播放质量监控**
   - 收集播放器事件（首帧时间、卡顿次数）
   - 上报到服务端
   - 数据驱动优化

### 6.3 长期（3 个月以上）
7. **考虑 SRS 或 MediaMTX**
   - 如果需要"正在录制也能看"
   - 录制时就切 HLS
   - 那是另一套产品形态

8. **CDN 接入**
   - 如果用户量大、跨地域
   - 对接 OSS + CDN
   - 但报告明确说不在范围内

---

## 七、常见问题

### Q1: 为什么不用 SRS？
**A**: SRS 适合"录制过程中就能看"的直播回放场景。本项目是"录完再看"的点播场景，Jellyfin/Plex 模型更合适。

### Q2: event 播放列表和 vod 有什么区别？
**A**: 
- `vod`: 固定长度，最后有 `#EXT-X-ENDLIST`
- `event`: 可增长，播放器会定期重新请求 m3u8，结束时追加 ENDLIST
- 本方案用 event 让播放器边拉边播

### Q3: 如果鸿蒙不支持 event 怎么办？
**A**: 不写 `#EXT-X-PLAYLIST-TYPE` 标签，两端都当"增长式点播"处理，兼容性最好。

### Q4: MP4 转换会不会让磁盘占用翻倍？
**A**: 
- FLV → MP4 是 `-c copy`，体积几乎相同
- 可以转完后删除 FLV（`delete_flv_after_convert: true`）
- 对比现在每次播放生成一份 HLS 缓存，MP4 方案更省

### Q5: 改动会影响已有的 HLS 缓存吗？
**A**: 不会。已经生成的完整播放列表仍然优先使用。只影响新生成的缓存。

### Q6: 如果 ffmpeg 转换到一半崩溃了？
**A**: 
- context 超时（30 分钟）仍然有效
- goroutine 会检查是否有可用分片
- 有分片就补 ENDLIST，没有才报错
- 客户端可以重试

---

## 八、总结

### 改动影响范围
- **代码**: 4 个文件，76 行，核心逻辑清晰
- **风险**: 中等，主要是鸿蒙兼容性需要真机验证
- **收益**: 首帧时间从分钟级降到秒级，用户体验显著提升

### 关键成功因素
1. **鸿蒙真机测试**：必须在真机上验证 event 播放列表
2. **灰度上线**：不要一次性全量，逐步扩大范围
3. **监控告警**：密切观察播放成功率和错误日志
4. **快速回滚**：出问题能 5 分钟内回退

### 建议
**现在就可以做的**：
1. 准备 3 个测试文件（不同格式、不同大小）
2. 在本地 Docker 跑完所有集成测试
3. 找一台鸿蒙真机做兼容性测试
4. 确认没问题后，单个直播间灰度

**如果测试通过**，这套改动可以合并到 main，预期能解决报告里提到的首帧慢问题。

**如果鸿蒙测试失败**，回退方案已准备好，改动可以快速撤销。
