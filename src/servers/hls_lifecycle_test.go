package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/stretchr/testify/require"
)

// 使用真实子进程，走生产参数、状态与 Wait 路径。
func lifecycleConfig(t *testing.T) *configs.Config {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("此辅助进程使用 POSIX 符号链接")
	}
	dir := t.TempDir()
	// race 模拟器只取消退出后的睡眠，保留所有竞态检测和原 3 秒期限。
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	// 复用已经运行的测试二进制，避免临时 shell 在执行首行前的启动延迟。
	for _, name := range []string{"ffmpeg.fixture", "ffprobe.fixture"} {
		require.NoError(t, os.Symlink(os.Args[0], filepath.Join(dir, name)))
	}
	return &configs.Config{HLSCache: configs.HLSCacheConfig{ReadGraceSeconds: 60}, FfmpegPath: filepath.Join(dir, "ffmpeg.fixture"), OutPutPath: t.TempDir(), AppDataPath: t.TempDir()}
}
func lifecycleJob(t *testing.T, m *hlsJobManager, cfg *configs.Config, name, mode string) *hlsJob {
	t.Helper()
	source := filepath.Join(cfg.OutPutPath, name+".flv")
	require.NoError(t, os.WriteFile(source, []byte(mode), 0644))
	info, err := os.Stat(source)
	require.NoError(t, err)
	key := hlsCacheKey(name+".flv", info)
	dir := filepath.Join(hlsCacheRoot(cfg), key)
	job := m.EnsureJob(cfg, key, source, dir, filepath.Join(dir, "index.m3u8"))
	// TempDir 先注册、拥有者后注册：逆序清理时先取消并等待全部写入退出。
	t.Cleanup(m.Close)
	return job
}
func awaitLifecycle(t *testing.T, job *hlsJob) hlsJobStatus {
	t.Helper()
	select {
	case <-job.done:
	case <-time.After(3 * time.Second):
		stack := make([]byte, 1<<20)
		n := runtime.Stack(stack, true)
		starts, _ := os.ReadFile(job.sourcePath + ".starts")
		t.Fatalf("作业未结束：子进程启动标记=%q\n%s", starts, stack[:n])
	}
	status, _ := job.GetStatus()
	return status
}
func TestHLSLifecycleSingleSegment(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	job := lifecycleJob(t, m, lifecycleConfig(t), "short", "short")
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, job))
}
func TestHLSLifecycleImmediateFailure(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	job := lifecycleJob(t, m, lifecycleConfig(t), "fail", "fail")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, job))
}
func TestHLSLifecyclePartial(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	job := lifecycleJob(t, m, lifecycleConfig(t), "partial", "partial")
	require.Equal(t, hlsJobPartial, awaitLifecycle(t, job))
}
func TestHLSLifecyclePlayableBeforeExit(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	job := lifecycleJob(t, m, lifecycleConfig(t), "hold", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s, err := m.WaitPlayable(ctx, job)
	require.NoError(t, err)
	require.Equal(t, hlsJobPlayable, s.status)
	select {
	case <-job.done:
		t.Fatal("尚未释放进程却已退出")
	default:
	}
	require.NoError(t, os.WriteFile(job.sourcePath+".release", nil, 0644))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, job))
}
func TestHLSCompleteCacheRejectsMissingSegments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.m3u8")
	require.NoError(t, os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:1,\nmissing.ts\n#EXT-X-ENDLIST\n"), 0644))
	require.False(t, isCompleteCache(path))
}

