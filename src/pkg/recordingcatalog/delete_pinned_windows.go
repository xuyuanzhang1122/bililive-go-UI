package recordingcatalog

import (
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 通过可信父句柄打开叶子，版本匹配后只删除该实际文件句柄。
func removePinnedFile(parent *os.File, name, expected string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return ErrPlaybackChanged
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var iosb windows.IO_STATUS_BLOCK
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.DELETE|windows.SYNCHRONIZE, &oa, &iosb, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(handle), name)
	defer f.Close()
	actual, err := SnapshotOpenedFileVersion(f)
	if err != nil || actual != expected {
		return ErrPlaybackChanged
	}
	flag := byte(1)
	return windows.NtSetInformationFile(handle, &iosb, &flag, 1, windows.FileDispositionInformation)
}
