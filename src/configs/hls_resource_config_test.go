package configs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHLSResourceDefaultsAndRoundTrip(t *testing.T) {
	for _, raw := range []string{"rpc: {}", "hls_cache:\n  max_age_hours: 0\n"} {
		cfg, err := NewConfigWithBytes([]byte(raw))
		require.NoError(t, err)
		encoded, err := json.Marshal(cfg.HLSCache)
		require.NoError(t, err)
		var fields map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &fields))
		require.Equal(t, float64(3), fields["max_concurrent"])
		require.Equal(t, float64(64), fields["max_queued"])
		require.Equal(t, float64(1), fields["min_free_space_gb"])
		require.Equal(t, float64(60), fields["read_grace_seconds"])
	}
	cfg, err := NewConfigWithBytes([]byte("hls_cache:\n  max_concurrent: 2\n  max_queued: 17\n  min_free_space_gb: 0\n  read_grace_seconds: 0\n"))
	require.NoError(t, err)
	for _, marshal := range []func(interface{}) ([]byte, error){json.Marshal, yaml.Marshal} {
		encoded, err := marshal(map[string]interface{}{"hls_cache": cfg.HLSCache})
		require.NoError(t, err)
		restored, err := NewConfigWithBytes(encoded)
		require.NoError(t, err)
		require.Equal(t, cfg.HLSCache, restored.HLSCache)
	}
}

func TestHLSResourceValidation(t *testing.T) {
	for _, field := range []string{"max_concurrent: 0", "max_concurrent: -1", "max_concurrent: 33", "max_queued: 0", "max_queued: -1", "max_queued: 1025", "min_free_space_gb: -1", "min_free_space_gb: .nan", "min_free_space_gb: .inf", "read_grace_seconds: -1"} {
		t.Run(field, func(t *testing.T) {
			cfg, err := NewConfigWithBytes([]byte("hls_cache:\n  " + field + "\n"))
			require.NoError(t, err)
			cfg.OutPutPath = t.TempDir()
			cfg.RPC.Enable = true
			require.Error(t, cfg.Verify())
		})
	}
	for _, field := range []string{"max_concurrent: 1", "max_concurrent: 32", "max_queued: 1", "max_queued: 1024", "min_free_space_gb: 0", "read_grace_seconds: 0"} {
		t.Run(field, func(t *testing.T) {
			cfg, err := NewConfigWithBytes([]byte("hls_cache:\n  " + field + "\n"))
			require.NoError(t, err)
			cfg.OutPutPath = t.TempDir()
			cfg.RPC.Enable = true
			require.NoError(t, cfg.Verify())
		})
	}
}
