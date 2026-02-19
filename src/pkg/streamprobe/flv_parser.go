package streamprobe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
)

// FLV 常量
const (
	// FLV tag types
	flvTagAudio  uint8 = 8
	flvTagVideo  uint8 = 9
	flvTagScript uint8 = 18

	// FLV header size
	flvHeaderSize = 9

	// Video CodecID (lower 4 bits of first video tag byte)
	codecAVC  uint8 = 7  // H.264
	codecHEVC uint8 = 12 // H.265 (非标准 FLV 扩展)

	// AVC packet type
	avcSeqHeader uint8 = 0
	avcNALU      uint8 = 1

	// Audio CodecID (upper 4 bits of first audio tag byte)
	audioCodecAAC  uint8 = 10
	audioCodecMP3  uint8 = 2
	audioCodecOPUS uint8 = 13 // 非标准扩展

	// FLV Enhanced Header
	// 参考: https://github.com/nickharvey/m3u/wiki/Enhanced-FLV
	frameTypeCommandFrame uint8 = 5 // Enhanced header 命令帧标志

	// Enhanced FLV PacketType
	enhancedPacketTypeSequenceStart uint8 = 0

	// Enhanced FLV FourCC
	fourCCAV1  = "av01"
	fourCCVP9  = "vp09"
	fourCCHEVC = "hvc1"
)

var (
	// ErrNotFLV 表示数据不是有效的 FLV 格式
	ErrNotFLV = errors.New("不是有效的 FLV 格式")
	// ErrTruncated 表示 FLV 数据被截断
	ErrTruncated = errors.New("FLV 数据被截断")
)

// parseFLVHeader 解析 FLV 文件头（9 字节）
// 返回头部的原始字节数据
func parseFLVHeader(r io.Reader) ([]byte, error) {
	header := make([]byte, flvHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("读取 FLV 头失败: %w", err)
	}

	// 验证 FLV 签名 "FLV"
	if header[0] != 'F' || header[1] != 'L' || header[2] != 'V' {
		return nil, ErrNotFLV
	}

	return header, nil
}

// flvTagHeader 表示一个 FLV tag 的头部信息
type flvTagHeader struct {
	PreviousTagSize uint32
	TagType         uint8
	DataSize        uint32
	Timestamp       uint32
}

// readFlvTagHeader 从 reader 中读取一个 FLV tag header（ PreviousTagSize(4) + TagType(1) + DataSize(3) + Timestamp(3) + TimestampExtended(1) + StreamId(3) = 15 bytes）
func readFlvTagHeader(r io.Reader) (*flvTagHeader, []byte, error) {
	buf := make([]byte, 15)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, nil, err
	}

	tag := &flvTagHeader{
		PreviousTagSize: binary.BigEndian.Uint32(buf[0:4]),
		TagType:         buf[4],
		DataSize:        uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7]),
		Timestamp:       uint32(buf[8])<<16 | uint32(buf[9])<<8 | uint32(buf[10]) | uint32(buf[11])<<24,
	}

	return tag, buf, nil
}

// parseFLVStreamInfo 从 FLV 流的前几个 tag 中解析流信息
// 最多读取 maxTags 个 tag，避免在遇到问题时无限读取
func parseFLVStreamInfo(r io.Reader, maxTags int) (*StreamHeaderInfo, []byte, error) {
	info := &StreamHeaderInfo{}

	// 解析 FLV 头
	headerData, err := parseFLVHeader(r)
	if err != nil {
		return nil, nil, err
	}

	// 收集所有已读取的数据（用于后续转发）
	buffered := make([]byte, 0, 64*1024) // 预分配 64KB
	buffered = append(buffered, headerData...)

	// 读取 tag，直到获取到足够的信息或达到上限
	gotVideo := false
	gotAudio := false
	gotScript := false

	for i := 0; i < maxTags; i++ {
		// 读取 tag header
		tagHeader, tagHeaderData, err := readFlvTagHeader(r)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, nil, fmt.Errorf("读取 FLV tag header 失败: %w", err)
		}
		buffered = append(buffered, tagHeaderData...)

		// 读取 tag body
		tagData := make([]byte, tagHeader.DataSize)
		if _, err := io.ReadFull(r, tagData); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, nil, fmt.Errorf("读取 FLV tag body 失败: %w", err)
		}
		buffered = append(buffered, tagData...)

		// 根据 tag 类型解析
		switch tagHeader.TagType {
		case flvTagScript:
			if !gotScript {
				parseScriptTag(tagData, info)
				gotScript = true
			}
		case flvTagVideo:
			if !gotVideo {
				parseVideoTag(tagData, info)
				if info.ParsedFromSPS || info.Unsupported {
					gotVideo = true
				}
			}
		case flvTagAudio:
			if !gotAudio {
				parseAudioTag(tagData, info)
				gotAudio = true
			}
		}

		// 已获取所有需要的信息
		if (gotVideo || info.Unsupported) && gotAudio && gotScript {
			break
		}
	}

	return info, buffered, nil
}

