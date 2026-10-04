package pipeline

import (
	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRecordingFinishedOptsIntoPreparationWithoutLegacyConversion(t *testing.T) {
	stages := GetEffectivePipelineConfig(&configs.OnRecordFinished{PreparePlayback: true, ConvertToMp4: true, DeleteFlvAfterConvert: true}).Stages
	require.Len(t, stages, 1)
	require.Equal(t, StageNamePreparePlayback, stages[0].Name)
	require.False(t, stages[0].GetBoolOption(OptionDeleteSource, false))
	legacy := GetEffectivePipelineConfig(&configs.OnRecordFinished{ConvertToMp4: true}).Stages
	require.Len(t, legacy, 1)
	require.Equal(t, StageNameConvertMp4, legacy[0].Name)
}
