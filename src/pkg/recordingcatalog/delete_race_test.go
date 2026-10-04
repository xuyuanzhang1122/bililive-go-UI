package recordingcatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPinnedDeleteRejectsReplacementAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.flv")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
	expected, err := SnapshotVersion(path)
	require.NoError(t, err)
	parent, err := os.Open(dir)
	require.NoError(t, err)
	defer parent.Close()
	require.NoError(t, os.Rename(path, path+".old"))
	require.NoError(t, os.WriteFile(path, []byte("replacement"), 0600))
	require.ErrorIs(t, removePinnedFile(parent, "clip.flv", expected), ErrPlaybackChanged)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
}

func TestPinnedDeleteStaysInOpenedParentAfterSymlinkReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 目录句柄会阻止此改名；其他用例仍校验句柄删源")
	}
	root, outside := t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "room")
	require.NoError(t, os.Mkdir(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "clip.flv"), []byte("original"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "clip.flv"), []byte("outside"), 0600))
	expected, err := SnapshotVersion(filepath.Join(dir, "clip.flv"))
	require.NoError(t, err)
	parent, err := os.Open(dir)
	require.NoError(t, err)
	defer parent.Close()
	moved := dir + "-moved"
	require.NoError(t, os.Rename(dir, moved))
	require.NoError(t, os.Symlink(outside, dir))
	require.NoError(t, removePinnedFile(parent, "clip.flv", expected))
	data, err := os.ReadFile(filepath.Join(outside, "clip.flv"))
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
	require.NoFileExists(t, filepath.Join(moved, "clip.flv"))
}

func TestDeletionLeaseRejectsActiveAndBlocksNewRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.flv")
	end := BeginRecording(path)
	called := false
	err := withInactiveSources([]string{path}, func() error { called = true; return nil })
	end()
	require.ErrorIs(t, err, ErrRecordingActive)
	require.False(t, called)
	started, entered, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	err = withInactiveSources([]string{path}, func() error {
		go func() {
			defer close(done)
			close(started)
			finish := BeginRecording(path)
			close(entered)
			finish()
		}()
		<-started
		select {
		case <-entered:
			t.Error("录制在删除租约内开始")
		case <-time.After(50 * time.Millisecond):
		}
		return nil
	})
	require.NoError(t, err)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("删除结束后录制未继续")
	}
}
