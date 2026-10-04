package servers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
	"github.com/gorilla/mux"
)

func requestCatalog(r *http.Request, cfg *configs.Config) *recordingcatalog.Manager {
	inst := instance.GetInstance(r.Context())
	if inst == nil || inst.RecordingCatalog == nil || !inst.RecordingCatalog.MatchesRoot(cfg.OutPutPath) {
		return nil
	}
	return inst.RecordingCatalog
}
func verifyHLSManifest(r *http.Request, cat *recordingcatalog.Manager, asset recordingcatalog.HLSAsset) (hlsasset.Manifest, error) {
	root := filepath.Dir(asset.Path)
	m, err := hlsasset.Load(root)
	if err != nil {
		return m, err
	}
	if m.RecordingID != asset.Entry.RecordingID || m.SourceVersion != asset.Entry.SourceVersion || m.Digest() != asset.ManifestDigest || filepath.Base(root) != m.SourceVersion+"-"+hlsasset.ProfileVersion(m.Profile) {
		return m, fmt.Errorf("持久 HLS 关联已失效")
	}
	current, err := recordingcatalog.SnapshotVersion(asset.Path)
	if err != nil {
		return m, err
	}
	if current != asset.FileVersion {
		return m, fmt.Errorf("持久 HLS 清单身份变化")
	}
	entry, err := cat.HLSSource(r.Context(), asset.Entry)
	if err != nil {
		return m, err
	}
	if entry.RecordingID != m.RecordingID {
		return m, fmt.Errorf("持久 HLS 来源归属变化")
	}
	return m, nil
}
func persistentHLSURL(r *http.Request, cfg *configs.Config, m hlsasset.Manifest, name string) string {
	raw := joinEscapedURLPath("/api/stream/hls-asset", m.RecordingID+"/"+m.SourceVersion+"-"+hlsasset.ProfileVersion(m.Profile)+"/"+name)
	key := signingSecretForRequest(r, cfg)
	if strings.TrimSpace(key) == "" {
		return raw
	}
	signed, _, err := securitypkg.SignURL(raw, http.MethodGet, time.Now().Add(signedURLTTL(cfg)), key)
	if err != nil {
		return ""
	}
	return signed
}
func servePersistentHLS(w http.ResponseWriter, r *http.Request, cfg *configs.Config, cat *recordingcatalog.Manager, asset recordingcatalog.HLSAsset, name string) {
	m, err := verifyHLSManifest(r, cat, asset)
	if err != nil {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "持久 HLS 已失效: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	f, err := m.OpenFile(filepath.Dir(asset.Path), name)
	if err != nil {
		w.Header().Set("Retry-After", "2")
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer f.Close()
	if name == "index.m3u8" {
		data, err := io.ReadAll(io.LimitReader(f, 4<<20))
		if err != nil {
			http.Error(w, "读取持久清单失败", http.StatusInternalServerError)
			return
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if line != "" && !strings.HasPrefix(line, "#") {
				lines[i] = persistentHLSURL(r, cfg, m, line)
			}
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(strings.Join(lines, "\n")))
		return
	}
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "读取持久分片失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "video/MP2T")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, name, info.ModTime(), f)
}
func tryPersistentHLS(w http.ResponseWriter, r *http.Request, cfg *configs.Config, rel string) bool {
	cat := requestCatalog(r, cfg)
	if cat == nil {
		return false
	}
	profile := r.URL.Query().Get("hls_profile")
	if profile == "" {
		profile = "source"
		if len(cfg.OnRecordFinished.PrepareHLSProfiles) > 0 {
			profile = cfg.OnRecordFinished.PrepareHLSProfiles[0]
		}
	}
	if profile != "source" && profile != "720p" && profile != "1080p" {
		http.Error(w, "未知 HLS 档位", http.StatusBadRequest)
		return true
	}
	asset, err := cat.HLSForPath(r.Context(), rel, profile)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		http.Error(w, "持久 HLS 目录不可用", http.StatusServiceUnavailable)
		return true
	}
	servePersistentHLS(w, r, cfg, cat, asset, "index.m3u8")
	return true
}
func getPersistentHLSFile(w http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		http.Error(w, "配置未加载", http.StatusInternalServerError)
		return
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		http.Error(w, "目录未就绪", http.StatusServiceUnavailable)
		return
	}
	vars := mux.Vars(r)
	name := vars["file"]
	if name != "index.m3u8" && !hlsSegmentPattern.MatchString(name) {
		http.Error(w, "非法持久 HLS 文件", http.StatusBadRequest)
		return
	}
	asset, err := cat.FindHLS(r.Context(), vars["recording_id"], vars["asset_version"])
	if err != nil {
		http.Error(w, "持久 HLS 不可用", http.StatusNotFound)
		return
	}
	servePersistentHLS(w, r, cfg, cat, asset, name)
}

func resolvePersistentHLS(w http.ResponseWriter, r *http.Request, cfg *configs.Config, rel string) bool {
	profile := r.URL.Query().Get("hls_profile")
	explicit := profile != ""
	if !explicit {
		profile = "source"
		if len(cfg.OnRecordFinished.PrepareHLSProfiles) > 0 {
			profile = cfg.OnRecordFinished.PrepareHLSProfiles[0]
		}
	}
	cat := requestCatalog(r, cfg)
	if cat != nil {
		asset, err := cat.HLSForPath(r.Context(), rel, profile)
		if err == nil {
			m, err := verifyHLSManifest(r, cat, asset)
			if err != nil {
				writePlaybackResponse(w, playbackResolveResponse{Status: "failed", Error: err.Error()})
				return true
			}
			url := persistentHLSURL(r, cfg, m, "index.m3u8")
			writePlaybackResponse(w, playbackResolveResponse{Status: "ready", Protocol: "hls", MimeType: "application/vnd.apple.mpegurl", URL: url, ExpiresAt: playbackURLExpiresAt(url)})
			return true
		}
		if err != sql.ErrNoRows && !explicit {
			writePlaybackResponse(w, playbackResolveResponse{Status: "failed", Error: err.Error()})
			return true
		}
	}
	if explicit {
		writePlaybackResponse(w, playbackResolveResponse{Status: "failed", Error: "此持久 HLS 档位尚未准备或已失效"})
		return true
	}
	return false
}
