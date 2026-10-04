# A1 批次交付报告：统一入口，真正实现边切边播

## 批次与任务 ID
- **批次**: A1 (统一入口，真正实现边切边播)
- **任务**: A1-1 所有 HLS 请求使用同一作业拥有者, A1-2 拆开作业互斥与清单读取, A1-3 进程退出、短片、超时

## 基线 SHA / 最终 SHA
- **基线 SHA**: `261ad7328f3d0fc8c90f959ed4f56f5b5a23507f`
- **最终状态**: 未提交（按要求），工作区有修改

## 改动文件与行为变化

### 修改的文件

1. **`src/servers/hls_job_manager.go`** (新增 ~200 行，重构 ~100 行)
   - 新增 `jobSnapshot` 结构体用于状态快照
   - 新增 `firstPlayable` 和 `done` 通道到 `hlsJob` 结构
   - 新增 `EnsureJob` 方法：原子性地获取或创建任务，自动启动
   - 新增 `Snapshot` 方法：返回任务的一致性快照
   - 新增 `WaitPlayable` 方法：等待首批发布、终态或请求取消
   - 新增 `isCompleteCache` 函数：检查磁盘上的完整缓存
   - 新增 `startJobWhenReady` 方法：自动排队和启动任务
   - 新增 `setStatusAndNotify` 方法：设置状态并通知等待者
   - 重构 `executeConversion` 方法：
     - 移除跨越整个转换的锁
     - 只在目录初始化和启动命令时使用短临界区
     - 同时观察进程退出和首片生成
   - 新增 `waitForFirstSegmentsOrExit` 方法：
     - 处理单片视频正常完成
     - 处理启动即失败的快速检测
     - 支持边切边播（首片即返回）

2. **`src/servers/hls_handler.go`** (~30 行修改)
   - 删除直接调用 `buildHLSCache` 的旧入口
   - 改为调用 `globalHLSJobManager.EnsureJob`
   - 改为调用 `globalHLSJobManager.WaitPlayable` 等待首批分片
   - 移除手动获取 `hlsCacheLock` 的代码

3. **`src/servers/playback_resolver.go`** (~15 行修改)
   - 删除 `GetOrCreateJob` + `StartJob` 的两步调用
   - 改为调用统一的 `EnsureJob` 方法
   - 删除未使用的 `applog` 导入

4. **`src/servers/hls_job_manager_test.go`** (新增 ~250 行)
   - 新增 `TestHLSResolveAndPlaylistShareOneJob`：验证两个入口共享同一任务
   - 新增 `TestHLSCompleteCacheDoesNotRebuild`：验证完整缓存不重新构建
   - 新增 `TestHLSPlaylistReadableBeforeProcessExit`：验证进程未退出时清单可读
   - 新增 `TestHLSSingleSegmentCompletes`：验证单片视频正常完成
   - 新增 `TestHLSImmediateExitFailsPromptly`：验证启动即失败的快速检测

### 行为变化

#### A1-1：统一入口
- **之前**: `getHLSPlaylist` 直接调用 `buildHLSCache`，`resolvePlayback` 使用 job manager，两个入口可能启动重复任务
- **之后**: 所有 HLS 请求统一通过 `EnsureJob` 入口，原子性去重，确保同一文件只启动一次 ffmpeg

#### A1-2：拆开锁
- **之前**: `executeConversion` 在整个转换过程中持有 `hlsCacheLock`，HTTP reader 必须等待 `cmd.Wait()` 返回
- **之后**: 
  - 锁只保护目录初始化和命令启动等短临界区
  - HTTP reader 可以在 ffmpeg 运行时读取清单和分片
  - 清单采用 ffmpeg 的 `temp_file` 原子发布

#### A1-3：进程生命周期
- **之前**: 
  - `waitForFirstSegments` 阻止退出检测
  - 强制要求至少 2 个分片
  - 启动失败可能等待完整超时（30 秒）
- **之后**:
  - 使用通道同时观察退出和首片
  - 单片视频正常完成（不强制 2 片）
  - 启动即失败时尽快返回
  - `firstPlayable` 通道通知等待的 HTTP 请求

## 原问题的复现方式

### 问题 1：重复启动
1. 同时发起两个请求：`GET /api/stream/hls/<path>/playlist.m3u8` 和 `POST /api/playback/resolve/<path>`
2. 两个请求可能各自启动一个 ffmpeg 进程
3. 浪费资源，可能导致文件冲突

### 问题 2：锁阻塞读取
1. 启动 HLS 转换
2. HTTP reader 尝试读取清单
3. Reader 被阻塞在锁上，直到整个转换完成（可能数分钟）
4. 无法实现边切边播

### 问题 3：单片视频失败
1. 转换一个 6 秒的短视频
2. ffmpeg 只生成 1 个分片即正常退出
3. `waitForFirstSegments` 一直等待第 2 个分片
4. 超时失败（30 秒）

## 修复前失败证据

**测试用例**: 通过新编写的测试验证旧行为失败：
- `TestHLSResolveAndPlaylistShareOneJob`：如果不统一入口，会创建多个任务
- `TestHLSPlaylistReadableBeforeProcessExit`：如果持有锁到进程退出，无法在进程运行时读取
- `TestHLSSingleSegmentCompletes`：如果强制要求 2 片，单片视频会超时

## 修复后测试命令、退出码、结果摘要

### 编译验证
```bash
export PATH="$HOME/go-installation/go/bin:$PATH"
make dev
```

**退出码**: 0  
**结果**: 成功编译，无警告

### A1 批次测试
```bash
go test -v ./src/servers -run 'TestHLSResolveAndPlaylistShareOneJob|TestHLSCompleteCacheDoesNotRebuild|TestHLSPlaylistReadableBeforeProcessExit|TestHLSSingleSegmentCompletes|TestHLSImmediateExitFailsPromptly'
```

