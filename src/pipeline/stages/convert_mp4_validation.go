package stages

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type mp4LimitedBuffer struct{ bytes.Buffer }

func (b *mp4LimitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remain := 8192 - b.Len(); remain > 0 {
		if len(p) > remain {
			p = p[:remain]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

type mp4Media struct {
	duration float64
	audio    bool
}

// ffprobe 扫描包确保轨道可读；设置独立预算且遵守任务取消。
func inspectMP4Media(ctx context.Context, ffmpeg, path string) (mp4Media, error) {
	var result mp4Media
	probe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe"+filepath.Ext(ffmpeg))
	if _, err := os.Stat(probe); err != nil {
		var e error
		probe, e = exec.LookPath("ffprobe")
		if e != nil {
			return result, e
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe, "-v", "error", "-count_packets", "-show_entries", "stream=codec_type,codec_name,nb_read_packets:format=duration", "-of", "json", path)
	var stdout bytes.Buffer
	stderr := &mp4LimitedBuffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return result, fmt.Errorf("ffprobe: %w: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		return result, fmt.Errorf("媒体读取错误: %s", stderr.String())
	}
	var data struct {
		Streams []struct {
			Type    string `json:"codec_type"`
			Codec   string `json:"codec_name"`
			Packets string `json:"nb_read_packets"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return result, err
	}
	result.duration, _ = strconv.ParseFloat(data.Format.Duration, 64)
	if result.duration <= 0 || math.IsNaN(result.duration) || math.IsInf(result.duration, 0) {
		return result, fmt.Errorf("媒体时长无效")
	}
	video := false
	for _, stream := range data.Streams {
		if (stream.Type == "video" && !video) || (stream.Type == "audio" && !result.audio) {
			packets, _ := strconv.ParseInt(stream.Packets, 10, 64)
			if packets <= 0 {
				return result, fmt.Errorf("媒体轨道没有可读包")
			}
			if stream.Type == "video" {
				if stream.Codec != "h264" {
					return result, fmt.Errorf("需要兼容版转换的视频编码: %s", stream.Codec)
				}
				video = true
			} else {
				if stream.Codec != "aac" {
					return result, fmt.Errorf("需要兼容版转换的音频编码: %s", stream.Codec)
				}
				result.audio = true
			}
		}
	}
	if !video {
		return result, fmt.Errorf("缺少视频轨道")
	}
	return result, nil
}

// 按顶层 box 边界读取，避免把 mdat 载荷中的 moov 文本当成索引。
func checkMP4Faststart(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	var moov, mdat bool
	for offset := int64(0); offset < info.Size(); {
		var header [16]byte
		if _, err := f.ReadAt(header[:8], offset); err != nil {
			return err
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := int64(8)
		switch size {
		case 1:
			if _, err := f.ReadAt(header[8:], offset+8); err != nil {
				return err
			}
			n := binary.BigEndian.Uint64(header[8:])
			if n > math.MaxInt64 {
				return fmt.Errorf("MP4 box 长度溢出")
			}
			size = int64(n)
			headerSize = 16
		case 0:
			size = info.Size() - offset
		}
		if size < headerSize || size > info.Size()-offset {
			return io.ErrUnexpectedEOF
		}
		switch string(header[4:8]) {
		case "moov":
			if mdat {
				return fmt.Errorf("MP4 索引不在媒体前")
			}
			moov = true
		case "mdat":
			if !moov {
				return fmt.Errorf("MP4 缺少前置索引")
			}
			if size > headerSize {
				mdat = true
			}
		}
		offset += size
	}
	if !moov || !mdat {
		return fmt.Errorf("MP4 缺少索引或媒体数据")
	}
	return nil
}
