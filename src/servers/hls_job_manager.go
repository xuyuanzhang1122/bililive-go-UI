package servers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	applog "github.com/bililive-go/bililive-go/src/log"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/shirou/gopsutil/v3/disk"
)

type hlsJobStatus string

const (
	hlsJobQueued    hlsJobStatus = "queued"
	hlsJobStarting  hlsJobStatus = "starting"
	hlsJobPlayable  hlsJobStatus = "playable"
	hlsJobCompleted hlsJobStatus = "completed"
	hlsJobFailed    hlsJobStatus = "failed"
	hlsJobPartial   hlsJobStatus = "partial"
)
const hlsRetryDelay = 5 * time.Second
const hlsReadGrace = time.Minute

type hlsJob struct {
	key, sourcePath, cacheDir, playlistPath     string
	cfg                                         *configs.Config
	ctx                                         context.Context
	cancel                                      context.CancelFunc
	status                                      hlsJobStatus
	err                                         string
	queuedAt, startedAt, playableAt, finishedAt time.Time
	executionAt                                 time.Time
	resourceWait                                time.Duration
	firstPlayable, done                         chan struct{}
	mutex                                       sync.Mutex
}
type jobSnapshot struct {
	status hlsJobStatus
	errMsg string
}

// 锁顺序固定为 manager.mutex → job.mutex。运行进程、等待和 HTTP 传输不持管理器锁。
type hlsJobManager struct {
	mutex                             sync.RWMutex
	jobs                              map[string]*hlsJob
	queue                             []*hlsJob
	maxConcurrent, maxQueued, running int
	closed                            bool
	workers                           sync.WaitGroup
	readers                           map[string]int
	lastRead                          map[string]time.Time
}

var globalHLSJobManager = &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 3}

func newHLSJob(cfg *configs.Config, key, source, dir, playlist string) *hlsJob {
	ctx, cancel := context.WithCancel(context.Background())
	return &hlsJob{key: key, sourcePath: source, cacheDir: dir, playlistPath: playlist, cfg: cfg, ctx: ctx, cancel: cancel, status: hlsJobQueued, queuedAt: time.Now(), firstPlayable: make(chan struct{}), done: make(chan struct{})}
}

