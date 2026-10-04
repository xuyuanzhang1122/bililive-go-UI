# A0 批次交付报告：编译恢复与测试设施

## 批次与任务 ID
- **批次**: A0 (编译恢复与测试设施)
- **任务**: A0-1 编译阻断修复, A0-2 可控子进程测试设施

## 基线 SHA / 最终 SHA
- **基线 SHA**: `261ad7328f3d0fc8c90f959ed4f56f5b5a23507f`
- **最终状态**: 未提交（按要求），工作区有修改

## 改动文件与行为变化

### 修改的文件
1. `src/servers/hls_job_manager.go`
   - 添加 `bytes` 包导入
   - 删除自定义 `contains([]byte, []byte)` 函数（23 行）
   - 3 处调用替换为 `bytes.Contains`
   
2. `src/servers/playback_resolver.go`
   - 删除未使用的 `sync` 导入

3. 新增 `src/servers/hls_job_manager_test.go` (247 行)
   - 测试辅助进程框架
   - 4 种可控行为模式
   - 最小有效 HLS 输出生成器
   - 基础验证测试

### 行为变化
- 编译从失败变为成功（退出码 0）
- 新增可重复的测试基础设施，支持后续 A1-A3 测试

## 原问题的复现方式

### 编译错误
执行 `make dev` 时出现以下错误：
```
src/servers/hls_job_manager.go:283:14: cannot use content (variable of type []byte) as []string value in argument to contains
src/servers/hls_job_manager.go:379:6: contains redeclared in this block
src/servers/playback_resolver.go:9:2: "sync" imported and not used
```

### 根本原因
1. `hls_job_manager.go` 定义了处理 `[]byte` 的 `contains` 函数
2. `handler.go` 中已存在处理 `[]string` 的同名 `contains` 函数
3. 同包内函数名冲突导致编译失败
4. `playback_resolver.go` 导入了 `sync` 但未使用

## 修复前失败证据

**环境信息**:
- 系统: macOS 26.6.2 (Darwin 25.6.0)
- 架构: arm64
- Go 版本: go1.27.1 darwin/arm64（新安装）
- FFmpeg 版本: 8.1.2
- HEAD: 261ad73 (fix: 修复 HLS JIT 任务生命周期缺陷（P0/P1）)

**编译失败日志**: 保存在 `/tmp/build-before-fix.log`

关键错误:
```
# github.com/bililive-go/bililive-go/src/servers
src/servers/hls_job_manager.go:283:14: cannot use content (variable of type []byte) as []string value in argument to contains
src/servers/hls_job_manager.go:283:23: cannot use []byte("#EXT-X-ENDLIST") (value of type []byte) as string value in argument to contains
src/servers/hls_job_manager.go:306:15: cannot use content (variable of type []byte) as []string value in argument to contains
src/servers/hls_job_manager.go:306:24: cannot use []byte("#EXT-X-ENDLIST") (value of type []byte) as string value in argument to contains
src/servers/hls_job_manager.go:348:18: cannot use content (variable of type []byte) as []string value in argument to contains
src/servers/hls_job_manager.go:348:27: cannot use []byte("#EXTM3U") (value of type []byte) as string value in argument to contains
src/servers/hls_job_manager.go:379:6: contains redeclared in this block
	src/servers/handler.go:3202:6: other declaration of contains
src/servers/playback_resolver.go:9:2: "sync" imported and not used
Command finished with error: exit status 1
```

## 修复后测试命令、退出码、结果摘要

### 编译验证
```bash
export PATH="$HOME/go-installation/go/bin:$PATH"
make dev
```

**退出码**: 0  
**结果**: 成功生成 `bin/bililive-darwin-arm64` (35MB)  
**编译日志**: 保存在 `/tmp/build-after-fix.log`

### 测试设施验证
```bash
go test -v ./src/servers -run TestHLSHelperProcess
```

**退出码**: 0  
**结果摘要**:
```
=== RUN   TestHLSHelperProcessBasic
=== RUN   TestHLSHelperProcessBasic/single_segment_complete
=== RUN   TestHLSHelperProcessBasic/immediate_failure
--- PASS: TestHLSHelperProcessBasic (0.03s)
    --- PASS: TestHLSHelperProcessBasic/single_segment_complete (0.02s)
    --- PASS: TestHLSHelperProcessBasic/immediate_failure (0.01s)
=== RUN   TestHLSHelperProcessWithControl
--- PASS: TestHLSHelperProcessWithControl (0.12s)
PASS
ok  	github.com/bililive-go/bililive-go/src/servers	0.830s
```

## 构建 / lint / 全量测试 / race

### 构建
- ✅ `make dev`: 退出码 0，二进制文件生成成功

### lint
- ⏸️ 未执行（等待完整批次后统一执行）

### 全量测试
- ⏸️ 未执行（等待完整批次后统一执行）

### race 检测
- ⏸️ 未执行（等待完整批次后统一执行）

## 真实 FFmpeg 集成结果
- ⏸️ A0 阶段为测试基础设施搭建，真实 FFmpeg 集成在 A1-A3 验证

## 双端设备、系统与首帧/续播结果
- ⏸️ 双端验证在 A3 和 B 批次进行

## 配置与迁移影响
- **无配置变更**
- **无数据库迁移**
- **无 API 变更**

## 测试设施说明

### 辅助进程架构
测试辅助进程通过 `TestHelperProcess` 实现，使用环境变量控制行为：

1. **`first_segment_then_wait`**: 生成首片后等待释放信号，模拟边切边播
2. **`single_segment_complete`**: 生成单片完整视频后正常退出
3. **`immediate_failure`**: 启动即失败，模拟 ffmpeg 启动错误
4. **`first_segment_then_fail`**: 生成首片后非零退出，模拟转换中途失败

### 可控性保证
- 使用信号文件 (`.release`) 控制进程继续/退出
- 使用事件通道观察状态，不依赖固定 sleep
- 生成真实格式的最小 HLS 清单和非空分片
- 所有等待都有超时保护（5 秒）
- 临时目录自动清理 (`t.TempDir()`)

### 已验证行为
- ✅ 单片正常完成
- ✅ 启动即失败
- ✅ 首片后等待释放，进程未退出时清单和分片可读
- ✅ 释放后正常完成并包含 ENDLIST
- ✅ 进程回收（通过 `cmd.Wait()` 确认）

## 未完成项及原因
- **Lint/全量测试**: 按 TODO 要求，等待完整批次完成后统一执行
- **真实 FFmpeg 测试**: A0 为基础设施，A1-A3 将集成真实 FFmpeg
- **客户端验证**: 按计划在 A3 和 B 批次进行

## 当前风险和回退边界

### 风险
- **低风险**: 仅修复编译错误和添加测试，未改变运行时行为
- 删除的 `contains` 函数已替换为标准库 `bytes.Contains`，行为等价

### 回退边界
- 可安全回退到 `261ad73`
- 未修改任何配置、数据结构或 API
- 新增的测试文件可直接删除

## 下一步
按 TODO 执行顺序，开始 **A1 批次：统一入口，真正实现边切边播**

### A1 主要目标
1. 合并所有 HLS 入口到统一作业管理器
2. 拆开作业互斥与清单读取锁
3. 进程退出、短片、超时处理
4. 验证 ffmpeg 未退出时清单和分片已可读

---

**交付时间**: 2026-09-05  
**执行 Agent**: Claude Fable 5
