package servers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHLSCacheJanitorDeletesExpiredDirectories(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	oldDir := createHLSCacheTestDir(t, root, "old", 128, now.Add(-25*time.Hour))
	recentDir := createHLSCacheTestDir(t, root, "recent", 128, now.Add(-time.Hour))

	result, err := runHLSCacheJanitor(root, 24, 0, now)
	require.NoError(t, err)
	assert.Equal(t, 1, result.DeletedDirectories)
	assert.NoDirExists(t, oldDir)
	assert.DirExists(t, recentDir)
}

func TestRunHLSCacheJanitorEvictsOldestUntilUnderSizeLimit(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	oldDir := createHLSCacheTestDir(t, root, "old", 700, now.Add(-2*time.Hour))
	newDir := createHLSCacheTestDir(t, root, "new", 700, now.Add(-time.Hour))
	limitGB := float64(1000) / float64(int64(1)<<30)

	result, err := runHLSCacheJanitor(root, 0, limitGB, now)
	require.NoError(t, err)
	assert.Equal(t, 1, result.DeletedDirectories)
	assert.Equal(t, int64(700), result.FreedBytes)
	assert.NoDirExists(t, oldDir)
	assert.DirExists(t, newDir)
}

func TestResetHLSCacheRemovesStartupContents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hls-cache")
	createHLSCacheTestDir(t, root, "stale", 32, time.Now())

	require.NoError(t, resetHLSCache(root))
	assert.DirExists(t, root)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRunHLSCacheJanitorDropsMissingLockEntries(t *testing.T) {
	root := t.TempDir()
	cacheKey := strings.Repeat("c", 64)
	lock := acquireHLSCacheLock(cacheKey)
	lock.users.Add(-1)

	_, err := runHLSCacheJanitor(root, 0, 0, time.Now())
	require.NoError(t, err)
	_, exists := hlsCacheLocks.Load(cacheKey)
	assert.False(t, exists)
}

func createHLSCacheTestDir(t *testing.T, root, name string, size int, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "segment.ts"), []byte(strings.Repeat("x", size)), 0o644))
	require.NoError(t, os.Chtimes(dir, modTime, modTime))
	return dir
}
