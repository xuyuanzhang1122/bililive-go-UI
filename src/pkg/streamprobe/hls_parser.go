package streamprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"

	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

// TS (MPEG-2 Transport Stream) 常量
const (
	tsPacketSize   = 188
	tsSyncByte     = 0x47
	tsMaxReadBytes = 64 * 1024 // 最多读取 64KB 的 TS 数据

	// PES stream type
	streamTypeH264 = 0x1B
	streamTypeH265 = 0x24
	streamTypeAAC  = 0x0F
	streamTypeAC3  = 0x81
	streamTypeMPEG = 0x03

	// H.264 NAL Unit Types
	nalTypeSPS    = 7
	nalTypeIDR    = 5
	nalTypeNonIDR = 1

	// H.265 NAL Unit Types
	hevcNalTypeSPS = 33
	hevcNalTypeVPS = 32
	hevcNalTypePPS = 34
)

// probeHLSStreamInfo 探测 HLS 流的实际信息
// 支持两种 HLS 格式：
//   - TS 格式：解析 m3u8 → 下载 TS 分段头部 → 解析 TS packets → SPS
//   - fMP4 格式（m4s）：解析 m3u8 的 #EXT-X-MAP → 下载 init 段 → 解析 moov box → SPS
func probeHLSStreamInfo(
	ctx context.Context,
	m3u8Body io.Reader,
	m3u8URL *url.URL,
	headers map[string]string,
	logger *livelogger.LiveLogger,
) (*StreamHeaderInfo, error) {
	// 1. 读取 m3u8 内容
	m3u8Content, err := io.ReadAll(m3u8Body)
	if err != nil {
		return nil, fmt.Errorf("读取 m3u8 失败: %w", err)
	}

	content := string(m3u8Content)

	// 2. 检测是 fMP4 格式还是 TS 格式
	//    fMP4 格式的 m3u8 包含 #EXT-X-MAP 标签指向 init 段
	initURL, err := parseEXTXMap(content, m3u8URL)
	if err == nil && initURL != "" {
		// fMP4 格式：下载 init 段并解析 moov box
		return probeFMP4Init(ctx, initURL, headers, logger)
	}

	// TS 格式：走原有逻辑
	segmentURL, err := parseFirstSegmentURL(content, m3u8URL)
	if err != nil {
		return nil, fmt.Errorf("解析 m3u8 分段失败: %w", err)
	}

	if logger != nil {
		logger.Infof("HLS 探测: 解析到第一个 TS 分段 URL: %s", segmentURL)
	}

	// 下载 TS 分段头部
	tsData, err := downloadSegmentHeader(ctx, segmentURL, headers, tsMaxReadBytes)
	if err != nil {
		return nil, fmt.Errorf("下载 TS 分段头部失败: %w", err)
	}

	if logger != nil {
		logger.Infof("HLS 探测: 已下载 TS 分段头部 %d 字节", len(tsData))
	}

	info, err := parseTSData(tsData)
	if err != nil {
		return nil, fmt.Errorf("解析 TS 数据失败: %w", err)
	}

	return info, nil
}

// parseEXTXMap 从 m3u8 内容中解析 #EXT-X-MAP 标签的 URI
// 返回 init 段的完整 URL。如果没有 #EXT-X-MAP 标签，返回空字符串。
func parseEXTXMap(content string, baseURL *url.URL) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXT-X-MAP:") {
			continue
		}

		// 解析 URI="..." 属性
		attrs := line[len("#EXT-X-MAP:"):]
		uri := ""
		for _, attr := range strings.Split(attrs, ",") {
			attr = strings.TrimSpace(attr)
			if strings.HasPrefix(attr, "URI=") {
				uri = strings.Trim(attr[4:], `"`)
				break
			}
		}
		if uri == "" {
			return "", errors.New("#EXT-X-MAP 缺少 URI 属性")
		}

		// 处理相对 URL
		if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
			parsed, err := baseURL.Parse(uri)
			if err != nil {
				return "", fmt.Errorf("解析 init 段 URL 失败: %w", err)
			}
			uri = parsed.String()
		}

		return uri, nil
	}

	return "", nil
}

