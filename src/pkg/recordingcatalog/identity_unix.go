//go:build !windows && !darwin && !linux

package recordingcatalog

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(_ string, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("文件系统不提供稳定文件身份")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
