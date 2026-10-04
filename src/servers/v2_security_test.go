package servers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
)

func TestV2SessionOwnershipAndAPIKeyRevocation(t *testing.T) {
	h, _, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	require.Nil(t, metadata.GetStore())
	require.NoError(t, metadata.Init(t.TempDir()))
	t.Cleanup(func() { metadata.Close() })
	user, key, err := metadata.GetStore().CreateAPIKeyUser(context.Background(), "甲")
	require.NoError(t, err)
	_, otherKey, err := metadata.GetStore().CreateAPIKeyUser(context.Background(), "乙")
	require.NoError(t, err)
	res := v2Request(t, h, "POST", "/api/v2/playback-sessions", key, map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion, "capabilities": v2Caps()})
	require.Equal(t, 201, res.Code, res.Body.String())
	session := v2ReadSession(t, res)
	require.Equal(t, 403, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, otherKey, nil).Code)
	require.Equal(t, 403, v2Request(t, h, "DELETE", "/api/v2/playback-sessions/"+session.ID, otherKey, nil).Code)
	require.Equal(t, 200, v2Request(t, h, "HEAD", session.URL, "", nil).Code)
	enabled := false
	_, err = metadata.GetStore().UpdateAPIKeyUser(context.Background(), user.ID, nil, &enabled)
	require.NoError(t, err)
	require.Equal(t, 403, v2Request(t, h, "GET", session.URL, "", nil).Code)
	require.Equal(t, 401, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, key, nil).Code)
	defaultSession := v2Create(t, h, cfg, entry, v2Caps())
	cfg.Security.APIKey = "rotated-test-key"
	require.Equal(t, 403, v2Request(t, h, "GET", defaultSession.URL, "", nil).Code)
}
func TestV2SessionLimitHasConsistentRetryAndExpiredSlotsRecover(t *testing.T) {
	h, s, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	now := time.Now()
	s.now = func() time.Time { return now }
	body := map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion, "capabilities": v2Caps()}
	for i := 0; i < 32; i++ {
		res := v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, body)
		require.Equal(t, 201, res.Code, res.Body.String())
	}
	res := v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, body)
	require.Equal(t, 429, res.Code, res.Body.String())
	require.Equal(t, "2", res.Header().Get("Retry-After"))
	v2AssertResponse(t, res, "ErrorEnvelope")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	require.EqualValues(t, 2, payload["error"].(map[string]any)["retry_after_seconds"])
	now = now.Add(16 * time.Minute)
	require.Equal(t, 201, v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, body).Code)
}
func TestV2RejectsInvalidCapabilityBodyAndActiveRecording(t *testing.T) {
	h, _, cfg, _, entry, source, _ := v2Fixture(t, "mp4")
	caps := v2Caps()
	caps.Video[0].MaxBitDepth = 0
	require.Equal(t, 400, v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion, "capabilities": caps}).Code)
	res := v2Request(t, h, "GET", "/api/v2/recordings?limit=101", cfg.Security.APIKey, nil)
	require.Equal(t, 400, res.Code)
	end := recordingcatalog.BeginRecording(source)
	defer end()
	res = v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion, "capabilities": v2Caps()})
	require.Equal(t, 409, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), "recording_active")
}
