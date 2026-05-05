package servers

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/sentry"
	"github.com/bililive-go/bililive-go/src/pkg/urlresolver"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
)

type localDoctorCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

func localDoctor(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	checks := make([]localDoctorCheck, 0, 8)

	checks = append(checks, localDoctorCheck{ID: "system", OK: true, Message: runtime.GOOS + "/" + runtime.GOARCH})
	if cfg == nil {
		writeJSON(writer, map[string]any{"ok": false, "checks": append(checks, localDoctorCheck{ID: "config", OK: false, Message: "配置未加载"})})
		return
	}

	configPath := cfg.File
	if configPath == "" {
		configPath = configs.GetDefaultConfigPath()
	}
	checks = append(checks, localDoctorCheck{ID: "config", OK: configPath != "", Message: configPath, Path: configPath})
	checks = append(checks, pathCheck("output_path", cfg.OutPutPath))
	checks = append(checks, pathCheck("app_data_path", cfg.AppDataPath))
	checks = append(checks, portCheck(cfg.RPC.Bind))

	ffmpegPath, ffmpegErr := utils.GetFFmpegPath(r.Context())
	ffmpegCheck := localDoctorCheck{ID: "ffmpeg", OK: ffmpegErr == nil, Path: ffmpegPath}
	if ffmpegErr != nil {
		ffmpegCheck.Message = ffmpegErr.Error()
	} else {
		ffmpegCheck.Message = "ffmpeg 可用"
		ffmpegCheck.Version = firstLine(commandOutput(r.Context(), ffmpegPath, "-version"))
	}
	checks = append(checks, ffmpegCheck)

	probe := urlresolver.ProbeHeadlessBrowser(r.Context(), cfg)
	headless := localDoctorCheck{ID: "headless-browser", OK: probe.Available, Path: probe.FoundPath, Version: probe.Version}
	if probe.Available {
		headless.Message = "无头浏览器可用"
	} else {
		headless.Message = probe.Error
	}
	checks = append(checks, headless)

	nodeCheck := localDoctorCheck{ID: "node-playwright", OK: false}
	if nodePath, err := exec.LookPath("node"); err == nil {
		nodeCheck.Path = nodePath
		nodeCheck.Version = strings.TrimSpace(commandOutput(r.Context(), nodePath, "--version"))
		nodeCheck.OK = true
		nodeCheck.Message = "Node 可用，Playwright 作为可选 fallback"
	} else {
		nodeCheck.Message = "Node 不可用；若 Chromium 可用可忽略"
	}
	checks = append(checks, nodeCheck)

	docker := localDoctorCheck{ID: "docker", OK: false}
	if dockerPath, err := exec.LookPath("docker"); err == nil {
		docker.Path = dockerPath
		docker.Version = strings.TrimSpace(commandOutput(r.Context(), dockerPath, "--version"))
		docker.OK = docker.Version != ""
		docker.Message = "Docker 可选"
	} else {
		docker.Message = "Docker 未安装；二进制模式可忽略"
	}
	checks = append(checks, docker)

	ok := true
	for _, check := range checks {
		if check.ID == "config" || check.ID == "output_path" || check.ID == "app_data_path" || check.ID == "port" || check.ID == "ffmpeg" {
			ok = ok && check.OK
		}
	}
	writeJSON(writer, map[string]any{
		"ok": ok,
		"system": map[string]string{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
		},
		"checks": checks,
	})
}

func localRestart(writer http.ResponseWriter, r *http.Request) {
	writeJSON(writer, map[string]any{"status": "restarting", "message": "正在重启服务"})
	if shutdownFunc != nil {
		sentry.Go(func() {
			time.Sleep(500 * time.Millisecond)
			shutdownFunc()
		})
	}
}

func pathCheck(id, path string) localDoctorCheck {
	check := localDoctorCheck{ID: id, Path: path}
	if strings.TrimSpace(path) == "" {
		check.Message = "路径为空"
		return check
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		check.Message = err.Error()
		return check
	}
	check.OK = true
	check.Message = "路径可用"
	return check
}

func portCheck(bind string) localDoctorCheck {
	check := localDoctorCheck{ID: "port", Message: bind}
	if strings.TrimSpace(bind) == "" {
		check.Message = "端口绑定为空"
		return check
	}
	addr, err := net.ResolveTCPAddr("tcp", bind)
	if err != nil {
		check.Message = err.Error()
		return check
	}
	check.OK = true
	check.Message = addr.String()
	return check
}

func commandOutput(ctx context.Context, name string, args ...string) string {
	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(cmdCtx, name, args...).CombinedOutput()
	return string(out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}
