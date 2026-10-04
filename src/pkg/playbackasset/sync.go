package playbackasset

import (
	"fmt"
	"os"
	"path/filepath"
)

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
