package pipeline

import (
	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHLSProfilesSnapshotBeforeUploadAndPreserveSource(t *testing.T) {
	cfg := &configs.OnRecordFinished{PrepareHLSProfiles: []string{"source", "720p"}, ConvertToMp4: true, FixFlvAtFirst: true, CloudUpload: configs.CloudUpload{Enable: true, StorageName: "test", DeleteAfterUpload: true}}
	stages := GetEffectivePipelineConfig(cfg).Stages
	require.Len(t, stages, 3)
	require.Equal(t, StageNamePreparePlayback, stages[0].Name)
	require.Equal(t, StageNamePrepareHLS, stages[1].Name)
	require.Equal(t, []string{"source", "720p"}, stages[1].GetStringSliceOption("profiles"))
	cfg.PrepareHLSProfiles[0] = "1080p"
	require.Equal(t, "source", stages[1].GetStringSliceOption("profiles")[0])
	require.False(t, stages[2].GetBoolOption(OptionDeleteAfter, true))
}
