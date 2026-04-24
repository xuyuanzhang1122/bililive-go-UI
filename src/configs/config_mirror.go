package configs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const portableConfigEnv = "BILILIVE_CONFIG_FILE"

// ConfigSyncStatus 描述当前配置文件与系统默认配置镜像之间的关系。
type ConfigSyncStatus struct {
	CurrentPath    string `json:"current_path"`
	PortablePath   string `json:"portable_path"`
	CurrentExists  bool   `json:"current_exists"`
	PortableExists bool   `json:"portable_exists"`
	SameFile       bool   `json:"same_file"`
	PortableNewer  bool   `json:"portable_newer"`
	SameContent    bool   `json:"same_content"`
	Message        string `json:"message,omitempty"`
}

// PortableConfigPath 返回跨版本保留的系统默认配置路径。
// 默认位置：
//   - Linux:   ~/.config/bililive-go/config.yml
//   - macOS:   ~/Library/Application Support/bililive-go/config.yml
//   - Windows: %AppData%\bililive-go\config.yml
//
// 测试或高级部署可通过 BILILIVE_CONFIG_FILE 覆盖。
func PortableConfigPath() (string, error) {
	if override := os.Getenv(portableConfigEnv); override != "" {
		return filepath.Abs(override)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bililive-go", "config.yml"), nil
}

// ResolveStartupConfigFile 在启动阶段解析实际要读取的配置文件。
// 如果显式指定的配置不存在，但系统默认配置镜像存在，会先同步镜像到显式路径。
// 如果没有显式指定配置，则优先使用系统默认配置镜像。
func ResolveStartupConfigFile(requested string) (string, string, error) {
	requested = filepath.Clean(requested)
	if requested == "." {
		requested = ""
	}
	portable, portableErr := PortableConfigPath()
	portableExists := portableErr == nil && fileExists(portable)

	if requested != "" {
		requestedAbs, err := filepath.Abs(requested)
		if err != nil {
			return "", "", err
		}
		if fileExists(requestedAbs) {
			status, _ := ConfigSyncInfo(requestedAbs)
			if status != nil && status.PortableNewer && !status.SameContent {
				return requestedAbs, fmt.Sprintf("检测到系统默认配置镜像较新：%s；当前仍使用：%s。如需同步，请在设置页保存一次或手动合并。", status.PortablePath, status.CurrentPath), nil
			}
			return requestedAbs, "", nil
		}
		if portableExists {
			if err := copyFile(portable, requestedAbs); err != nil {
				return "", "", err
			}
			return requestedAbs, fmt.Sprintf("未找到指定配置 %s，已从系统默认配置镜像同步：%s", requestedAbs, portable), nil
		}
		return requestedAbs, "", nil
	}

	if portableExists {
		return portable, fmt.Sprintf("未指定配置文件，已使用系统默认配置镜像：%s", portable), nil
	}
	return "", "", nil
}

// MirrorConfigToPortable 将当前配置内容同步到系统默认配置镜像。
func MirrorConfigToPortable(sourcePath string, data []byte) error {
	portablePath, err := PortableConfigPath()
	if err != nil {
		return err
	}
	if samePath(sourcePath, portablePath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(portablePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(portablePath, data, 0644)
}

// ConfigSyncInfo 返回配置镜像状态，供启动日志或前端提示使用。
func ConfigSyncInfo(currentPath string) (*ConfigSyncStatus, error) {
	portablePath, err := PortableConfigPath()
	if err != nil {
		return nil, err
	}
	if currentPath == "" {
		currentPath = GetDefaultConfigPath()
	}
	if currentPath != "" {
		currentPath, _ = filepath.Abs(currentPath)
	}

	status := &ConfigSyncStatus{
		CurrentPath:  currentPath,
		PortablePath: portablePath,
		SameFile:     samePath(currentPath, portablePath),
	}

	currentInfo, currentErr := os.Stat(currentPath)
	status.CurrentExists = currentErr == nil && !currentInfo.IsDir()
	portableInfo, portableErr := os.Stat(portablePath)
	status.PortableExists = portableErr == nil && !portableInfo.IsDir()

	if status.CurrentExists && status.PortableExists {
		status.PortableNewer = portableInfo.ModTime().After(currentInfo.ModTime().Add(time.Second))
		status.SameContent = filesHaveSameContent(currentPath, portablePath)
	}

	switch {
	case status.SameFile && status.CurrentExists:
		status.Message = "当前配置已位于系统默认配置目录"
	case status.PortableExists && !status.CurrentExists:
		status.Message = "检测到系统默认配置镜像，可用于恢复当前配置"
	case status.PortableNewer && !status.SameContent:
		status.Message = "系统默认配置镜像比当前配置新，建议确认是否同步"
	case status.CurrentExists && !status.PortableExists:
		status.Message = "当前配置将在下次保存时同步到系统默认配置目录"
	case status.CurrentExists && status.PortableExists && status.SameContent:
		status.Message = "当前配置与系统默认配置镜像一致"
	}
	return status, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func filesHaveSameContent(a, b string) bool {
	ab, errA := os.ReadFile(a)
	bb, errB := os.ReadFile(b)
	return errA == nil && errB == nil && bytes.Equal(ab, bb)
}