// probeFMP4Init 下载 fMP4 init 段并解析编解码器信息
func probeFMP4Init(ctx context.Context, initURL string, headers map[string]string, logger *livelogger.LiveLogger) (*StreamHeaderInfo, error) {
	if logger != nil {
		logger.Infof("HLS 探测 (fMP4): 下载 init 段: %s", initURL)
	}

	// init 段通常很小（几百字节到几 KB），完整下载即可
	initData, err := downloadSegmentHeader(ctx, initURL, headers, 128*1024)
	if err != nil {
		return nil, fmt.Errorf("下载 fMP4 init 段失败: %w", err)
	}

	if logger != nil {
		logger.Infof("HLS 探测 (fMP4): 已下载 init 段 %d 字节", len(initData))
	}

	info, err := parseFMP4InitSegment(initData)
	if err != nil {
		return nil, fmt.Errorf("解析 fMP4 init 段失败: %w", err)
	}

	return info, nil
}

// parseFMP4InitSegment 解析 fMP4 init 段（包含 moov box）
// 路径：moov → trak → mdia → minf → stbl → stsd → avc1/hev1/hvc1/mp4a
func parseFMP4InitSegment(data []byte) (*StreamHeaderInfo, error) {
	if len(data) < 8 {
		return nil, errors.New("init 段数据太短")
	}

	info := &StreamHeaderInfo{}

	// 找到 moov box
	moovData := findBox(data, "moov")
	if moovData == nil {
		return nil, errors.New("未找到 moov box")
	}

	// 遍历所有 trak box
	offset := 0
	for offset < len(moovData) {
		boxSize, boxType, headerLen := readBoxHeader(moovData[offset:])
		if boxSize == 0 || headerLen == 0 {
			break
		}
		if int(boxSize) > len(moovData)-offset {
			break
		}

		if boxType == "trak" {
			parseTrakBox(moovData[offset+headerLen:offset+int(boxSize)], info)
		}

		offset += int(boxSize)
	}

	if info.VideoCodec == "" && info.AudioCodec == "" {
		return nil, errors.New("init 段中未找到任何编解码器信息")
	}

	return info, nil
}

// parseTrakBox 解析 trak → mdia → minf → stbl → stsd 获取编解码器信息
func parseTrakBox(data []byte, info *StreamHeaderInfo) {
	mdiaData := findBox(data, "mdia")
	if mdiaData == nil {
		return
	}

	minfData := findBox(mdiaData, "minf")
	if minfData == nil {
		return
	}

	stblData := findBox(minfData, "stbl")
	if stblData == nil {
		return
	}

	stsdData := findBox(stblData, "stsd")
	if stsdData == nil {
		return
	}

	// stsd box 内容：4 字节版本/标志 + 4 字节 entry_count + entries...
	if len(stsdData) < 8 {
		return
	}

	// 跳过版本/标志和 entry_count
	entryData := stsdData[8:]
	if len(entryData) < 8 {
		return
	}

	// 读取第一个 entry
	entrySize, entryType, _ := readBoxHeader(entryData)
	if entrySize == 0 {
		return
	}

	switch entryType {
	case "avc1", "avc3":
		info.VideoCodec = "h264"
		// avc1 box: 78 字节固定头部后面跟子 box（包括 avcC）
		if int(entrySize) <= len(entryData) {
			parseAvcCBox(entryData[86:int(entrySize)], info)
		}
	case "hev1", "hvc1":
		info.VideoCodec = "h265"
		// hev1/hvc1 box: 78 字节固定头部后面跟子 box（包括 hvcC）
		if int(entrySize) <= len(entryData) {
			parseHvcCBox(entryData[86:int(entrySize)], info)
		}
	case "mp4a":
		info.AudioCodec = "aac"
	case "Opus", "opus":
		info.AudioCodec = "opus"
	case "ac-3":
		info.AudioCodec = "ac3"
	case "ec-3":
		info.AudioCodec = "eac3"
	}
}

