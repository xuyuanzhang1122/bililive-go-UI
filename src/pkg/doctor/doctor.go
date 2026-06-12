// Package doctor 提供安装/运维健康检查（bililive-go --doctor）。
// 检查项：配置文件、ffmpeg、无头浏览器、输出目录、数据目录、端口、磁盘空间。
package doctor

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
)

type check struct {
	Name     string
	OK       bool
	Required bool
	Detail   string
}

// Run 执行全部检查并打印结果，返回进程退出码（必需项全过为 0）。
func Run(confPath string) int {
	checks := []check{}

	// 1. 配置文件
	resolved, _, err := configs.ResolveStartupConfigFile(confPath)
	var cfg *configs.Config
	if err != nil || resolved == "" {
		checks = append(checks, check{"配置文件", false, true, fmt.Sprintf("无法定位配置文件: %v", err)})
	} else if cfg, err = configs.NewConfigWithFile(resolved); err != nil {
		checks = append(checks, check{"配置文件", false, true, fmt.Sprintf("%s 解析失败: %v", resolved, err)})
	} else {
		checks = append(checks, check{"配置文件", true, true, resolved})
	}

	// 2. ffmpeg
	ffmpegPath := ""
	if cfg != nil && cfg.FfmpegPath != "" {
		ffmpegPath = cfg.FfmpegPath
	}
	if ffmpegPath == "" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = p
		}
	}
	if ffmpegPath == "" {
		for _, candidate := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"} {
			if _, err := os.Stat(candidate); err == nil {
				ffmpegPath = candidate
				break
			}
		}
	}
	if ffmpegPath == "" {
		checks = append(checks, check{"ffmpeg", false, true, "未找到，录制/缩略图/HLS 不可用"})
	} else if version := probeVersion(ffmpegPath, "-version"); version != "" {
		checks = append(checks, check{"ffmpeg", true, true, fmt.Sprintf("%s（%s）", ffmpegPath, version)})
	} else {
		checks = append(checks, check{"ffmpeg", false, true, ffmpegPath + " 存在但无法执行"})
	}

	// 3. 无头浏览器（可选）
	headlessPath := ""
	if cfg != nil && cfg.HeadlessBrowser.Path != "" {
		headlessPath = cfg.HeadlessBrowser.Path
	}
	if headlessPath == "" {
		for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "chrome"} {
			if p, err := exec.LookPath(candidate); err == nil {
				headlessPath = p
				break
			}
		}
	}
	if headlessPath == "" {
		for _, candidate := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		} {
			if _, err := os.Stat(candidate); err == nil {
				headlessPath = candidate
				break
			}
		}
	}
	if headlessPath != "" {
		checks = append(checks, check{"无头浏览器", true, false, headlessPath})
	} else {
		checks = append(checks, check{"无头浏览器", false, false, "未找到（可选）：非标准短链解析降级"})
	}

	// 4/5. 输出目录与数据目录可写
	if cfg != nil {
		checks = append(checks, dirWritableCheck("输出目录", cfg.OutPutPath, true))
		appData := cfg.AppDataPath
		if appData == "" {
			appData = ".appdata"
		}
		checks = append(checks, dirWritableCheck("数据目录", appData, true))

		// 6. 端口
		bind := cfg.RPC.Bind
		if bind != "" {
			addr := bind
			if strings.HasPrefix(addr, ":") {
				addr = "127.0.0.1" + addr
			}
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				checks = append(checks, check{"端口 " + bind, true, false, "已有服务监听（bililive-go 可能正在运行）"})
			} else {
				checks = append(checks, check{"端口 " + bind, true, false, "空闲，可启动"})
			}
		}

		// 7. 磁盘空间
		if free, total, err := diskUsage(cfg.OutPutPath); err == nil && total > 0 {
			freeGB := float64(free) / (1 << 30)
			c := check{"磁盘空间", true, false, fmt.Sprintf("输出目录所在盘剩余 %.1f GB", freeGB)}
			if freeGB < 5 {
				c.OK = false
				c.Detail += "（不足 5GB，录制可能很快写满）"
			}
			checks = append(checks, c)
		}
	}

	// 打印
	fmt.Println("bililive-go doctor")
	fmt.Println(strings.Repeat("-", 50))
	exitCode := 0
	for _, c := range checks {
		mark := "✓"
		if !c.OK {
			if c.Required {
				mark = "✗"
				exitCode = 1
			} else {
				mark = "!"
			}
		}
		fmt.Printf("  %s %-14s %s\n", mark, c.Name, c.Detail)
	}
	fmt.Println(strings.Repeat("-", 50))
	if exitCode == 0 {
		fmt.Println("全部必需检查通过")
	} else {
		fmt.Println("存在未通过的必需检查，请按提示处理")
	}
	return exitCode
}

func dirWritableCheck(name, dir string, required bool) check {
	if dir == "" {
		dir = "./"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return check{name, false, required, fmt.Sprintf("%s 无法创建: %v", dir, err)}
	}
	probe := filepath.Join(dir, ".doctor-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0644); err != nil {
		return check{name, false, required, fmt.Sprintf("%s 不可写: %v", dir, err)}
	}
	_ = os.Remove(probe)
	return check{name, true, required, dir}
}

func probeVersion(bin string, arg string) string {
	out, err := exec.Command(bin, arg).Output()
	if err != nil {
		return ""
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(line)
}
