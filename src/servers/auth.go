package servers

import (
	"net/http"
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

func apiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := configs.GetCurrentConfig()
		if !requiresAPIAuth(cfg) || r.Method == http.MethodOptions {
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
