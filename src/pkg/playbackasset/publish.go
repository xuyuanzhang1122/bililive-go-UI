package playbackasset

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const IntentSuffix = ".publishing.json"

type publicationIntent struct {
	Manifest
	TempName string `json:"temp_name"`
}

func removePublicationFile(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(expected, current) || stamp(expected) != stamp(current) {
		return fmt.Errorf("文件已被替换，拒绝回滚: %s", path)
	}
	return os.Remove(path)
}

// PublishValidated 仅接受调用方已经验证并同步到磁盘的唯一临时文件。
// 来源和产物版本先写入不可覆盖的发布意图，再发布媒体与关联。
func PublishValidated(source, target, temp string, sourceInfo os.FileInfo) error {
	return publishValidated(source, target, temp, sourceInfo, false)
}

// PublishDerivedValidated 复用持久发布意图及不可覆盖发布，派生关联禁止删源。
func PublishDerivedValidated(source, target, temp string, sourceInfo os.FileInfo) error {
	return publishValidated(source, target, temp, sourceInfo, true)
}

func publishValidated(source, target, temp string, sourceInfo os.FileInfo, derived bool) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	temp, err = filepath.Abs(temp)
	if err != nil {
		return err
	}
	if sourceInfo == nil || (!derived && filepath.Dir(source) != filepath.Dir(target)) || filepath.Dir(temp) != filepath.Dir(target) || source == target || temp == target || temp == source || !strings.HasPrefix(filepath.Base(temp), ".converting-") {
		return fmt.Errorf("无效的播放产物发布路径")
	}
	current, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(sourceInfo, current) || stamp(current) != stamp(sourceInfo) {
		return fmt.Errorf("发布前来源已变化")
	}
	tempInfo, err := os.Lstat(temp)
	if err != nil {
		return err
	}
	if !tempInfo.Mode().IsRegular() || tempInfo.Size() == 0 {
		return fmt.Errorf("发布临时文件无效")
	}
	intent := publicationIntent{Manifest: makeManifest(source, target, sourceInfo, tempInfo, derived), TempName: filepath.Base(temp)}
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	journal, err := os.CreateTemp(filepath.Dir(target), ".publication-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(journal.Name())
	if _, err = journal.Write(data); err != nil {
		journal.Close()
		return err
	}
	if err = journal.Sync(); err != nil {
		journal.Close()
		return err
	}
	info, err := journal.Stat()
	if err != nil {
		journal.Close()
		return err
	}
	if err = journal.Close(); err != nil {
		return err
	}
	intentPath := target + IntentSuffix
	if err = os.Link(journal.Name(), intentPath); err != nil {
		return fmt.Errorf("已有发布意图，拒绝覆盖: %w", err)
	}
	if err = syncPublicationDir(intentPath); err != nil {
		_ = removePublicationFile(intentPath, info)
		return err
	}
	if err = os.Link(temp, target); err != nil {
		// 并发恢复可能已发布同一文件；仅同一 inode 允许继续。
		existing, e := os.Lstat(target)
		if e != nil || !os.SameFile(tempInfo, existing) {
			_ = removePublicationFile(intentPath, info)
			return fmt.Errorf("原子发布失败: %w", err)
		}
	}
	return finishPublication(source, target, tempInfo, sourceInfo, intentPath, info, derived)
}

func finishPublication(source, target string, targetInfo, sourceInfo os.FileInfo, intentPath string, intentInfo os.FileInfo, derived bool) error {
	expected := makeManifest(source, target, sourceInfo, targetInfo, derived)
	current, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !os.SameFile(current, targetInfo) || stamp(current) != expected.Target {
		return fmt.Errorf("发布提交前产物发生变化")
	}
	if err := saveManifest(source, target, sourceInfo, derived); err != nil {
		// 另一恢复流程可能已经提交同一来源、同一产物版本。只有完整
		// Manifest 精确匹配时才允许幂等收尾，不能仅凭 Load 成功判断。
		if existing, loadErr := Load(target); loadErr == nil {
			if existing == expected {
				if removeErr := removePublicationFile(intentPath, intentInfo); removeErr != nil {
					return fmt.Errorf("并发发布已完成，但清理发布意图失败: %w", removeErr)
				}
				return syncPublicationDir(target)
			}
			// 目标已被其他来源可信提交，绝不能把它当作本流程产物回滚。
			return fmt.Errorf("目标已有不同来源的播放关联")
		}
		// 仅回滚本次创建的文件，不碰同名替换文件，更不删除源文件。
		rollbackErr := removePublicationFile(target, targetInfo)
		if rollbackErr == nil {
			_ = removePublicationFile(intentPath, intentInfo)
		}
		if rollbackErr != nil {
			return fmt.Errorf("保存关联失败: %w；回滚失败: %v", err, rollbackErr)
		}
		return fmt.Errorf("保存关联失败，已回滚本次产物: %w", err)
	}
	if err := syncPublicationDir(target); err != nil {
		return err
	}
	if err := removePublicationFile(intentPath, intentInfo); err != nil {
		return err
	}
	return syncPublicationDir(target)
}

