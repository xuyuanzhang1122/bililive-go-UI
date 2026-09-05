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

	// 使用新的任务管理器
	job, created := globalHLSJobManager.GetOrCreateJob(cfg, cacheKey, sourcePath, cacheDir, playlistPath)

	if created {
		// 新创建的任务，启动它
		if err := globalHLSJobManager.StartJob(cfg, job); err != nil {
			applog.GetLogger().WithError(err).WithField("path", relPath).Warn("启动 HLS 任务失败")
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "processing", RetryAfter: 2}})
		return
	}

	// 已存在的任务，检查状态
	status, errMsg := job.GetStatus()

	switch status {
	case hlsJobCompleted, hlsJobPlayable, hlsJobPartial:
		// 可以播放
		urlValue := signedHLSURLForVideoFile(r, ext, relPath, cfg)
		if urlValue == "" {
			urlValue = joinEscapedURLPath("/api/stream/hls", relPath) + "/playlist.m3u8"
		}
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{
			Status: "ready", Protocol: "hls", MimeType: "application/vnd.apple.mpegurl", URL: urlValue, ExpiresAt: playbackURLExpiresAt(urlValue),
		}})
		return

	case hlsJobFailed:
		// 失败
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "failed", Error: errMsg, RetryAfter: 5}})
		return

	case hlsJobQueued, hlsJobStarting:
		// 处理中
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "processing", RetryAfter: 2}})
		return

	default:
		// 未知状态
		writeJSON(writer, commonResp{ErrNo: 0, Data: playbackResolveResponse{Status: "processing", RetryAfter: 2}})
		return
	}
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
