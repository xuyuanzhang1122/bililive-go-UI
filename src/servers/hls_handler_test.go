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
	hlsURL := signedHLSURLForVideoFile(req, ".flv", "抖音/主播/video.flv", cfg)
	require.NotEmpty(t, hlsURL)
	// 鸿蒙 AVPlayer 依赖 .m3u8 后缀识别 HLS 播放列表。
	assert.True(t, strings.HasSuffix(strings.SplitN(hlsURL, "?", 2)[0], "/playlist.m3u8"))
	signedReq := httptest.NewRequest(http.MethodGet, hlsURL, nil)
	assert.NoError(t, securitypkg.ValidateSignedRequest(signedReq, "secret", time.Now()))
	assert.Empty(t, signedHLSURLForVideoFile(req, ".mp4", "抖音/主播/video.mp4", cfg))
}

func TestTrimHLSPlaylistSuffix(t *testing.T) {
	assert.Equal(t, "抖音/主播/video.flv", trimHLSPlaylistSuffix("抖音/主播/video.flv/playlist.m3u8"))
	assert.Equal(t, "抖音/主播/video.flv", trimHLSPlaylistSuffix("抖音/主播/video.flv/index.m3u8"))
	assert.Equal(t, "抖音/主播/video.flv", trimHLSPlaylistSuffix("抖音/主播/video.flv"))
	assert.Equal(t, "抖音/主播/video.flv", trimHLSPlaylistSuffix("抖音/主播/video.flv/"))
	assert.Equal(t, "", trimHLSPlaylistSuffix(""))
	assert.Equal(t, "playlist.m3u8", trimHLSPlaylistSuffix("playlist.m3u8"))
}