// EnsureJob 原子登记和调度；仅在退避结束、没有读者时替换失败尝试。
func (m *hlsJobManager) EnsureJob(cfg *configs.Config, key, source, dir, playlist string) *hlsJob {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.jobs == nil {
		m.jobs = make(map[string]*hlsJob)
	}
	if cfg != nil && cfg.HLSCache.MaxConcurrent > 0 {
		m.maxConcurrent = cfg.HLSCache.MaxConcurrent
	}
	if cfg != nil && cfg.HLSCache.MaxQueued > 0 {
		m.maxQueued = cfg.HLSCache.MaxQueued
	}
	if m.maxQueued <= 0 {
		m.maxQueued = 64
	}
	if m.maxConcurrent <= 0 {
		m.maxConcurrent = 3
	}
	now := time.Now()
	if m.closed {
		j := newHLSJob(cfg, key, source, dir, playlist)
		j.finish(hlsJobFailed, "服务正在关闭")
		return j
	}
	if j := m.jobs[key]; j != nil {
		j.mutex.Lock()
		retry := (j.status == hlsJobFailed || j.status == hlsJobPartial) && !j.finishedAt.IsZero() && now.Sub(j.finishedAt) >= hlsRetryDelay && m.readers[key] == 0 && now.Sub(m.lastRead[key]) >= j.readGrace()
		j.mutex.Unlock()
		if !retry {
			return j
		}
		// 失败尝试不能通过其尾部 ENDLIST 在下一次请求中被重新认作完整缓存。
		delete(m.jobs, key)
		j.cancel()
		next := newHLSJob(cfg, key, source, dir, playlist)
		return m.enqueueLocked(next)
	}
	// 无目录的失败记录也会过期；元数据总量有界。
	for k, j := range m.jobs {
		j.mutex.Lock()
		expired := (j.status == hlsJobFailed || j.status == hlsJobPartial) && !j.finishedAt.IsZero() && now.Sub(j.finishedAt) > time.Hour && m.readers[k] == 0 && now.Sub(m.lastRead[k]) >= j.readGrace()
		j.mutex.Unlock()
		if expired {
			delete(m.jobs, k)
			delete(m.lastRead, k)
		}
	}
	j := newHLSJob(cfg, key, source, dir, playlist)
	if len(m.jobs) >= 1024 {
		j.finish(hlsJobFailed, "HLS 任务记录已满，请稍后重试")
		return j
	}
	if isCompleteCache(playlist) {
		j.finish(hlsJobCompleted, "")
		m.jobs[key] = j
		return j
	}
	return m.enqueueLocked(j)
}
func (m *hlsJobManager) enqueueLocked(j *hlsJob) *hlsJob {
	if len(m.queue) >= m.maxQueued {
		j.finish(hlsJobFailed, "HLS 等待队列已满，请稍后重试")
		return j
	}
	m.jobs[j.key] = j
	m.queue = append(m.queue, j)
	m.dispatchLocked()
	return j
}
func (m *hlsJobManager) dispatchLocked() {
	for !m.closed && m.running < m.maxConcurrent && len(m.queue) > 0 {
		j := m.queue[0]
		m.queue[0] = nil
		m.queue = m.queue[1:]
		if j.ctx.Err() != nil {
			j.finish(hlsJobFailed, "排队任务已取消")
			continue
		}
		m.running++
		j.setStatus(hlsJobStarting, "")
		m.workers.Add(1)
		go m.run(j)
	}
}
func (m *hlsJobManager) run(j *hlsJob) {
	defer func() {
		if r := recover(); r != nil {
			j.finish(hlsJobFailed, fmt.Sprintf("HLS 任务异常: %v", r))
		}
		m.mutex.Lock()
		m.running--
		m.dispatchLocked()
		m.mutex.Unlock()
		m.workers.Done()
	}()
	err := j.executeConversion(j.cfg)
	if err != nil {
		status := hlsJobFailed
		if _, e := readHLSPlaylist(j.playlistPath, false); e == nil {
			status = hlsJobPartial
		}
		// 标记不可信缓存，避免记录过期后把失败产物重新当成成功。
		_ = os.WriteFile(filepath.Join(j.cacheDir, ".failed"), []byte("incomplete"), 0600)
		j.finish(status, err.Error())
		applog.GetLogger().WithError(err).WithField("job", j.key).Warn("HLS 作业失败")
		return
	}
	if _, err = readHLSPlaylist(j.playlistPath, true); err != nil {
		j.finish(hlsJobFailed, err.Error())
		return
	}
	j.finish(hlsJobCompleted, "")
}
func (m *hlsJobManager) Snapshot(j *hlsJob) jobSnapshot {
	s, e := j.GetStatus()
	return jobSnapshot{s, e}
}
func (m *hlsJobManager) WaitPlayable(ctx context.Context, j *hlsJob) (jobSnapshot, error) {
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case <-j.firstPlayable:
		return m.Snapshot(j), nil
	case <-j.done:
		return m.Snapshot(j), nil
	case <-ctx.Done():
		return jobSnapshot{}, ctx.Err()
	case <-timer.C:
		return m.Snapshot(j), fmt.Errorf("HLS 正在准备，请稍后重试")
	}
}
func (m *hlsJobManager) GetJob(key string) *hlsJob {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.jobs[key]
}