func TestHLSLifecycleQueueAdvances(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	first := lifecycleJob(t, m, cfg, "first", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := m.WaitPlayable(ctx, first)
	require.NoError(t, err)
	second := lifecycleJob(t, m, cfg, "second", "short")
	status, _ := second.GetStatus()
	require.Equal(t, hlsJobQueued, status)
	require.NoError(t, os.WriteFile(first.sourcePath+".release", nil, 0644))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, first))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, second))
}
func TestHLSLifecycleRetryAfterFailure(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	j := lifecycleJob(t, m, cfg, "retry", "fail")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, j))
	require.Same(t, j, m.EnsureJob(cfg, j.key, j.sourcePath, j.cacheDir, j.playlistPath))
	j.mutex.Lock()
	j.finishedAt = time.Now().Add(-2 * hlsRetryDelay)
	j.mutex.Unlock()
	require.NoError(t, os.WriteFile(j.sourcePath, []byte("short"), 0644))
	next := m.EnsureJob(cfg, j.key, j.sourcePath, j.cacheDir, j.playlistPath)
	require.NotSame(t, j, next)
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, next))
}
func TestHLSLifecycleRetryRespectsReadGrace(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	j := lifecycleJob(t, m, cfg, "retry", "partial")
	require.Equal(t, hlsJobPartial, awaitLifecycle(t, j))
	release, ok := m.acquireRead(j.key, j)
	require.True(t, ok)
	release()
	j.mutex.Lock()
	j.finishedAt = time.Now().Add(-2 * hlsRetryDelay)
	j.mutex.Unlock()
	require.Same(t, j, m.EnsureJob(cfg, j.key, j.sourcePath, j.cacheDir, j.playlistPath), "刚访问过的缓存不能被重试覆盖")
}
func TestHLSLifecycleQueueDoesNotAdmitSegmentRead(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	j := lifecycleJob(t, m, cfg, "first", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := m.WaitPlayable(ctx, j)
	require.NoError(t, err)
	queued := lifecycleJob(t, m, cfg, "queued", "short")
	release, ok := m.acquireRead(queued.key, nil)
	if ok {
		release()
	}
	require.False(t, ok, "初始化前不能接纳旧分片读取")
}
func TestHLSLifecycleShutdownReaps(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	cfg := lifecycleConfig(t)
	first := lifecycleJob(t, m, cfg, "first", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := m.WaitPlayable(ctx, first)
	require.NoError(t, err)
	queued := lifecycleJob(t, m, cfg, "queued", "short")
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("关闭未回收进程")
	}
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, queued))
	awaitLifecycle(t, first)
}
func TestHLSPlaylistRejectsTraversalAndEmpty(t *testing.T) {
	for _, uri := range []string{"../outside.ts", "missing.ts", "empty.ts"} {
		t.Run(uri, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "index.m3u8")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.ts"), nil, 0644))
			require.NoError(t, os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:1,\n"+uri+"\n#EXT-X-ENDLIST\n"), 0644))
			_, err := readHLSPlaylist(path, true)
			require.Error(t, err)
		})
	}
}

