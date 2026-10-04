package servers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbeCacheInvalidatesSourceAndTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 POSIX shell")
	}
	root := t.TempDir()
	source := filepath.Join(root, "input.flv")
	tool := filepath.Join(root, "ffprobe")
	count := filepath.Join(root, "calls")
	script := "#!/bin/sh\necho x >> '" + count + "'\necho '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\"}]}'\n"
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	require.NoError(t, os.WriteFile(tool, []byte(script), 0700))
	cache := filepath.Join(root, "cache")
	for i := 0; i < 2; i++ {
		_, err := probeMediaCached(context.Background(), tool, source, cache)
		require.NoError(t, err)
	}
	b, err := os.ReadFile(count)
	require.NoError(t, err)
	require.Equal(t, "x\n", string(b))
	require.NoError(t, os.WriteFile(source, []byte("different source"), 0600))
	_, err = probeMediaCached(context.Background(), tool, source, cache)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tool, []byte(script+"# changed binary\n"), 0700))
	_, err = probeMediaCached(context.Background(), tool, source, cache)
	require.NoError(t, err)
	b, err = os.ReadFile(count)
	require.NoError(t, err)
	require.Equal(t, "x\nx\nx\n", string(b))
}
func TestProbeCacheDoesNotCacheFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("需要 POSIX shell")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	tool := filepath.Join(root, "probe")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	// 相同工具和来源：第一次失败，第二次成功，证明失败没有写入缓存。
	script := "#!/bin/sh\nif [ ! -f '" + root + "/attempt' ]; then touch '" + root + "/attempt'; exit 1; fi\necho '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\"}]}'\n"
	require.NoError(t, os.WriteFile(tool, []byte(script), 0700))
	_, err := probeMediaCached(context.Background(), tool, source, filepath.Join(root, "cache"))
	require.Error(t, err)
	_, err = probeMediaCached(context.Background(), tool, source, filepath.Join(root, "cache"))
	require.NoError(t, err)
}

func TestProbeCacheKeyRejectsSameSizeTimeReplacementIdentity(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	tool := filepath.Join(dir, "ffprobe")
	require.NoError(t, os.WriteFile(source, []byte("old"), 0600))
	require.NoError(t, os.WriteFile(tool, []byte("tool"), 0700))
	before, err := os.Stat(source)
	require.NoError(t, err)
	key, err := probeCacheKey(tool, source)
	require.NoError(t, err)
	replacement := filepath.Join(dir, "replacement.mp4")
	require.NoError(t, os.WriteFile(replacement, []byte("new"), 0600))
	require.NoError(t, os.Chtimes(replacement, before.ModTime(), before.ModTime()))
	require.NoError(t, os.Rename(replacement, source))
	next, err := probeCacheKey(tool, source)
	require.NoError(t, err)
	require.NotEqual(t, key, next, "相同大小/时间的不同文件不能复用旧探测")
}
