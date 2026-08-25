package servers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bililive-go/bililive-go/src/configs"
)

func TestResolvePlaybackReturnsDirectFileForMP4(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("fixture"), 0o644))
	previous := configs.GetCurrentConfig()
	configs.SetCurrentConfig(&configs.Config{OutPutPath: root})
	defer configs.SetCurrentConfig(previous)

	req := httptest.NewRequest(http.MethodGet, "/api/playback/resolve/clip.mp4", nil)
	req = mux.SetURLVars(req, map[string]string{"path": "clip.mp4"})
	res := httptest.NewRecorder()

	resolvePlayback(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var response commonResp
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ready", data["status"])
	assert.Equal(t, "file", data["protocol"])
	assert.Equal(t, "video/mp4", data["mime_type"])
	assert.True(t, strings.HasSuffix(data["url"].(string), "/files/clip.mp4"))
}

func TestMimeTypeForVideoExtension(t *testing.T) {
	assert.Equal(t, "video/mp4", mimeTypeForVideoExtension(".MP4"))
	assert.Equal(t, "video/quicktime", mimeTypeForVideoExtension(".mov"))
	assert.Equal(t, "video/webm", mimeTypeForVideoExtension(".webm"))
	assert.Equal(t, "application/octet-stream", mimeTypeForVideoExtension(".flv"))
}

func TestPlaybackURLExpiresAtReadsSignedQuery(t *testing.T) {
	assert.Equal(t, int64(12345), playbackURLExpiresAt("/files/clip.mp4?expires=12345&signature=redacted"))
	assert.Zero(t, playbackURLExpiresAt("/files/clip.mp4"))
}