// RecoverPublication 只恢复与持久化发布意图完全匹配的版本，并强制重新验证媒体。
// 无意图时返回 false；不根据同名、时长或扩展名推测产物来源。
func RecoverPublication(source, target string, sourceInfo os.FileInfo, validate func(string) error) (bool, error) {
	return recoverPublication(source, target, sourceInfo, validate, false)
}

func RecoverDerivedPublication(source, target string, sourceInfo os.FileInfo, validate func(string) error) (bool, error) {
	return recoverPublication(source, target, sourceInfo, validate, true)
}

func recoverPublication(source, target string, sourceInfo os.FileInfo, validate func(string) error, derived bool) (bool, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return false, err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false, err
	}
	intentPath := target + IntentSuffix
	f, err := os.Open(intentPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	intentInfo, err := f.Stat()
	if err != nil {
		f.Close()
		return false, err
	}
	var intent publicationIntent
	err = json.NewDecoder(io.LimitReader(f, 8192)).Decode(&intent)
	f.Close()
	if err != nil {
		return false, err
	}
	if sourceInfo == nil || validate == nil || (intent.Version != 1 && !derived) || (derived && (intent.Version != 2 || intent.SourcePath != source)) || (!derived && filepath.Dir(source) != filepath.Dir(target)) || intent.SourceName != filepath.Base(source) || intent.TargetName != filepath.Base(target) || !safeName(intent.TempName) || !strings.HasPrefix(intent.TempName, ".converting-") || stamp(sourceInfo) != intent.Source {
		return false, fmt.Errorf("发布意图不匹配来源")
	}
	current, err := os.Lstat(source)
	if err != nil {
		return false, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(sourceInfo, current) || stamp(current) != intent.Source {
		return false, fmt.Errorf("发布来源已变化")
	}
	temp := filepath.Join(filepath.Dir(target), intent.TempName)
	candidate := target
	targetInfo, err := os.Lstat(target)
	missing := os.IsNotExist(err)
	if missing {
		candidate = temp
		targetInfo, err = os.Lstat(temp)
	}
	if err != nil {
		return false, err
	}
	if !targetInfo.Mode().IsRegular() || stamp(targetInfo) != intent.Target {
		return false, fmt.Errorf("发布产物版本不匹配意图")
	}
	// 临时文件仍存在时，用硬链接身份补强版本检查。
	if tempInfo, e := os.Lstat(temp); e == nil && (!tempInfo.Mode().IsRegular() || !os.SameFile(tempInfo, targetInfo)) {
		return false, fmt.Errorf("发布临时文件身份不匹配")
	}
	if err = validate(candidate); err != nil {
		return false, err
	}
	after, err := os.Lstat(candidate)
	if err != nil {
		return false, err
	}
	if !os.SameFile(after, targetInfo) || stamp(after) != intent.Target {
		return false, fmt.Errorf("验证期间产物发生变化")
	}
	if missing {
		if err = os.Link(temp, target); err != nil {
			existing, e := os.Lstat(target)
			if e != nil || !os.SameFile(targetInfo, existing) {
				return false, err
			}
		}
	}
	if err = finishPublication(source, target, targetInfo, sourceInfo, intentPath, intentInfo, derived); err != nil {
		return false, err
	}
	if tempInfo, e := os.Lstat(temp); e == nil && os.SameFile(tempInfo, targetInfo) {
		_ = removePublicationFile(temp, tempInfo)
	}
	return true, nil
}
