# JIT HLS 修复交付记录

日期：2026-09-07。基线提交：`261ad73`，在原有未提交 A0–A1 改动上继续修复。未提交、未部署。

## 本轮范围

本轮处理代码审查发现的 JIT 服务端缺陷以及接手时新增的退出结果重复消费问题，对应执行 TODO 的 A 阶段核心功能。不是 B/C/D 全部完成声明。

- HTTP resolver 与 m3u8 入口统一使用 EnsureJob；删除旧 buildHLSCache 进程管理分支。
- ffmpeg 只 Wait 一次，首片、退出和取消在同一事件循环处理。单分片正常结束、启动失败、部分输出失败都能够进入终态。
- 进程不持有 HTTP 读取共用锁；请求取消不终止共享作业。
- FIFO 队列，最多三个运行任务、六十四个排队任务；任务记录上限一千零二十四。
- 失败请求五秒退避后可重新执行；最近读取宽限期内不覆盖缓存。没有目录的失败记录也能过期清理。
- 清单验证实际引用，拒绝路径越界、空文件、缺失文件；完成时验证全部引用和 ENDLIST。
- partial 对现有客户端映射 failed，不伪装成正常完整内容；失败缓存带标记，避免随后被当作完整缓存复用。
- 读租约保护 HTTP 读取，最近访问保护一分钟；回收和作业登记使用同一同步协议，queued/starting 不接纳旧分片读取。
- 服务关闭取消排队和运行任务并回收子进程。
- 探测错误明确失败，不回退未知编码 TS copy；视频轨道选择与探测决策一致。
- 探测墙钟上限十五秒，首片三十秒，两分钟无清单进度时结束作业；不再以固定三十分钟截断所有长转码。

## 回归证据

修复前新增的生产路径测试复现：单片、立即失败、部分输出均无法完成；小于 1 KB 的非空测试分片无法触发可播；带 ENDLIST 但引用缺失文件的缓存被误接受。

独立复核又发现重试与分片读准入竞态、未知 key 的读取记录无界增长；均已修正，并增加失败重试宽限期、排队禁止读取、未知 key 不分配记录的测试。

测试主文件：`src/servers/hls_lifecycle_test.go`。涵盖真实 handler → manager → 子进程执行与 Wait，未仅验证辅助脚本输出。HTTP 用例在辅助进程仍等待释放时成功读取清单和分片，并检查 resolver 复用同一作业。

额外场景：二十个并发请求只启动一次子进程、运行名额释放后自动调度、队列上限、取消等待不取消共享任务、关闭回收、partial 公共协议、租约阻止回收，以及完整缓存元数据不会提前消失。

真实 FFmpeg 集成用例分别生成 H.264 和 HEVC 样本，通过生产转换路径，验证最终 MPEG-TS 视频编码为 H.264。普通运行可通过 `BILIVE_HLS_INTEGRATION=1` 开启。

## 验证命令

本机 Go 位于 `/Users/xumy/go-installation/go/bin`，使用 Go 1.27.1；lint 使用 golangci-lint 2.13.1。

结果：以下构建、全量测试、真实 FFmpeg 集成及 race 检查均通过；lint 报告 0 issues。首次全量测试因缺少前端嵌入产物未运行到服务端，完成 `make build-web dev` 后重跑通过。最终全量测试同时启用了真实 FFmpeg 集成。

```sh
make dev
make build-web dev
make lint
make test
BILIVE_HLS_INTEGRATION=1 go test -tags=dev ./src/servers -run TestHLSRealFFmpeg -count=1 -v
go test -race -tags=dev ./src/servers -run 'TestHLS|TestResolvePlayback|TestRunHLS|TestResetHLS' -count=1 -timeout=90s
```

前端构建生成的无关 API 文件和 yarn.lock 变动已恢复。本轮保留接手前已有未提交文档。

## 尚未覆盖的验收与后续工作

- 未执行 iOS/鸿蒙真机首帧、时长增长、续播和结束事件验收；HTTP 就绪不等于实际首帧。
- B 阶段录后 MP4、C 阶段设备能力/兼容版、D 阶段历史迁移没有在本轮实施。
- A 阶段完整规划中的可配置资源参数、最低空闲磁盘准入、持久探测缓存和详细延迟统计仍是后续增强，本轮不宣称其已实现。
- 两分钟无清单进度是当前保守看门狗，极端长 GOP、极慢转码仍需基于真实样本评估。
- 播放闲置超过读取宽限期后可能被容量回收，当前不提供长期暂停会话的持久缓存保证。
- 测试进程驱动器中的 POSIX shell 用例在 Windows 跳过；跨平台行为仍需对应 CI 验证。
