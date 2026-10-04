package recordingcatalog

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func fileIdentity(path string, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("文件系统不提供稳定文件身份")
	}
	var extended unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_INO|unix.STATX_BTIME, &extended); err != nil {
		return "", err
	}
	if extended.Mask&unix.STATX_BTIME == 0 {
		return "", fmt.Errorf("文件系统未提供创建时间，拒绝猜测 inode 的重用身份")
	}
	if extended.Ino != stat.Ino {
		return "", fmt.Errorf("读取文件身份时来源已替换")
	}
	return fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, extended.Btime.Sec, extended.Btime.Nsec), nil
}
