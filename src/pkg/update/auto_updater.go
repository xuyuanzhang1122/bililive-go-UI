// Package update 提供 bililive-go 的自动更新功能
package update

import (
	"context"
	"sync"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	applog "github.com/bililive-go/bililive-go/src/log"
	bilisentry "github.com/bililive-go/bililive-go/src/pkg/sentry"
)

// AutoUpdater 自动更新器，负责后台自动检查和下载更新
type AutoUpdater struct {
	manager *Manager

	mu              sync.RWMutex
	running         bool
	stopCh          chan struct{}
	lastCheckTime   time.Time
	lastCheckResult *ReleaseInfo
	lastCheckError  error

	// 状态变化回调
	onUpdateAvailable  func(info *ReleaseInfo)
	onDownloadProgress func(progress DownloadProgress)
	onUpdateReady      func(info *ReleaseInfo)
	onError            func(err error)
}

// NewAutoUpdater 创建新的自动更新器
func NewAutoUpdater(manager *Manager) *AutoUpdater {
	return &AutoUpdater{
		manager: manager,
		stopCh:  make(chan struct{}),
	}
}

// SetOnUpdateAvailable 设置检测到更新时的回调
func (a *AutoUpdater) SetOnUpdateAvailable(handler func(info *ReleaseInfo)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onUpdateAvailable = handler
}

// SetOnDownloadProgress 设置下载进度回调
func (a *AutoUpdater) SetOnDownloadProgress(handler func(progress DownloadProgress)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onDownloadProgress = handler
}

// SetOnUpdateReady 设置更新准备就绪时的回调
func (a *AutoUpdater) SetOnUpdateReady(handler func(info *ReleaseInfo)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onUpdateReady = handler
}

// SetOnError 设置错误回调
func (a *AutoUpdater) SetOnError(handler func(err error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onError = handler
}

// Start 启动自动更新器
func (a *AutoUpdater) Start(ctx context.Context) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.stopCh = make(chan struct{})
	a.mu.Unlock()

	bilisentry.GoWithContext(ctx, func(ctx context.Context) {
		a.run(ctx)
	})
}

// Stop 停止自动更新器
func (a *AutoUpdater) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	a.running = false
	close(a.stopCh)
}

// IsRunning 检查是否正在运行
func (a *AutoUpdater) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// GetLastCheckTime 获取上次检查时间
func (a *AutoUpdater) GetLastCheckTime() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastCheckTime
}

// GetLastCheckResult 获取上次检查结果
func (a *AutoUpdater) GetLastCheckResult() *ReleaseInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastCheckResult
}

// run 运行自动更新检查循环
func (a *AutoUpdater) run(ctx context.Context) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil || !cfg.Update.AutoCheck {
		applog.GetLogger().Info("自动更新检查已禁用")
		return
	}

	// 首次检查延迟 30 秒，避免启动时的资源竞争
	initialDelay := 30 * time.Second
	applog.GetLogger().Infof("自动更新器已启动，将在 %v 后进行首次检查", initialDelay)

	select {
	case <-time.After(initialDelay):
	case <-a.stopCh:
		return
	case <-ctx.Done():
		return
	}

	// 执行首次检查
	a.doCheck(ctx)

	// 计算检查间隔
	intervalHours := cfg.Update.CheckIntervalHours
	if intervalHours <= 0 {
		intervalHours = 6 // 默认 6 小时
	}
	checkInterval := time.Duration(intervalHours) * time.Hour

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	applog.GetLogger().Infof("自动更新检查间隔: %v", checkInterval)

	for {
		select {
		case <-ticker.C:
			// 重新读取配置，支持动态修改
			cfg = configs.GetCurrentConfig()
			if cfg == nil || !cfg.Update.AutoCheck {
				applog.GetLogger().Debug("自动更新检查已禁用，跳过本次检查")
				continue
			}
			a.doCheck(ctx)

		case <-a.stopCh:
			applog.GetLogger().Info("自动更新器已停止")
			return

		case <-ctx.Done():
			applog.GetLogger().Info("自动更新器因上下文取消而停止")
			return
		}
	}
}

// doCheck 执行一次更新检查
func (a *AutoUpdater) doCheck(ctx context.Context) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		return
	}

	includePrerelease := cfg.Update.IncludePrerelease
	applog.GetLogger().Infof("正在检查更新 (includePrerelease=%v)...", includePrerelease)

	info, err := a.manager.CheckForUpdate(ctx, includePrerelease)

	a.mu.Lock()
	a.lastCheckTime = time.Now()
	a.lastCheckResult = info
	a.lastCheckError = err
	a.mu.Unlock()

	if err != nil {
		applog.GetLogger().WithError(err).Warn("检查更新失败")
		a.notifyError(err)
		return
	}

	if info == nil {
		applog.GetLogger().Info("当前已是最新版本")
		return
	}

	applog.GetLogger().Infof("发现新版本: %s (当前: %s)", info.Version, a.manager.currentVer)
	a.notifyUpdateAvailable(info)

	// 如果启用了自动下载
	if cfg.Update.AutoDownload {
		a.doDownload(ctx)
	}
}

// doDownload 执行下载
func (a *AutoUpdater) doDownload(ctx context.Context) {
	applog.GetLogger().Info("开始自动下载更新...")

	// 设置进度回调
	progressCh := make(chan DownloadProgress, 10)
	a.manager.SetProgressCallback(progressCh)

	// 启动进度监听
	bilisentry.Go(func() {
		for progress := range progressCh {
			a.notifyDownloadProgress(progress)
		}
	})

	// 执行下载
	err := a.manager.DownloadUpdate(ctx)
	close(progressCh)

	if err != nil {
		applog.GetLogger().WithError(err).Error("自动下载更新失败")
		a.notifyError(err)
		return
	}

	// 下载完成
	info := a.manager.GetAvailableInfo()
	if info != nil {
		applog.GetLogger().Infof("更新下载完成: %s", info.Version)
		a.notifyUpdateReady(info)
	}
}

// TriggerCheck 手动触发一次检查
func (a *AutoUpdater) TriggerCheck(ctx context.Context) {
	bilisentry.GoWithContext(ctx, func(ctx context.Context) {
		a.doCheck(ctx)
	})
}

// 通知回调辅助方法

func (a *AutoUpdater) notifyUpdateAvailable(info *ReleaseInfo) {
	a.mu.RLock()
	handler := a.onUpdateAvailable
	a.mu.RUnlock()
	if handler != nil {
		handler(info)
	}
}

func (a *AutoUpdater) notifyDownloadProgress(progress DownloadProgress) {
	a.mu.RLock()
	handler := a.onDownloadProgress
	a.mu.RUnlock()
	if handler != nil {
		handler(progress)
	}
}

func (a *AutoUpdater) notifyUpdateReady(info *ReleaseInfo) {
	a.mu.RLock()
	handler := a.onUpdateReady
	a.mu.RUnlock()
	if handler != nil {
		handler(info)
	}
}

func (a *AutoUpdater) notifyError(err error) {
	a.mu.RLock()
	handler := a.onError
	a.mu.RUnlock()
	if handler != nil {
		handler(err)
	}
}
