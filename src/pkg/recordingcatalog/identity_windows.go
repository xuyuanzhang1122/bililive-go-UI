package recordingcatalog

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func fileIdentity(path string, expected os.FileInfo) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, expected) {
		return "", fmt.Errorf("读取身份时文件变化")
	}
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &identity); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow, identity.CreationTime.HighDateTime, identity.CreationTime.LowDateTime), nil
}