// parseVideoTag 解析视频 tag，提取编码格式和 SPS 信息
func parseVideoTag(data []byte, info *StreamHeaderInfo) {
	if len(data) < 2 {
		return
	}

	frameType := (data[0] >> 4) & 0x0F
	codecID := data[0] & 0x0F

	// 检查是否为 Enhanced FLV header
	// Enhanced FLV: FrameType 的高 4 位第 8 位为标志位
	// 如果 frameType 的最高位(bit 7)被设置，说明是 Enhanced FLV
	isEnhanced := (data[0] & 0x80) != 0

	if isEnhanced {
		parseEnhancedVideoTag(data, info)
		return
	}

	// 标准 FLV 视频 tag
	switch codecID {
	case codecAVC:
		info.VideoCodec = "h264"
		if len(data) >= 5 {
			// AVCPacketType
			avcPacketType := data[1]
			if avcPacketType == avcSeqHeader {
				// AVC Sequence Header - 包含 SPS/PPS
				parseAVCDecoderConfig(data[5:], info)
			}
		}
	case codecHEVC:
		info.VideoCodec = "h265"
		if len(data) >= 5 {
			avcPacketType := data[1]
			if avcPacketType == avcSeqHeader {
				// HEVC Decoder Configuration Record
				parseHEVCDecoderConfig(data[5:], info)
			}
		}
	default:
		// 未知 or 命令帧
		if frameType != frameTypeCommandFrame {
			info.VideoCodec = "unknown"
			info.Unsupported = true
			info.UnsupportedMsg = fmt.Sprintf("未知的视频编码格式 (CodecID: %d)", codecID)
		}
	}
}

// parseEnhancedVideoTag 解析 Enhanced FLV 视频 tag
// Enhanced FLV 使用 FourCC 标识编码格式
func parseEnhancedVideoTag(data []byte, info *StreamHeaderInfo) {
	if len(data) < 5 {
		return
	}

	// Enhanced header 格式:
	// byte 0: FrameType(4) + PacketType(4)
	// byte 1-4: FourCC (4 bytes)
	packetType := data[0] & 0x0F
	fourCC := string(data[1:5])

	switch fourCC {
	case fourCCHEVC:
		info.VideoCodec = "h265"
		if packetType == enhancedPacketTypeSequenceStart && len(data) > 5 {
			parseHEVCDecoderConfig(data[5:], info)
		}
	case fourCCAV1:
		info.VideoCodec = "av1"
		info.Unsupported = true
		info.UnsupportedMsg = "AV1 编码格式暂不支持 SPS 深度解析，无法获取精确分辨率"
	case fourCCVP9:
		info.VideoCodec = "vp9"
		info.Unsupported = true
		info.UnsupportedMsg = "VP9 编码格式暂不支持深度解析，无法获取精确分辨率"
	default:
		info.VideoCodec = "unknown"
		info.Unsupported = true
		info.UnsupportedMsg = fmt.Sprintf("未知的 Enhanced FLV 编码格式 (FourCC: %s)", fourCC)
	}
}

