package hlsasset

import (
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func bundleFixture(t *testing.T) (string, Manifest) {
	t.Helper()
	root := t.TempDir()
	data, err := os.MkdirTemp(root, ".hls-build-")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(data, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:2\n#EXTINF:1.0,\nseg_00000.ts\n#EXT-X-ENDLIST\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(data, "seg_00000.ts"), []byte("segment payload"), 0600))
	m, err := Seal(data, Manifest{Version: 1, RecordingID: "recording", SourceVersion: "source", SourcePath: "/recording.mkv", Profile: "source", Duration: 1})
	require.NoError(t, err)
	return root, m
}

func TestBundleCommitRecoveryPreservesFilesAndRejectsTamper(t *testing.T) {
	root, m := bundleFixture(t)
	require.NoError(t, Publish(root, m, nil))
	loaded, err := Load(root)
	require.NoError(t, err)
	require.Equal(t, m.Digest(), loaded.Digest())
	require.NoError(t, loaded.ValidateAll(root))
	require.NoError(t, os.WriteFile(loaded.FilePath(root, "seg_00000.ts"), []byte("damaged payload"), 0600))
	require.Error(t, loaded.ValidateAll(root))
}

func TestBundleRecoversAfterIndexBeforeCommit(t *testing.T) {
	root, m := bundleFixture(t)
	injected := errors.New("publication interrupted")
	require.ErrorIs(t, Publish(root, m, func() error { return injected }), injected)
	require.FileExists(t, filepath.Join(root, "index.m3u8"))
	require.NoFileExists(t, filepath.Join(root, ".hls.json"))
	recovered, ok, err := Recover(root, m)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, m.Digest(), recovered.Digest())
	require.NoError(t, recovered.ValidateAll(root))
	wrong := m
	wrong.SourceVersion = "another-source"
	_, _, err = Recover(root, wrong)
	require.Error(t, err)
}

func TestBundleRejectsGrowingOrEscapingPlaylist(t *testing.T) {
	for _, contents := range []string{
		"#EXTM3U\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXTINF:1,\nseg_00000.ts\n",
		"#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:1,\n../escape.ts\n#EXT-X-ENDLIST\n",
		"#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:1,\nhttps://example.test/seg.ts\n#EXT-X-ENDLIST\n",
	} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte(contents), 0600))
		_, err := Seal(dir, Manifest{Duration: 1})
		require.Error(t, err)
	}
}

func TestAbandonedBuildCleanupProtectsIntentAndCommittedData(t *testing.T) {
	root, m := bundleFixture(t)
	owner := m
	owner.Files = nil
	data, err := json.Marshal(owner)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, m.DataDir, ".build-owner.json"), data, 0600))
	injected := errors.New("interrupted")
	require.ErrorIs(t, Publish(root, m, func() error { return injected }), injected)
	require.NoError(t, CleanAbandoned(root, m))
	require.DirExists(t, filepath.Join(root, m.DataDir))
	_, ok, err := Recover(root, m)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, CleanAbandoned(root, m))
	require.NoError(t, m.ValidateAll(root))
	orphan, err := NewBuild(root, owner)
	require.NoError(t, err)
	require.NoError(t, CleanAbandoned(root, m))
	require.NoDirExists(t, orphan)
	require.NoError(t, m.ValidateAll(root))
}
