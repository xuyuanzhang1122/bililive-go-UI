package servers

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
)

const maxSignedURLTTLSeconds = 24 * 60 * 60

type signedURLResponse struct {
	URL       string `json:"url"`
	Expires   int64  `json:"expires"`
	ExpiresIn int    `json:"expires_in"`
}

func createSignedURL(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{
			ErrNo:  http.StatusInternalServerError,
			ErrMsg: "配置未加载",
		})
		return
	}

	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	relPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if kind == "" || relPath == "" {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo:  http.StatusBadRequest,
			ErrMsg: "缺少 kind 或 path 参数",
		})
		return
	}
	relPath = filepath.ToSlash(relPath)
	if err := validateSignedURLTarget(cfg, kind, relPath); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo:  http.StatusBadRequest,
			ErrMsg: err.Error(),
		})
		return
	}

	ttl := signedURLTTL(cfg)
	if raw := strings.TrimSpace(r.URL.Query().Get("expires_in")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
				ErrNo:  http.StatusBadRequest,
				ErrMsg: "expires_in 必须为正整数秒数",
			})
			return
		}
		if seconds > maxSignedURLTTLSeconds {
			seconds = maxSignedURLTTLSeconds
		}
		ttl = time.Duration(seconds) * time.Second
	}

	rawURL, err := signedURLRawPath(kind, relPath)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo:  http.StatusBadRequest,
			ErrMsg: err.Error(),
		})
		return
	}
	signingKey := signingSecretForRequest(r, cfg)
	urlValue, expires, err := securitypkg.SignURL(rawURL, http.MethodGet, time.Now().Add(ttl), signingKey)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{
			ErrNo:  http.StatusInternalServerError,
			ErrMsg: "生成签名 URL 失败: " + err.Error(),
		})
		return
	}
	writeJSON(writer, commonResp{
		Data: signedURLResponse{
			URL:       urlValue,
			Expires:   expires,
			ExpiresIn: int(ttl.Seconds()),
		},
	})
}

func validateSignedURLTarget(cfg *configs.Config, kind, relPath string) error {
	switch kind {
	case "file", "thumbnail", "hls":
	default:
		return errInvalidSignedURLKind(kind)
	}
	absPath, err := getSafePath(cfg.OutPutPath, filepath.FromSlash(relPath))
	if err != nil {
		return err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errSignedURLTargetIsDir
	}
	return nil
}

func signedURLRawPath(kind, relPath string) (string, error) {
	switch kind {
	case "file":
		return joinEscapedURLPath("/files", relPath), nil
	case "thumbnail":
		return joinEscapedURLPath("/api/thumbnail", relPath), nil
	case "hls":
		return joinEscapedURLPath("/api/stream/hls", relPath), nil
	default:
		return "", errInvalidSignedURLKind(kind)
	}
}

func signedURLForAsset(r *http.Request, kind, relPath string, cfg *configs.Config) string {
	signingKey := signingSecretForRequest(r, cfg)
	if strings.TrimSpace(signingKey) == "" {
		return ""
	}
	rawURL, err := signedURLRawPath(kind, relPath)
	if err != nil {
		return ""
	}
	urlValue, _, err := securitypkg.SignURL(rawURL, http.MethodGet, time.Now().Add(signedURLTTL(cfg)), signingKey)
	if err != nil {
		return ""
	}
	return urlValue
}

func signedURLTTL(cfg *configs.Config) time.Duration {
	seconds := cfg.Security.SignedURLTTLSeconds
	if seconds <= 0 {
		seconds = 3600
	}
	if seconds > maxSignedURLTTLSeconds {
		seconds = maxSignedURLTTLSeconds
	}
	return time.Duration(seconds) * time.Second
}

func joinEscapedURLPath(prefix, relPath string) string {
	prefix = "/" + strings.Trim(strings.TrimSpace(prefix), "/")
	relPath = strings.Trim(filepath.ToSlash(relPath), "/")
	if relPath == "" {
		return prefix
	}
	parts := strings.Split(relPath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return prefix + "/" + strings.Join(parts, "/")
}

type signedURLError string

func (e signedURLError) Error() string { return string(e) }

const errSignedURLTargetIsDir = signedURLError("签名 URL 目标不能是目录")

func errInvalidSignedURLKind(kind string) error {
	return signedURLError("不支持的签名 URL 类型: " + kind)
}
