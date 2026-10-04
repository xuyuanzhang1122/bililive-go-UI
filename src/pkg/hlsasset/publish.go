package hlsasset

import (
	"fmt"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"os"
	"path/filepath"
)

// Publish 最后提交 .hls.json，只有该标记和目录证明均有效时 HTTP 才能读取。
// 不覆盖任何现存文件；中断意图携带完整集合，可在源版本相同的下次执行中恢复。
func Publish(root string, m Manifest, afterIndex func() error) error {
	if err := m.valid(); err != nil {
		return err
	}
	if err := safeDataDir(root, m); err != nil {
		return err
	}
	stored, err := readManifest(filepath.Join(root, m.DataDir, ".manifest.json"))
	if err != nil {
		return err
	}
	if stored.Digest() != m.Digest() {
		return fmt.Errorf("发布 HLS 关联变化")
	}
	for _, file := range m.Files {
		actual, err := fileHash(filepath.Join(root, m.DataDir, file.Name))
		if err != nil {
			return err
		}
		if actual != file {
			return fmt.Errorf("发布前 HLS 文件变化")
		}
	}
	intent := filepath.Join(root, ".hls-intent.json")
	if err := linkSame(filepath.Join(root, m.DataDir, ".manifest.json"), intent); err != nil {
		return err
	}
	if err := playbackasset.SyncDirectory(intent); err != nil {
		return err
	}
	if err := linkSame(filepath.Join(root, m.DataDir, "index.m3u8"), filepath.Join(root, "index.m3u8")); err != nil {
		return err
	}
	if afterIndex != nil {
		if err := afterIndex(); err != nil {
			return err
		}
	}
	if err := m.ValidateAll(root); err != nil {
		return err
	}
	if err := linkSame(intent, filepath.Join(root, ".hls.json")); err != nil {
		return err
	}
	if err := playbackasset.SyncDirectory(intent); err != nil {
		return err
	}
	if err := os.Remove(intent); err != nil {
		return err
	}
	return playbackasset.SyncDirectory(intent)
}
func linkSame(source, target string) error {
	if err := os.Link(source, target); err != nil {
		if !os.IsExist(err) {
			return err
		}
		a, ae := os.Lstat(source)
		b, be := os.Lstat(target)
		if ae != nil || be != nil || !a.Mode().IsRegular() || !b.Mode().IsRegular() || !os.SameFile(a, b) {
			return fmt.Errorf("已有 HLS 目标归属不符，拒绝覆盖")
		}
	}
	return nil
}
func Recover(root string, expected Manifest) (Manifest, bool, error) {
	m, err := Load(root)
	committed := err == nil
	if err != nil {
		if !os.IsNotExist(err) {
			return m, false, err
		}
		m, err = readManifest(filepath.Join(root, ".hls-intent.json"))
		if os.IsNotExist(err) {
			return m, false, nil
		}
		if err != nil {
			return m, false, err
		}
	}
	// 旧路径只作发布时证据；调用者必须先通过目录核验当前 ID/来源版本。
	if m.RecordingID != expected.RecordingID || m.SourceVersion != expected.SourceVersion || m.Profile != expected.Profile {
		return m, false, fmt.Errorf("HLS 恢复来源或规则不匹配")
	}
	if !committed {
		if err := Publish(root, m, nil); err != nil {
			return m, false, err
		}
	}
	if err := m.ValidateAll(root); err != nil {
		return m, false, err
	}
	return m, true, nil
}
