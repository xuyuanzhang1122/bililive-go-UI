package recordingcatalog

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparationRejectsReplacementAndActiveRecording(t *testing.T) {
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	snapshot, err := SnapshotVersion(source)
	require.NoError(t, err)
	entry, err := s.PreparationEntry(ctx, source, snapshot)
	require.NoError(t, err)
	require.NotEmpty(t, entry.RecordingID)
	replacement := filepath.Join(filepath.Dir(source), "replacement.flv")
	info, err := os.Stat(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(replacement, []byte("source recording"), 0600))
	require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
	require.NoError(t, os.Rename(replacement, source))
	_, err = s.PreparationEntry(ctx, source, snapshot)
	require.Error(t, err, "同大小同mtime替换也必须拒绝")
	require.NoError(t, s.Reconcile(ctx))
	_, err = s.PreparationEntry(ctx, source, snapshot)
	require.Error(t, err, "重新核验后不能接纳旧队列输入")
	snapshot, err = SnapshotVersion(source)
	require.NoError(t, err)
	release := BeginRecording(source)
	_, err = s.PreparationEntry(ctx, source, snapshot)
	require.ErrorIs(t, err, ErrRecordingActive)
	release()
	_, err = s.PreparationEntry(ctx, source, snapshot)
	require.NoError(t, err)
}

func TestCatalogV1MigratesPreparationProofWithoutLosingIdentity(t *testing.T) {
	s, root, _ := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	original, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	_, err = s.db.Exec("DROP TABLE IF EXISTS prepared_verifications; PRAGMA user_version=1")
	require.NoError(t, err)
	dbPath := s.dbPath
	require.NoError(t, s.Close())
	reopened, err := Open(dbPath, root)
	require.NoError(t, err)
	defer reopened.Close()
	current, err := reopened.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.Equal(t, original, current)
	var schemaVersion int
	require.NoError(t, reopened.db.QueryRow("PRAGMA user_version").Scan(&schemaVersion))
	require.Equal(t, 2, schemaVersion)
	var proofCount int
	require.NoError(t, reopened.db.QueryRow("SELECT COUNT(*) FROM prepared_verifications").Scan(&proofCount))
	require.Zero(t, proofCount)
}