// parseAVCDecoderConfig 解析 AVC Decoder Configuration Record
// 从中提取 SPS 并解析分辨率
func parseAVCDecoderConfig(data []byte, info *StreamHeaderInfo) {
	if len(data) < 8 {
		return
	}

	// AVCDecoderConfigurationRecord 格式:
	// configurationVersion(1) + AVCProfileIndication(1) + ...
	// numOfSequenceParameterSets(1) & 0x1F = SPS 数量
	// sequenceParameterSetLength(2) + spsNALUnit(N)

	// 跳过前 5 个字节到 numOfSequenceParameterSets
	numSPS := int(data[5] & 0x1F)
	if numSPS == 0 {
		return
	}

	offset := 6
	if offset+2 > len(data) {
		return
	}

	spsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if offset+spsLen > len(data) || spsLen == 0 {
		return
	}

	spsData := data[offset : offset+spsLen]

	// 使用 mediacommon 解析 SPS
	var sps h264.SPS
	if err := sps.Unmarshal(spsData); err != nil {
		// SPS 解析失败，不影响录制
		return
	}

	info.Width = sps.Width()
	info.Height = sps.Height()
	info.ParsedFromSPS = true

	// 尝试获取帧率
	if fps := sps.FPS(); fps > 0 && fps < 300 {
		info.FrameRate = fps
	}
}

// parseHEVCDecoderConfig 解析 HEVC Decoder Configuration Record
// 从中提取 SPS 并解析分辨率
func parseHEVCDecoderConfig(data []byte, info *StreamHeaderInfo) {
	if len(data) < 23 {
		return
	}

	// HEVCDecoderConfigurationRecord 格式:
	// 参考 ISO 14496-15 Section 8.3.3.1.2
	// 前 22 字节是固定配置
	// byte 22: numOfArrays

	numArrays := int(data[22])
	offset := 23

	for i := 0; i < numArrays; i++ {
		if offset+3 > len(data) {
			break
		}

		naluType := data[offset] & 0x3F
		numNalus := int(binary.BigEndian.Uint16(data[offset+1 : offset+3]))
		offset += 3

		for j := 0; j < numNalus; j++ {
			if offset+2 > len(data) {
				return
			}
			naluLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+naluLen > len(data) {
				return
			}

			naluData := data[offset : offset+naluLen]
			offset += naluLen

			// NAL type 33 = SPS
			if naluType == 33 && naluLen > 0 {
				var sps h265.SPS
				if err := sps.Unmarshal(naluData); err != nil {
					continue
				}

				info.Width = sps.Width()
				info.Height = sps.Height()
				info.ParsedFromSPS = true

				if fps := sps.FPS(); fps > 0 && fps < 300 {
					info.FrameRate = fps
				}
				return
			}
		}
	}
}

// parseAudioTag 解析音频 tag，提取编码格式信息
func parseAudioTag(data []byte, info *StreamHeaderInfo) {
	if len(data) < 1 {
		return
	}

	soundFormat := (data[0] >> 4) & 0x0F
	switch soundFormat {
	case audioCodecAAC:
		info.AudioCodec = "aac"
	case audioCodecMP3:
		info.AudioCodec = "mp3"
	case audioCodecOPUS:
		info.AudioCodec = "opus"
	default:
		info.AudioCodec = fmt.Sprintf("audio_%d", soundFormat)
	}
}

// parseScriptTag 解析 Script (onMetaData) tag
// 从中提取 width, height, framerate, videodatarate, audiodatarate 等
func parseScriptTag(data []byte, info *StreamHeaderInfo) {
	// AMF0 格式解析 onMetaData
	// Script tag data 通常包含:
	// 1. AMF0 string "onMetaData"
	// 2. AMF0 ECMA array 或 object 包含元数据

	metadata := parseAMF0(data)
	if metadata == nil {
		return
	}

	info.RawMetaData = metadata

	// 提取视频信息
	if width, ok := getNumberFromMeta(metadata, "width"); ok && width > 0 {
		if info.Width == 0 { // 不覆盖 SPS 解析的值
			info.Width = int(width)
			info.ParsedFromMeta = true
		}
	}
	if height, ok := getNumberFromMeta(metadata, "height"); ok && height > 0 {
		if info.Height == 0 { // 不覆盖 SPS 解析的值
			info.Height = int(height)
			info.ParsedFromMeta = true
		}
	}
	if framerate, ok := getNumberFromMeta(metadata, "framerate"); ok && framerate > 0 {
		if info.FrameRate == 0 { // 不覆盖 SPS 解析的值
			info.FrameRate = framerate
		}
	}
	if videodatarate, ok := getNumberFromMeta(metadata, "videodatarate"); ok && videodatarate > 0 {
		info.VideoBitrate = int(videodatarate)
	}
	if audiodatarate, ok := getNumberFromMeta(metadata, "audiodatarate"); ok && audiodatarate > 0 {
		info.AudioBitrate = int(audiodatarate)
	}

	// 编码器信息
	if videoCodecStr, ok := getStringFromMeta(metadata, "videocodecid"); ok {
		if info.VideoCodec == "" {
			info.VideoCodec = normalizeCodecName(videoCodecStr)
		}
	}
	if audioCodecStr, ok := getStringFromMeta(metadata, "audiocodecid"); ok {
		if info.AudioCodec == "" {
			info.AudioCodec = normalizeCodecName(audioCodecStr)
		}
	}
}

