package recordingcatalog

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func fileHandleIdentity(f *os.File, _ os.FileInfo) (string, error) {
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &identity); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow, identity.CreationTime.HighDateTime, identity.CreationTime.LowDateTime), nil
}
