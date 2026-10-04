// Package hlsasset 发布带完整文件校验的不可变 VOD 资产集合。
package hlsasset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
)

var segmentName = regexp.MustCompile(`^seg_[0-9]{5,10}\.ts$`)

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Version       int     `json:"version"`
	RecordingID   string  `json:"recording_id"`
	SourceVersion string  `json:"source_version"`
	SourcePath    string  `json:"source_path"`
	Profile       string  `json:"profile"`
	DataDir       string  `json:"data_dir"`
	Duration      float64 `json:"duration"`
	Files         []File  `json:"files"`
}

func ProfileVersion(profile string) string { return "hls-" + profile + "-v1" }
func (m Manifest) Digest() string {
	data, _ := json.Marshal(m)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (m Manifest) valid() error {
	if m.Version != 1 || m.RecordingID == "" || m.SourceVersion == "" || !filepath.IsAbs(m.SourcePath) || (m.Profile != "source" && m.Profile != "720p" && m.Profile != "1080p") || !strings.HasPrefix(m.DataDir, ".hls-build-") || filepath.Base(m.DataDir) != m.DataDir || strings.ContainsAny(m.DataDir, "/\\") || m.Duration <= 0 || math.IsNaN(m.Duration) || math.IsInf(m.Duration, 0) {
		return fmt.Errorf("非法持久 HLS 关联")
	}
	if len(m.Files) < 2 || len(m.Files) > 30000 || m.Files[0].Name != "index.m3u8" {
		return fmt.Errorf("非法持久 HLS 文件集合")
	}
	seen := make(map[string]bool)
	for _, file := range m.Files {
		if (file.Name != "index.m3u8" && !segmentName.MatchString(file.Name)) || seen[file.Name] || file.Size <= 0 || len(file.SHA256) != 64 {
			return fmt.Errorf("非法持久 HLS 文件描述")
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return err
		}
		seen[file.Name] = true
	}
	return nil
}

func regularFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("HLS 资产不是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		f.Close()
		return nil, fmt.Errorf("HLS 资产打开期间变化")
	}
	return f, nil
}
func fileHash(path string) (File, error) {
	f, err := regularFile(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return File{}, err
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return File{}, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return File{}, err
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || size != info.Size() {
		return File{}, fmt.Errorf("HLS 资产核验期间变化")
	}
	return File{Name: filepath.Base(path), Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
func safeDataDir(root string, m Manifest) error {
	for _, path := range []string{root, filepath.Join(root, m.DataDir)} {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("HLS 资产目录不是可信目录")
		}
	}
	return nil
}

// Seal 只接受完整 VOD 清单，逐个核验引用文件；不会把 EVENT 的已有分片标作完整资产。
func Seal(dataDir string, m Manifest) (Manifest, error) {
	content, err := os.ReadFile(filepath.Join(dataDir, "index.m3u8"))
	if err != nil {
		return m, err
	}
	names, duration, err := playlistFiles(content)
	if err != nil {
		return m, err
	}
	if math.Abs(duration-m.Duration) > math.Max(1, m.Duration*0.01) {
		return m, fmt.Errorf("HLS 清单与源时长不符")
	}
	m.DataDir = filepath.Base(dataDir)
	for _, name := range append([]string{"index.m3u8"}, names...) {
		file, err := fileHash(filepath.Join(dataDir, name))
		if err != nil {
			return m, err
		}
		m.Files = append(m.Files, file)
	}
	if err := m.valid(); err != nil {
		return m, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, ".manifest.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return m, err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return m, writeErr
	}
	if syncErr != nil {
		return m, syncErr
	}
	if closeErr != nil {
		return m, closeErr
	}
	for _, file := range m.Files {
		f, err := os.OpenFile(filepath.Join(dataDir, file.Name), os.O_RDWR, 0)
		if err != nil {
			return m, err
		}
		err = f.Sync()
		closeErr = f.Close()
		if err != nil {
			return m, err
		}
		if closeErr != nil {
			return m, closeErr
		}
	}
	return m, playbackasset.SyncDirectory(filepath.Join(dataDir, ".manifest.json"))
}

func playlistFiles(content []byte) ([]string, float64, error) {
	text := strings.TrimSpace(string(content))
	if !strings.HasPrefix(text, "#EXTM3U\n") || !strings.Contains(text, "#EXT-X-PLAYLIST-TYPE:VOD\n") || !strings.HasSuffix(text, "#EXT-X-ENDLIST") {
		return nil, 0, fmt.Errorf("持久清单必须完整且为 VOD")
	}
	var names []string
	duration, pending := 0.0, 0.0
	seen := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXTINF:") {
			if pending != 0 {
				return nil, 0, fmt.Errorf("缺少 HLS 分片")
			}
			value := strings.SplitN(strings.TrimPrefix(line, "#EXTINF:"), ",", 2)[0]
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, 0, fmt.Errorf("非法分片时长")
			}
			pending = n
		} else if line != "" && !strings.HasPrefix(line, "#") {
			if pending <= 0 || !segmentName.MatchString(line) || seen[line] {
				return nil, 0, fmt.Errorf("非法 HLS 分片引用")
			}
			names = append(names, line)
			seen[line] = true
			duration += pending
			pending = 0
		} else if strings.HasPrefix(line, "#EXT-X-KEY") || strings.HasPrefix(line, "#EXT-X-MAP") || strings.HasPrefix(line, "#EXT-X-BYTERANGE") || strings.Contains(line, "PLAYLIST-TYPE:EVENT") {
			return nil, 0, fmt.Errorf("不支持的持久 HLS 清单")
		}
	}
	if len(names) == 0 || pending != 0 {
		return nil, 0, fmt.Errorf("清单没有完整分片")
	}
	return names, duration, nil
}

func readManifest(path string) (Manifest, error) {
	var m Manifest
	f, err := regularFile(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4<<20))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, m.valid()
}
func Load(root string) (Manifest, error) {
	m, err := readManifest(filepath.Join(root, ".hls.json"))
	if err != nil {
		return m, err
	}
	return m, safeDataDir(root, m)
}
func (m Manifest) FilePath(root, name string) string {
	if name == "index.m3u8" {
		return filepath.Join(root, name)
	}
	return filepath.Join(root, m.DataDir, name)
}

// OpenFile 核验后返回同一个句柄，HTTP 不会在校验与传输之间重新打开被替换的路径。
func (m Manifest) OpenFile(root, name string) (*os.File, error) {
	if err := m.valid(); err != nil {
		return nil, err
	}
	if err := safeDataDir(root, m); err != nil {
		return nil, err
	}
	var expected *File
	for i := range m.Files {
		if m.Files[i].Name == name {
			expected = &m.Files[i]
			break
		}
	}
	if expected == nil {
		return nil, fmt.Errorf("文件不属于持久 HLS 资产")
	}
	f, err := regularFile(m.FilePath(root, name))
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil || size != expected.Size || hex.EncodeToString(h.Sum(nil)) != expected.SHA256 {
		f.Close()
		return nil, fmt.Errorf("持久 HLS 资产内容已变化")
	}
	if _, err = f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func (m Manifest) ValidateAll(root string) error {
	for _, file := range m.Files {
		f, err := m.OpenFile(root, file.Name)
		if err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	}
	return nil
}
