package servers

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
)

// 录制断流重连会按时间戳另起新文件，留下大量小体积碎片（部分因截断而不可播放）。
// 这里提供"待清理小文件"的列举与确认删除/保留接口，删除必须由用户显式确认。

const (
	cleanupIgnoredNamespace   = "cleanup_ignored"
	defaultCleanupThresholdMB = 50
)

// CleanupCandidate 待清理的小文件
type CleanupCandidate struct {
	Name    string `json:"name"`
	RelPath string `json:"rel_path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

type cleanupActionRequest struct {
	Action   string   `json:"action"` // delete | keep
	RelPaths []string `json:"rel_paths"`
}

type cleanupActionResult struct {
	RelPath string `json:"rel_path"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// getCleanupCandidates 列出小于阈值且不在录制中的视频文件
// 路由：GET /api/cleanup-candidates?threshold_mb=50
func getCleanupCandidates(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		writeJSON(writer, []CleanupCandidate{})
		return
	}
	rootPath := cfg.OutPutPath
	if rootPath == "" {
		rootPath = "./"
	}

	thresholdMB := defaultCleanupThresholdMB
	if v := r.URL.Query().Get("threshold_mb"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 4096 {
			thresholdMB = n
		}
	}
	thresholdBytes := int64(thresholdMB) * 1024 * 1024

	recordingFiles := currentRecordingRelPathSet(r.Context(), rootPath)

	ignored := map[string]string{}
	if store := metadata.GetStore(); store != nil {
		if m, err := store.GetAll(r.Context(), cleanupIgnoredNamespace); err == nil {
			ignored = m
		}
	}

	candidates := make([]CleanupCandidate, 0)
	_ = filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// 跳过隐藏目录与 .appdata 等系统目录
			if strings.HasPrefix(d.Name(), ".") && path != rootPath {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !videoExtensions[ext] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() >= thresholdBytes {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if recordingFiles[relSlash] {
			return nil
		}
		if _, ok := ignored[relSlash]; ok {
			return nil
		}
		candidates = append(candidates, CleanupCandidate{
			Name:    d.Name(),
			RelPath: relSlash,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
		return nil
	})

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModTime > candidates[j].ModTime
	})
	writeJSON(writer, candidates)
}

// postCleanupAction 对候选文件执行用户确认后的操作
// 路由：POST /api/cleanup-candidates/action
// body: {"action": "delete"|"keep", "rel_paths": ["..."]}
func postCleanupAction(writer http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		writeJsonWithStatusCode(writer, http.StatusServiceUnavailable, commonResp{
			ErrNo: http.StatusServiceUnavailable, ErrMsg: "配置未加载",
		})
		return
	}
	rootPath := cfg.OutPutPath
	if rootPath == "" {
		rootPath = "./"
	}

	var req cleanupActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo: http.StatusBadRequest, ErrMsg: "请求体解析失败: " + err.Error(),
		})
		return
	}
	if req.Action != "delete" && req.Action != "keep" {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo: http.StatusBadRequest, ErrMsg: "action 必须为 delete 或 keep",
		})
		return
	}
	if len(req.RelPaths) == 0 || len(req.RelPaths) > 500 {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{
			ErrNo: http.StatusBadRequest, ErrMsg: "rel_paths 数量非法",
		})
		return
	}

	absRoot, _ := filepath.Abs(rootPath)
	recordingFiles := currentRecordingRelPathSet(r.Context(), rootPath)
	store := metadata.GetStore()

	results := make([]cleanupActionResult, 0, len(req.RelPaths))
	for _, relPath := range req.RelPaths {
		res := cleanupActionResult{RelPath: relPath}
		relSlash := filepath.ToSlash(relPath)

		absTarget, err := filepath.Abs(filepath.Join(rootPath, filepath.FromSlash(relSlash)))
		if err != nil || !strings.HasPrefix(filepath.Clean(absTarget)+string(filepath.Separator), filepath.Clean(absRoot)+string(filepath.Separator)) {
			res.Error = "非法路径"
			results = append(results, res)
			continue
		}

		switch req.Action {
		case "keep":
			if store == nil {
				res.Error = "元数据存储不可用"
			} else if err := store.Set(r.Context(), cleanupIgnoredNamespace, relSlash, time.Now().Format(time.RFC3339)); err != nil {
				res.Error = err.Error()
			} else {
				res.OK = true
			}
		case "delete":
			if recordingFiles[relSlash] {
				res.Error = "文件正在录制中"
				results = append(results, res)
				continue
			}
			if err := os.Remove(absTarget); err != nil {
				res.Error = err.Error()
			} else {
				res.OK = true
				removeCleanupSideEffects(r, cfg, relSlash)
			}
		}
		results = append(results, res)
	}
	writeJSON(writer, results)
}

// removeCleanupSideEffects 删除文件后的附属清理：缩略图缓存与"保留"标记
func removeCleanupSideEffects(r *http.Request, cfg *configs.Config, relSlash string) {
	appDataPath := cfg.AppDataPath
	if appDataPath == "" {
		appDataPath = ".appdata"
	}
	thumbName := strings.ReplaceAll(relSlash, "/", "_") + "_v2.jpg"
	_ = os.Remove(filepath.Join(appDataPath, "thumbnails", thumbName))
	if store := metadata.GetStore(); store != nil {
		_ = store.Delete(r.Context(), cleanupIgnoredNamespace, relSlash)
	}
}
