package servers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

// 原路径存在时保持原文件操作语义；仅缺失的可信来源别名映射到产物。
func resolvePlaybackOperationPath(cfg *configs.Config, rel string) (string, error) {
	original, err := getSafePath(cfg.OutPutPath, rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(original); err == nil {
		return original, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if target, _, ok := preparedPlayback(cfg, rel); ok {
		return getSafePath(cfg.OutPutPath, target)
	}
	return original, nil
}

// 单文件删除/改名不连带操作其他媒体版本，只在成功后解除原关联。
// 剩余媒体按自己的真实文件名展示；已更换的关联文件不会被清理。
func playbackOperationCleanup(cfg *configs.Config, rel string) func() error {
	target, _, ok := preparedPlayback(cfg, rel)
	if !ok {
		return func() error { return nil }
	}
	abs, err := getSafePath(cfg.OutPutPath, target)
	if err != nil {
		return func() error { return nil }
	}
	sidecar := abs + playbackasset.Suffix
	before, err := os.Lstat(sidecar)
	if err != nil || !before.Mode().IsRegular() {
		return func() error { return nil }
	}
	return func() error {
		after, err := os.Lstat(sidecar)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("文件操作已完成，但关联已变化，未清理关联")
		}
		if err := os.Remove(sidecar); err != nil {
			return fmt.Errorf("文件操作已完成，但关联清理失败: %w", err)
		}
		return nil
	}
}
func renamePlaybackFile(cfg *configs.Config, rel, oldPath, newPath string) error {
	cleanup := playbackOperationCleanup(cfg, rel)
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	return cleanup()
}
func deletePlaybackFile(cfg *configs.Config, rel, absPath string) error {
	cleanup := playbackOperationCleanup(cfg, rel)
	if err := os.RemoveAll(absPath); err != nil {
		return err
	}
	return cleanup()
}

// 缩略图沿用原请求签名授权，实际读取经过安全验证的对应媒体版本。
func preparedThumbnailPath(cfg *configs.Config, rel string) string {
	if target, _, ok := preparedPlayback(cfg, rel); ok {
		return filepath.ToSlash(target)
	}
	return rel
}
