package servers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bililive-go/bililive-go/src/configs"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
)

func TestRewriteHLSPlaylistSignsSegments(t *testing.T) {
	cfg := &configs.Config{
		Security: configs.Security{
			APIKey:              "secret",
			SignedURLTTLSeconds: 3600,
		},
	}
	cacheKey := strings.Repeat("a", 64)
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:6.0,\nseg_00000.ts\n#EXT-X-ENDLIST\n"

	rewritten := string(rewriteHLSPlaylist([]byte(playlist), cacheKey, cfg))
	lines := strings.Split(rewritten, "\n")
	require.Len(t, lines, 6)
	assert.True(t, strings.HasPrefix(lines[3], "/api/stream/hls-segment/"+cacheKey+"/seg_00000.ts?"))

	req := httptest.NewRequest(http.MethodGet, lines[3], nil)
	assert.NoError(t, securitypkg.ValidateSignedRequest(req, "secret", time.Now()))
}

func TestSignedHLSURLForVideoFile(t *testing.T) {
	cfg := &configs.Config{
		Security: configs.Security{
			APIKey:              "secret",
			SignedURLTTLSeconds: 3600,
		},
	}

	assert.NotEmpty(t, signedHLSURLForVideoFile(".flv", "抖音/主播/video.flv", cfg))
	assert.Empty(t, signedHLSURLForVideoFile(".mp4", "抖音/主播/video.mp4", cfg))
}
