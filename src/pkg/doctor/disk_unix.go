//go:build !windows

package doctor

import "golang.org/x/sys/unix"

func diskUsage(path string) (free, total uint64, err error) {
	if path == "" {
		path = "."
	}
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize), nil
}