func TestHLSHTTPPlaylistReadableBeforeExit(t *testing.T) {
	cfg := lifecycleConfig(t)
	previousCfg := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	previousManager := globalHLSJobManager
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); globalHLSJobManager = previousManager; configs.SetCurrentConfig(previousCfg) })
	source := filepath.Join(cfg.OutPutPath, "http.flv")
	require.NoError(t, os.WriteFile(source, []byte("hold"), 0644))
	// 首次请求直接访问 m3u8，验证旧裸构建入口确已移除。
	req := httptest.NewRequest(http.MethodGet, "/api/stream/hls/http.flv/playlist.m3u8", nil)
	req = mux.SetURLVars(req, map[string]string{"path": "http.flv/playlist.m3u8"})
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	res := httptest.NewRecorder()
	getHLSPlaylist(res, req.WithContext(ctx))
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	info, err := os.Stat(source)
	require.NoError(t, err)
	key := hlsCacheKey("http.flv", info)
	job := m.GetJob(key)
	require.NotNil(t, job)
	select {
	case <-job.done:
		t.Fatal("首片响应前进程不应结束")
	default:
	}
	// resolver 与清单入口共享同一作业，并能够返回 ready。
	resolveReq := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/playback/resolve/http.flv", nil), map[string]string{"path": "http.flv"})
	resolveRes := httptest.NewRecorder()
	resolvePlayback(resolveRes, resolveReq)
	var result struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resolveRes.Body.Bytes(), &result))
	require.Equal(t, "ready", result.Data.Status)
	require.Same(t, job, m.GetJob(key))
	segmentReq := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/segment", nil), map[string]string{"cache_key": key, "segment": "seg_00000.ts"})
	segmentRes := httptest.NewRecorder()
	getHLSSegment(segmentRes, segmentReq)
	require.Equal(t, http.StatusOK, segmentRes.Code)
	require.Equal(t, "segment payload", segmentRes.Body.String())
	require.NoError(t, os.WriteFile(source+".release", nil, 0644))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, job))
}
func TestHLSHTTPPartialIsFailed(t *testing.T) {
	cfg := lifecycleConfig(t)
	oldCfg := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	old := globalHLSJobManager
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); globalHLSJobManager = old; configs.SetCurrentConfig(oldCfg) })
	j := lifecycleJob(t, m, cfg, "partial", "partial")
	require.Equal(t, hlsJobPartial, awaitLifecycle(t, j))
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve", nil), map[string]string{"path": "partial.flv"})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	require.Contains(t, res.Body.String(), `"status":"failed"`)
}
func TestHLSReadLeasePreventsJanitor(t *testing.T) {
	cfg := lifecycleConfig(t)
	old := globalHLSJobManager
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); globalHLSJobManager = old })
	j := lifecycleJob(t, m, cfg, "short", "short")
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, j))
	info, err := os.Stat(j.cacheDir)
	require.NoError(t, err)
	entry := hlsCacheEntry{key: j.key, path: j.cacheDir, modTime: info.ModTime()}
	release, ok := m.acquireRead(j.key, j)
	require.True(t, ok)
	removed, err := removeHLSCacheEntryIfUnchanged(entry)
	require.NoError(t, err)
	require.False(t, removed)
	release()
	removed, err = removeHLSCacheEntryIfUnchanged(entry)
	require.NoError(t, err)
	require.False(t, removed)
	m.mutex.Lock()
	m.lastRead[j.key] = time.Now().Add(-2 * hlsReadGrace)
	m.mutex.Unlock()
	removed, err = removeHLSCacheEntryIfUnchanged(entry)
	require.NoError(t, err)
	require.True(t, removed)
	require.Nil(t, m.GetJob(j.key))
}
func TestHLSReadUnknownCacheDoesNotAllocate(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob)}
	release, ok := m.acquireRead("unknown", nil)
	if ok {
		release()
	}
	require.False(t, ok)
	require.Empty(t, m.lastRead)
}

func TestHLSRealFFmpeg(t *testing.T) {
	if os.Getenv("BILIVE_HLS_INTEGRATION") != "1" {
		t.Skip("设置 BILIVE_HLS_INTEGRATION=1 执行真实 FFmpeg 验证")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	require.NoError(t, err)
	for _, codec := range []string{"libx264", "libx265"} {
		t.Run(codec, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "sample.mkv")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=128x72:rate=10", "-t", "2", "-c:v", codec, "-threads", "1"}
			if codec == "libx265" {
				args = append(args, "-x265-params", "pools=none:frame-threads=1:log-level=error")
			}
			output, err := exec.CommandContext(ctx, ffmpeg, append(args, source)...).CombinedOutput()
			require.NoError(t, err, string(output))
			cfg := &configs.Config{FfmpegPath: ffmpeg, OutPutPath: root, AppDataPath: t.TempDir()}
			m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
			t.Cleanup(m.Close)
			dir := filepath.Join(cfg.AppDataPath, "cache")
			j := m.EnsureJob(cfg, codec, source, dir, filepath.Join(dir, "index.m3u8"))
			select {
			case <-j.done:
			case <-ctx.Done():
				t.Fatal("真实转换超时")
			}
			s, e := j.GetStatus()
			require.Equal(t, hlsJobCompleted, s, e)
			probePath, err := findFFprobePath(ctx, cfg)
			require.NoError(t, err)
			probe, err := probeMedia(ctx, probePath, filepath.Join(dir, "seg_00000.ts"))
			require.NoError(t, err)
			require.NotEmpty(t, probe.Streams)
			require.Equal(t, "h264", probe.Streams[0].CodecName)
		})
	}
}

