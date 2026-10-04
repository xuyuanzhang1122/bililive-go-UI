package servers

import "fmt"

// 统一使用十进制单位，客户端优先原样显示该文本。
func formatVideoLibrarySize(size int64) string {
	if size < 1000 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	amount := float64(size)
	unit := 0
	for amount >= 1000 && unit < len(units)-1 {
		amount /= 1000
		unit++
	}
	return fmt.Sprintf("%.1f %s", amount, units[unit])
}
