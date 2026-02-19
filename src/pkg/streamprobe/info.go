// Package streamprobe 提供统一的直播流探测代理功能
// 在转发直播流数据给下载器的同时，解析流头信息（分辨率、编码、帧率、码率等）
// 替代旧的 flvproxy 包，同时支持 SPS/PPS 变化分段检测
package streamprobe

import "fmt"

// StreamHeaderInfo 包含从直播流头部解析出的实际信息
type StreamHeaderInfo struct {
	// 视频信息
	VideoCodec   string  `json:"video_codec"`   // "h264", "h265", "av1", "unknown"
	Width        int     `json:"width"`         // 从 SPS 解析的实际宽度（像素）
	Height       int     `json:"height"`        // 从 SPS 解析的实际高度（像素）
	FrameRate    float64 `json:"frame_rate"`    // 帧率（来自 SPS 或 onMetaData）
	VideoBitrate int     `json:"video_bitrate"` // 视频码率 (kbps, 来自 onMetaData)

	// 音频信息
	AudioCodec   string `json:"audio_codec"`   // "aac", "mp3", "opus", "unknown"
	AudioBitrate int    `json:"audio_bitrate"` // 音频码率 (kbps)

	// 解析来源和状态
	ParsedFromSPS  bool `json:"parsed_from_sps"`  // 分辨率是否从 SPS 解析（最可靠）
	ParsedFromMeta bool `json:"parsed_from_meta"` // 分辨率是否从 onMetaData 解析

	// 不支持的编码格式信息
	Unsupported    bool   `json:"unsupported"`     // 编码格式是否不支持深度解析
	UnsupportedMsg string `json:"unsupported_msg"` // 不支持时的说明信息

	// 原始数据（供 debug 使用）
	RawMetaData map[string]interface{} `json:"raw_meta_data,omitempty"`
}

// Resolution 返回格式化的分辨率字符串，如 "1920x1080"
// 如果宽高都为 0，返回空字符串
func (info *StreamHeaderInfo) Resolution() string {
	if info.Width > 0 && info.Height > 0 {
		return fmt.Sprintf("%dx%d", info.Width, info.Height)
	}
	return ""
}

// ProbeStatus 返回探测状态字符串
// 用于前端展示："success" | "unsupported" | "pending"
func (info *StreamHeaderInfo) ProbeStatus() string {
	if info.Unsupported {
		return "unsupported"
	}
	if info.ParsedFromSPS || info.ParsedFromMeta {
		return "success"
	}
	return "pending"
}
