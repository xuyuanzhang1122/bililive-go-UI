package servers

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

// 来源已删除时，可信关联的版本必须与既有作业完全一致才能延续旧清单。
// 排队、启动和失败作业无法保证已打开来源，交由 MP4 重新生成。
func preparedOriginalHLSJob(cfg *configs.Config, original, target string) *hlsJob {
	targetPath, err := getSafePath(cfg.OutPutPath, filepath.FromSlash(target))
	if err != nil {
		return nil
	}
	manifest, err := playbackasset.Load(targetPath)
	if err != nil {
		return nil
	}
	source, err := getSafePath(cfg.OutPutPath, filepath.FromSlash(original))
	if err != nil {
		return nil
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{filepath.ToSlash(original), strconv.FormatInt(manifest.Source.ModTime, 10), strconv.FormatInt(manifest.Source.Size, 10)}, "\n")))
	key := hex.EncodeToString(sum[:])
	m := globalHLSJobManager
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	j := m.jobs[key]
	if j == nil || j.sourcePath != source {
		return nil
	}
	status, _ := j.GetStatus()
	if status == hlsJobPlayable || status == hlsJobCompleted {
		return j
	}
	return nil
}
