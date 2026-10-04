package recordingcatalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDerivedAssetsAreSeparateAndVersionBound(t *testing.T) {
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	entry, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	rel := entry.RecordingID + "/v1/segment.ts"
	abs := filepath.Join(s.assetRoot, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
	require.NoError(t, os.WriteFile(abs, []byte("derived bytes"), 0600))
	require.Error(t, s.RegisterDerived(ctx, entry.RecordingID, "wrong-version", "v1", rel, "hls"))
	require.Error(t, s.RegisterDerived(ctx, entry.RecordingID, entry.SourceVersion, "v1", "../outside.ts", "hls"))
	require.NoError(t, s.RegisterDerived(ctx, entry.RecordingID, entry.SourceVersion, "v1", rel, "hls"))
	require.NoError(t, s.Reconcile(ctx))
	rooms, err := s.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(len("source recording")), rooms[0].TotalSize)
	require.Equal(t, int64(len("derived bytes")), rooms[0].DerivedSize)
	require.Equal(t, 1, rooms[0].VideoCount)
	require.NoError(t, os.WriteFile(source, []byte("new source version"), 0600))
	require.NoError(t, s.Reconcile(ctx))
	var status string
	require.NoError(t, s.db.QueryRow("SELECT status FROM media_assets WHERE kind='hls'").Scan(&status))
	require.Equal(t, "unverified", status, "旧来源版本产物不能再次标为有效")
	require.NoError(t, os.Remove(abs))
	require.NoError(t, s.Reconcile(ctx))
	rooms, err = s.Rooms(ctx)
	require.NoError(t, err)
	require.Zero(t, rooms[0].DerivedSize)
}

func TestDerivedRootInsideOutputCannotBecomeAMainVideoRoom(t *testing.T) {
	s, root, _ := catalogFixture(t)
	s.assetRoot = filepath.Join(root, "derived")
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	entry, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	rel := entry.RecordingID + "/v1/segment.ts"
	path := filepath.Join(s.assetRoot, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("derived"), 0600))
	require.NoError(t, s.RegisterDerived(ctx, entry.RecordingID, entry.SourceVersion, "v1", rel, "hls"))
	require.NoError(t, s.Reconcile(ctx))
	rooms, err := s.Rooms(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	require.Equal(t, "抖音/主播", rooms[0].FolderPath)
}
