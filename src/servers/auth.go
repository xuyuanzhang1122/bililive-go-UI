package servers

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
)

const (
	disableAPIAuthEnv    = "BILILIVE_DISABLE_API_AUTH"
	legacyDisableAuthEnv = "BGO_DISABLE_API_AUTH"
)

type authContextKey string

const (
	authAPIKeyUserKey authContextKey = "api_key_user"
	authRawAPIKeyKey  authContextKey = "raw_api_key"
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
			next.ServeHTTP(w, withDefaultAPIKeyUser(r))
			return
		}
		if user, rawKey, ok, status := authenticateAPIKeyRequest(r, cfg); ok {
			next.ServeHTTP(w, withAPIKeyUser(r, user, rawKey))
			return
		} else if status == http.StatusForbidden {
			if !isSameOriginRequest(r) {
				writeForbiddenError(w)
				return
			}
		}
		// 同源请求（内嵌 Web UI）免认证；如果带了有效 Key，上方已经绑定到对应用户。
		if isSameOriginRequest(r) {
			next.ServeHTTP(w, withDefaultAPIKeyUser(r))
			return
		}
		if user, rawKey, ok := authenticateSignedRequest(r, cfg); ok {
			next.ServeHTTP(w, withAPIKeyUser(r, user, rawKey))
			return
		}
		writeAuthError(w)
	})
}

func fileAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := configs.GetCurrentConfig()
		if !requiresAPIAuth(cfg) || r.Method == http.MethodOptions {
			next.ServeHTTP(w, withDefaultAPIKeyUser(r))
			return
		}
		if user, rawKey, ok, status := authenticateAPIKeyRequest(r, cfg); ok {
			next.ServeHTTP(w, withAPIKeyUser(r, user, rawKey))
			return
		} else if status == http.StatusForbidden {
			if !isSameOriginRequest(r) {
				writeForbiddenError(w)
				return
			}
		}
		// 同源请求（内嵌 Web UI）免认证；如果带了有效 Key，上方已经绑定到对应用户。
		if isSameOriginRequest(r) {
			next.ServeHTTP(w, withDefaultAPIKeyUser(r))
			return
		}
		if user, rawKey, ok := authenticateSignedRequest(r, cfg); ok {
			next.ServeHTTP(w, withAPIKeyUser(r, user, rawKey))
			return
		}
		writeAuthError(w)
	})
}

func requiresAPIAuth(cfg *configs.Config) bool {
	if truthyEnv(disableAPIAuthEnv) || truthyEnv(legacyDisableAuthEnv) {
		return false
	}
	if cfg != nil && cfg.Security.EnableAPIKey {
		return true
	}
	if store := metadata.GetStore(); store != nil && store.HasAPIKeyUsers(context.Background()) {
		return true
	}
	return false
}

func authenticateAPIKeyRequest(r *http.Request, cfg *configs.Config) (*metadata.APIKeyUser, string, bool, int) {
	rawKey := securitypkg.ExtractAPIKey(r)
	if rawKey == "" {
		return nil, "", false, http.StatusUnauthorized
	}
	if store := metadata.GetStore(); store != nil {
		user, err := store.FindActiveAPIKeyUserByKey(r.Context(), rawKey)
		if err == nil {
			return user, rawKey, true, http.StatusOK
		}
		if err != sql.ErrNoRows {
			return nil, "", false, http.StatusForbidden
		}
	}
	if cfg != nil && securitypkg.ConstantTimeEqual(rawKey, cfg.Security.APIKey) {
		return defaultAPIKeyUser(), rawKey, true, http.StatusOK
	}
	return nil, "", false, http.StatusForbidden
}

func authenticateSignedRequest(r *http.Request, cfg *configs.Config) (*metadata.APIKeyUser, string, bool) {
	if !canUseSignedURL(r) {
		return nil, "", false
	}
	rawKey := securitypkg.ExtractAPIKey(r)
	if rawKey != "" {
		if user, _, ok, _ := authenticateAPIKeyRequest(r, cfg); ok &&
			securitypkg.ValidateSignedRequest(r, signingSecretForAPIKey(user, rawKey), time.Now()) == nil {
			return user, rawKey, true
		}
	}
	if store := metadata.GetStore(); store != nil {
		user, err := store.FindActiveAPIKeyUserBySignedRequest(r.Context(), func(secret string) bool {
			return securitypkg.ValidateSignedRequest(r, secret, time.Now()) == nil
		})
		if err == nil {
			return user, "", true
		}
	}
	if cfg != nil && strings.TrimSpace(cfg.Security.APIKey) != "" &&
		securitypkg.ValidateSignedRequest(r, cfg.Security.APIKey, time.Now()) == nil {
		return defaultAPIKeyUser(), cfg.Security.APIKey, true
	}
	return nil, "", false
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

func writeForbiddenError(w http.ResponseWriter) {
	writeJsonWithStatusCode(w, http.StatusForbidden, commonResp{
		ErrNo:  http.StatusForbidden,
		ErrMsg: "无权限：API Key 无效、已禁用或已吊销",
	})
}

func withAPIKeyUser(r *http.Request, user *metadata.APIKeyUser, rawKey string) *http.Request {
	if user == nil {
		user = defaultAPIKeyUser()
	}
	ctx := context.WithValue(r.Context(), authAPIKeyUserKey, user)
	if strings.TrimSpace(rawKey) != "" {
		ctx = context.WithValue(ctx, authRawAPIKeyKey, strings.TrimSpace(rawKey))
	}
	return r.WithContext(ctx)
}

func withDefaultAPIKeyUser(r *http.Request) *http.Request {
	return withAPIKeyUser(r, defaultAPIKeyUser(), "")
}

func currentAPIKeyUser(r *http.Request) *metadata.APIKeyUser {
	if r != nil {
		if user, ok := r.Context().Value(authAPIKeyUserKey).(*metadata.APIKeyUser); ok && user != nil {
			return user
		}
	}
	return defaultAPIKeyUser()
}

func currentAPIKeyUserID(r *http.Request) string {
	return currentAPIKeyUser(r).ID
}

func currentRawAPIKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if key, ok := r.Context().Value(authRawAPIKeyKey).(string); ok {
		return key
	}
	return securitypkg.ExtractAPIKey(r)
}

func signingSecretForAPIKey(user *metadata.APIKeyUser, rawKey string) string {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return ""
	}
	if user != nil && user.ID != "" && user.ID != metadata.DefaultAPIKeyUserID {
		return securitypkg.HashAPIKey(rawKey)
	}
	return rawKey
}

func signingSecretForRequest(r *http.Request, cfg *configs.Config) string {
	rawKey := currentRawAPIKey(r)
	if rawKey != "" {
		return signingSecretForAPIKey(currentAPIKeyUser(r), rawKey)
	}
	if cfg != nil {
		return cfg.Security.APIKey
	}
	return ""
}

func defaultAPIKeyUser() *metadata.APIKeyUser {
	return &metadata.APIKeyUser{
		ID:      metadata.DefaultAPIKeyUserID,
		Name:    "默认用户",
		Enabled: true,
	}
}
