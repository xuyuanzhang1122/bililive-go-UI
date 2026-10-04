package hlsasset

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

func sameBuildOwner(a, b Manifest) bool {
	return a.Version == 1 && b.Version == 1 && a.RecordingID == b.RecordingID && a.SourceVersion == b.SourceVersion && a.Profile == b.Profile
}

// NewBuild 在写媒体前持久化归属，进程中断后只清理本任务明确拥有的未发布集合。
func NewBuild(root string, owner Manifest) (string, error) {
	dir, err := os.MkdirTemp(root, ".hls-build-")
	if err != nil {
		return "", err
	}
	owner.DataDir = filepath.Base(dir)
	data, err := json.Marshal(owner)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	path := filepath.Join(dir, ".build-owner.json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	for _, err := range []error{writeErr, syncErr, closeErr, playbackasset.SyncDirectory(path), playbackasset.SyncDirectory(dir)} {
		if err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

// CleanAbandoned 只能在已认领对应持久任务后调用；保护已提交、intent 引用、其他归属和未知目录。
func CleanAbandoned(root string, owner Manifest) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("构建根目录不可信")
	}
	protected := make(map[string]bool)
	for _, name := range []string{".hls.json", ".hls-intent.json"} {
		m, err := readManifest(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !sameBuildOwner(m, owner) {
			return fmt.Errorf("已发布 HLS 不属于当前构建")
		}
		protected[m.DataDir] = true
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".hls-build-") || protected[entry.Name()] {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		f, err := regularFile(filepath.Join(dir, ".build-owner.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		var previous Manifest
		decodeErr := json.NewDecoder(io.LimitReader(f, 16<<10)).Decode(&previous)
		closeErr := f.Close()
		if decodeErr != nil {
			continue
		}
		if closeErr != nil {
			return closeErr
		}
		if !sameBuildOwner(previous, owner) || previous.DataDir != entry.Name() {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := playbackasset.SyncDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}
