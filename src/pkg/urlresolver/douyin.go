package urlresolver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

const douyinDesktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var (
	httpURLPattern       = regexp.MustCompile(`https?://[^\s　，。！？；：'"【】（）<>]+`)
	bareDouyinURLPattern = regexp.MustCompile(`(?i)(?:v|live)\.douyin\.com/[^\s　，。！？；：'"【】（）<>]+`)
	roomIDPattern        = regexp.MustCompile(`^\d{6,}$`)
	bodyRoomPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`https://live\.douyin\.com/(\d{6,})`),
		regexp.MustCompile(`live\.douyin\.com/(\d{6,})`),
		regexp.MustCompile(`"web_rid"\s*:\s*"(\d{6,})"`),
		regexp.MustCompile(`"roomId"\s*:\s*"(\d{6,})"`),
		regexp.MustCompile(`"room_id"\s*:\s*"(\d{6,})"`),
	}
)

type DouyinResolver struct {
	client *http.Client
}

func NewDouyinResolver() *DouyinResolver {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxy.ApplyInfoProxyToTransport(transport)

	return &DouyinResolver{
		client: &http.Client{
			Transport: transport,
			Timeout:   12 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 15 {
					return fmt.Errorf("重定向次数超过上限 (15)")
				}
				req.Header.Set("User-Agent", douyinDesktopUA)
				if len(via) > 0 {
					req.Header.Set("Referer", via[len(via)-1].URL.String())
				}
				return nil
			},
		},
	}
}

func (r *DouyinResolver) Resolve(ctx context.Context, raw string) (string, error) {
	candidate, err := extractCandidateURL(raw)
	if err != nil {
		return "", err
	}
	if canonical, ok := normalizeDouyinURL(candidate); ok {
		return canonical, nil
	}
	if !isDouyinURL(candidate) {
		return "", ErrUnsupportedURL
	}

	finalURL, body, err := r.follow(ctx, http.MethodGet, candidate)
	if err != nil {
		var headURL string
		headURL, _, err = r.follow(ctx, http.MethodHead, candidate)
		if err != nil {
			return "", fmt.Errorf("请求失败: %w", err)
		}
		finalURL = headURL
	}

	if canonical, ok := normalizeDouyinURL(finalURL); ok {
		return canonical, nil
	}
	if canonical, ok := canonicalFromHTML(body); ok {
		return canonical, nil
	}
	if canonical, err := resolveWithHeadlessBrowser(ctx, candidate); err == nil {
		return canonical, nil
	} else if !errors.Is(err, errHeadlessBrowserUnavailable) {
		return "", fmt.Errorf("%w: %v", ErrUnresolved, err)
	}
	return "", fmt.Errorf("%w: HTTP 解析未得到稳定地址，且无头浏览器不可用；请运行 npm install && npm run install:browser 后重试，或设置 BILILIVE_DOUYIN_HEADLESS=0 关闭兜底", ErrUnresolved)
}

func (r *DouyinResolver) follow(ctx context.Context, method, rawURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", douyinDesktopUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if cfg := configs.GetCurrentConfig(); cfg != nil {
		if cookie := strings.TrimSpace(cfg.Douyin.Cookie); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body := ""
	if method == http.MethodGet && resp.Body != nil {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		body = string(data)
	}
	return resp.Request.URL.String(), body, nil
}

func extractCandidateURL(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", ErrNoURL
	}
	if match := httpURLPattern.FindString(text); match != "" {
		return trimURLPunctuation(match), nil
	}
	if match := bareDouyinURLPattern.FindString(text); match != "" {
		return "https://" + trimURLPunctuation(match), nil
	}
	return "", ErrNoURL
}

func trimURLPunctuation(raw string) string {
	return strings.TrimRight(raw, ".,;:!?，。；：！？)]】\"'")
}

func normalizeDouyinURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host != "live.douyin.com" {
		return "", false
	}

	for _, seg := range strings.Split(u.Path, "/") {
		if roomIDPattern.MatchString(seg) {
			return "https://live.douyin.com/" + seg, true
		}
	}
	for _, key := range []string{"room_id", "web_rid", "roomId"} {
		if value := u.Query().Get(key); roomIDPattern.MatchString(value) {
			return "https://live.douyin.com/" + value, true
		}
	}
	return "", false
}

func isDouyinURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "douyin.com" ||
		host == "iesdouyin.com" ||
		strings.HasSuffix(host, ".douyin.com") ||
		host == "amemv.com" ||
		strings.HasSuffix(host, ".amemv.com")
}

func canonicalFromHTML(body string) (string, bool) {
	for _, pattern := range bodyRoomPatterns {
		match := pattern.FindStringSubmatch(body)
		if len(match) == 2 {
			return "https://live.douyin.com/" + match[1], true
		}
	}
	return "", false
}
