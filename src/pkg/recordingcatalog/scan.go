package recordingcatalog

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

var videoExtensions = map[string]bool{".flv": true, ".mp4": true, ".ts": true, ".mkv": true, ".mov": true, ".webm": true}

type observedFile struct {
	path     string
	room     string
	identity string
	size     int64
	mtime    int64
	manifest *playbackasset.Manifest
}

func version(identity string, size, mtime int64) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", identity, size, mtime))))
}

// 扫描只能由后台/录制完成事件调用，任何错误均拒绝部分快照。
func scan(ctx context.Context, root, assetRoot string) ([]observedFile, error) {
	files := make([]observedFile, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() && path == assetRoot {
			return filepath.SkipDir
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !videoExtensions[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parts := strings.Split(relative, "/")
		if len(parts) < 3 {
			return nil
		}
		identity, err := fileIdentity(path, info)
		if err != nil {
			return err
		}
		// 不允许身份读取期间的大小/版本变化冒充稳定快照。
		after, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("核验期间文件变化: %s", relative)
		}
		file := observedFile{path: relative, room: strings.Join(parts[:2], "/"), identity: identity, size: info.Size(), mtime: info.ModTime().UnixNano()}
		if strings.EqualFold(filepath.Ext(path), ".mp4") {
			if manifest, err := playbackasset.Load(path); err == nil && manifest.Version == 1 {
				file.manifest = &manifest
			}
		}
		files = append(files, file)
		return nil
	})
	return files, err
}
