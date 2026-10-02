// Package playbackasset 保存已验证录后产物的来源关系，不按同名猜测录播身份。
package playbackasset

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Suffix = ".playback.json"

type Stamp struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mtime_ns"`
}
type Manifest struct {
	Version    int    `json:"version"`
	SourceName string `json:"source_name"`
	TargetName string `json:"target_name"`
	Source     Stamp  `json:"source"`
	Target     Stamp  `json:"target"`
}

func stamp(info os.FileInfo) Stamp { return Stamp{info.Size(), info.ModTime().UnixNano()} }
func safeName(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s && !strings.ContainsAny(s, "/\\")
}
func read(target string) (Manifest, error) {
	var m Manifest
	f, err := os.Open(target + Suffix)
	if err != nil {
		return m, err
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, 4096)).Decode(&m)
	if err != nil {
		return m, err
	}
	if m.Version != 1 || !safeName(m.SourceName) || !safeName(m.TargetName) || m.TargetName != filepath.Base(target) || m.SourceName == m.TargetName {
		return m, fmt.Errorf("无效播放资产关联")
	}
	return m, nil
}
func Load(target string) (Manifest, error) {
	m, err := read(target)
	if err != nil {
		return m, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return m, err
	}
	if !info.Mode().IsRegular() || stamp(info) != m.Target {
		return m, fmt.Errorf("播放产物已变化")
	}
	source := filepath.Join(filepath.Dir(target), m.SourceName)
	info, err = os.Lstat(source)
	if err != nil && !os.IsNotExist(err) {
		return m, err
	}
	if err == nil && (!info.Mode().IsRegular() || stamp(info) != m.Source) {
		return m, fmt.Errorf("录播源已变化")
	}
	return m, nil
}

// Save 必须在产物验证成功后调用，sourceInfo 为转换开始时的源版本。
func Save(source, target string, sourceInfo os.FileInfo) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if filepath.Dir(source) != filepath.Dir(target) || source == target || sourceInfo == nil {
		return fmt.Errorf("播放资产必须与来源同目录且路径不同")
	}
	current, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || stamp(current) != stamp(sourceInfo) {
		return fmt.Errorf("转换期间录播源发生变化")
	}
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("播放产物不是有效文件")
	}
	m := Manifest{1, filepath.Base(source), filepath.Base(target), stamp(sourceInfo), stamp(info)}
	if old, err := read(target); err == nil {
		if old == m {
			return nil
		}
		return fmt.Errorf("已有不同来源的播放关联")
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".playback-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// 同目录硬链接原子发布，拒绝覆盖其他任务已经写出的关联。
	if err = os.Link(f.Name(), target+Suffix); err != nil {
		old, e := read(target)
		if e == nil && old == m {
			return nil
		}
		return err
	}
	return nil
}
