package servers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestHelperProcess 是一个测试辅助进程，用于模拟 ffmpeg 行为
// 通过环境变量控制其行为模式
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	behavior := os.Getenv("HELPER_BEHAVIOR")
	outputDir := os.Getenv("HELPER_OUTPUT_DIR")
	playlistPath := os.Getenv("HELPER_PLAYLIST_PATH")

	switch behavior {
	case "first_segment_then_wait":
		// 首片后等待释放信号
		if err := writeMinimalHLSOutput(outputDir, playlistPath, 1, false); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
		// 等待释放信号文件
		releaseFile := filepath.Join(outputDir, ".release")
		for {
			if _, err := os.Stat(releaseFile); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		// 写入完整输出
		if err := writeMinimalHLSOutput(outputDir, playlistPath, 2, true); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write final output: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "single_segment_complete":
		// 单片视频正常完成
		if err := writeMinimalHLSOutput(outputDir, playlistPath, 1, true); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "immediate_failure":
		// 启动即失败
		fmt.Fprintf(os.Stderr, "simulated immediate failure\n")
		os.Exit(1)

	case "first_segment_then_fail":
		// 首片后非零退出
		if err := writeMinimalHLSOutput(outputDir, playlistPath, 1, false); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(500 * time.Millisecond)
		fmt.Fprintf(os.Stderr, "simulated failure after first segment\n")
		os.Exit(1)

	default:
		fmt.Fprintf(os.Stderr, "unknown behavior: %s\n", behavior)
		os.Exit(1)
	}
}

// writeMinimalHLSOutput 写出最小有效的 HLS 清单和分片
func writeMinimalHLSOutput(cacheDir, playlistPath string, segmentCount int, withEndList bool) error {
	// 确保目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	// 写入分片文件（非空）
	for i := 0; i < segmentCount; i++ {
		segmentPath := filepath.Join(cacheDir, fmt.Sprintf("segment%d.ts", i))
		// 写入一些数据以确保非空
		data := make([]byte, 1024)
		for j := range data {
			data[j] = byte(i)
		}
		if err := os.WriteFile(segmentPath, data, 0644); err != nil {
			return err
		}
	}

	// 写入播放列表
	playlist := "#EXTM3U\n"
	playlist += "#EXT-X-VERSION:3\n"
	playlist += "#EXT-X-TARGETDURATION:10\n"
	playlist += "#EXT-X-MEDIA-SEQUENCE:0\n"

	for i := 0; i < segmentCount; i++ {
		playlist += fmt.Sprintf("#EXTINF:10.0,\nsegment%d.ts\n", i)
	}

	if withEndList {
		playlist += "#EXT-X-ENDLIST\n"
	}

	return os.WriteFile(playlistPath, []byte(playlist), 0644)
}

// helperCommand 创建一个测试辅助进程命令
func helperCommand(t *testing.T, behavior, outputDir, playlistPath string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{
		"GO_WANT_HELPER_PROCESS=1",
		"HELPER_BEHAVIOR=" + behavior,
		"HELPER_OUTPUT_DIR=" + outputDir,
		"HELPER_PLAYLIST_PATH=" + playlistPath,
	}
	return cmd
}

// TestHLSHelperProcessBasic 测试辅助进程基本行为
func TestHLSHelperProcessBasic(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	playlistPath := filepath.Join(tmpDir, "playlist.m3u8")

	t.Run("single_segment_complete", func(t *testing.T) {
		cmd := helperCommand(t, "single_segment_complete", cacheDir, playlistPath)

		if err := cmd.Run(); err != nil {
			t.Fatalf("helper process failed: %v", err)
		}

		// 验证输出
		if !isPlaylistReadable(playlistPath) {
			t.Error("playlist not readable")
		}

		content, err := os.ReadFile(playlistPath)
		if err != nil {
			t.Fatalf("failed to read playlist: %v", err)
		}

		if !bytes.Contains(content, []byte("#EXT-X-ENDLIST")) {
			t.Error("playlist missing ENDLIST")
		}

		segmentPath := filepath.Join(cacheDir, "segment0.ts")
		if _, err := os.Stat(segmentPath); err != nil {
			t.Errorf("segment file not found: %v", err)
		}
	})

	t.Run("immediate_failure", func(t *testing.T) {
		cmd := helperCommand(t, "immediate_failure", cacheDir, playlistPath)

		err := cmd.Run()
		if err == nil {
			t.Fatal("expected process to fail, but it succeeded")
		}
	})
}

// TestHLSHelperProcessWithControl 测试可控的辅助进程
func TestHLSHelperProcessWithControl(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	playlistPath := filepath.Join(tmpDir, "playlist.m3u8")
	releaseFile := filepath.Join(cacheDir, ".release")

	cmd := helperCommand(t, "first_segment_then_wait", cacheDir, playlistPath)

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}

	// 等待首片生成
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	firstSegmentReady := false
	for {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			t.Fatal("timeout waiting for first segment")
		default:
			if isPlaylistReadable(playlistPath) {
				segmentPath := filepath.Join(cacheDir, "segment0.ts")
				if _, err := os.Stat(segmentPath); err == nil {
					firstSegmentReady = true
					goto checkProcess
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

checkProcess:
	if !firstSegmentReady {
		t.Fatal("first segment not ready")
	}

	// 验证进程仍在运行
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		t.Fatal("process exited before release signal")
	}

	// 发送释放信号
	if err := os.WriteFile(releaseFile, []byte("release"), 0644); err != nil {
		t.Fatalf("failed to write release file: %v", err)
	}

	// 等待进程结束
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-waitCtx.Done():
		cmd.Process.Kill()
		t.Fatal("timeout waiting for process to exit")
	case err := <-done:
		if err != nil {
			t.Fatalf("process exited with error: %v", err)
		}
	}

	// 验证最终输出
	content, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("failed to read final playlist: %v", err)
	}

	if !bytes.Contains(content, []byte("#EXT-X-ENDLIST")) {
		t.Error("final playlist missing ENDLIST")
	}
}

func isPlaylistReadable(path string) bool {
	data, err := os.ReadFile(path)
	return err == nil && bytes.HasPrefix(data, []byte("#EXTM3U"))
}
