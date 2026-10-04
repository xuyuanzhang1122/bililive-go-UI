package servers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWatchIdentityRenameDedupAndReplacement(t *testing.T) {
	_, _, cfg, cat, entry, source, ctx := v2Fixture(t, "mp4")
	require.NoError(t, metadata.Init(t.TempDir()))
	t.Cleanup(func() { require.NoError(t, metadata.Close()) })
	oldPath := "平台/主播/clip.mp4"
	newPath := "平台/主播/renamed.mp4"
	require.NoError(t, os.Rename(source, filepath.Join(cfg.OutPutPath, filepath.FromSlash(newPath))))
	require.NoError(t, cat.Reconcile(context.Background()))
	older := metadata.WatchHistoryEntry{VideoPath: oldPath, RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, PositionSeconds: 10, UpdatedAt: "2026-10-03 10:00:00"}
	newer := older
	newer.VideoPath = newPath
	newer.PositionSeconds = 20
	newer.UpdatedAt = "2026-10-04 10:00:00"
	entries, err := canonicalWatchHistory(ctx, []metadata.WatchHistoryEntry{newer, older}, cat)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, newPath, entries[0].VideoPath)
	require.Equal(t, float64(20), entries[0].PositionSeconds)
	store := metadata.GetStore()
	require.NoError(t, store.UpsertWatchHistoryForUser(ctx, "u", &older))
	saved, err := store.GetWatchHistoryForUser(ctx, "u")
	require.NoError(t, err)
	require.Equal(t, entry.RecordingID, saved[0].RecordingID)
	// 请求旧别名仍定位同一录播；替换来源后不能沿用旧进度。
	recording, err := cat.RecordingForPath(ctx, oldPath)
	require.NoError(t, err)
	require.Equal(t, entry.RecordingID, recording.RecordingID)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.OutPutPath, filepath.FromSlash(newPath)), []byte("replacement"), 0644))
	require.NoError(t, cat.Reconcile(ctx))
	entries, err = canonicalWatchHistory(ctx, saved, cat)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestWatchWriteRequiresCapturedSourceIdentity(t *testing.T) {
	_, _, _, _, entry, _, ctx := v2Fixture(t, "mp4")
	require.NoError(t, metadata.Init(t.TempDir()))
	t.Cleanup(func() { require.NoError(t, metadata.Close()) })
	request := func(body map[string]any) int {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/history", bytes.NewReader(data)).WithContext(ctx)
		upsertWatchHistory(rec, req)
		return rec.Code
	}
	body := map[string]any{"video_path": entry.Path, "position_seconds": 30, "duration_seconds": 100}
	require.Equal(t, http.StatusConflict, request(body))
	body["recording_id"] = entry.RecordingID
	body["source_version"] = "stale"
	require.Equal(t, http.StatusConflict, request(body))
	body["source_version"] = entry.SourceVersion
	require.Equal(t, http.StatusOK, request(body))
	saved, err := metadata.GetStore().GetWatchHistoryForUser(ctx, "")
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, entry.SourceVersion, saved[0].SourceVersion)
}
