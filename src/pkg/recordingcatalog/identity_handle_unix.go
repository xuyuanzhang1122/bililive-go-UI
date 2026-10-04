//go:build !windows && !linux

package recordingcatalog

import "os"

func fileHandleIdentity(_ *os.File, info os.FileInfo) (string, error) {
	return fileIdentity("", info)
}
