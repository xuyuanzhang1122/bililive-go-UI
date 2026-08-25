package servers

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/mux"

	"github.com/bililive-go/bililive-go/src/configs"
	applog "github.com/bililive-go/bililive-go/src/log"
)

type playbackResolveResponse struct {
	Status     string `json:"status"`
	Protocol   string `json:"protocol,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
	URL        string `json:"url,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	Error      string `json:"error,omitempty"`
	RetryAfter int    `json:"retry_after_seconds,omitempty"`
}

type hlsBuildState struct {
	status string
	err    string
}

var hlsBuildStates = struct {
	sync.Mutex
	items map[string]hlsBuildState
}{items: make(map[string]hlsBuildState)}

func resolvePlayback(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: "配置未加载"})
		return
	}
	relPath := strings.TrimSpace(mux.Vars(r)["path"])
	if relPath == "" {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "缺少视频路径"})
		return
	}

	sourcePath, sourceInfo, err := getSafeVideoFile(cfg, relPath)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusNotFound, commonResp{ErrNo: http.StatusNotFound, ErrMsg: err.Error()})
		return
	}
	if isCurrentRecordingFile(r.Context(), cfg.OutPutPath, sourcePath) {
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "recording", Error: "文件正在录制中", RetryAfter: 10}})
		return
	}

	ext := strings.ToLower(filepath.Ext(sourceInfo.Name()))
	if ext != ".flv" && ext != ".ts" && ext != ".mkv" {
		urlValue := signedURLForAsset(r, "file", relPath, cfg)
		if urlValue == "" {
			urlValue = joinEscapedURLPath("/files", relPath)
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{
			Status: "ready", Protocol: "file", MimeType: mimeTypeForVideoExtension(ext), URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue),
		}})
		return
	}

	cacheKey := hlsCacheKey(relPath, sourceInfo)
	cacheDir := filepath.Join(hlsCacheRoot(cfg), cacheKey)
	playlistPath := filepath.Join(cacheDir, "index.m3u8")
	if isPlaylistComplete(playlistPath) {
		urlValue := signedHLSURLForVideoFile(r, ext, relPath, cfg)
		if urlValue == "" {
			urlValue = joinEscapedURLPath("/api/stream/hls", relPath) + "/playlist.m3u8"
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{
			Status: "ready", Protocol: "hls", MimeType: "application/vnd.apple.mpegurl", URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue),
		}})
		return
	}

	if beginHLSBuild(cfg, relPath, sourcePath, cacheDir, playlistPath, cacheKey) {
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "processing", RetryAfter: 2}})
		return
	}
	state := getHLSBuildState(cacheKey)
	if state.status == "failed" {
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "failed", Error: state.err, RetryAfter: 5}})
		return
	}
	writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "processing", RetryAfter: 2}})
}

func beginHLSBuild(cfg *configs.Config, relPath, sourcePath, cacheDir, playlistPath, cacheKey string) bool {
	hlsBuildStates.Lock()
	if state, ok := hlsBuildStates.items[cacheKey]; ok && state.status == "processing" {
		hlsBuildStates.Unlock()
		return false
	}
	hlsBuildStates.items[cacheKey] = hlsBuildState{status: "processing"}
	hlsBuildStates.Unlock()

	go func() {
		lock := acquireHLSCacheLock(cacheKey)
		lock.mutex.Lock()
		err := error(nil)
		if !isPlaylistComplete(playlistPath) {
			err = buildHLSCache(cfg, sourcePath, cacheDir, playlistPath)
		}
		lock.mutex.Unlock()
		lock.users.Add(-1)

		hlsBuildStates.Lock()
		if err != nil {
			hlsBuildStates.items[cacheKey] = hlsBuildState{status: "failed", err: err.Error()}
			applog.GetLogger().WithError(err).WithField("path", relPath).Warn("后台 HLS 播放缓存构建失败")
		} else {
			hlsBuildStates.items[cacheKey] = hlsBuildState{status: "ready"}
		}
		hlsBuildStates.Unlock()
	}()
	return true
}

func getHLSBuildState(cacheKey string) hlsBuildState {
	hlsBuildStates.Lock()
	defer hlsBuildStates.Unlock()
	return hlsBuildStates.items[cacheKey]
}

func playbackURLExpiresAt(urlValue string) int64 {
	parsed, err := url.Parse(urlValue)
	if err != nil {
		return 0
	}
	expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	if err != nil || expires <= 0 {
		return 0
	}
	return expires
}

func mimeTypeForVideoExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}
