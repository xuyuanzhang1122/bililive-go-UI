package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingSignature = errors.New("缺少签名参数")
	ErrExpiredSignature = errors.New("签名已过期")
	ErrInvalidSignature = errors.New("签名无效")
	ErrMissingAPIKey    = errors.New("API Key 为空")
)

// GenerateAPIKey 生成一个适合放入配置文件的随机 API Key。
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashAPIKey 返回 API Key 的不可逆 SHA-256 摘要，用于持久化匹配。
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

// ExtractAPIKey 从 Authorization: Bearer / X-API-Key / _key 查询参数中读取 API Key。
func ExtractAPIKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	if key := strings.TrimSpace(r.Header.Get("X-API-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth != "" {
		const bearer = "bearer "
		if len(auth) > len(bearer) && strings.EqualFold(auth[:len(bearer)], bearer) {
			return strings.TrimSpace(auth[len(bearer):])
		}
	}
	if r.URL != nil {
		if key := strings.TrimSpace(r.URL.Query().Get("_key")); key != "" {
			return key
		}
	}
	return ""
}

// ConstantTimeEqual 比较两个 API Key，避免普通字符串比较的时序差异。
func ConstantTimeEqual(provided, expected string) bool {
	provided = strings.TrimSpace(provided)
	expected = strings.TrimSpace(expected)
	if provided == "" || expected == "" {
		return false
	}
	if len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func HasValidAPIKey(r *http.Request, expected string) bool {
	return ConstantTimeEqual(ExtractAPIKey(r), expected)
}

// SignPath 对 HTTP 方法、URL path 与过期时间生成 HMAC-SHA256 签名。
func SignPath(method, escapedPath string, expires int64, apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", ErrMissingAPIKey
	}
	payload := signaturePayload(method, escapedPath, expires)
	mac := hmac.New(sha256.New, []byte(apiKey))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// SignURL 给相对 URL 增加 expires 与 sig 参数。
func SignURL(rawURL, method string, expires time.Time, apiKey string) (string, int64, error) {
	if !strings.HasPrefix(rawURL, "/") {
		rawURL = "/" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, err
	}
	expiresUnix := expires.Unix()
	sig, err := SignPath(method, u.EscapedPath(), expiresUnix, apiKey)
	if err != nil {
		return "", 0, err
	}
	q := u.Query()
	q.Set("expires", strconv.FormatInt(expiresUnix, 10))
	q.Set("sig", sig)
	u.RawQuery = q.Encode()
	return u.String(), expiresUnix, nil
}

// ValidateSignedRequest 校验请求 URL 中的 expires 与 sig 参数。
func ValidateSignedRequest(r *http.Request, apiKey string, now time.Time) error {
	if r == nil || r.URL == nil {
		return ErrInvalidSignature
	}
	if strings.TrimSpace(apiKey) == "" {
		return ErrMissingAPIKey
	}
	q := r.URL.Query()
	expiresRaw := q.Get("expires")
	sig := q.Get("sig")
	if expiresRaw == "" || sig == "" {
		return ErrMissingSignature
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: expires", ErrInvalidSignature)
	}
	if expires < now.Unix() {
		return ErrExpiredSignature
	}
	expected, err := SignPath(r.Method, r.URL.EscapedPath(), expires, apiKey)
	if err != nil {
		return err
	}
	if !ConstantTimeEqual(sig, expected) {
		return ErrInvalidSignature
	}
	return nil
}

func signaturePayload(method, escapedPath string, expires int64) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == http.MethodHead {
		method = http.MethodGet
	}
	if escapedPath == "" {
		escapedPath = "/"
	}
	if !strings.HasPrefix(escapedPath, "/") {
		escapedPath = "/" + escapedPath
	}
	return method + "\n" + escapedPath + "\n" + strconv.FormatInt(expires, 10)
}
