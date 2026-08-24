package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyHLSCacheConfigV210(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantAge  int
		wantSize float64
	}{
		{name: "旧配置补默认值", raw: "rpc: {}\n", wantAge: 24, wantSize: 10},
		{name: "显式零值保留", raw: "hls_cache:\n  max_age_hours: 0\n  max_total_size_gb: 0\n", wantAge: 0, wantSize: 0},
		{name: "只补缺失字段", raw: "hls_cache:\n  max_age_hours: 6\n", wantAge: 6, wantSize: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age, size := -1, -1.0
			require.NoError(t, ApplyHLSCacheConfigV210([]byte(tt.raw), &age, &size, 24, 10))
			assert.Equal(t, tt.wantAge, age)
			assert.Equal(t, tt.wantSize, size)
		})
	}
}
