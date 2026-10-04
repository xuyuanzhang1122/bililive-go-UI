package playbackasset

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestDerivedPublicationUsesSameDurableProtocolAndKeepsSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.flv")
	target := filepath.Join(root, "assets", "compat.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	info, err := os.Stat(source)
	require.NoError(t, err)
	temp := filepath.Join(filepath.Dir(target), ".converting-test.mp4")
	require.NoError(t, os.WriteFile(temp, []byte("validated output"), 0600))
	require.NoError(t, PublishDerivedValidated(source, target, temp, info))
	m, err := Load(target)
	require.NoError(t, err)
	require.Equal(t, source, m.SourcePath)
	require.Error(t, DeleteSourceValidated(source, target, info))
	require.FileExists(t, source)
	require.NoError(t, os.WriteFile(source, []byte("replacement source"), 0600))
	_, err = Load(target)
	require.Error(t, err)
}

func TestDerivedPublicationRecoversBeforeTargetLink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.flv")
	target := filepath.Join(root, "assets", "compat.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	sourceInfo, err := os.Stat(source)
	require.NoError(t, err)
	temp := filepath.Join(filepath.Dir(target), ".converting-recovery.mp4")
	require.NoError(t, os.WriteFile(temp, []byte("verified output"), 0600))
	tempInfo, err := os.Stat(temp)
	require.NoError(t, err)
	intent := publicationIntent{Manifest: makeManifest(source, target, sourceInfo, tempInfo, true), TempName: filepath.Base(temp)}
	data, err := json.Marshal(intent)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target+IntentSuffix, data, 0600))
	recovered, err := RecoverPublication(source, target, sourceInfo, func(string) error { return nil })
	require.Error(t, err)
	require.False(t, recovered)
	validated := false
	recovered, err = RecoverDerivedPublication(source, target, sourceInfo, func(path string) error {
		bytes, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		require.Equal(t, "verified output", string(bytes))
		validated = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, recovered)
	require.True(t, validated)
	require.FileExists(t, source)
	require.FileExists(t, target)
	require.NoFileExists(t, target+IntentSuffix)
	_, err = Load(target)
	require.NoError(t, err)
}