// parseAvcCBox 在数据中找到 avcC box 并解析 SPS
func parseAvcCBox(data []byte, info *StreamHeaderInfo) {
	avcCData := findBox(data, "avcC")
	if len(avcCData) < 8 {
		return
	}

	// avcC 格式：
	// 1 byte  configurationVersion
	// 1 byte  AVCProfileIndication
	// 1 byte  profile_compatibility
	// 1 byte  AVCLevelIndication
	// 1 byte  lengthSizeMinusOne (低 2 位)
	// 1 byte  numOfSPS (低 5 位)
	// 2 bytes spsLength
	// N bytes spsNALU
	if len(avcCData) < 7 {
		return
	}

	numSPS := int(avcCData[5] & 0x1F)
	if numSPS == 0 {
		return
	}

	offset := 6
	if offset+2 > len(avcCData) {
		return
	}
	spsLen := int(binary.BigEndian.Uint16(avcCData[offset : offset+2]))
	offset += 2

	if offset+spsLen > len(avcCData) || spsLen == 0 {
		return
	}

	spsData := avcCData[offset : offset+spsLen]
	var sps h264.SPS
	if err := sps.Unmarshal(spsData); err == nil {
		info.Width = sps.Width()
		info.Height = sps.Height()
		if fps := sps.FPS(); fps > 0 && fps < 300 {
			info.FrameRate = fps
		}
		info.ParsedFromSPS = true
	}
}

// parseHvcCBox 在数据中找到 hvcC box 并解析 SPS
func parseHvcCBox(data []byte, info *StreamHeaderInfo) {
	hvcCData := findBox(data, "hvcC")
	if len(hvcCData) < 23 {
		return
	}

	// hvcC 格式比较复杂，需要遍历 array of arrays
	// offset 22: numOfArrays
	numArrays := int(hvcCData[22])
	offset := 23

	for i := 0; i < numArrays && offset < len(hvcCData); i++ {
		if offset+3 > len(hvcCData) {
			break
		}
		nalType := hvcCData[offset] & 0x3F
		numNALUs := int(binary.BigEndian.Uint16(hvcCData[offset+1 : offset+3]))
		offset += 3

		for j := 0; j < numNALUs && offset+2 <= len(hvcCData); j++ {
			nalLen := int(binary.BigEndian.Uint16(hvcCData[offset : offset+2]))
			offset += 2

			if offset+nalLen > len(hvcCData) {
				break
			}

			// SPS NAL type = 33
			if nalType == hevcNalTypeSPS {
				var sps h265.SPS
				if err := sps.Unmarshal(hvcCData[offset : offset+nalLen]); err == nil {
					info.Width = sps.Width()
					info.Height = sps.Height()
					if fps := sps.FPS(); fps > 0 && fps < 300 {
						info.FrameRate = fps
					}
					info.ParsedFromSPS = true
				}
				return
			}

			offset += nalLen
		}
	}
}

// findBox 在 MP4 数据中查找指定类型的 box，返回 box 内容（不含 header）
func findBox(data []byte, boxType string) []byte {
	offset := 0
	for offset+8 <= len(data) {
		size, typ, headerLen := readBoxHeader(data[offset:])
		if size == 0 || headerLen == 0 {
			break
		}
		if int(size) > len(data)-offset {
			break
		}
		if typ == boxType {
			return data[offset+headerLen : offset+int(size)]
		}
		offset += int(size)
	}
	return nil
}

// readBoxHeader 读取 MP4 box 头部，返回 (size, type, headerLength)
func readBoxHeader(data []byte) (uint64, string, int) {
	if len(data) < 8 {
		return 0, "", 0
	}
	size := uint64(binary.BigEndian.Uint32(data[0:4]))
	boxType := string(data[4:8])

	if size == 1 {
		// 64 位扩展 size
		if len(data) < 16 {
			return 0, "", 0
		}
		size = binary.BigEndian.Uint64(data[8:16])
		return size, boxType, 16
	}
	if size == 0 {
		// box 延伸到文件末尾
		size = uint64(len(data))
	}
	return size, boxType, 8
}