// getNumberFromMeta 从 metadata 中获取数值类型的字段
func getNumberFromMeta(meta map[string]interface{}, key string) (float64, bool) {
	v, ok := meta[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// getStringFromMeta 从 metadata 中获取字符串类型的字段
func getStringFromMeta(meta map[string]interface{}, key string) (string, bool) {
	v, ok := meta[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// normalizeCodecName 标准化编码名称
func normalizeCodecName(name string) string {
	switch name {
	case "avc1", "AVC", "7":
		return "h264"
	case "hvc1", "hev1", "HEVC", "12":
		return "h265"
	case "mp4a", "AAC", "10":
		return "aac"
	default:
		return name
	}
}

// ==================== 简化的 AMF0 解析器 ====================

// parseAMF0 解析 AMF0 格式的 script tag data
// 只提取 onMetaData 中的键值对
func parseAMF0(data []byte) map[string]interface{} {
	if len(data) < 3 {
		return nil
	}

	offset := 0

	// 跳过第一个 AMF0 值（通常是 "onMetaData" 字符串）
	if data[offset] == 0x02 { // AMF0 string type
		offset++
		if offset+2 > len(data) {
			return nil
		}
		strLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2 + strLen
	} else {
		return nil
	}

	if offset >= len(data) {
		return nil
	}

	// 解析第二个 AMF0 值（ECMA array 或 object）
	return readAMF0Value(data, &offset)
}

// readAMF0Value 读取一个 AMF0 值
func readAMF0Value(data []byte, offset *int) map[string]interface{} {
	if *offset >= len(data) {
		return nil
	}

	amfType := data[*offset]
	*offset++

	switch amfType {
	case 0x03: // Object
		return readAMF0Object(data, offset)
	case 0x08: // ECMA Array
		if *offset+4 > len(data) {
			return nil
		}
		*offset += 4 // 跳过 array count
		return readAMF0Object(data, offset)
	default:
		return nil
	}
}

// readAMF0Object 读取 AMF0 object/ECMA array 的键值对
func readAMF0Object(data []byte, offset *int) map[string]interface{} {
	result := make(map[string]interface{})

	for *offset+2 < len(data) {
		// 读取 key（AMF0 string without type marker）
		keyLen := int(binary.BigEndian.Uint16(data[*offset : *offset+2]))
		*offset += 2

		if keyLen == 0 {
			// 检查 object end marker (0x000009)
			if *offset < len(data) && data[*offset] == 0x09 {
				*offset++
			}
			break
		}

		if *offset+keyLen > len(data) {
			break
		}

		key := string(data[*offset : *offset+keyLen])
		*offset += keyLen

		if *offset >= len(data) {
			break
		}

		// 读取 value
		valueType := data[*offset]
		*offset++

		switch valueType {
		case 0x00: // Number (float64)
			if *offset+8 > len(data) {
				return result
			}
			bits := binary.BigEndian.Uint64(data[*offset : *offset+8])
			result[key] = math.Float64frombits(bits)
			*offset += 8

		case 0x01: // Boolean
			if *offset >= len(data) {
				return result
			}
			result[key] = data[*offset] != 0
			*offset++

		case 0x02: // String
			if *offset+2 > len(data) {
				return result
			}
			strLen := int(binary.BigEndian.Uint16(data[*offset : *offset+2]))
			*offset += 2
			if *offset+strLen > len(data) {
				return result
			}
			result[key] = string(data[*offset : *offset+strLen])
			*offset += strLen

		case 0x05: // Null
			result[key] = nil

		case 0x06: // Undefined
			result[key] = nil

		default:
			// 遇到未知类型，停止解析避免越界
			return result
		}
	}

	return result
}
