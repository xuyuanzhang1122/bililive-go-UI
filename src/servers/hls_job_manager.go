package servers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	applog "github.com/bililive-go/bililive-go/src/log"
)

// HLS 任务状态
type hlsJobStatus string

const (
	hlsJobQueued    hlsJobStatus = "queued"    // 等待资源
	hlsJobStarting  hlsJobStatus = "starting"  // 探测或启动中，暂无首片
	hlsJobPlayable  hlsJobStatus = "playable"  // 已有首批分片，任务仍在运行
	hlsJobCompleted hlsJobStatus = "completed" // 正常结束，完整产物验证通过
	hlsJobFailed    hlsJobStatus = "failed"    // 失败且无可用内容
	hlsJobPartial   hlsJobStatus = "partial"   // 仅有部分可用内容
)

// hlsJob 代表一个后台 HLS 转换任务
type hlsJob struct {
	key         string
	sourcePath  string
	cacheDir    string
	playlistPath string

	ctx         context.Context
	cancel      context.CancelFunc
	cmd         *exec.Cmd

	status      hlsJobStatus
	err         string
	startedAt   time.Time

	mutex       sync.Mutex
}

// hlsJobManager 管理所有 HLS 转换任务
type hlsJobManager struct {
	jobs    map[string]*hlsJob
	mutex   sync.RWMutex

	maxConcurrent int
	running       int
}

var globalHLSJobManager = &hlsJobManager{
	jobs:          make(map[string]*hlsJob),
	maxConcurrent: 3, // 默认最多 3 个并发转码任务
}

// GetOrCreateJob 获取或创建 HLS 转换任务
func (m *hlsJobManager) GetOrCreateJob(cfg *configs.Config, cacheKey, sourcePath, cacheDir, playlistPath string) (*hlsJob, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 已存在的任务
	if job, exists := m.jobs[cacheKey]; exists {
		return job, false
	}

	// 创建新任务
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	job := &hlsJob{
		key:          cacheKey,
		sourcePath:   sourcePath,
		cacheDir:     cacheDir,
		playlistPath: playlistPath,
		ctx:          ctx,
		cancel:       cancel,
		status:       hlsJobQueued,
		startedAt:    time.Now(),
	}

	m.jobs[cacheKey] = job
	return job, true
}

// GetJob 获取已存在的任务
func (m *hlsJobManager) GetJob(cacheKey string) *hlsJob {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.jobs[cacheKey]
}

// RemoveJob 移除任务
func (m *hlsJobManager) RemoveJob(cacheKey string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if job, exists := m.jobs[cacheKey]; exists {
		job.cancel()
		delete(m.jobs, cacheKey)
	}
}

// StartJob 启动任务
func (m *hlsJobManager) StartJob(cfg *configs.Config, job *hlsJob) error {
	m.mutex.Lock()
	// 检查并发限制
	if m.running >= m.maxConcurrent {
		m.mutex.Unlock()
		job.setStatus(hlsJobQueued, "")
		return fmt.Errorf("达到最大并发转码限制")
	}
	m.running++
	m.mutex.Unlock()

	// 标记为启动中
	job.setStatus(hlsJobStarting, "")

	// 后台执行任务
	go func() {
		defer func() {
			m.mutex.Lock()
			m.running--
			m.mutex.Unlock()

			job.cancel()

			if r := recover(); r != nil {
				applog.GetLogger().WithField("panic", r).Error("HLS 任务 goroutine panic")
				job.setStatus(hlsJobFailed, fmt.Sprintf("panic: %v", r))
			}
		}()

		// 执行转换
		err := job.executeConversion(cfg)

		if err != nil {
			// 检查是否有部分可用内容
			if isPlaylistReady(job.playlistPath) {
				job.setStatus(hlsJobPartial, err.Error())
				applog.GetLogger().WithError(err).WithField("source", filepath.Base(job.sourcePath)).Warn("HLS 转换未完成但有部分可用内容")
			} else {
				job.setStatus(hlsJobFailed, err.Error())
				applog.GetLogger().WithError(err).WithField("source", filepath.Base(job.sourcePath)).Error("HLS 转换失败")
			}
			return
		}

		// 验证完整性
		if err := job.verifyCompletion(); err != nil {
			job.setStatus(hlsJobPartial, err.Error())
			applog.GetLogger().WithError(err).WithField("source", filepath.Base(job.sourcePath)).Warn("HLS 转换完成但验证失败")
			return
		}

		job.setStatus(hlsJobCompleted, "")
		applog.GetLogger().WithField("source", filepath.Base(job.sourcePath)).Info("HLS 转换完成")
	}()

	return nil
}