// parseFirstSegmentURL 从 m3u8 内容中解析第一个媒体分段的 URL（TS 或 m4s）
func parseFirstSegmentURL(content string, baseURL *url.URL) (string, error) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 第一个非注释、非空行就是分段 URL
		segmentURL := line

		// 处理相对 URL
		if !strings.HasPrefix(segmentURL, "http://") && !strings.HasPrefix(segmentURL, "https://") {
			parsed, err := baseURL.Parse(segmentURL)
			if err != nil {
				return "", fmt.Errorf("解析相对 URL 失败: %w", err)
			}
			segmentURL = parsed.String()
		}

		return segmentURL, nil
	}

	return "", errors.New("m3u8 中未找到 TS 分段")
}

// downloadSegmentHeader 下载 TS 分段的头部数据
func downloadSegmentHeader(ctx context.Context, segmentURL string, headers map[string]string, maxBytes int) ([]byte, error) {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	proxy.ApplyDownloadProxyToTransport(transport)

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxTimeout, http.MethodGet, segmentURL, nil)
	if err != nil {
		return nil, err
	}

	// 使用 Range 请求只下载头部
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxBytes-1))

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 200 或 206 都可以
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 读取限定字节数
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// parseTSData 解析 TS 数据，提取视频/音频信息
func parseTSData(data []byte) (*StreamHeaderInfo, error) {
	if len(data) < tsPacketSize {
		return nil, errors.New("TS 数据太短")
	}

	info := &StreamHeaderInfo{}

	// 1. 找到 sync byte 对齐
	offset := findTSSyncOffset(data)
	if offset < 0 {
		return nil, errors.New("未找到 TS sync byte")
	}

	// 2. 解析 PAT 找到 PMT PID
	pmtPID := -1

	// 3. 解析 PMT 找到视频/音频 PID 和类型
	videoPID := -1
	videoType := 0
	audioPID := -1
	audioType := 0

	// 4. 收集视频 PES 中的 NAL units
	var videoPESData []byte

	// 第一轮：找 PAT 和 PMT
	for pos := offset; pos+tsPacketSize <= len(data); pos += tsPacketSize {
		pkt := data[pos : pos+tsPacketSize]
		if pkt[0] != tsSyncByte {
			break
		}

		pid := (int(pkt[1]&0x1F) << 8) | int(pkt[2])
		payloadStart := pkt[1]&0x40 != 0
		hasPayload := pkt[3]&0x10 != 0
		hasAdaptation := pkt[3]&0x20 != 0

		if !hasPayload {
			continue
		}

		// 计算 payload 起始偏移
		payloadOffset := 4
		if hasAdaptation {
			adaptLen := int(pkt[4])
			payloadOffset = 5 + adaptLen
			if payloadOffset >= tsPacketSize {
				continue
			}
		}

		payload := pkt[payloadOffset:]

		// PAT (PID 0)
		if pid == 0 && payloadStart && pmtPID < 0 {
			if len(payload) > 0 {
				// 跳过 pointer field
				pointerField := int(payload[0])
				tableStart := 1 + pointerField
				if tableStart+8 < len(payload) {
					table := payload[tableStart:]
					// PAT table_id should be 0x00
					if table[0] == 0x00 {
						sectionLength := (int(table[1]&0x0F) << 8) | int(table[2])
						if sectionLength > 5 {
							// 跳到 program entries (offset 8)
							progOffset := 8
							if tableStart+progOffset+4 <= len(payload) {
								entries := table[progOffset:]
								if len(entries) >= 4 {
									pmtPID = (int(entries[2]&0x1F) << 8) | int(entries[3])
								}
							}
						}
					}
				}
			}
		}

		// PMT
		if pid == pmtPID && payloadStart && videoPID < 0 {
			if len(payload) > 0 {
				pointerField := int(payload[0])
				tableStart := 1 + pointerField
				if tableStart+12 < len(payload) {
					table := payload[tableStart:]
					if table[0] == 0x02 { // PMT table_id
						sectionLength := (int(table[1]&0x0F) << 8) | int(table[2])
						progInfoLen := (int(table[10]&0x0F) << 8) | int(table[11])
						streamOffset := 12 + progInfoLen

						// 最大长度不超过 section
						maxOffset := 3 + sectionLength - 4 // 减去 CRC
						if maxOffset > len(table) {
							maxOffset = len(table)
						}

						for streamOffset+5 <= maxOffset {
							sType := int(table[streamOffset])
							sPID := (int(table[streamOffset+1]&0x1F) << 8) | int(table[streamOffset+2])
							esInfoLen := (int(table[streamOffset+3]&0x0F) << 8) | int(table[streamOffset+4])

							switch sType {
							case streamTypeH264:
								videoPID = sPID
								videoType = streamTypeH264
								info.VideoCodec = "H.264"
							case streamTypeH265:
								videoPID = sPID
								videoType = streamTypeH265
								info.VideoCodec = "H.265"
							case streamTypeAAC:
								audioPID = sPID
								audioType = streamTypeAAC
								info.AudioCodec = "AAC"
							case streamTypeAC3:
								audioPID = sPID
								audioType = streamTypeAC3
								info.AudioCodec = "AC-3"
							case streamTypeMPEG:
								if audioPID < 0 {
									audioPID = sPID
									audioType = streamTypeMPEG
									info.AudioCodec = "MP3"
								}
							}

							streamOffset += 5 + esInfoLen
						}
					}
				}
			}
		}
	}

	// 如果没找到视频 PID，尝试标记不支持
	if videoPID < 0 {
		if pmtPID < 0 {
			return nil, errors.New("未找到 PAT/PMT，TS 数据可能不完整")
		}
		info.Unsupported = true
		info.UnsupportedMsg = "TS 中未发现支持的视频编码"
		return info, nil
	}

	// 第二轮：提取视频 PES 数据
	for pos := offset; pos+tsPacketSize <= len(data); pos += tsPacketSize {
		pkt := data[pos : pos+tsPacketSize]
		if pkt[0] != tsSyncByte {
			break
		}

		pid := (int(pkt[1]&0x1F) << 8) | int(pkt[2])
		hasPayload := pkt[3]&0x10 != 0
		hasAdaptation := pkt[3]&0x20 != 0

		if pid != videoPID || !hasPayload {
			continue
		}

		payloadOffset := 4
		if hasAdaptation {
			adaptLen := int(pkt[4])
			payloadOffset = 5 + adaptLen
			if payloadOffset >= tsPacketSize {
				continue
			}
		}

		payload := pkt[payloadOffset:]

		// 检查 PES header
		payloadStart := pkt[1]&0x40 != 0
		if payloadStart && len(payload) >= 9 {
			// PES start code: 00 00 01
			if payload[0] == 0 && payload[1] == 0 && payload[2] == 1 {
				// 跳过 PES header
				pesHeaderLen := int(payload[8])
				pesPayloadStart := 9 + pesHeaderLen
				if pesPayloadStart < len(payload) {
					videoPESData = append(videoPESData, payload[pesPayloadStart:]...)
				}
				continue
			}
		}

		// 非 PES 起始的 payload，直接追加
		videoPESData = append(videoPESData, payload...)
	}

	// 5. 从 PES 数据中提取 NAL units 并解析 SPS
	if len(videoPESData) > 0 {
		switch videoType {
		case streamTypeH264:
			parseH264NALUnits(videoPESData, info)
		case streamTypeH265:
			parseH265NALUnits(videoPESData, info)
		}
	}

	// 如果有 video codec 但没解析到分辨率，标记可能需要更多数据
	if info.Width == 0 && info.Height == 0 && !info.Unsupported {
		if info.VideoCodec != "" {
			info.Unsupported = true
			info.UnsupportedMsg = "在有限的 TS 数据中未找到 SPS，无法获取分辨率"
		}
	}

	_ = audioPID
	_ = audioType

	return info, nil
}

