package configs

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPersistentHLSOptInAndBoundedProfiles(t *testing.T) {
	cfg, err := NewConfigWithBytes([]byte("on_record_finished:\n  prepare_playback: true\n"))
	require.NoError(t, err)
	require.Empty(t, cfg.OnRecordFinished.PrepareHLSProfiles)
	cfg, err = NewConfigWithBytes([]byte("on_record_finished:\n  prepare_hls_profiles: [source, 720p]\n"))
	require.NoError(t, err)
	require.Equal(t, []string{"source", "720p"}, cfg.OnRecordFinished.PrepareHLSProfiles)
	for _, profiles := range []string{"[unknown]", "[source, source]", "[../escape]"} {
		_, err := NewConfigWithBytes([]byte("on_record_finished:\n  prepare_hls_profiles: " + profiles + "\n"))
		require.Error(t, err)
	}
}
