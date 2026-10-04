package servers

import (
	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHLSRetryAfterIncludesReadGrace(t *testing.T) {
	cfg := &configs.Config{HLSCache: configs.HLSCacheConfig{ReadGraceSeconds: 60}}
	j := newHLSJob(cfg, "key", "source", "dir", "playlist")
	defer j.cancel()
	j.finishedAt = time.Now().Add(-10 * time.Second)
	m := &hlsJobManager{lastRead: map[string]time.Time{"key": time.Now()}}
	require.InDelta(t, 60, m.RetryAfter(j), 1)
	m.lastRead["key"] = time.Now().Add(-time.Hour)
	require.Equal(t, 1, m.RetryAfter(j))
}
