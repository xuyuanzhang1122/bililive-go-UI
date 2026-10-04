// Package playbackdecision 对真实媒体参数与客户端明确上报的有限能力做保守匹配。
package playbackdecision

import (
	"fmt"
	"math"
	"strings"
)

type Capabilities struct {
	Containers []string          `json:"containers"`
	Protocols  []string          `json:"protocols"`
	Video      []VideoCapability `json:"video"`
	Audio      []AudioCapability `json:"audio"`
}
type VideoCapability struct {
	Codec       string   `json:"codec"`
	Profiles    []string `json:"profiles"`
	MaxLevel    int      `json:"max_level"`
	MaxWidth    int      `json:"max_width"`
	MaxHeight   int      `json:"max_height"`
	MaxFPS      float64  `json:"max_fps"`
	MaxBitDepth int      `json:"max_bit_depth"`
	HDR         bool     `json:"hdr"`
}
type AudioCapability struct {
	Codec         string   `json:"codec"`
	Profiles      []string `json:"profiles"`
	MaxChannels   int      `json:"max_channels"`
	MaxSampleRate int      `json:"max_sample_rate"`
}
type Video struct {
	Codec    string  `json:"codec"`
	Profile  string  `json:"profile"`
	Level    int     `json:"level"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      float64 `json:"fps"`
	BitDepth int     `json:"bit_depth"`
	HDR      bool    `json:"hdr"`
}
type Audio struct {
	Codec      string `json:"codec"`
	Profile    string `json:"profile"`
	Channels   int    `json:"channels"`
	SampleRate int    `json:"sample_rate"`
}
type Media struct {
	Container string `json:"container"`
	Protocol  string `json:"protocol"`
	Video     Video  `json:"video"`
	Audio     *Audio `json:"audio"`
}
type Decision struct {
	Mode       string `json:"mode"`
	Compatible bool   `json:"compatible"`
}

func normalized(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "constrained baseline" {
		return "baseline"
	}
	return s
}
func contains(list []string, s string) bool {
	s = normalized(s)
	if s == "" || s == "unknown" {
		return false
	}
	for _, v := range list {
		if normalized(v) == s {
			return true
		}
	}
	return false
}
func validNames(list []string) bool {
	if len(list) == 0 || len(list) > 16 {
		return false
	}
	for _, v := range list {
		if v == "" || len(v) > 64 || normalized(v) == "unknown" {
			return false
		}
	}
	return true
}
func Validate(c Capabilities) error {
	if !validNames(c.Containers) || !validNames(c.Protocols) || len(c.Video) == 0 || len(c.Video) > 16 || len(c.Audio) > 16 {
		return fmt.Errorf("能力列表为空或超过上限")
	}
	for _, p := range c.Protocols {
		if p != "file" && p != "hls" {
			return fmt.Errorf("未知播放协议")
		}
	}
	for _, p := range c.Containers {
		if !contains([]string{"mp4", "mov", "flv", "mkv", "ts", "webm", "hls"}, p) {
			return fmt.Errorf("未知媒体容器")
		}
	}
	for _, v := range c.Video {
		if !contains([]string{"h264", "hevc", "av1", "vp9", "mpeg4", "mpeg2video"}, v.Codec) || !validNames(v.Profiles) || v.MaxLevel <= 0 || v.MaxLevel > 1000 || v.MaxWidth <= 0 || v.MaxWidth > 16384 || v.MaxHeight <= 0 || v.MaxHeight > 16384 || v.MaxFPS <= 0 || v.MaxFPS > 240 || math.IsNaN(v.MaxFPS) || math.IsInf(v.MaxFPS, 0) || v.MaxBitDepth < 8 || v.MaxBitDepth > 16 {
			return fmt.Errorf("视频能力缺少明确支持范围")
		}
	}
	for _, a := range c.Audio {
		if !contains([]string{"aac", "opus", "mp3", "ac3", "eac3", "flac", "pcm_s16le", "vorbis"}, a.Codec) || !validNames(a.Profiles) || a.MaxChannels <= 0 || a.MaxChannels > 32 || a.MaxSampleRate <= 0 || a.MaxSampleRate > 384000 {
			return fmt.Errorf("音频能力缺少明确支持范围")
		}
	}
	return nil
}
func videoSupported(v Video, c Capabilities) bool {
	if v.Level <= 0 || v.Width <= 0 || v.Height <= 0 || v.FPS <= 0 || math.IsNaN(v.FPS) || math.IsInf(v.FPS, 0) || v.BitDepth < 8 {
		return false
	}
	for _, a := range c.Video {
		if normalized(a.Codec) == normalized(v.Codec) && contains(a.Profiles, v.Profile) && v.Level <= a.MaxLevel && v.Width <= a.MaxWidth && v.Height <= a.MaxHeight && v.FPS <= a.MaxFPS && v.BitDepth <= a.MaxBitDepth && (!v.HDR || a.HDR) {
			return true
		}
	}
	return false
}
func audioSupported(v *Audio, c Capabilities) bool {
	if v == nil {
		return true
	}
	if v.Channels <= 0 || v.SampleRate <= 0 {
		return false
	}
	for _, a := range c.Audio {
		if normalized(a.Codec) == normalized(v.Codec) && contains(a.Profiles, v.Profile) && v.Channels <= a.MaxChannels && v.SampleRate <= a.MaxSampleRate {
			return true
		}
	}
	return false
}

// Decide 的转码模式表示该客户端所需工作；只有 Compatible 才能发布 ready URL。
func Decide(m Media, c Capabilities) Decision {
	mode := "direct"
	if Validate(c) != nil || !videoSupported(m.Video, c) {
		mode = "video-transcode"
	} else if !audioSupported(m.Audio, c) {
		mode = "audio-transcode"
	} else if !contains(c.Containers, m.Container) || !contains(c.Protocols, m.Protocol) {
		mode = "remux"
	}
	return Decision{Mode: mode, Compatible: mode == "direct"}
}