func TestHLSConcurrentEnsureStartsOneProcess(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 3}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	source := filepath.Join(cfg.OutPutPath, "same.flv")
	require.NoError(t, os.WriteFile(source, []byte("hold"), 0644))
	info, err := os.Stat(source)
	require.NoError(t, err)
	key := hlsCacheKey("same.flv", info)
	dir := filepath.Join(hlsCacheRoot(cfg), key)
	results := make(chan *hlsJob, 20)
	for i := 0; i < 20; i++ {
		go func() { results <- m.EnsureJob(cfg, key, source, dir, filepath.Join(dir, "index.m3u8")) }()
	}
	j := <-results
	for i := 1; i < 20; i++ {
		require.Same(t, j, <-results)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = m.WaitPlayable(ctx, j)
	require.NoError(t, err)
	starts, err := os.ReadFile(source + ".starts")
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(starts), "start\n"))
	require.NoError(t, os.WriteFile(source+".release", nil, 0644))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, j))
}
func TestHLSCancelledWaitKeepsJob(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	j := lifecycleJob(t, m, lifecycleConfig(t), "hold", "hold")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.WaitPlayable(ctx, j)
	require.ErrorIs(t, err, context.Canceled)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	snapshot, err := m.WaitPlayable(ctx2, j)
	require.NoError(t, err)
	require.Equal(t, hlsJobPlayable, snapshot.status)
	require.NoError(t, os.WriteFile(j.sourcePath+".release", nil, 0644))
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, j))
}
func TestHLSQueueBounded(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	first := lifecycleJob(t, m, cfg, "first", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := m.WaitPlayable(ctx, first)
	require.NoError(t, err)
	for i := 0; i < 64; i++ {
		j := lifecycleJob(t, m, cfg, fmt.Sprintf("queued%d", i), "short")
		s, _ := j.GetStatus()
		require.Equal(t, hlsJobQueued, s)
	}
	overflow := lifecycleJob(t, m, cfg, "overflow", "short")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, overflow))
	require.Nil(t, m.GetJob(overflow.key))
}
func TestHLSCompletedJobRemainsAddressable(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	t.Cleanup(m.Close)
	cfg := lifecycleConfig(t)
	j := lifecycleJob(t, m, cfg, "old", "short")
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, j))
	j.mutex.Lock()
	j.finishedAt = time.Now().Add(-2 * time.Hour)
	j.mutex.Unlock()
	next := lifecycleJob(t, m, cfg, "new", "short")
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, next))
	require.Same(t, j, m.GetJob(j.key), "完整缓存的分片不能因任务元数据提前过期而失联")
}

// 辅助作业清理必须先关闭拥有者，再让 TempDir 删除仍可能写入的目录。
func TestHLSLifecycleFixtureCleanupClosesOwnerBeforeData(t *testing.T) {
	m := &hlsJobManager{jobs: make(map[string]*hlsJob), maxConcurrent: 1}
	cfg := lifecycleConfig(t)
	var job *hlsJob
	t.Cleanup(func() {
		finished := false
		select {
		case <-job.done:
			finished = true
		default:
		}
		dir := filepath.Join(hlsCacheRoot(cfg), "after-cleanup")
		next := m.EnsureJob(cfg, "after-cleanup", job.sourcePath, dir, filepath.Join(dir, "index.m3u8"))
		status, _ := next.GetStatus()
		// 即使断言失败，也须在 TempDir 清理之前回收本测试拥有的工作。
		m.Close()
		require.True(t, finished, "作业清理返回前必须完成回收")
		require.Equal(t, hlsJobFailed, status, "清理后拥有者不能再接纳新作业")
	})
	job = lifecycleJob(t, m, cfg, "cleanup", "hold")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := m.WaitPlayable(ctx, job)
	require.NoError(t, err)
}