// executeConversion 执行 ffmpeg 转换
func (job *hlsJob) executeConversion(cfg *configs.Config) error {
	lock := acquireHLSCacheLock(job.key)
	lock.mutex.Lock()
	defer lock.mutex.Unlock()
	defer lock.users.Add(-1)

	// 清理旧缓存
	if err := os.RemoveAll(job.cacheDir); err != nil {
		return fmt.Errorf("清理 HLS 缓存失败: %w", err)
	}
	if err := os.MkdirAll(job.cacheDir, 0755); err != nil {
		return fmt.Errorf("创建 HLS 缓存目录失败: %w", err)
	}

	// 查找 ffmpeg
	ffmpegPath, err := findFFmpegPath(job.ctx, cfg)
	if err != nil {
		return err
	}

	// 构建命令参数
	segmentPattern := filepath.Join(job.cacheDir, "seg_%05d.ts")
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-probesize", "1000000",
		"-analyzeduration", "1000000",
		"-i", job.sourcePath,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		"-hls_flags", "independent_segments+temp_file",
		"-hls_segment_filename", segmentPattern,
		job.playlistPath,
	}

	// 探测媒体并决定转码模式
	if ffprobePath, probeErr := findFFprobePath(job.ctx, cfg); probeErr == nil {
		if probe, probeErr := probeMedia(job.ctx, ffprobePath, job.sourcePath); probeErr == nil {
			if hlsEncodeModeForProbe(probe) == hlsEncodeTranscode {
				applog.GetLogger().WithFields(map[string]interface{}{
					"source": filepath.Base(job.sourcePath),
					"format": probe.Format.FormatName,
				}).Info("媒体不满足跨平台 HLS 直封装条件，改用 H.264/AAC 转码")
				args = hlsTranscodeArgs(job.sourcePath, segmentPattern, job.playlistPath)
			}
		} else {
			applog.GetLogger().WithError(probeErr).WithField("source", filepath.Base(job.sourcePath)).Warn("媒体探测失败，回退到 HLS 直封装")
		}
	}

	// 启动 ffmpeg
	job.mutex.Lock()
	job.cmd = exec.CommandContext(job.ctx, ffmpegPath, args...)
	job.mutex.Unlock()

	if err := job.cmd.Start(); err != nil {
		return fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}

	// 等待首批分片生成
	if err := job.waitForFirstSegments(); err != nil {
		_ = job.cmd.Process.Kill()
		_ = job.cmd.Wait()
		return fmt.Errorf("等待首批分片超时: %w", err)
	}

	// 标记为可播放
	job.setStatus(hlsJobPlayable, "")

	// 等待 ffmpeg 完成
	err = job.cmd.Wait()

	// 检查是否需要手动添加 ENDLIST
	if err == nil {
		job.ensureEndList()
	}

	return err
}

// waitForFirstSegments 等待首批分片生成
func (job *hlsJob) waitForFirstSegments() error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("等待首批分片超时")
		case <-job.ctx.Done():
			return job.ctx.Err()
		case <-ticker.C:
			// 检查清单文件是否存在且可解析
			if !isPlaylistReadable(job.playlistPath) {
				continue
			}

			// 检查是否有至少 2 个分片可访问
			if hasMinimumSegments(job.cacheDir, 2) {
				return nil
			}
		}
	}
}

// ensureEndList 确保播放列表有 ENDLIST 标记
func (job *hlsJob) ensureEndList() {
	content, err := os.ReadFile(job.playlistPath)
	if err != nil {
		return
	}

	// 已有 ENDLIST，无需添加
	if contains(content, []byte("#EXT-X-ENDLIST")) {
		return
	}

	// 手动追加 ENDLIST
	f, err := os.OpenFile(job.playlistPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		applog.GetLogger().WithError(err).Warn("无法打开播放列表添加 ENDLIST")
		return
	}
	defer f.Close()

	_, _ = f.WriteString("#EXT-X-ENDLIST\n")
}

// verifyCompletion 验证转换完整性
func (job *hlsJob) verifyCompletion() error {
	// 检查播放列表是否有 ENDLIST
	content, err := os.ReadFile(job.playlistPath)
	if err != nil {
		return fmt.Errorf("无法读取播放列表: %w", err)
	}

	if !contains(content, []byte("#EXT-X-ENDLIST")) {
		return fmt.Errorf("播放列表缺少 ENDLIST 标记")
	}

	// 检查是否有足够的分片
	if !hasMinimumSegments(job.cacheDir, 1) {
		return fmt.Errorf("没有可用的分片文件")
	}

	return nil
}

// setStatus 设置任务状态
func (job *hlsJob) setStatus(status hlsJobStatus, errMsg string) {
	job.mutex.Lock()
	defer job.mutex.Unlock()
	job.status = status
	job.err = errMsg
}

// GetStatus 获取任务状态
func (job *hlsJob) GetStatus() (hlsJobStatus, string) {
	job.mutex.Lock()
	defer job.mutex.Unlock()
	return job.status, job.err
}

// IsActive 任务是否活跃（正在运行或队列中）
func (job *hlsJob) IsActive() bool {
	job.mutex.Lock()
	defer job.mutex.Unlock()
	return job.status == hlsJobQueued || job.status == hlsJobStarting || job.status == hlsJobPlayable
}

// 辅助函数

func isPlaylistReadable(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// 检查是否是有效的 M3U8
	return contains(content, []byte("#EXTM3U"))
}

func hasMinimumSegments(cacheDir string, minCount int) bool {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return false
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".ts" {
			// 检查文件大小，避免空文件
			info, err := entry.Info()
			if err != nil || info.Size() < 1024 {
				continue
			}
			count++
			if count >= minCount {
				return true
			}
		}
	}

	return false
}

func contains(data []byte, pattern []byte) bool {
	if len(pattern) == 0 {
		return true
	}
	if len(data) < len(pattern) {
		return false
	}
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
