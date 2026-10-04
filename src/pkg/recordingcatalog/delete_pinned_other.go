//go:build !linux && !darwin && !windows

package recordingcatalog

import (
	"fmt"
	"os"
)

func removePinnedFile(_ *os.File, _, _ string) error {
	return fmt.Errorf("当前平台尚未实现绑定文件身份的安全删除，保留文件")
}