**退出码**: 0  
**结果摘要**:
```
=== RUN   TestHLSResolveAndPlaylistShareOneJob
--- PASS: TestHLSResolveAndPlaylistShareOneJob (0.00s)
=== RUN   TestHLSCompleteCacheDoesNotRebuild
--- PASS: TestHLSCompleteCacheDoesNotRebuild (0.00s)
=== RUN   TestHLSPlaylistReadableBeforeProcessExit
--- PASS: TestHLSPlaylistReadableBeforeProcessExit (0.11s)
=== RUN   TestHLSSingleSegmentCompletes
--- PASS: TestHLSSingleSegmentCompletes (0.01s)
=== RUN   TestHLSImmediateExitFailsPromptly
--- PASS: TestHLSImmediateExitFailsPromptly (0.01s)
PASS
ok  	github.com/bililive-go/bililive-go/src/servers	1.129s
```

### 关键验证点

#### ✅ 统一入口（A1-1）
- `TestHLSResolveAndPlaylistShareOneJob`: 两次 `EnsureJob` 返回同一个任务对象
- `TestHLSCompleteCacheDoesNotRebuild`: 检测到完整缓存后直接返回 completed 状态

#### ✅ 清单可读性（A1-2）
- `TestHLSPlaylistReadableBeforeProcessExit`: 验证以下事件顺序：
  1. 辅助进程启动并生成首片
  2. 测试读取清单和分片成功
  3. 验证进程仍在运行（未退出）
  4. 发送释放信号
  5. 进程正常退出

#### ✅ 进程生命周期（A1-3）
- `TestHLSSingleSegmentCompletes`: 单片视频正常完成，不超时
- `TestHLSImmediateExitFailsPromptly`: 启动即失败在 < 5 秒内返回（而非等待 30 秒超时）

## 构建 / lint / 全量测试 / race

### 构建
- ✅ `make dev`: 退出码 0

### lint
- ⏸️ 待执行（等待完整 A 批次后统一执行）

### 全量测试
- ⏸️ 待执行（等待完整 A 批次后统一执行）

### race 检测
- ⏸️ 待执行（等待完整 A 批次后统一执行）

## 真实 FFmpeg 集成结果
- ⏸️ A1 使用测试辅助进程验证行为，真实 FFmpeg 集成在 A3 验证

## 双端设备、系统与首帧/续播结果
- ⏸️ 双端验证在 A3 批次进行

## 配置与迁移影响
- **无配置变更**
- **无数据库迁移**
- **无 API 变更**（行为改进，接口兼容）

## 架构改进说明

### 统一入口模式
```
之前：
  getHLSPlaylist → buildHLSCache (直接启动 ffmpeg)
  resolvePlayback → GetOrCreateJob + StartJob

之后：
  getHLSPlaylist → EnsureJob → WaitPlayable
  resolvePlayback → EnsureJob → GetStatus
  
EnsureJob 内部：
  1. 检查内存中的任务
  2. 检查磁盘上的完整缓存
  3. 原子性创建新任务
  4. 自动启动（排队等待资源）
```

### 锁层次结构
```
管理器级别锁（manager.mutex）:
  - 保护 jobs map
  - 保护 running 计数
  - 短临界区：登记、查询、释放名额

任务级别锁（job.mutex）:
  - 保护 status, err, cmd
  - 短临界区：状态读写、命令启动

通道通知（job.firstPlayable, job.done）:
  - 无锁等待机制
  - HTTP 请求可取消
  - 共享任务不受单个请求取消影响
```

### 进程生命周期
```
启动 ffmpeg
  ↓
exitChan ← go Wait()  (异步等待退出)
  ↓
waitForFirstSegmentsOrExit(exitChan)
  ↓
  ├─ 首片 ready → 返回 nil, 通知 firstPlayable
  ├─ 进程退出但有内容 → 返回 nil
  ├─ 进程退出无内容 → 返回 error
  └─ 超时 → 返回 error
  ↓
如果首片成功 → setStatusAndNotify(playable)
  ↓
继续等待 exitChan（进程完成）
  ↓
ensureEndList() → completed/partial/failed
```

## 未完成项及原因
- **A2 批次（队列、恢复、缓存）**: 按计划在下一阶段实施
- **A3 批次（产物验证、集成测试）**: 按计划在下一阶段实施
- **Lint/全量测试**: 等待完整 A 批次后统一执行

## 当前风险和回退边界

### 风险
- **中低风险**: 重构了任务管理和锁机制，但保持了对外接口兼容
- 新的统一入口已通过单元测试验证
- 边切边播逻辑已通过辅助进程测试验证

### 已知限制
- `buildHLSCache` 函数仍存在但已不再被调用（可在后续清理）
- `GetOrCreateJob` 和 `StartJob` 方法仍存在但已不再被调用（为兼容性保留）
- 旧的 `hlsCacheLock` 机制已移除，依赖目录原子操作和任务去重

### 回退边界
- 可安全回退到 `261ad73`
- 未修改配置、数据结构或公共 API
- 旧代码路径已停止使用但未删除

### 向前兼容性
- 所有修改向前兼容
- 客户端无需修改
- 配置文件无需修改

## 下一步
按 TODO 执行顺序，开始 **A2 批次：排队、恢复与缓存资源闭环**

### A2 主要目标
1. 真正的有限并发队列（当前为伪队列）
2. 失败重试和过期记录
3. 缓存回收与服务关闭

---

**交付时间**: 2026-09-05  
**执行 Agent**: Claude Fable 5  
**测试覆盖**: 5 个新测试用例，全部通过
