package servers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	applog "github.com/bililive-go/bililive-go/src/log"
	bilisentry "github.com/bililive-go/bililive-go/src/pkg/sentry"
)

const hlsCacheJanitorInterval = 30 * time.Minute

type hlsCacheEntry struct {
	key     string
	path    string
	modTime time.Time
	size    int64
}

type hlsCacheJanitorResult struct {
	DeletedDirectories int
	FreedBytes         int64
}

// StartHLSCacheJanitor 启动 HLS 播放缓存生命周期管理。
// 缓存可随时重建，因此每次进程启动都先清空，随后按配置定期回收。
func StartHLSCacheJanitor(ctx context.Context) {
	cfg := configs.GetCurrentConfig()
	root := hlsCacheRoot(cfg)
	if err := resetHLSCache(root); err != nil {
		applog.GetLogger().WithError(err).Error("重置 HLS 播放缓存失败")
	} else {
		applog.GetLogger().Info("HLS 播放缓存已在启动时清空")
	}

	bilisentry.GoWithContext(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(hlsCacheJanitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := configs.GetCurrentConfig()
				if current == nil {
					continue
				}
				result, err := runHLSCacheJanitor(
					hlsCacheRoot(current),
					current.HLSCache.MaxAgeHours,
					current.HLSCache.MaxTotalSizeGB,
					time.Now(),
				)
				if err != nil {
					applog.GetLogger().WithError(err).Warn("回收 HLS 播放缓存时部分操作失败")
				}
				if result.DeletedDirectories > 0 {
					applog.GetLogger().WithFields(map[string]any{
						"directories": result.DeletedDirectories,
						"freed_bytes": result.FreedBytes,
					}).Info("HLS 播放缓存回收完成")
				}
			}
		}
	})
}

func resetHLSCache(root string) error {
	removeErr := os.RemoveAll(root)
	mkdirErr := os.MkdirAll(root, 0o755)
	return errors.Join(removeErr, mkdirErr)
}

func runHLSCacheJanitor(root string, maxAgeHours int, maxTotalSizeGB float64, now time.Time) (hlsCacheJanitorResult, error) {
	var result hlsCacheJanitorResult
	entries, err := scanHLSCacheEntries(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cleanupMissingHLSCacheLocks(root)
			return result, nil
		}
		return result, err
	}

	var errs []error
	remaining := make([]hlsCacheEntry, 0, len(entries))
	totalSize := int64(0)
	for _, entry := range entries {
		expired := maxAgeHours > 0 && now.Sub(entry.modTime).Hours() > float64(maxAgeHours)
		if expired {
			removed, err := removeHLSCacheEntryIfUnchanged(entry)
			if err != nil {
				errs = append(errs, err)
				remaining = append(remaining, entry)
				totalSize += entry.size
				continue
			}
			if !removed {
				remaining = append(remaining, entry)
				totalSize += entry.size
				continue
			}
			result.DeletedDirectories++
			result.FreedBytes += entry.size
			continue
		}
		remaining = append(remaining, entry)
		totalSize += entry.size
	}

	if maxTotalSizeGB > 0 {
		limitBytes := maxTotalSizeGB * float64(int64(1)<<30)
		sort.Slice(remaining, func(i, j int) bool {
			return remaining[i].modTime.Before(remaining[j].modTime)
		})
		for _, entry := range remaining {
			if float64(totalSize) <= limitBytes {
				break
			}
			removed, err := removeHLSCacheEntryIfUnchanged(entry)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if !removed {
				continue
			}
			totalSize -= entry.size
			result.DeletedDirectories++
			result.FreedBytes += entry.size
		}
	}

	cleanupMissingHLSCacheLocks(root)
	return result, errors.Join(errs...)
}

// removeHLSCacheEntryIfUnchanged 避免 janitor 删除正在生成或刚被访问的缓存。
// 扫描后的目录 mtime 若已变化，说明该条目已经被使用，本轮跳过并留待下次重算。
func removeHLSCacheEntryIfUnchanged(entry hlsCacheEntry) (bool, error) {
	lock := acquireHLSCacheLock(entry.key)
	lock.mutex.Lock()
	info, statErr := os.Stat(entry.path)
	if statErr != nil {
		lock.mutex.Unlock()
		lock.users.Add(-1)
		if errors.Is(statErr, fs.ErrNotExist) {
			deleteHLSCacheLockIfUnused(entry.key)
			return true, nil
		}
		return false, statErr
	}
	if !info.ModTime().Equal(entry.modTime) {
		lock.mutex.Unlock()
		lock.users.Add(-1)
		return false, nil
	}
	removeErr := os.RemoveAll(entry.path)
	lock.mutex.Unlock()
	lock.users.Add(-1)
	if removeErr == nil {
		deleteHLSCacheLockIfUnused(entry.key)
	}
	return removeErr == nil, removeErr
}

func scanHLSCacheEntries(root string) ([]hlsCacheEntry, error) {
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	entries := make([]hlsCacheEntry, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(root, dirEntry.Name())
		entries = append(entries, hlsCacheEntry{
			key:     dirEntry.Name(),
			path:    path,
			modTime: info.ModTime(),
			size:    hlsCacheDirectorySize(path),
		})
	}
	return entries, nil
}

func hlsCacheDirectorySize(root string) int64 {
	var size int64
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil && info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func cleanupMissingHLSCacheLocks(root string) {
	hlsCacheLocks.Range(func(key, _ any) bool {
		cacheKey, ok := key.(string)
		if !ok {
			return true
		}
		if _, err := os.Stat(filepath.Join(root, cacheKey)); errors.Is(err, fs.ErrNotExist) {
			deleteHLSCacheLockIfUnused(cacheKey)
		}
		return true
	})
}
