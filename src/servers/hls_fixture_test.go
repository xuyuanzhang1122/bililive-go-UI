package servers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 子进程入口不解析 testing 的参数，接收生产路径发出的真实 ffmpeg 参数。
func TestMain(m *testing.M) {
	switch filepath.Base(os.Args[0]) {
	case "ffprobe.fixture":
		fmt.Print(`{"streams":[{"codec_type":"video","codec_name":"h264"}]}`)
		os.Exit(0)
	case "ffmpeg.fixture":
		os.Exit(runHLSFixture(os.Args[1:]))
	default:
		os.Exit(m.Run())
	}
}

func runHLSFixture(args []string) int {
	var source string
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) {
			source = args[i+1]
		}
	}
	if source == "" || len(args) == 0 {
		return 2
	}
	playlist := args[len(args)-1]
	starts, err := os.OpenFile(source+".starts", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 2
	}
	_, err = starts.WriteString("start\n")
	closeErr := starts.Close()
	if err != nil || closeErr != nil {
		return 2
	}
	mode, err := os.ReadFile(source)
	if err != nil {
		return 2
	}
	if string(mode) == "fail" {
		fmt.Fprintln(os.Stderr, "fixture failure")
		return 9
	}
	dir := filepath.Dir(playlist)
	if err = os.WriteFile(filepath.Join(dir, "seg_00000.ts"), []byte("segment payload"), 0600); err != nil {
		return 2
	}
	data := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:1.0,\nseg_00000.ts\n")
	if err = os.WriteFile(playlist+".tmp", data, 0600); err != nil {
		return 2
	}
	if err = os.Rename(playlist+".tmp", playlist); err != nil {
		return 2
	}
	if string(mode) == "hold" {
		for {
			if _, err := os.Stat(source + ".release"); err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if string(mode) == "partial" {
		return 9
	}
	f, err := os.OpenFile(playlist, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 2
	}
	_, err = f.WriteString("#EXT-X-ENDLIST\n")
	closeErr = f.Close()
	if err != nil || closeErr != nil {
		return 2
	}
	return 0
}
