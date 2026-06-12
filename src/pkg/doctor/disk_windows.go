//go:build windows

package doctor

import "golang.org/x/sys/windows"

func diskUsage(path string) (free, total uint64, err error) {
	if path == "" {
		path = "."
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytes, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeBytes, &totalBytes, &totalFree); err != nil {
		return 0, 0, err
	}
	return freeBytes, totalBytes, nil
}
