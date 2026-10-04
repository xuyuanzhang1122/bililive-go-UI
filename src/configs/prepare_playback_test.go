package configs

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreparePlaybackOptInMigration(t *testing.T) {
	cfg, err := NewConfigWithBytes([]byte("on_record_finished:\n  convert_to_mp4: true\n"))
	require.NoError(t, err)
	require.False(t, cfg.OnRecordFinished.PreparePlayback)
	cfg, err = NewConfigWithBytes([]byte("on_record_finished:\n  prepare_playback: true\n"))
	require.NoError(t, err)
	require.True(t, cfg.OnRecordFinished.PreparePlayback)
}
