package servers

import (
	"io"
	"os"
	"strings"
)

// demuxer 名称列表不能区分 MOV/MP4、Matroska/WebM；未知品牌保持未知。
func actualProbeContainer(format mediaProbeFormat, path string) string {
	if strings.Contains(format.FormatName, "mov") || strings.Contains(format.FormatName, "mp4") {
		brand := strings.TrimSpace(format.Tags["major_brand"])
		if brand == "qt" {
			return "mov"
		}
		switch brand {
		case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "iso7", "iso8", "iso9", "mp41", "mp42", "avc1", "M4V", "M4A", "MSNV", "dash", "cmfc", "cmfs":
			return "mp4"
		}
		return ""
	}
	if !strings.Contains(format.FormatName, "matroska") && !strings.Contains(format.FormatName, "webm") {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<10))
	if err != nil || len(data) < 5 || string(data[:4]) != "\x1a\x45\xdf\xa3" {
		return ""
	}
	size, n, ok := ebmlVint(data[4:], false)
	if !ok || size > uint64(len(data)-4-n) {
		return ""
	}
	header := data[4+n : 4+n+int(size)]
	for len(header) > 0 {
		id, n, ok := ebmlVint(header, true)
		if !ok {
			return ""
		}
		header = header[n:]
		size, n, ok := ebmlVint(header, false)
		if !ok || size > uint64(len(header)-n) {
			return ""
		}
		header = header[n:]
		if id == 0x4282 {
			switch string(header[:int(size)]) {
			case "webm":
				return "webm"
			case "matroska":
				return "mkv"
			}
			return ""
		}
		header = header[int(size):]
	}
	return ""
}

func ebmlVint(data []byte, identifier bool) (uint64, int, bool) {
	if len(data) == 0 || data[0] == 0 {
		return 0, 0, false
	}
	mask, n := byte(0x80), 1
	for data[0]&mask == 0 {
		mask >>= 1
		n++
	}
	if n > len(data) {
		return 0, 0, false
	}
	v := uint64(data[0])
	if !identifier {
		v &= uint64(mask - 1)
	}
	for _, b := range data[1:n] {
		v = v<<8 | uint64(b)
	}
	return v, n, true
}
