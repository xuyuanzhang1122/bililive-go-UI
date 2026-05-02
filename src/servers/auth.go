package servers

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
)

const (
	disableAPIAuthEnv    = "BILILIVE_DISABLE_API_AUTH"
	legacyDisableAuthEnv = "BGO_DISABLE_API_AUTH"
)

func isAuthExemptPath(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	return strings.HasPrefix(r.URL.Path, "/api/auth-status")
}

// isSameOriginRequest 判断请求是否来自内嵌的同源 Web UI。
// 优先使用浏览器自动添加且不可伪造的 Sec-Fetch-Site header，
// 回退到 Referer host 匹配作为兼容检查。
// 非浏览器客户端（如 iOS App）不会发送这些 header，因此不会被误判。
func isSameOriginRequest(r *http.Request) bool {
	// 方法 1：Sec-Fetch-Site（现代浏览器，不可由 JS 伪造）
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
		return sfs == "same-origin"
	}
	// 方法 2：Referer 的 host 与请求 Host 一致（兼容旧浏览器）
	if ref := r.Header.Get("Referer"); ref != "" {
		if refURL, err := url.Parse(ref); err == nil {
			reqHost := r.Host
			refHost := refURL.Host
			// 去掉默认端口后比较
			return strings.EqualFold(stripDefaultPort(refHost), stripDefaultPort(reqHost))
		}
	}
	return false
}

// stripDefaultPort 去掉 :80 / :443 等默认端口，便于比较。
func stripDefaultPort(host string) string {
	if strings.HasSuffix(host, ":80") || strings.HasSuffix(host, ":443") {
		if h, _, ok := strings.Cut(host, ":"); ok {
			return h
		}
	}
	return host
}

func apiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := configs.GetCurrentConfig()
		if !requiresAPIAuth(cfg) || r.Method == http.MethodOptions || isAuthExemptPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		// 同源请求（内嵌 Web UI）免认证
		if isSameOriginRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if securitypkg.HasValidAPIKey(r, cfg.Security.APIKey) {
			next.ServeHTTP(w, r)
			return
		}
		if canUseSignedURL(r) && securitypkg.ValidateSignedRequest(r, cfg.Security.APIKey, time.Now()) == nil {
			next.ServeHTTP(w, r)
			return
		}
		writeAuthError(w)
	})
}

func fileAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := configs.GetCurrentConfig()
		if !requiresAPIAuth(cfg) || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// 同源请求（内嵌 Web UI）免认证
		if isSameOriginRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if securitypkg.HasValidAPIKey(r, cfg.Security.APIKey) ||
			securitypkg.ValidateSignedRequest(r, cfg.Security.APIKey, time.Now()) == nil {
			next.ServeHTTP(w, r)
			return
		}
		writeAuthError(w)
	})
}

func requiresAPIAuth(cfg *configs.Config) bool {
	if cfg == nil || !cfg.Security.EnableAPIKey {
		return false
	}
	if truthyEnv(disableAPIAuthEnv) || truthyEnv(legacyDisableAuthEnv) {
		return false
	}
	return true
}

func canUseSignedURL(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return strings.HasPrefix(r.URL.Path, "/api/thumbnail/") ||
		strings.HasPrefix(r.URL.Path, "/api/stream/hls/") ||
		strings.HasPrefix(r.URL.Path, "/api/stream/hls-segment/")
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeAuthError(w http.ResponseWriter) {
	writeJsonWithStatusCode(w, http.StatusUnauthorized, commonResp{
		ErrNo:  http.StatusUnauthorized,
		ErrMsg: "未授权：缺少或无效的 API Key",
	})
}
