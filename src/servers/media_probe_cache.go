package servers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// 缓存键绑定来源和探测工具的版本；只缓存成功结果，缓存故障不阻断播放。
func probeCacheKey(tool, source string) (string, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	tool, err = filepath.EvalSymlinks(tool)
	if err != nil {
		return "", err
	}
	tool, err = filepath.Abs(tool)
	if err != nil {
		return "", err
	}
	si, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	ti, err := os.Stat(tool)
	if err != nil {
		return "", err
	}
	if !si.Mode().IsRegular() || !ti.Mode().IsRegular() {
		return "", fmt.Errorf("媒体或探测工具不是普通文件")
	}
	sourceVersion, err := recordingcatalog.SnapshotVersion(source)
	if err != nil {
		return "", err
	}
	toolVersion, err := recordingcatalog.SnapshotVersion(tool)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("probe-v4\x00%s\x00%s\x00%s\x00%s", source, sourceVersion, tool, toolVersion)))
	return hex.EncodeToString(key[:]), nil
}
func validProbe(p mediaProbe) bool {
	for _, s := range p.Streams {
		if s.CodecType == "video" && s.CodecName != "" {
			return true
		}
	}
	return false
}
func probeMediaCached(ctx context.Context, tool, source, root string) (mediaProbe, error) {
	key, err := probeCacheKey(tool, source)
	if err != nil {
		return mediaProbe{}, err
	}
	path := filepath.Join(root, key+".json")
	if info, e := os.Stat(path); e == nil && info.Size() <= 64*1024 && time.Since(info.ModTime()) < 7*24*time.Hour {
		if b, e := os.ReadFile(path); e == nil {
			var p mediaProbe
			if json.Unmarshal(b, &p) == nil && validProbe(p) {
				return p, nil
			}
		}
	}
	p, err := probeMedia(ctx, tool, source)
	if err != nil {
		return p, err
	}
	if !validProbe(p) {
		return mediaProbe{}, fmt.Errorf("媒体探测未返回有效视频轨道")
	}
	after, err := probeCacheKey(tool, source)
	if err != nil {
		return mediaProbe{}, err
	}
	if key != after {
		return mediaProbe{}, fmt.Errorf("媒体或探测工具在探测期间发生变化")
	}
	if os.MkdirAll(root, 0700) != nil {
		return p, nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return p, nil
	}
	f, err := os.CreateTemp(root, ".probe-*.tmp")
	if err != nil {
		return p, nil
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr == nil && closeErr == nil {
		if os.Rename(f.Name(), path) == nil {
			pruneProbeCache(root)
		}
	}
	return p, nil
}

// 写入时清理过期记录并限制数量，不依赖HLS启动清理或常驻协程。
func pruneProbeCache(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type record struct {
		path string
		when time.Time
	}
	records := make([]record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if time.Since(info.ModTime()) >= 7*24*time.Hour {
			_ = os.Remove(path)
			continue
		}
		records = append(records, record{path, info.ModTime()})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].when.Before(records[j].when) })
	for i := 0; i < len(records)-512; i++ {
		_ = os.Remove(records[i].path)
	}
}
