package playbackasset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func publicationFixture(t *testing.T) (source, target, temp string, sourceInfo os.FileInfo) {
	t.Helper()
	dir := t.TempDir()
	source = filepath.Join(dir, "clip.flv")
	target = filepath.Join(dir, "clip.mp4")
	temp = filepath.Join(dir, ".converting-test.mp4")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	require.NoError(t, os.WriteFile(temp, []byte("validated mp4"), 0600))
	sourceInfo, _ = os.Stat(source)
	return
}
func writePublicationIntent(t *testing.T, source, target, temp string, sourceInfo os.FileInfo) {
	t.Helper()
	info, err := os.Stat(temp)
	require.NoError(t, err)
	data, err := json.Marshal(publicationIntent{Manifest: makeManifest(source, target, sourceInfo, info, false), TempName: filepath.Base(temp)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target+IntentSuffix, data, 0600))
}
func TestPublicationRejectsUnrelatedExistingTarget(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, os.WriteFile(target, []byte("unrelated recording"), 0600))
	require.Error(t, PublishValidated(source, target, temp, info))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "unrelated recording", string(data))
	_, err = os.Stat(source)
	require.NoError(t, err)
}
func TestPublicationRecoveryRejectsModifiedVersion(t *testing.T) {
	for _, change := range []string{"source", "target", "invalid_media"} {
		t.Run(change, func(t *testing.T) {
			source, target, temp, info := publicationFixture(t)
			writePublicationIntent(t, source, target, temp, info)
			require.NoError(t, os.Link(temp, target))
			if change == "source" {
				require.NoError(t, os.WriteFile(source, []byte("new source"), 0600))
				info, _ = os.Stat(source)
			}
			if change == "target" {
				require.NoError(t, os.WriteFile(target, []byte("new target"), 0600))
			}
			valid := func(string) error {
				if change == "invalid_media" {
					return os.ErrInvalid
				}
				return nil
			}
			recovered, err := RecoverPublication(source, target, info, valid)
			require.Error(t, err)
			require.False(t, recovered)
			_, err = os.Stat(target + Suffix)
			require.True(t, os.IsNotExist(err))
			_, err = os.Stat(source)
			require.NoError(t, err)
		})
	}
}
func TestPublicationRecoveryDoesNotDeleteReplacement(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	writePublicationIntent(t, source, target, temp, info)
	require.NoError(t, os.Link(temp, target))
	replacement := filepath.Join(filepath.Dir(target), "replacement.mp4")
	require.NoError(t, os.WriteFile(replacement, []byte("other media"), 0600))
	_, err := RecoverPublication(source, target, info, func(string) error { return os.Rename(replacement, target) })
	require.Error(t, err)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "other media", string(data))
}
func TestPublicationRecoveryRejectsDifferentCommittedManifest(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	otherSource := filepath.Join(filepath.Dir(source), "other.flv")
	require.NoError(t, os.WriteFile(otherSource, []byte("other source"), 0600))
	otherInfo, err := os.Stat(otherSource)
	require.NoError(t, err)
	writePublicationIntent(t, source, target, temp, info)
	require.NoError(t, os.Link(temp, target))
	require.NoError(t, Save(otherSource, target, otherInfo))

	_, err = RecoverPublication(source, target, info, func(string) error { return nil })
	require.Error(t, err)
	_, err = os.Stat(source)
	require.NoError(t, err, "不同来源的已提交关联不得触发删源")
	manifest, err := Load(target)
	require.NoError(t, err)
	require.Equal(t, filepath.Base(otherSource), manifest.SourceName)
}

func TestPublicationRecoveryKeepsSameConcurrentCommit(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	writePublicationIntent(t, source, target, temp, info)
	require.NoError(t, os.Link(temp, target))
	recovered, err := RecoverPublication(source, target, info, func(string) error {
		if err := Save(source, target, info); err != nil {
			return err
		}
		return os.Remove(source)
	})
	require.NoError(t, err)
	require.True(t, recovered)
	manifest, err := Load(target)
	require.NoError(t, err)
	require.Equal(t, filepath.Base(source), manifest.SourceName)
	_, err = os.Stat(target + IntentSuffix)
	require.True(t, os.IsNotExist(err))
}
