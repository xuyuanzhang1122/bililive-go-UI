package servers

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
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
	if r.URL.Query().Get("hls_profile") != "" && resolvePersistentHLS(writer, r, cfg, relPath) {
		return
	}

	// 保持原录播 relPath 身份，同时把播放地址指向已准备的产物。
	if target, original, ok := preparedPlayback(cfg, relPath); ok {
		originalPath, _ := getSafePath(cfg.OutPutPath, filepath.FromSlash(original))
		if !isCurrentRecordingFile(r.Context(), cfg.OutPutPath, originalPath) {
			urlValue := signedURLForAsset(r, "file", target, cfg)
			if urlValue == "" {
				urlValue = joinEscapedURLPath("/files", target)
			}
			writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "ready", Protocol: "file", MimeType: "video/mp4", URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue)}})
			return
		}
	}
	// 改名/可信替换后的旧入口保持原请求身份，媒体地址使用目录核验的现存路径。
	resolvedPath := relPath
	if inst := instance.GetInstance(r.Context()); inst != nil && inst.RecordingCatalog != nil && inst.RecordingCatalog.MatchesRoot(cfg.OutPutPath) {
		if target, err := inst.RecordingCatalog.ResolveAlias(r.Context(), relPath); err == nil {
			resolvedPath = target
		}
	}
	sourcePath, sourceInfo, err := getSafeVideoFile(cfg, resolvedPath)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusNotFound, commonResp{ErrNo: http.StatusNotFound, ErrMsg: err.Error()})
		return
	}
	if isCurrentRecordingFile(r.Context(), cfg.OutPutPath, sourcePath) {
		writePlaybackResponse(writer, playbackResolveResponse{Status: "recording", Error: "文件正在录制中", RetryAfter: 10})
		return
	}

	ext := strings.ToLower(filepath.Ext(sourceInfo.Name()))
	if ext != ".flv" && ext != ".ts" && ext != ".mkv" {
		urlValue := signedURLForAsset(r, "file", resolvedPath, cfg)
		if urlValue == "" {
			urlValue = joinEscapedURLPath("/files", resolvedPath)
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{
			Status: "ready", Protocol: "file", MimeType: mimeTypeForVideoExtension(ext), URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue),
		}})
		return
	}

	cacheKey := hlsCacheKey(resolvedPath, sourceInfo)
	if resolvePersistentHLS(writer, r, cfg, resolvedPath) {
		return
	}
	cacheDir := filepath.Join(hlsCacheRoot(cfg), cacheKey)
	playlistPath := filepath.Join(cacheDir, "index.m3u8")

	// 使用统一的任务管理器（A1-1：统一入口）
	job := globalHLSJobManager.EnsureJob(cfg, cacheKey, sourcePath, cacheDir, playlistPath)

	// 检查当前状态
	status, errMsg := job.GetStatus()

	switch status {
	case hlsJobCompleted, hlsJobPlayable:
		// 可以播放
		urlValue := signedHLSURLForVideoFile(r, ext, resolvedPath, cfg)
		if urlValue == "" {
			urlValue = joinEscapedURLPath("/api/stream/hls", resolvedPath) + "/playlist.m3u8"
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{
			Status: "ready", Protocol: "hls", MimeType: "application/vnd.apple.mpegurl", URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue),
		}})
		return

	case hlsJobFailed, hlsJobPartial:
		// 失败
		writePlaybackResponse(writer, playbackResolveResponse{Status: "failed", Error: errMsg, RetryAfter: globalHLSJobManager.RetryAfter(job)})
		return

	case hlsJobQueued, hlsJobStarting:
		// 处理中
		writePlaybackResponse(writer, playbackResolveResponse{Status: "processing", RetryAfter: 2})
		return

	default:
		// 未知状态
		writePlaybackResponse(writer, playbackResolveResponse{Status: "processing", RetryAfter: 2})
		return
	}
}

func writePlaybackResponse(w http.ResponseWriter, data playbackResolveResponse) {
	if data.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(data.RetryAfter))
	}
	if strings.Contains(data.Error, "队列已满") || strings.Contains(data.Error, "任务记录已满") {
		writeJsonWithStatusCode(w, http.StatusTooManyRequests, commonResp{ErrNo: 0, Data: data})
		return
	}
	writeJSON(w, commonResp{ErrNo: 0, Data: data})
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