// findTSSyncOffset 在数据中查找 TS sync byte 对齐位置
func findTSSyncOffset(data []byte) int {
	for i := 0; i < len(data)-tsPacketSize*3; i++ {
		if data[i] == tsSyncByte &&
			data[i+tsPacketSize] == tsSyncByte &&
			data[i+tsPacketSize*2] == tsSyncByte {
			return i
		}
	}
	return -1
}

// parseH264NALUnits 从 Annex B 字节流中提取 H.264 NAL units
func parseH264NALUnits(data []byte, info *StreamHeaderInfo) {
	nals := extractNALUnits(data)
	for _, nal := range nals {
		if len(nal) == 0 {
			continue
		}
		nalType := nal[0] & 0x1F
		if nalType == nalTypeSPS {
			// 使用 mediacommon 解析 SPS
			var sps h264.SPS
			if err := sps.Unmarshal(nal); err == nil {
				info.Width = sps.Width()
				info.Height = sps.Height()
				if fps := sps.FPS(); fps > 0 && fps < 300 {
					info.FrameRate = fps
				}
			}
			return
		}
	}
}

// parseH265NALUnits 从 Annex B 字节流中提取 H.265 NAL units
func parseH265NALUnits(data []byte, info *StreamHeaderInfo) {
	nals := extractNALUnits(data)
	for _, nal := range nals {
		if len(nal) < 2 {
			continue
		}
		nalType := (nal[0] >> 1) & 0x3F
		if nalType == hevcNalTypeSPS {
			var sps h265.SPS
			if err := sps.Unmarshal(nal); err == nil {
				info.Width = sps.Width()
				info.Height = sps.Height()
				if fps := sps.FPS(); fps > 0 && fps < 300 {
					info.FrameRate = fps
				}
			}
			return
		}
	}
}

