package playbackasset

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const SourceDeletionSuffix = ".deleting-source.json"

type sourceDeletionIntent struct {
	Manifest
	DeleteSource bool `json:"delete_source"`
}

func readSourceDeletionIntent(target string) (sourceDeletionIntent, os.FileInfo, error) {
	var intent sourceDeletionIntent
	f, err := os.Open(target + SourceDeletionSuffix)
	if err != nil {
		return intent, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return intent, nil, err
	}
	err = json.NewDecoder(io.LimitReader(f, 8192)).Decode(&intent)
	return intent, info, err
}

func saveSourceDeletionIntent(target string, expected sourceDeletionIntent) (os.FileInfo, error) {
	if existing, info, err := readSourceDeletionIntent(target); err == nil {
		if existing != expected {
			return nil, fmt.Errorf("已有不同版本或策略的删源意图")
		}
		return info, syncPublicationDir(target)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	data, err := json.Marshal(expected)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".source-deletion-*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	info, err := f.Stat()
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := os.Link(f.Name(), target+SourceDeletionSuffix); err != nil {
		// 并发写入只接受完整相同意图，不能覆盖别的任务。
		existing, existingInfo, readErr := readSourceDeletionIntent(target)
		if readErr != nil || existing != expected {
			return nil, fmt.Errorf("无法发布删源意图: %w", err)
		}
		info = existingInfo
	}
	return info, syncPublicationDir(target)
}

func finishSourceDeletion(target string, info os.FileInfo) error {
	if err := syncPublicationDir(target); err != nil {
		return err
	}
	if err := removePublicationFile(target+SourceDeletionSuffix, info); err != nil {
		return err
	}
	return syncPublicationDir(target)
}

// DeleteSourceValidated 仅供显式删源策略调用。发布成功不代表稍后仍可安全删源，
// 此处重新核验来源、目标和持久关联，并在删除前同步意图。跨文件复核与删除
// 仍不是原子事务，不能对外部进程同时替换文件作无条件保证。
func DeleteSourceValidated(source, target string, sourceInfo os.FileInfo) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if sourceInfo == nil || !sourceInfo.Mode().IsRegular() || source == target || filepath.Dir(source) != filepath.Dir(target) {
		return fmt.Errorf("无效的删源路径或来源身份")
	}
	manifest, err := Load(target)
	if err != nil {
		return fmt.Errorf("删除前播放产物或关联失效，已保留源文件: %w", err)
	}
	if manifest.Version != 1 || manifest.SourceName != filepath.Base(source) || manifest.Source != stamp(sourceInfo) {
		return fmt.Errorf("删除前播放关联不匹配来源，已保留源文件")
	}
	intentInfo, err := saveSourceDeletionIntent(target, sourceDeletionIntent{Manifest: manifest, DeleteSource: true})
	if err != nil {
		return err
	}
	if current, err := Load(target); err != nil || current != manifest {
		return fmt.Errorf("持久化删源意图后产物或关联变化，已保留源文件")
	}
	// removePublicationFile 使用 Lstat 和 SameFile，拒绝软链接及同名替换来源。
	if err := removePublicationFile(source, sourceInfo); err != nil {
		return fmt.Errorf("删除前来源复核或删除失败: %w", err)
	}
	return finishSourceDeletion(target, intentInfo)
}

// RecoverSourceDeletion 只收尾已删除来源的完整相同提交；来源仍在时需由
// 调用方重新验证媒体并执行显式删除策略，不在这里盲目恢复删除。
func RecoverSourceDeletion(target string) (bool, error) {
	target, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	intent, info, err := readSourceDeletionIntent(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	manifest, err := Load(target)
	if err != nil {
		return false, err
	}
	if manifest.Version != 1 || !intent.DeleteSource || intent.Manifest != manifest {
		return false, fmt.Errorf("删源恢复意图与已提交产物不匹配")
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(target), manifest.SourceName)); !os.IsNotExist(err) {
		if err != nil {
			return false, err
		}
		return false, fmt.Errorf("来源仍存在，必须重新验证后显式删源")
	}
	if err := finishSourceDeletion(target, info); err != nil {
		return false, err
	}
	return true, nil
}
