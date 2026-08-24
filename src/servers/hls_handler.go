package servers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"

	"github.com/bililive-go/bililive-go/src/configs"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
)

var (
	hlsCacheLocks      sync.Map
	hlsCacheKeyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	hlsSegmentPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+\.ts$`)
)

type hlsCacheLock struct {
	mutex sync.Mutex
	users atomic.Int64
}

var currentRecordingRelPathSetForHLS = currentRecordingRelPathSet

func getHLSPlaylist(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		http.Error(writer, "配置未加载", http.StatusInternalServerError)
		return
	}
	relPath := trimHLSPlaylistSuffix(mux.Vars(r)["path"])
	if relPath == "" {
		http.Error(writer, "缺少视频路径", http.StatusBadRequest)
		return
	}

	sourcePath, sourceInfo, err := getSafeVideoFile(cfg, relPath)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if isCurrentRecordingFile(r.Context(), cfg.OutPutPath, sourcePath) {
		http.Error(writer, "文件正在录制中，暂不能生成 HLS 播放缓存，请等录制结束后再试", http.StatusConflict)
		return
	}
	cacheKey := hlsCacheKey(relPath, sourceInfo)
	cacheDir := filepath.Join(hlsCacheRoot(cfg), cacheKey)
	playlistPath := filepath.Join(cacheDir, "index.m3u8")

	lock := acquireHLSCacheLock(cacheKey)
	lock.mutex.Lock()
	defer func() {
		lock.mutex.Unlock()
		lock.users.Add(-1)
	}()

	if !isPlaylistComplete(playlistPath) {
		if err := buildHLSCache(cfg, sourcePath, cacheDir, playlistPath); err != nil {
			http.Error(writer, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}

	content, err := os.ReadFile(playlistPath)
	if err != nil {
		http.Error(writer, "读取 HLS 播放列表失败", http.StatusInternalServerError)
		return
	}
	touchHLSCacheDir(cacheDir)
	content = rewriteHLSPlaylist(r, content, cacheKey, cfg)
	writer.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write(content)
}

// trimHLSPlaylistSuffix 兼容以 /playlist.m3u8 或 /index.m3u8 结尾的播放列表地址：
// 鸿蒙 AVPlayer 按 URI 后缀选择解封装器，HLS 地址必须以 .m3u8 结尾才能被识别。
func trimHLSPlaylistSuffix(relPath string) string {
	relPath = strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(relPath)), "/")
	for _, suffix := range []string{"/playlist.m3u8", "/index.m3u8"} {
		if strings.HasSuffix(relPath, suffix) {
			return strings.TrimSuffix(relPath, suffix)
		}
	}
	return relPath
}

func getHLSSegment(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		http.Error(writer, "配置未加载", http.StatusInternalServerError)
		return
	}
	vars := mux.Vars(r)
	cacheKey := vars["cache_key"]
	segment := vars["segment"]
	if !hlsCacheKeyPattern.MatchString(cacheKey) || !hlsSegmentPattern.MatchString(segment) {
		http.Error(writer, "非法 HLS 分段路径", http.StatusBadRequest)
		return
	}
	cacheDir := filepath.Join(hlsCacheRoot(cfg), cacheKey)
	segmentPath := filepath.Join(cacheDir, segment)
	touchHLSCacheDir(cacheDir)
	writer.Header().Set("Content-Type", "video/MP2T")
	writer.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(writer, r, segmentPath)
}

func isPlaylistComplete(path string) bool {
	content, err := os.ReadFile(path)
	return err == nil && bytes.Contains(content, []byte("#EXT-X-ENDLIST"))
}

func isCurrentRecordingFile(ctx context.Context, rootPath, sourcePath string) bool {
	if strings.TrimSpace(rootPath) == "" {
		rootPath = "./"
	}
	absRootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return false
	}
	relPath, err := filepath.Rel(absRootPath, sourcePath)
	if err != nil {
		return false
	}
	return currentRecordingRelPathSetForHLS(ctx, rootPath)[filepath.ToSlash(relPath)]
}

func getSafeVideoFile(cfg *configs.Config, relPath string) (string, os.FileInfo, error) {
	relPath = filepath.ToSlash(relPath)
	absPath, err := getSafePath(cfg.OutPutPath, filepath.FromSlash(relPath))
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, fmt.Errorf("视频路径不能是目录")
	}
	if !videoExtensions[strings.ToLower(filepath.Ext(info.Name()))] {
		return "", nil, fmt.Errorf("不支持的视频文件类型")
	}
	return absPath, info, nil
}

func buildHLSCache(cfg *configs.Config, sourcePath, cacheDir, playlistPath string) error {
	conversionCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ffmpegPath, err := findFFmpegPath(conversionCtx, cfg)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("清理 HLS 缓存失败: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建 HLS 缓存目录失败: %w", err)
	}
	segmentPattern := filepath.Join(cacheDir, "seg_%05d.ts")
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", sourcePath,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	}
	cmd := exec.CommandContext(conversionCtx, ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("HLS 转封装失败: %w: %s", err, strings.TrimSpace(string(output)))
}

func findFFmpegPath(ctx context.Context, cfg *configs.Config) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.FfmpegPath) != "" {
		if _, err := os.Stat(cfg.FfmpegPath); err == nil {
			return cfg.FfmpegPath, nil
		}
	}
	if p, err := utils.GetFFmpegPath(ctx); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	for _, candidate := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("ffmpeg 未安装或未配置")
}

func hlsCacheRoot(cfg *configs.Config) string {
	appDataPath := ".appdata"
	if cfg != nil && strings.TrimSpace(cfg.AppDataPath) != "" {
		appDataPath = cfg.AppDataPath
	}
	return filepath.Join(appDataPath, "hls-cache")
}

func hlsCacheKey(relPath string, info os.FileInfo) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		filepath.ToSlash(relPath),
		strconv.FormatInt(info.ModTime().UnixNano(), 10),
		strconv.FormatInt(info.Size(), 10),
	}, "\n")))
	return hex.EncodeToString(sum[:])
}

func acquireHLSCacheLock(cacheKey string) *hlsCacheLock {
	for {
		value, _ := hlsCacheLocks.LoadOrStore(cacheKey, &hlsCacheLock{})
		lock := value.(*hlsCacheLock)
		for {
			users := lock.users.Load()
			if users < 0 {
				break
			}
			if lock.users.CompareAndSwap(users, users+1) {
				return lock
			}
		}
	}
}

func deleteHLSCacheLockIfUnused(cacheKey string) {
	value, ok := hlsCacheLocks.Load(cacheKey)
	if !ok {
		return
	}
	lock := value.(*hlsCacheLock)
	if lock.users.CompareAndSwap(0, -1) {
		hlsCacheLocks.CompareAndDelete(cacheKey, lock)
	}
}

func touchHLSCacheDir(cacheDir string) {
	now := time.Now()
	_ = os.Chtimes(cacheDir, now, now)
}

func rewriteHLSPlaylist(r *http.Request, content []byte, cacheKey string, cfg *configs.Config) []byte {
	lines := strings.Split(string(content), "\n")
	signingKey := signingSecretForRequest(r, cfg)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			continue
		}
		segment := filepath.Base(trimmed)
		if !hlsSegmentPattern.MatchString(segment) {
			continue
		}
		rawURL := joinEscapedURLPath("/api/stream/hls-segment/"+cacheKey, segment)
		if strings.TrimSpace(signingKey) == "" {
			lines[i] = rawURL
			continue
		}
		if urlValue, _, err := securitypkg.SignURL(rawURL, http.MethodGet, time.Now().Add(signedURLTTL(cfg)), signingKey); err == nil {
			lines[i] = urlValue
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func signedHLSURLForVideoFile(r *http.Request, ext, relPath string, cfg *configs.Config) string {
	switch strings.ToLower(ext) {
	case ".flv", ".ts", ".mkv":
		return signedURLForAsset(r, "hls", relPath, cfg)
	default:
		return ""
	}
}
