package servers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/bililive-go/bililive-go/src/pkg/urlresolver"
)

const metadataKeyDouyinCookieUpdatedAt = "douyin_cookie_updated_at"

func getHeadlessBrowserConfig(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: "配置未加载"})
		return
	}
	probe := urlresolver.ProbeHeadlessBrowser(r.Context(), cfg)
	writeJSON(writer, map[string]any{
		"path":            cfg.HeadlessBrowser.Path,
		"auto_install":    cfg.HeadlessBrowser.AutoInstall,
		"timeout_seconds": cfg.HeadlessBrowser.TimeoutSeconds,
		"detected_path":   probe.FoundPath,
		"available":       probe.Available,
		"version":         probe.Version,
		"error":           probe.Error,
	})
}

func updateHeadlessBrowserConfig(writer http.ResponseWriter, r *http.Request) {
	var req struct {
		Path           *string `json:"path"`
		AutoInstall    *bool   `json:"auto_install"`
		TimeoutSeconds *int    `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "请求格式错误: " + err.Error()})
		return
	}
	_, err := configs.UpdateWithRetry(func(c *configs.Config) error {
		if req.Path != nil {
			c.HeadlessBrowser.Path = strings.TrimSpace(*req.Path)
		}
		if req.AutoInstall != nil {
			c.HeadlessBrowser.AutoInstall = *req.AutoInstall
		}
		if req.TimeoutSeconds != nil {
			c.HeadlessBrowser.TimeoutSeconds = *req.TimeoutSeconds
		}
		return c.Verify()
	}, 3, 10*time.Millisecond)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "更新无头浏览器配置失败: " + err.Error()})
		return
	}
	getHeadlessBrowserConfig(writer, r)
}

func probeHeadlessBrowser(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	probe := urlresolver.ProbeHeadlessBrowser(r.Context(), cfg)
	writeJSON(writer, map[string]any{
		"path":          probe.Path,
		"detected_path": probe.FoundPath,
		"available":     probe.Available,
		"version":       probe.Version,
		"error":         probe.Error,
	})
}

func getDouyinCookieConfig(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	updatedAt := ""
	if store := metadata.GetStore(); store != nil {
		updatedAt, _ = store.Get(r.Context(), metadata.NamespaceConfig, metadataKeyDouyinCookieUpdatedAt)
	}
	writeJSON(writer, map[string]any{
		"configured": cfg != nil && strings.TrimSpace(cfg.Douyin.Cookie) != "",
		"updated_at": updatedAt,
	})
}

func putDouyinCookieConfig(writer http.ResponseWriter, r *http.Request) {
	var req struct {
		Cookie string `json:"cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "请求格式错误: " + err.Error()})
		return
	}
	cookie := strings.TrimSpace(req.Cookie)
	_, err := configs.UpdateWithRetry(func(c *configs.Config) error {
		c.Douyin.Cookie = cookie
		return c.Verify()
	}, 3, 10*time.Millisecond)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "保存 Douyin Cookie 失败: " + err.Error()})
		return
	}
	if store := metadata.GetStore(); store != nil {
		value := ""
		if cookie != "" {
			value = time.Now().Format(time.RFC3339)
		}
		_ = store.Set(context.Background(), metadata.NamespaceConfig, metadataKeyDouyinCookieUpdatedAt, value)
	}
	getDouyinCookieConfig(writer, r)
}
