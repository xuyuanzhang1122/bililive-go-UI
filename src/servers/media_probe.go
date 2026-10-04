package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
)

type mediaProbe struct {
	Streams []mediaProbeStream `json:"streams"`
	Format  mediaProbeFormat   `json:"format"`
}

type mediaProbeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
}

type mediaProbeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

type hlsEncodeMode string

const (
	hlsEncodeCopy      hlsEncodeMode = "copy"
	hlsEncodeTranscode hlsEncodeMode = "transcode"
)

// hlsEncodeModeForProbe chooses the conservative cross-platform HLS profile.
// Copy is safe only when the first video/audio tracks are H.264/AAC (or video-only).
func hlsEncodeModeForProbe(probe mediaProbe) hlsEncodeMode {
	videoSeen := false
	audioSeen := false
	for _, stream := range probe.Streams {
		switch strings.ToLower(stream.CodecType) {
		case "video":
			if videoSeen {
				continue
			}
			videoSeen = true
			if !strings.EqualFold(stream.CodecName, "h264") {
				return hlsEncodeTranscode
			}
		case "audio":
			if audioSeen {
				continue
			}
			audioSeen = true
			if !strings.EqualFold(stream.CodecName, "aac") {
				return hlsEncodeTranscode
			}
		}
	}
	if !videoSeen {
		return hlsEncodeTranscode
	}
	return hlsEncodeCopy
}

func probeMedia(ctx context.Context, ffprobePath, sourcePath string) (mediaProbe, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, ffprobePath,
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name:format=format_name",
		"-of", "json",
		sourcePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return mediaProbe{}, fmt.Errorf("ffprobe failed: %w", err)
	}
	var result mediaProbe
	if err := json.Unmarshal(output, &result); err != nil {
		return mediaProbe{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return result, nil
}

func findFFprobePath(ctx context.Context, cfg *configs.Config) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.FfmpegPath) != "" {
		configured := strings.TrimSpace(cfg.FfmpegPath)
		base := filepath.Base(configured)
		ext := filepath.Ext(base)
		candidate := filepath.Join(filepath.Dir(configured), "ffprobe"+ext)
		if _, err := exec.CommandContext(ctx, candidate, "-version").Output(); err == nil {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("ffprobe"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("ffprobe 未安装或未配置")
}

func hlsTranscodeArgs(sourcePath, segmentPattern, playlistPath string) []string {
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-probesize", "1000000", // 流信息探测预算，不是输入总读取上限
		"-analyzeduration", "1000000", // 分析媒体时间预算，不是墙钟超时
		"-fflags", "+genpts",
		"-i", sourcePath,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-profile:v", "main",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ar", "48000",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		"-hls_flags", "independent_segments+temp_file",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	}
}

// hlsCopyArgs 与转码路径使用相同的轨道选择，避免探测和实际输出不一致。
func hlsCopyArgs(source, segments, playlist string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-y",
		"-probesize", "1000000", "-analyzeduration", "1000000", "-i", source,
		"-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-avoid_negative_ts", "make_zero",
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "event",
		"-hls_flags", "temp_file", "-hls_segment_filename", segments, playlist}
}
