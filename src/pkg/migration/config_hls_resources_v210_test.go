package migration

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestApplyHLSResourceConfigV210(t *testing.T) {
	for _, tt := range []struct {
		name, raw          string
		concurrent, queued int
		free               float64
		grace              int
	}{
		{"旧配置", "rpc: {}", 3, 64, 1, 60},
		{"空配置块", "hls_cache: null", 3, 64, 1, 60},
		{"部分配置", "hls_cache:\n  max_queued: 12", 3, 12, 1, 60},
		{"显式零", "hls_cache:\n  max_concurrent: 0\n  max_queued: 0\n  min_free_space_gb: 0\n  read_grace_seconds: 0", 0, 0, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			concurrent, queued, free, grace := -1, -1, -1.0, -1
			require.NoError(t, ApplyHLSResourceConfigV210([]byte(tt.raw), &concurrent, &queued, &free, &grace, 3, 64, 1, 60))
			require.Equal(t, tt.concurrent, concurrent)
			require.Equal(t, tt.queued, queued)
			require.Equal(t, tt.free, free)
			require.Equal(t, tt.grace, grace)
		})
	}
}
