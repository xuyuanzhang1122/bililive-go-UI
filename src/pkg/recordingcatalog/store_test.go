package recordingcatalog

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/stretchr/testify/require"
)

func catalogFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "catalog.db")
	s, err := Open(db, root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	path := filepath.Join(root, "抖音", "主播", "clip.flv")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte("source recording"), 0600))
	return s, root, path
}

func TestCatalogStableIdentityRenameAndReplacement(t *testing.T) {
	s, root, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	first, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.NoError(t, s.Reconcile(ctx))
	again, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.Equal(t, first.RecordingID, again.RecordingID)
	db := s.dbPath
	require.NoError(t, s.Close())
	reopened, err := Open(db, root)
	require.NoError(t, err)
	defer reopened.Close()
	newPath := filepath.Join(filepath.Dir(source), "renamed.flv")
	require.NoError(t, os.Rename(source, newPath))
	require.NoError(t, reopened.Reconcile(ctx))
	renamed, err := reopened.Lookup(ctx, "抖音/主播/renamed.flv")
	require.NoError(t, err)
	require.Equal(t, first.RecordingID, renamed.RecordingID)
	old, err := reopened.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.False(t, old.Present)
	resolved, err := reopened.ResolveAlias(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.Equal(t, "抖音/主播/renamed.flv", resolved)
	require.NoError(t, os.WriteFile(source, []byte("a different recording"), 0600))
	_, err = reopened.ResolveAlias(ctx, "抖音/主播/clip.flv")
	require.Error(t, err, "目录刷新前也不能用旧别名覆盖新真实文件")
	require.NoError(t, reopened.Reconcile(ctx))
	replacement, err := reopened.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.NotEqual(t, first.RecordingID, replacement.RecordingID)
}

func TestCatalogPreparedAssetKeepsIdentityAndPhysicalSpace(t *testing.T) {
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	first, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	target := filepath.Join(filepath.Dir(source), "clip.mp4")
	temp := filepath.Join(filepath.Dir(source), ".converting-test.mp4")
	require.NoError(t, os.WriteFile(temp, []byte("validated mp4"), 0600))
	info, err := os.Stat(source)
	require.NoError(t, err)
	require.NoError(t, playbackasset.PublishValidated(source, target, temp, info))
	require.NoError(t, s.Reconcile(ctx))
	asset, err := s.Lookup(ctx, "抖音/主播/clip.mp4")
	require.NoError(t, err)
	require.Equal(t, first.RecordingID, asset.RecordingID)
	rooms, err := s.Rooms(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	require.Equal(t, 1, rooms[0].VideoCount)
	require.Equal(t, int64(len("source recording")+len("validated mp4")), rooms[0].TotalSize)
	require.NoError(t, playbackasset.DeleteSourceValidated(source, target, info))
	require.NoError(t, s.Reconcile(ctx))
	asset, err = s.Lookup(ctx, "抖音/主播/clip.mp4")
	require.NoError(t, err)
	require.Equal(t, first.RecordingID, asset.RecordingID)
	require.Equal(t, first.SourceVersion, asset.SourceVersion)
	rooms, err = s.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(len("validated mp4")), rooms[0].TotalSize)
}

func TestCatalogStatisticsTransactionRollsBack(t *testing.T) {
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	before, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	_, err = s.db.Exec(`CREATE TRIGGER reject_statistics BEFORE INSERT ON room_statistics BEGIN SELECT RAISE(ABORT, 'injected'); END`)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte("growing recording"), 0600))
	require.Error(t, s.Reconcile(ctx))
	after, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.Equal(t, before.SourceVersion, after.SourceVersion)
	rooms, err := s.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, "unavailable", rooms[0].Status)
	require.Zero(t, rooms[0].TotalSize)
}

func TestCatalogRoomQueriesDoNotScanFilesystem(t *testing.T) {
	s, root, _ := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	require.NoError(t, os.Rename(root, root+"-offline"))
	t.Cleanup(func() { _ = os.Rename(root+"-offline", root) })
	rooms, err := s.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, "verified", rooms[0].Status)
	require.Error(t, s.Reconcile(ctx))
	rooms, err = s.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, "unavailable", rooms[0].Status)
	require.Zero(t, rooms[0].TotalSize)
}

func TestCatalogSwappedPathsKeepBothIdentities(t *testing.T) {
	s, _, firstPath := catalogFixture(t)
	secondPath := filepath.Join(filepath.Dir(firstPath), "second.flv")
	require.NoError(t, os.WriteFile(secondPath, []byte("second recording"), 0600))
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	first, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	second, err := s.Lookup(ctx, "抖音/主播/second.flv")
	require.NoError(t, err)
	temp := firstPath + ".swap"
	require.NoError(t, os.Rename(firstPath, temp))
	require.NoError(t, os.Rename(secondPath, firstPath))
	require.NoError(t, os.Rename(temp, secondPath))
	require.NoError(t, s.Reconcile(ctx))
	swappedFirst, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	swappedSecond, err := s.Lookup(ctx, "抖音/主播/second.flv")
	require.NoError(t, err)
	require.Equal(t, second.RecordingID, swappedFirst.RecordingID)
	require.Equal(t, first.RecordingID, swappedSecond.RecordingID)
}

func TestCatalogCrashRollsBackUncommittedStatistics(t *testing.T) {
	if os.Getenv("BILIVE_CATALOG_CRASH_CHILD") == "1" {
		s, err := Open(os.Getenv("BILIVE_CATALOG_TEST_DB"), os.Getenv("BILIVE_CATALOG_TEST_ROOT"))
		require.NoError(t, err)
		tx, err := s.db.Begin()
		require.NoError(t, err)
		_, err = tx.Exec("UPDATE room_statistics SET total_size=999999; UPDATE recordings SET source_version='uncommitted'")
		require.NoError(t, err)
		os.Exit(72) // 不执行清理，模拟统计事务中进程崩溃。
	}
	s, root, _ := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	before, err := s.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	db := s.dbPath
	require.NoError(t, s.Close())
	childCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestCatalogCrashRollsBackUncommittedStatistics$")
	command.Env = append(os.Environ(), "BILIVE_CATALOG_CRASH_CHILD=1", "BILIVE_CATALOG_TEST_DB="+db, "BILIVE_CATALOG_TEST_ROOT="+root)
	output, err := command.CombinedOutput()
	var exited *exec.ExitError
	require.True(t, errors.As(err, &exited), string(output))
	require.Equal(t, 72, exited.ExitCode(), string(output))
	reopened, err := Open(db, root)
	require.NoError(t, err)
	defer reopened.Close()
	after, err := reopened.Lookup(ctx, "抖音/主播/clip.flv")
	require.NoError(t, err)
	require.Equal(t, before, after)
	rooms, err := reopened.Rooms(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(len("source recording")), rooms[0].TotalSize)
}
