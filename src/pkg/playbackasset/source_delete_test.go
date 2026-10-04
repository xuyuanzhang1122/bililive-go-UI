package playbackasset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteSourceRejectsUnrelatedDeletionIntent(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, PublishValidated(source, target, temp, info))
	require.NoError(t, os.WriteFile(target+".deleting-source.json", []byte(`{"delete_source":false}`), 0600))
	require.Error(t, DeleteSourceValidated(source, target, info))
	_, err := os.Stat(source)
	require.NoError(t, err)
}

func TestRecoverSourceDeletionAfterRemoval(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, PublishValidated(source, target, temp, info))
	manifest, err := Load(target)
	require.NoError(t, err)
	data, err := json.Marshal(sourceDeletionIntent{Manifest: manifest, DeleteSource: true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target+SourceDeletionSuffix, data, 0600))
	// 模拟删源后、同步/清理意图前退出。
	require.NoError(t, os.Remove(source))
	recovered, err := RecoverSourceDeletion(target)
	require.NoError(t, err)
	require.True(t, recovered)
	_, err = os.Stat(target + SourceDeletionSuffix)
	require.True(t, os.IsNotExist(err))
	recovered, err = RecoverSourceDeletion(target)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverSourceDeletionKeepsReplacement(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, PublishValidated(source, target, temp, info))
	manifest, err := Load(target)
	require.NoError(t, err)
	data, err := json.Marshal(sourceDeletionIntent{Manifest: manifest, DeleteSource: true})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target+SourceDeletionSuffix, data, 0600))
	require.NoError(t, os.WriteFile(source, []byte("new recording"), 0600))
	_, err = RecoverSourceDeletion(target)
	require.Error(t, err)
	contents, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "new recording", string(contents))
}

func TestSourceDeletionResumesOnlyAfterExplicitRevalidation(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, PublishValidated(source, target, temp, info))
	manifest, err := Load(target)
	require.NoError(t, err)
	_, err = saveSourceDeletionIntent(target, sourceDeletionIntent{Manifest: manifest, DeleteSource: true})
	require.NoError(t, err)
	// 模拟意图同步后、删除前退出；恢复入口不能自行删除来源。
	_, err = RecoverSourceDeletion(target)
	require.Error(t, err)
	_, err = os.Stat(source)
	require.NoError(t, err)
	require.NoError(t, DeleteSourceValidated(source, target, info))
	_, err = os.Stat(target + SourceDeletionSuffix)
	require.True(t, os.IsNotExist(err))
}

func TestSourceDeletionRecoveryRetainsIntentWhenTargetChanges(t *testing.T) {
	source, target, temp, info := publicationFixture(t)
	require.NoError(t, PublishValidated(source, target, temp, info))
	manifest, err := Load(target)
	require.NoError(t, err)
	_, err = saveSourceDeletionIntent(target, sourceDeletionIntent{Manifest: manifest, DeleteSource: true})
	require.NoError(t, err)
	require.NoError(t, os.Remove(source))
	require.NoError(t, os.WriteFile(target, []byte("changed target"), 0600))
	_, err = RecoverSourceDeletion(target)
	require.Error(t, err)
	_, err = os.Stat(target + SourceDeletionSuffix)
	require.NoError(t, err, "失效产物不能被误标为已恢复")
}

func TestDeleteSourceValidatedRequiresCommittedMatchingTarget(t *testing.T) {
	for _, change := range []string{"missing_target", "changed_target", "missing_manifest", "different_source", "changed_source", "valid"} {
		t.Run(change, func(t *testing.T) {
			source, target, temp, info := publicationFixture(t)
			require.NoError(t, PublishValidated(source, target, temp, info))
			switch change {
			case "missing_target":
				require.NoError(t, os.Remove(target))
			case "changed_target":
				require.NoError(t, os.WriteFile(target, []byte("unrelated replacement recording"), 0600))
			case "missing_manifest":
				require.NoError(t, os.Remove(target+Suffix))
			case "different_source":
				require.NoError(t, os.Remove(target+Suffix))
				other := filepath.Join(filepath.Dir(source), "other.flv")
				require.NoError(t, os.WriteFile(other, []byte("other recording"), 0600))
				otherInfo, err := os.Stat(other)
				require.NoError(t, err)
				require.NoError(t, Save(other, target, otherInfo))
			case "changed_source":
				require.NoError(t, os.WriteFile(source, []byte("new recording"), 0600))
			}
			err := DeleteSourceValidated(source, target, info)
			if change == "valid" {
				require.NoError(t, err)
				_, err = os.Stat(source)
				require.True(t, os.IsNotExist(err))
				_, err = Load(target)
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				_, err = os.Stat(source)
				require.NoError(t, err, "复核失败必须保留来源")
			}
		})
	}
}