// acquireRead 与回收在同一把管理器锁下决定，保护整个 HTTP 读操作。
func (m *hlsJobManager) acquireRead(key string, j *hlsJob) (func(), bool) {
	m.mutex.Lock()
	current := m.jobs[key]
	if m.closed || current == nil || (j != nil && current != j) {
		m.mutex.Unlock()
		return nil, false
	}
	status, _ := current.GetStatus()
	if status == hlsJobQueued || status == hlsJobStarting {
		m.mutex.Unlock()
		return nil, false
	}
	if m.readers == nil {
		m.readers = make(map[string]int)
		m.lastRead = make(map[string]time.Time)
	}
	m.readers[key]++
	m.lastRead[key] = time.Now()
	m.mutex.Unlock()
	return func() {
		m.mutex.Lock()
		m.readers[key]--
		if m.readers[key] == 0 {
			delete(m.readers, key)
		}
		m.lastRead[key] = time.Now()
		m.mutex.Unlock()
	}, true
}
func (m *hlsJobManager) Close() {
	m.mutex.Lock()
	m.closed = true
	for _, j := range m.jobs {
		j.cancel()
	}
	for _, j := range m.queue {
		j.finish(hlsJobFailed, "服务关闭，排队任务已取消")
	}
	m.queue = nil
	m.mutex.Unlock()
	m.workers.Wait()
}
func (j *hlsJob) setStatus(s hlsJobStatus, e string) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.status = s
	j.err = e
	if s == hlsJobStarting && j.startedAt.IsZero() {
		j.startedAt = time.Now()
	}
}
func (j *hlsJob) playable() {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	j.status = hlsJobPlayable
	if j.playableAt.IsZero() {
		j.playableAt = time.Now()
	}
	select {
	case <-j.firstPlayable:
	default:
		close(j.firstPlayable)
	}
}
func (j *hlsJob) finish(s hlsJobStatus, e string) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	select {
	case <-j.done:
		return
	default:
	}
	j.status = s
	j.err = e
	j.finishedAt = time.Now()
	j.cancel()
	if s == hlsJobCompleted {
		if j.playableAt.IsZero() {
			j.playableAt = j.finishedAt
		}
		select {
		case <-j.firstPlayable:
		default:
			close(j.firstPlayable)
		}
	}
	fields := map[string]any{"job": j.key, "status": s, "queued_at": j.queuedAt, "started_at": j.startedAt, "playable_at": j.playableAt, "finished_at": j.finishedAt, "total_ms": j.finishedAt.Sub(j.queuedAt).Milliseconds()}
	if !j.startedAt.IsZero() {
		fields["queue_ms"] = j.startedAt.Sub(j.queuedAt).Milliseconds()
		fields["resource_wait_ms"] = j.resourceWait.Milliseconds()
	}
	if !j.executionAt.IsZero() {
		fields["conversion_ms"] = j.finishedAt.Sub(j.executionAt).Milliseconds()
	}
	if !j.playableAt.IsZero() {
		fields["first_playable_ms"] = j.playableAt.Sub(j.queuedAt).Milliseconds()
	}
	applog.GetLogger().WithFields(fields).Info("HLS 作业结束")
	close(j.done)
}
func (j *hlsJob) GetStatus() (hlsJobStatus, string) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	return j.status, j.err
}
func (j *hlsJob) IsActive() bool {
	s, _ := j.GetStatus()
	return s == hlsJobQueued || s == hlsJobStarting || s == hlsJobPlayable
}

// 日志只保留尾部，避免长时间运行的错误输出耗尽内存。Wait 后才读取。
type hlsErrorTail struct{ data []byte }

