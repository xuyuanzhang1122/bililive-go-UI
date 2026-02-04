package build

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// getDevBinaryName 返回开发版二进制文件名（不包含架构，便于跨平台调试）
func getDevBinaryName() string {
	goHostOS := os.Getenv("PLATFORM")
	if goHostOS == "" {
		goHostOS = runtime.GOOS
	}
	if goHostOS == "windows" {
		return "bililive-dev.exe"
	}
	return "bililive-dev"
}

// BuildDevIncremental 增量构建：只在源码变化时重新编译
// 使用类似 make 的方式比较文件修改时间，而不是计算校验和
// 输出到 bin/bililive-dev[.exe]，便于跨平台调试
// 返回 true 表示进行了编译，false 表示跳过
func BuildDevIncremental() bool {
	binaryPath := "bin/" + getDevBinaryName()

	// 检查二进制文件是否存在
	binaryInfo, err := os.Stat(binaryPath)
	if os.IsNotExist(err) {
		fmt.Println("[增量构建] 二进制文件不存在，需要编译")
		buildDevBinary()
		return true
	}
	if err != nil {
		fmt.Printf("[增量构建] 无法访问二进制文件: %v，需要编译\n", err)
		buildDevBinary()
		return true
	}

	binaryModTime := binaryInfo.ModTime()

	// 检查是否有任何源文件比二进制文件更新
	needsRebuild, reason := checkSourcesNewer(binaryModTime)
	if needsRebuild {
		fmt.Printf("[增量构建] %s，需要重新编译\n", reason)
		buildDevBinary()
		return true
	}

	fmt.Println("[增量构建] 源码无变化，跳过编译")
	return false
}

// checkSourcesNewer 检查是否有源文件比目标文件更新
// 返回 (是否需要重新编译, 原因)
func checkSourcesNewer(targetModTime time.Time) (bool, string) {
	var newerFile string

	// 首先检查 go.mod 和 go.sum（最常见的依赖变化）
	for _, file := range []string{"go.mod", "go.sum"} {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if info.ModTime().After(targetModTime) {
			return true, fmt.Sprintf("%s 已更新", file)
		}
	}

	// 遍历 src 目录下的所有 Go 文件
	err := filepath.Walk("src", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		// 只检查 Go 源码文件
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		if info.ModTime().After(targetModTime) {
			newerFile = path
			// 返回一个特殊错误来中断遍历
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		// 遍历出错，保守起见重新编译
		return true, "无法遍历源码目录"
	}

	if newerFile != "" {
		return true, fmt.Sprintf("%s 已更新", newerFile)
	}

	return false, ""
}

// buildDevBinary 编译开发版二进制文件
func buildDevBinary() {
	outputPath := "bin/" + getDevBinaryName()
	BuildGoBinaryWithOutput(true, outputPath)
}

// GetDevBinaryPath 返回开发版二进制文件的路径
func GetDevBinaryPath() string {
	return "bin/" + getDevBinaryName()
}
