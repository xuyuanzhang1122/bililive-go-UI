package servers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

// preparedPlayback 只接受验证后发布的同目录来源关联，不猜测同名 MP4。
func preparedPlayback(cfg *configs.Config, rel string) (target, original string, ok bool) {
	source, err := getSafePath(cfg.OutPutPath, filepath.FromSlash(rel))
	if err != nil {
		return "", "", false
	}
	base := strings.TrimSuffix(source, filepath.Ext(source))
	candidates := []string{source, base + ".mp4", base + ".prepared.mp4"}
	for _, candidate := range candidates {
		m, err := playbackasset.Load(candidate)
		if err != nil {
			continue
		}
		canonical := filepath.Join(filepath.Dir(candidate), m.SourceName)
		if source != canonical && source != candidate {
			continue
		}
		root, _ := filepath.Abs(cfg.OutPutPath)
		targetRel, e := filepath.Rel(root, candidate)
		if e != nil {
			continue
		}
		originalRel, e := filepath.Rel(root, canonical)
		if e != nil {
			continue
		}
		// 最终交给既有安全文件解析，包含格式和边界检查。
		if _, _, e = getSafeVideoFile(cfg, filepath.ToSlash(targetRel)); e != nil {
			continue
		}
		return filepath.ToSlash(targetRel), filepath.ToSlash(originalRel), true
	}
	return "", "", false
}
func preferPreparedFiles(r *http.Request, cfg *configs.Config, files []VideoFileInfo) []VideoFileInfo {
	result := make([]VideoFileInfo, 0, len(files))
	seen := make(map[string]bool)
	for _, file := range files {
		if !file.Recording {
			if target, original, ok := preparedPlayback(cfg, file.RelPath); ok {
				_, info, err := getSafeVideoFile(cfg, target)
				if err != nil {
					continue
				}
				originalPath, _ := getSafePath(cfg.OutPutPath, filepath.FromSlash(original))
				if isCurrentRecordingFile(r.Context(), cfg.OutPutPath, originalPath) {
					continue
				}
				file.RelPath = original
				file.Name = filepath.Base(original)
				file.Size = info.Size()
				file.ModTime = info.ModTime().Unix()
				file.FileURL = signedURLForAsset(r, "file", target, cfg)
				if file.FileURL == "" {
					file.FileURL = joinEscapedURLPath("/files", target)
				}
				file.ThumbnailURL = signedURLForAsset(r, "thumbnail", target, cfg)
				file.HLSURL = ""
				file.PlaybackStatus = "ready"
			}
		}
		if seen[file.RelPath] {
			continue
		}
		seen[file.RelPath] = true
		result = append(result, file)
	}
	return result
}
