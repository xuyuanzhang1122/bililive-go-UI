package servers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
	"github.com/gorilla/mux"
)

type v2TokenClaims struct {
	SessionID     string `json:"session_id"`
	SourceVersion string `json:"source_version"`
	AssetVersion  string `json:"asset_version"`
	Expires       int64  `json:"expires"`
}

func (s *v2Service) signToken(session v2Session, expires int64) (string, error) {
	data, err := json.Marshal(v2TokenClaims{session.DTO.ID, session.DTO.SourceVersion, session.DTO.AssetVersion, expires})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (s *v2Service) readToken(raw string) (v2TokenClaims, bool) {
	var claims v2TokenClaims
	if len(raw) > 4096 {
		return claims, false
	}
	payload, signature, ok := strings.Cut(raw, ".")
	if !ok {
		return claims, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return claims, false
	}
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(payload))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return claims, false
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(data, &claims) != nil {
		return claims, false
	}
	return claims, true
}
func v2MediaURL(id, name, token string) string {
	return joinEscapedURLPath("/api/v2/media", id+"/"+name) + "?play_token=" + url.QueryEscape(token)
}
func v2CredentialActive(r *http.Request, session v2Session, cfg *configs.Config) bool {
	if session.Owner == metadata.DefaultAPIKeyUserID {
		return cfg != nil && securitypkg.HashAPIKey(cfg.Security.APIKey) == session.Credential && strings.TrimSpace(cfg.Security.APIKey) != ""
	}
	store := metadata.GetStore()
	if store == nil {
		return false
	}
	user, err := store.GetAPIKeyUserByID(r.Context(), session.Owner)
	return err == nil && user.Enabled && user.RevokedAt == nil
}
func (s *v2Service) media(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("play_token")
	if raw == "" {
		v2Fail(w, r, http.StatusUnauthorized, "unauthorized", "缺少播放授权", false)
		return
	}
	claims, ok := s.readToken(raw)
	vars := mux.Vars(r)
	if !ok || claims.SessionID != vars["id"] {
		v2Fail(w, r, http.StatusForbidden, "invalid_play_token", "播放授权无效或会话不符", false)
		return
	}
	if claims.Expires <= s.now().Unix() {
		v2Fail(w, r, http.StatusGone, "play_token_expired", "播放授权已过期，请刷新会话", false)
		return
	}
	s.mu.Lock()
	session, exists := s.sessions[claims.SessionID]
	s.mu.Unlock()
	if !exists || session.DTO.ExpiresAt <= s.now().Unix() || session.DTO.Status == "cancelled" {
		v2Fail(w, r, http.StatusGone, "session_expired", "播放会话已过期、取消或服务已重启", false)
		return
	}
	if session.DTO.Status != "ready" || session.Invalidated || claims.SourceVersion != session.DTO.SourceVersion || claims.AssetVersion != session.DTO.AssetVersion || claims.Expires > session.DTO.ExpiresAt {
		v2Fail(w, r, http.StatusForbidden, "invalid_play_token", "授权资产或会话状态不符", false)
		return
	}
	cfg := configs.GetCurrentConfig()
	if !v2CredentialActive(r, session, cfg) {
		v2Fail(w, r, http.StatusForbidden, "authorization_revoked", "管理授权已撤销", false)
		return
	}
	if cfg.OutPutPath != session.Root {
		v2Fail(w, r, http.StatusConflict, "source_changed", "录播根目录已变化", false)
		return
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "录播目录未就绪", true)
		return
	}
	status, err := cat.Status(r.Context())
	if err != nil || status != "verified" {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录尚未核验或不可用", true)
		return
	}
	name := vars["file"]
	if session.File != nil {
		if name != "file" {
			v2Fail(w, r, http.StatusNotFound, "media_not_found", "此会话没有该媒体文件", false)
			return
		}
		current, err := cat.RebindPlaybackFile(r.Context(), *session.File)
		if err != nil {
			v2Fail(w, r, http.StatusConflict, "source_changed", "媒体或来源版本已变化", false)
			return
		}
		session.File = &current
		before, err := os.Lstat(session.File.Path)
		if err != nil {
			v2Fail(w, r, http.StatusConflict, "source_changed", "媒体已消失", false)
			return
		}
		f, err := os.Open(session.File.Path)
		if err != nil {
			v2Fail(w, r, http.StatusServiceUnavailable, "media_unavailable", "媒体打开失败", true)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || v2VerifyOpenedPlaybackFile(r.Context(), cat, *session.File, before, f) != nil {
			v2Fail(w, r, http.StatusConflict, "source_changed", "媒体打开期间变化", false)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", session.MimeType)
		http.ServeContent(w, r, filepath.Base(session.File.Path), info.ModTime(), f)
		return
	}
	if session.HLSProfile == "" || name != "index.m3u8" && !hlsSegmentPattern.MatchString(name) {
		v2Fail(w, r, http.StatusNotFound, "media_not_found", "此会话没有该 HLS 文件", false)
		return
	}
	asset, err := cat.FindHLS(r.Context(), session.DTO.RecordingID, session.DTO.AssetVersion)
	if err != nil {
		v2Fail(w, r, http.StatusConflict, "source_changed", "所选 HLS 已失效", false)
		return
	}
	manifest, err := verifyHLSManifest(r, cat, asset)
	if err != nil {
		v2Fail(w, r, http.StatusConflict, "source_changed", "HLS 或来源版本已变化", false)
		return
	}
	found := false
	for _, member := range manifest.Files {
		found = found || member.Name == name
	}
	if !found {
		v2Fail(w, r, http.StatusNotFound, "media_not_found", "清单中不存在此分片", false)
		return
	}
	f, err := manifest.OpenFile(filepath.Dir(asset.Path), name)
	if err != nil {
		v2Fail(w, r, http.StatusConflict, "asset_changed", "HLS 文件核验失败", false)
		return
	}
	defer f.Close()
	w.Header().Set("Cache-Control", "private, no-store")
	if name == "index.m3u8" {
		data, err := io.ReadAll(io.LimitReader(f, 4<<20))
		if err != nil {
			v2Fail(w, r, http.StatusServiceUnavailable, "media_unavailable", "读取 HLS 清单失败", true)
			return
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if line != "" && !strings.HasPrefix(line, "#") {
				lines[i] = v2MediaURL(session.DTO.ID, line, raw)
			}
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(strings.Join(lines, "\n")))
		}
		return
	}
	info, err := f.Stat()
	if err != nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "media_unavailable", "读取 HLS 分片失败", true)
		return
	}
	w.Header().Set("Content-Type", "video/MP2T")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

func v2VerifyOpenedPlaybackFile(ctx context.Context, cat *recordingcatalog.Manager, selected recordingcatalog.PlaybackFile, before os.FileInfo, f *os.File) error {
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) {
		return recordingcatalog.ErrPlaybackChanged
	}
	actual, err := recordingcatalog.SnapshotOpenedFileVersion(f)
	if err != nil || actual != selected.FileVersion {
		return recordingcatalog.ErrPlaybackChanged
	}
	return cat.VerifyPlaybackFile(ctx, selected)
}