// extractNALUnits 从 Annex B 字节流提取 NAL units
// Annex B 格式使用 00 00 01 或 00 00 00 01 作为起始码
func extractNALUnits(data []byte) [][]byte {
	var nals [][]byte
	startCode3 := []byte{0, 0, 1}
	startCode4 := []byte{0, 0, 0, 1}

	// 找到所有起始码位置
	var positions []int
	for i := 0; i < len(data)-3; i++ {
		if bytes.Equal(data[i:i+3], startCode3) {
			// 检查是否是 4 字节起始码
			if i > 0 && data[i-1] == 0 {
				continue // 已经被 4 字节起始码处理
			}
			positions = append(positions, i+3)
		} else if i < len(data)-4 && bytes.Equal(data[i:i+4], startCode4) {
			positions = append(positions, i+4)
		}
	}

	// 提取每个 NAL unit
	for i, start := range positions {
		var end int
		if i+1 < len(positions) {
			// 下一个起始码前面可能有 00 00 00 01 或 00 00 01
			end = positions[i+1]
			// 回退到起始码之前
			for end > start && data[end-1] == 0 {
				end--
			}
			// 处理可能的 3 字节起始码
			if end >= 3 && bytes.Equal(data[end-3:end], startCode3) {
				end -= 3
			} else if end >= 4 && bytes.Equal(data[end-4:end], startCode4) {
				end -= 4
			}
		} else {
			end = len(data)
		}

		if end > start {
			nals = append(nals, data[start:end])
		}
	}

	return nals
}

// ProbeHLS 独立的 HLS 流探测函数（不经过 StreamProbe 代理）
// 流程：下载 m3u8 → 解析第一个 TS 分段 URL → 下载 TS 头部 → 解析 SPS
func ProbeHLS(ctx context.Context, m3u8URL *url.URL, headers map[string]string, logger *livelogger.LiveLogger) (*StreamHeaderInfo, error) {
	if logger != nil {
		logger.Debugf("HLS 探测开始: URL=%s, Headers=%v", m3u8URL.String(), headers)
	}
	// 1. 下载 m3u8 内容
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	proxy.ApplyDownloadProxyToTransport(transport)

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxTimeout, http.MethodGet, m3u8URL.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载 m3u8 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("m3u8 返回 HTTP %d", resp.StatusCode)
	}

	// 2. 解析并探测
	return probeHLSStreamInfo(ctxTimeout, resp.Body, m3u8URL, headers, logger)
}
