package playbackasset

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestTracksSourceAndRejectsStaleOutput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "clip.flv")
	target := filepath.Join(dir, "clip.mp4")
	require.NoError(t, os.WriteFile(source, []byte("original"), 0644))
	require.NoError(t, os.WriteFile(target, []byte("prepared"), 0644))
	info, err := os.Stat(source)
	require.NoError(t, err)
	require.NoError(t, Save(source, target, info))
	m, err := Load(target)
	require.NoError(t, err)
	require.Equal(t, "clip.flv", m.SourceName)
	require.NoError(t, os.Remove(source))
	_, err = Load(target)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, []byte("replacement"), 0644))
	_, err = Load(target)
	require.Error(t, err)
}
func TestManifestRejectsSourceReplacementAndConflict(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "clip.flv")
	other := filepath.Join(dir, "other.flv")
	target := filepath.Join(dir, "clip.mp4")
	require.NoError(t, os.WriteFile(source, []byte("old"), 0644))
	require.NoError(t, os.WriteFile(target, []byte("prepared"), 0644))
	info, _ := os.Stat(source)
	require.NoError(t, Save(source, target, info))
	require.NoError(t, os.WriteFile(other, []byte("other"), 0644))
	info, _ = os.Stat(other)
	require.Error(t, Save(other, target, info))
	require.NoError(t, os.WriteFile(source, []byte("new recording"), 0644))
	_, err := Load(target)
	require.Error(t, err)
}
