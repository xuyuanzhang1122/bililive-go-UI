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

	playlistReq := httptest.NewRequest(http.MethodGet, "/api/stream/hls/video.flv?_key=secret", nil)
	rewritten := string(rewriteHLSPlaylist(playlistReq, []byte(playlist), cacheKey, cfg))
	lines := strings.Split(rewritten, "\n")
	require.Len(t, lines, 6)
	assert.True(t, strings.HasPrefix(lines[3], "/api/stream/hls-segment/"+cacheKey+"/seg_00000.ts?"))

	req := httptest.NewRequest(http.MethodGet, lines[3], nil)
	assert.NoError(t, securitypkg.ValidateSignedRequest(req, "secret", time.Now()))
}

func TestRewriteHLSPlaylistUsesRawSegmentURLWithoutAPIKey(t *testing.T) {
	cfg := &configs.Config{}
	cacheKey := strings.Repeat("b", 64)
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:6.0,\nseg_00000.ts\n#EXT-X-ENDLIST\n"

	playlistReq := httptest.NewRequest(http.MethodGet, "/api/stream/hls/video.flv", nil)
	rewritten := string(rewriteHLSPlaylist(playlistReq, []byte(playlist), cacheKey, cfg))
	lines := strings.Split(rewritten, "\n")
	require.Len(t, lines, 6)
	assert.Equal(t, "/api/stream/hls-segment/"+cacheKey+"/seg_00000.ts", lines[3])
}

func TestSignedHLSURLForVideoFile(t *testing.T) {
	cfg := &configs.Config{
		Security: configs.Security{
			APIKey:              "secret",
			SignedURLTTLSeconds: 3600,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/video-files/%E6%8A%96%E9%9F%B3/%E4%B8%BB%E6%92%AD?_key=secret", nil)
	assert.NotEmpty(t, signedHLSURLForVideoFile(req, ".flv", "抖音/主播/video.flv", cfg))
	assert.Empty(t, signedHLSURLForVideoFile(req, ".mp4", "抖音/主播/video.mp4", cfg))
}
