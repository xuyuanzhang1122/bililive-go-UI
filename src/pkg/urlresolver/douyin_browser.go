package urlresolver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

var errHeadlessBrowserUnavailable = errors.New("无头浏览器不可用")

func resolveWithHeadlessBrowser(ctx context.Context, candidate string) (string, error) {
	if isHeadlessResolverDisabled() {
		return "", errHeadlessBrowserUnavailable
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", errHeadlessBrowserUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, "-e", douyinPlaywrightScript, candidate)
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
