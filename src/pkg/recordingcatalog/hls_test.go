package recordingcatalog

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHLSDerivedSetProofSurvivesReconcileAndSourceChange(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "平台", "主播", "clip.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0755))
	require.NoError(t, os.WriteFile(source, []byte("original"), 0600))
	store, err := Open(filepath.Join(t.TempDir(), "catalog.db"), root)
	require.NoError(t, err)
	m := NewManager(store, 0)
	defer m.Close()
	require.NoError(t, m.Reconcile(context.Background()))
	version, err := SnapshotVersion(source)
	require.NoError(t, err)
	entry, err := m.PreparationEntry(context.Background(), source, version)
	require.NoError(t, err)
	assetVersion := version + "-hls-source-v1"
	dir := filepath.Join(m.AssetRoot(), entry.RecordingID, assetVersion)
	require.NoError(t, os.MkdirAll(dir, 0755))
	proofs := make(map[string]string)
	for _, name := range []string{"index.m3u8", "seg_00000.ts"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(name), 0600))
		proofs[path], err = SnapshotVersion(path)
		require.NoError(t, err)
	}
	require.NoError(t, m.RegisterHLS(context.Background(), entry, "hls-source-v1", strings.Repeat("a", 64), proofs))
	require.NoError(t, m.Reconcile(context.Background()))
	asset, err := m.FindHLS(context.Background(), entry.RecordingID, assetVersion)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "index.m3u8"), asset.Path)
	rooms, err := m.Rooms(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(len("index.m3u8")+len("seg_00000.ts")), rooms[0].DerivedSize)
	require.Equal(t, int64(len("original")), rooms[0].TotalSize)
	require.NoError(t, os.WriteFile(source, []byte("new source"), 0600))
	require.NoError(t, m.Reconcile(context.Background()))
	_, err = m.FindHLS(context.Background(), entry.RecordingID, assetVersion)
	require.Error(t, err)
}
