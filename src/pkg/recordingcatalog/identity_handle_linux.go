package recordingcatalog

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func fileHandleIdentity(f *os.File, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrPlaybackChanged
	}
	var extended unix.Statx_t
	if err := unix.Statx(int(f.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_INO|unix.STATX_BTIME, &extended); err != nil {
		return "", err
	}
	if extended.Mask&unix.STATX_BTIME == 0 || extended.Ino != stat.Ino {
		return "", ErrPlaybackChanged
	}
	return fmt.Sprintf("%d:%d:%d:%d", stat.Dev, stat.Ino, extended.Btime.Sec, extended.Btime.Nsec), nil
}
