package servers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildVisibleFileListEntriesFiltersProjectFilesAtRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "抖音", "主播A"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".appdata", "hls-cache"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("doc"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "抖音", "主播A", "record.flv"), []byte("video"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".appdata", "hls-cache", "seg_00000.ts"), []byte("segment"), 0644))

	files, err := os.ReadDir(root)
	require.NoError(t, err)

	entries := buildVisibleFileListEntries("", root, files)

	require.Len(t, entries, 1)
	assert.Equal(t, "抖音", entries[0].Name)
	assert.True(t, entries[0].IsFolder)
}

func TestBuildVisibleFileListEntriesKeepsRelatedFilesInsideRecordingFolder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "record.flv"), []byte("video"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cover.jpg"), []byte("cover"), 0644))

	files, err := os.ReadDir(root)
	require.NoError(t, err)

	entries := buildVisibleFileListEntries("抖音/主播A", root, files)

	require.Len(t, entries, 2)
	names := []string{entries[0].Name, entries[1].Name}
	assert.ElementsMatch(t, []string{"record.flv", "cover.jpg"}, names)
}
