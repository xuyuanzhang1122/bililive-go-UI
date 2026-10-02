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
	data, err := json.Marshal(publicationIntent{Manifest: Manifest{1, filepath.Base(source), filepath.Base(target), stamp(sourceInfo), stamp(info)}, TempName: filepath.Base(temp)})
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
func TestPublicationRecoveryKeepsConcurrentCommit(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	writePublicationIntent(t, source, target, temp, info)
	require.NoError(t, os.Link(temp, target))
	_, _ = RecoverPublication(source, target, info, func(string) error {
		require.NoError(t, Save(source, target, info))
		return os.Remove(source)
	})
	_, err := os.Stat(target)
	require.NoError(t, err, "另一恢复已提交且按策略删源时，不得回滚已提交产物")
	_, err = Load(target)
	require.NoError(t, err)
}
