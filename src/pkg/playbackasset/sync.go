package playbackasset

import (
	"fmt"
	"os"
	"path/filepath"
)

// SyncDirectory 供集合资产复用主资产的目录持久化平台策略，path 是目录中的一个文件。
func SyncDirectory(path string) error { return syncPublicationDir(path) }

func syncPublicationParent(path string, flush bool) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	info, err := dir.Stat()
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("发布父路径不是目录")
	}
	if err == nil && flush {
		err = dir.Sync()
	}
	closeErr := dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}
