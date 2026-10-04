package servers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHLSResourceConfiguredQueue(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.MaxConcurrent = 1
	cfg.HLSCache.MaxQueued = 1
	m := &hlsJobManager{maxConcurrent: 3}
	defer m.Close()
	first := lifecycleJob(t, m, cfg, "first", "hold")
	_, err := m.WaitPlayable(context.Background(), first)
	require.NoError(t, err)
	second := lifecycleJob(t, m, cfg, "second", "short")
	status, _ := second.GetStatus()
	require.Equal(t, hlsJobQueued, status)
	third := lifecycleJob(t, m, cfg, "third", "short")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, third))
	_, message := third.GetStatus()
	require.Contains(t, message, "队列已满")
}

func TestHLSResourceLowSpacePreventsStart(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.MinFreeSpaceGB = 1e20
	m := &hlsJobManager{}
	defer m.Close()
	job := lifecycleJob(t, m, cfg, "low-space", "short")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, job))
	_, message := job.GetStatus()
	require.Contains(t, message, "剩余磁盘空间")
	_, err := os.Stat(job.sourcePath + ".starts")
	require.True(t, os.IsNotExist(err), "低空间时不能启动 FFmpeg")
}

func TestHLSResourceZeroGraceAllowsRetry(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.ReadGraceSeconds = 0
	m := &hlsJobManager{}
	defer m.Close()
	job := lifecycleJob(t, m, cfg, "retry-zero", "fail")
	require.Equal(t, hlsJobFailed, awaitLifecycle(t, job))
	job.mutex.Lock()
	job.finishedAt = time.Now().Add(-2 * hlsRetryDelay)
	job.mutex.Unlock()
	m.mutex.Lock()
	m.lastRead = map[string]time.Time{job.key: time.Now()}
	m.mutex.Unlock()
	replacement := m.EnsureJob(cfg, job.key, job.sourcePath, job.cacheDir, job.playlistPath)
	require.NotSame(t, job, replacement)
}

func TestHLSResourceConfiguredConcurrentAndTiming(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.MaxConcurrent = 2
	m := &hlsJobManager{maxConcurrent: 1}
	defer m.Close()
	first := lifecycleJob(t, m, cfg, "parallel-first", "hold")
	second := lifecycleJob(t, m, cfg, "parallel-second", "hold")
	for _, job := range []*hlsJob{first, second} {
		snapshot, err := m.WaitPlayable(context.Background(), job)
		require.NoError(t, err)
		require.Equal(t, hlsJobPlayable, snapshot.status)
		require.NoError(t, os.WriteFile(job.sourcePath+".release", nil, 0644))
		require.Equal(t, hlsJobCompleted, awaitLifecycle(t, job))
		job.mutex.Lock()
		queued, started, playable, finished := job.queuedAt, job.startedAt, job.playableAt, job.finishedAt
		job.mutex.Unlock()
		require.False(t, queued.IsZero())
		require.False(t, started.Before(queued))
		require.False(t, playable.Before(started))
		require.False(t, finished.Before(playable))
	}
}

func TestHLSResourceZeroGraceStillProtectsReaders(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.ReadGraceSeconds = 0
	old := globalHLSJobManager
	m := &hlsJobManager{}
	globalHLSJobManager = m
	defer func() { m.Close(); globalHLSJobManager = old }()
	job := lifecycleJob(t, m, cfg, "reader-zero", "short")
	require.Equal(t, hlsJobCompleted, awaitLifecycle(t, job))
	info, err := os.Stat(job.cacheDir)
	require.NoError(t, err)
	entry := hlsCacheEntry{key: job.key, path: job.cacheDir, modTime: info.ModTime()}
	release, ok := m.acquireRead(job.key, job)
	require.True(t, ok)
	removed, err := removeHLSCacheEntryIfUnchanged(entry)
	require.NoError(t, err)
	require.False(t, removed)
	release()
	removed, err = removeHLSCacheEntryIfUnchanged(entry)
	require.NoError(t, err)
	require.True(t, removed)
}
