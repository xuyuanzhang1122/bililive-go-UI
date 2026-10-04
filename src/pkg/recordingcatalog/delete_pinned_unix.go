//go:build linux || darwin

package recordingcatalog

import (
	"fmt"
	"os"
	"strings"

	uuid "github.com/satori/go.uuid"
	"golang.org/x/sys/unix"
)

// 先在已打开的父目录内捕获，再核验捕获对象；不对重新解析的源路径 unlink。
func removePinnedFile(parent *os.File, name, expected string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return ErrPlaybackChanged
	}
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}
	dir := ".bilive-delete-" + id.String()
	pfd := int(parent.Fd())
	if err := unix.Mkdirat(pfd, dir, 0700); err != nil {
		return err
	}
	qfd, err := unix.Openat(pfd, dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Unlinkat(pfd, dir, unix.AT_REMOVEDIR)
		return err
	}
	defer unix.Close(qfd)
	defer unix.Unlinkat(pfd, dir, unix.AT_REMOVEDIR)
	if err := unix.Renameat(pfd, name, qfd, "media"); err != nil {
		return err
	}
	restore := func(reason error) error {
		// Linkat 不覆盖后来占用原路径的新文件；恢复冲突时保留隔离对象。
		if err := unix.Linkat(qfd, "media", pfd, name, 0); err != nil {
			return fmt.Errorf("%w；隔离对象保留在父目录 %s/media，恢复失败: %v", reason, dir, err)
		}
		if err := unix.Unlinkat(qfd, "media", 0); err != nil {
			return fmt.Errorf("%w；恢复成功但隔离副本仍保留在 %s/media: %v", reason, dir, err)
		}
		return reason
	}
	fd, err := unix.Openat(qfd, "media", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return restore(ErrPlaybackChanged)
	}
	f := os.NewFile(uintptr(fd), dir+"/media")
	actual, err := SnapshotOpenedFileVersion(f)
	f.Close()
	if err != nil || actual != expected {
		return restore(ErrPlaybackChanged)
	}
	if err := unix.Unlinkat(qfd, "media", 0); err != nil {
		return restore(err)
	}
	return nil
}