func (b *hlsErrorTail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= 8192 {
		b.data = append(b.data[:0], p[n-8192:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > 8192 {
			b.data = append([]byte(nil), b.data[len(b.data)-8192:]...)
		}
	}
	return n, nil
}

func (j *hlsJob) readGrace() time.Duration {
	if j.cfg == nil {
		return hlsReadGrace
	}
	return time.Duration(j.cfg.HLSCache.ReadGraceSeconds) * time.Second
}

// 使用缓存所在文件系统的可用空间；零值禁用检查，查询失败时停止写入。
func (j *hlsJob) checkFreeSpace(ctx context.Context) error {
	if j.cfg == nil || j.cfg.HLSCache.MinFreeSpaceGB <= 0 {
		return nil
	}
	usage, err := disk.UsageWithContext(ctx, j.cacheDir)
	if err != nil {
		return fmt.Errorf("检查 HLS 剩余磁盘空间失败: %w", err)
	}
	availableGB := float64(usage.Free) / float64(uint64(1)<<30)
	if availableGB < j.cfg.HLSCache.MinFreeSpaceGB {
		return fmt.Errorf("HLS 剩余磁盘空间不足: 可用 %.2f GB，需要至少 %.2f GB", availableGB, j.cfg.HLSCache.MinFreeSpaceGB)
	}
	return nil
}

func (j *hlsJob) executeConversion(cfg *configs.Config) error {
	limit := 0
	if cfg != nil {
		limit = cfg.HLSCache.MaxConcurrent
	}
	budgetStart := time.Now()
	release, err := mediawork.Shared.Acquire(j.ctx, limit)
	j.mutex.Lock()
	j.resourceWait = time.Since(budgetStart)
	if err == nil {
		j.executionAt = time.Now()
	}
	j.mutex.Unlock()
	if err != nil {
		return err
	}
	defer release()

	// 排队不消耗执行时限；取消仍由管理器传播。长任务使用进度超时而非固定三十分钟。
	ctx, cancel := context.WithCancel(j.ctx)
	defer cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := os.RemoveAll(j.cacheDir); err != nil {
		return err
	}
	if err := os.MkdirAll(j.cacheDir, 0755); err != nil {
		return err
	}
	if err := j.checkFreeSpace(ctx); err != nil {
		return err
	}
	ffmpeg, err := findFFmpegPath(ctx, cfg)
	if err != nil {
		return err
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 15*time.Second)
	ffprobe, err := findFFprobePath(probeCtx, cfg)
	if err != nil {
		probeCancel()
		return err
	}
	probeStart := time.Now()
	probe, err := probeMediaCached(probeCtx, ffprobe, j.sourcePath, filepath.Join(filepath.Dir(hlsCacheRoot(cfg)), "media-probe"))
	probeCancel()
	applog.GetLogger().WithFields(map[string]any{"job": j.key, "probe_ms": time.Since(probeStart).Milliseconds(), "success": err == nil}).Info("HLS 媒体探测结束")
	if err != nil {
		return fmt.Errorf("媒体探测失败: %w", err)
	}
	args := hlsCopyArgs(j.sourcePath, filepath.Join(j.cacheDir, "seg_%05d.ts"), j.playlistPath)
	if hlsEncodeModeForProbe(probe) == hlsEncodeTranscode {
		args = hlsTranscodeArgs(j.sourcePath, filepath.Join(j.cacheDir, "seg_%05d.ts"), j.playlistPath)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	tail := &hlsErrorTail{}
	cmd.Stderr = tail
	if err := j.checkFreeSpace(ctx); err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	exit := make(chan error, 1)
	go func() { exit <- cmd.Wait() }()
	reaped := false
	defer func() {
		if !reaped {
			cancel()
			<-exit
		}
	}()
	spaceTicker := time.NewTicker(time.Second)
	defer spaceTicker.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	first := time.NewTimer(30 * time.Second)
	defer first.Stop()
	firstC := first.C
	progress := time.NewTimer(2 * time.Minute)
	defer progress.Stop()
	var previousSize int64 = -1
	for {
		select {
		case err = <-exit:
			reaped = true
			if err != nil {
				return fmt.Errorf("ffmpeg 退出失败: %w: %s", err, strings.TrimSpace(string(tail.data)))
			}
			_, err = readHLSPlaylist(j.playlistPath, true)
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-spaceTicker.C:
			if err := j.checkFreeSpace(ctx); err != nil {
				return err
			}
		case <-firstC:
			return fmt.Errorf("等待首批分片超时")
		case <-progress.C:
			return fmt.Errorf("HLS 两分钟未产生新清单进度")
		case <-ticker.C:
			if info, e := os.Stat(j.playlistPath); e == nil && info.Size() != previousSize {
				previousSize = info.Size()
				if !progress.Stop() {
					select {
					case <-progress.C:
					default:
					}
				}
				progress.Reset(2 * time.Minute)
			}
			if firstC != nil {
				if _, e := readHLSPlaylist(j.playlistPath, false); e == nil {
					first.Stop()
					firstC = nil
					j.playable()
				}
			}
		}
	}
}

// 校验清单实际引用；运行中验证首片，完成时校验全部引用。
func readHLSPlaylist(path string, complete bool) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "#EXTM3U" {
		return nil, fmt.Errorf("无效 HLS 清单头")
	}
	pending := false
	count := 0
	ended := false
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			if pending {
				return nil, fmt.Errorf("分片时长缺少对应 URI")
			}
			value := strings.SplitN(strings.TrimPrefix(line, "#EXTINF:"), ",", 2)[0]
			duration, e := strconv.ParseFloat(value, 64)
			if e != nil || !(duration > 0) {
				return nil, fmt.Errorf("无效分片时长")
			}
			pending = true
			continue
		}
		if line == "#EXT-X-ENDLIST" {
			ended = true
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !pending || ended || !hlsSegmentPattern.MatchString(line) || filepath.Base(line) != line || strings.ContainsAny(line, "/\\") {
			return nil, fmt.Errorf("无效分片引用")
		}
		info, e := os.Lstat(filepath.Join(filepath.Dir(path), line))
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("分片缺失或为空")
		}
		count++
		pending = false
		if !complete {
			return data, nil
		}
	}
	if count == 0 || pending || !ended {
		return nil, fmt.Errorf("HLS 清单未完整发布")
	}
	return data, nil
}
func isCompleteCache(path string) bool {
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), ".failed")); err == nil {
		return false
	}
	_, err := readHLSPlaylist(path, true)
	return err == nil
}
