package urlresolver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

var errHeadlessBrowserUnavailable = errors.New("无头浏览器不可用")

type HeadlessBrowserProbe struct {
	Path      string `json:"path"`
	FoundPath string `json:"found_path"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

func resolveWithHeadlessBrowser(ctx context.Context, candidate string) (string, error) {
	if isHeadlessResolverDisabled() {
		return "", errHeadlessBrowserUnavailable
	}
	cfg := configs.GetCurrentConfig()
	timeout := 25 * time.Second
	cookie := ""
	if cfg != nil {
		if cfg.HeadlessBrowser.TimeoutSeconds > 0 {
			timeout = time.Duration(cfg.HeadlessBrowser.TimeoutSeconds) * time.Second
		}
		cookie = strings.TrimSpace(cfg.Douyin.Cookie)
	}

	if browserPath, _ := DetectHeadlessBrowserPath(cfg); browserPath != "" {
		if canonical, err := resolveWithBrowserExecutable(ctx, browserPath, candidate, timeout); err == nil {
			return canonical, nil
		} else if !errors.Is(err, errHeadlessBrowserUnavailable) {
			// 保留 Node/Playwright 作为兜底，但让最终错误能说明浏览器路径的问题。
			if fallback, fallbackErr := resolveWithPlaywrightNode(ctx, candidate, cookie, timeout); fallbackErr == nil {
				return fallback, nil
			}
			return "", err
		}
	}
	return resolveWithPlaywrightNode(ctx, candidate, cookie, timeout)
}

func resolveWithBrowserExecutable(ctx context.Context, browserPath, candidate string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(browserPath) == "" {
		return "", errHeadlessBrowserUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-setuid-sandbox",
		"--disable-blink-features=AutomationControlled",
		"--dump-dom",
		candidate,
	}
	cmd := exec.CommandContext(ctx, browserPath, args...)
	cmd.Env = append(os.Environ(), proxy.GetInfoProxyEnvVars()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("无头浏览器解析超时")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("无头浏览器解析失败: %s", msg)
	}
	if canonical, ok := canonicalFromHTML(stdout.String()); ok {
		return canonical, nil
	}
	return "", ErrUnresolved
}

func resolveWithPlaywrightNode(ctx context.Context, candidate, cookie string, timeout time.Duration) (string, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", errHeadlessBrowserUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, "-e", douyinPlaywrightScript, candidate, cookie)
	cmd.Env = append(os.Environ(), proxy.GetInfoProxyEnvVars()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("无头浏览器解析超时")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if strings.Contains(msg, "Cannot find module 'playwright'") {
			return "", errHeadlessBrowserUnavailable
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("无头浏览器解析失败: %s", msg)
	}

	var result struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return "", fmt.Errorf("无头浏览器返回无效结果: %w", err)
	}
	if result.Error != "" {
		return "", errors.New(result.Error)
	}
	if canonical, ok := normalizeDouyinURL(result.URL); ok {
		return canonical, nil
	}
	return "", ErrUnresolved
}

func ProbeHeadlessBrowser(ctx context.Context, cfg *configs.Config) HeadlessBrowserProbe {
	requested := ""
	if cfg != nil {
		requested = strings.TrimSpace(cfg.HeadlessBrowser.Path)
	}
	found, source := DetectHeadlessBrowserPath(cfg)
	probe := HeadlessBrowserProbe{Path: requested, FoundPath: found}
	if found == "" {
		probe.Error = "未找到 Chromium/Chrome 可执行文件"
		if source != "" {
			probe.Error = source
		}
		return probe
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, found, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		probe.Error = strings.TrimSpace(string(out))
		if probe.Error == "" {
			probe.Error = err.Error()
		}
		return probe
	}
	probe.Available = true
	probe.Version = strings.TrimSpace(string(out))
	return probe
}

func DetectHeadlessBrowserPath(cfg *configs.Config) (string, string) {
	if cfg != nil {
		if p := strings.TrimSpace(cfg.HeadlessBrowser.Path); p != "" {
			if isExecutableFile(p) {
				return p, "config"
			}
			return "", "配置的无头浏览器路径不可执行或不存在"
		}
	}
	if p := strings.TrimSpace(os.Getenv("BILILIVE_HEADLESS_BROWSER_PATH")); p != "" {
		if isExecutableFile(p) {
			return p, "env"
		}
		return "", "BILILIVE_HEADLESS_BROWSER_PATH 不可执行或不存在"
	}
	for _, name := range browserCommandCandidates() {
		if p, err := exec.LookPath(name); err == nil && isExecutableFile(p) {
			return p, "path"
		}
	}
	for _, p := range browserPathCandidates() {
		if isExecutableFile(p) {
			return p, "common"
		}
	}
	return "", ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func browserCommandCandidates() []string {
	if runtime.GOOS == "windows" {
		return []string{"chrome.exe", "msedge.exe", "chromium.exe"}
	}
	return []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome", "msedge"}
}

func browserPathCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/opt/homebrew/bin/chromium",
			"/usr/local/bin/chromium",
		}
	case "windows":
		roots := []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")}
		var paths []string
		for _, root := range roots {
			if root == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return paths
	default:
		return []string{"/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/snap/bin/chromium"}
	}
}

func isHeadlessResolverDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BILILIVE_DOUYIN_HEADLESS"))) {
	case "0", "false", "off", "no":
		return true
	default:
		return false
	}
}

const douyinPlaywrightScript = `
let chromium;
try {
  chromium = require('playwright').chromium;
} catch (error) {
  console.error(error && error.message ? error.message : String(error));
  process.exit(2);
}

const raw = process.argv[1] || '';
const cookieText = process.argv[2] || '';
const ua = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36';
const roomIDPattern = /^\d{6,}$/;
const bodyRoomPatterns = [
  /https:\/\/live\.douyin\.com\/(\d{6,})/,
  /live\.douyin\.com\/(\d{6,})/,
  /"web_rid"\s*:\s*"(\d{6,})"/,
  /"roomId"\s*:\s*"(\d{6,})"/,
  /"room_id"\s*:\s*"(\d{6,})"/,
  /web_rid=([0-9]{6,})/,
  /room_id=([0-9]{6,})/
];

function normalize(rawURL) {
  try {
    const u = new URL(rawURL);
    if (u.hostname.toLowerCase() !== 'live.douyin.com') {
      return '';
    }
    for (const seg of u.pathname.split('/')) {
      if (roomIDPattern.test(seg)) {
        return 'https://live.douyin.com/' + seg;
      }
    }
    for (const key of ['room_id', 'web_rid', 'roomId']) {
      const value = u.searchParams.get(key);
      if (value && roomIDPattern.test(value)) {
        return 'https://live.douyin.com/' + value;
      }
    }
  } catch (_) {}
  return '';
}

function fromText(text) {
  for (const pattern of bodyRoomPatterns) {
    const match = pattern.exec(text || '');
    if (match && match[1]) {
      return 'https://live.douyin.com/' + match[1];
    }
  }
  return '';
}

(async () => {
  let browser;
  try {
    browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-blink-features=AutomationControlled']
    });
    const context = await browser.newContext({
      userAgent: ua,
      locale: 'zh-CN',
      timezoneId: 'Asia/Shanghai',
      viewport: { width: 1365, height: 900 }
    });
    if (cookieText) {
      const cookies = cookieText.split(';').map((part) => {
        const idx = part.indexOf('=');
        if (idx <= 0) return null;
        return {
          name: part.slice(0, idx).trim(),
          value: part.slice(idx + 1).trim(),
          domain: '.douyin.com',
          path: '/'
        };
      }).filter(Boolean);
      if (cookies.length) {
        await context.addCookies(cookies).catch(() => {});
      }
    }
    const page = await context.newPage();
    await page.goto(raw, { waitUntil: 'domcontentloaded', timeout: 18000 });
    await page.waitForTimeout(2500);
    const finalURL = page.url();
    const html = await page.content().catch(() => '');
    const result = normalize(finalURL) || fromText(html);
    if (!result) {
      console.log(JSON.stringify({ error: '无头浏览器未找到稳定的抖音直播间地址' }));
      return;
    }
    console.log(JSON.stringify({ url: result }));
  } catch (error) {
    console.log(JSON.stringify({ error: error && error.message ? error.message : String(error) }));
  } finally {
    if (browser) {
      await browser.close().catch(() => {});
    }
  }
})();
`
