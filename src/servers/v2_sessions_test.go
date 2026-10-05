package servers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/bililive-go/bililive-go/src/pkg/playbackdecision"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func v2Caps() playbackdecision.Capabilities {
	return playbackdecision.Capabilities{Containers: []string{"mp4", "hls"}, Protocols: []string{"file", "hls"}, Video: []playbackdecision.VideoCapability{{Codec: "h264", Profiles: []string{"baseline", "main", "high"}, MaxLevel: 42, MaxWidth: 1920, MaxHeight: 1080, MaxFPS: 60, MaxBitDepth: 8}}, Audio: []playbackdecision.AudioCapability{{Codec: "aac", Profiles: []string{"lc"}, MaxChannels: 2, MaxSampleRate: 48000}}}
}
func v2Fixture(t *testing.T, ext string) (*mux.Router, *v2Service, *configs.Config, *recordingcatalog.Manager, recordingcatalog.Entry, string, context.Context) {
	return v2CodecFixture(t, ext, "libx264", "aac")
}
func v2CodecFixture(t *testing.T, ext, video, audio string) (*mux.Router, *v2Service, *configs.Config, *recordingcatalog.Manager, recordingcatalog.Entry, string, context.Context) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 FFmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("未安装 ffprobe")
	}
	cfg := configs.NewConfig()
	cfg.OutPutPath = t.TempDir()
	cfg.AppDataPath = t.TempDir()
	cfg.Security.EnableAPIKey = true
	cfg.Security.APIKey = "v2-test-key"
	old := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	t.Cleanup(func() { configs.SetCurrentConfig(old) })
	source := filepath.Join(cfg.OutPutPath, "平台", "主播", "clip."+ext)
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0755))
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "1", "-c:v", video, "-pix_fmt", "yuv420p", "-c:a", audio}
	if video == "libx264" {
		args = append(args, "-profile:v", "main", "-level:v", "3.1")
	}
	args = append(args, source)
	out, err := exec.Command(ffmpeg, args...).CombinedOutput()
	require.NoError(t, err, string(out))
	store, err := recordingcatalog.Open(filepath.Join(cfg.AppDataPath, "catalog.db"), cfg.OutPutPath)
	require.NoError(t, err)
	cat := recordingcatalog.NewManager(store, 0)
	t.Cleanup(func() { require.NoError(t, cat.Close()) })
	require.NoError(t, cat.Reconcile(context.Background()))
	version, err := recordingcatalog.SnapshotVersion(source)
	require.NoError(t, err)
	entry, err := cat.PreparationEntry(context.Background(), source, version)
	require.NoError(t, err)
	inst := &instance.Instance{RecordingCatalog: cat}
	ctx := context.WithValue(context.Background(), instance.Key, inst)
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), instance.Key, inst)))
		})
	})
	service := registerV2Routes(router)
	return router, service, cfg, cat, entry, source, ctx
}
func v2Request(t *testing.T, h http.Handler, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}
func v2ReadSession(t *testing.T, res *httptest.ResponseRecorder) v2SessionDTO {
	t.Helper()
	var e struct {
		Data      v2SessionDTO `json:"data"`
		Error     *v2Error     `json:"error"`
		RequestID string       `json:"request_id"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &e))
	require.Nil(t, e.Error, res.Body.String())
	require.NotEmpty(t, e.RequestID)
	return e.Data
}
func v2Create(t *testing.T, h http.Handler, cfg *configs.Config, e recordingcatalog.Entry, caps playbackdecision.Capabilities) v2SessionDTO {
	t.Helper()
	res := v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, map[string]any{"recording_id": e.RecordingID, "source_version": e.SourceVersion, "capabilities": caps})
	require.Equal(t, 201, res.Code, res.Body.String())
	return v2ReadSession(t, res)
}
func TestV2SessionBoundTokenRangeRefreshCancelAndExpiry(t *testing.T) {
	h, s, cfg, _, e, source, _ := v2Fixture(t, "mp4")
	now := time.Now()
	s.now = func() time.Time { return now }
	session := v2Create(t, h, cfg, e, v2Caps())
	require.Equal(t, "ready", session.Status)
	require.Equal(t, "direct", session.Decision)
	require.NotContains(t, session.URL, cfg.Security.APIKey)
	require.Contains(t, session.URL, "play_token=")
	req := httptest.NewRequest("GET", session.URL, nil)
	req.Header.Set("Range", "bytes=0-31")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	require.Equal(t, 206, res.Code, res.Body.String())
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, data[:32], res.Body.Bytes())
	bad := session.URL + "tamper"
	require.Equal(t, 403, v2Request(t, h, "GET", bad, "", nil).Code)
	other := v2Create(t, h, cfg, e, v2Caps())
	require.Equal(t, 403, v2Request(t, h, "GET", strings.Replace(session.URL, session.ID, other.ID, 1), "", nil).Code)
	require.Equal(t, 401, v2Request(t, h, "GET", "/api/v2/recordings?_key="+cfg.Security.APIKey, "", nil).Code)
	sameOrigin := httptest.NewRequest("GET", "/api/v2/recordings", nil)
	sameOrigin.Header.Set("Sec-Fetch-Site", "same-origin")
	r := httptest.NewRecorder()
	h.ServeHTTP(r, sameOrigin)
	require.Equal(t, 401, r.Code)
	now = now.Add(301 * time.Second)
	require.Equal(t, 410, v2Request(t, h, "GET", session.URL, "", nil).Code)
	refreshed := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "ready", refreshed.Status)
	require.Greater(t, refreshed.TokenExpiresAt, session.TokenExpiresAt)
	require.Equal(t, 200, v2Request(t, h, "GET", refreshed.URL, "", nil).Code)
	cancelled := v2ReadSession(t, v2Request(t, h, "DELETE", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "cancelled", cancelled.Status)
	require.Empty(t, cancelled.URL)
	require.Equal(t, 410, v2Request(t, h, "GET", refreshed.URL, "", nil).Code)
	now = now.Add(601 * time.Second)
	require.Equal(t, 410, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+other.ID, cfg.Security.APIKey, nil).Code)
}
func TestV2SessionSourceReplacementAndChangedVersionRejected(t *testing.T) {
	h, _, cfg, cat, e, source, _ := v2Fixture(t, "mp4")
	session := v2Create(t, h, cfg, e, v2Caps())
	body := map[string]any{"recording_id": e.RecordingID, "source_version": strings.Repeat("b", 64), "capabilities": v2Caps()}
	require.Equal(t, 409, v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, body).Code)
	require.NoError(t, os.WriteFile(source, []byte("replacement"), 0600))
	require.Equal(t, 409, v2Request(t, h, "GET", session.URL, "", nil).Code)
	current := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "failed", current.Status)
	require.Empty(t, current.URL)
	require.NoError(t, cat.Reconcile(context.Background()))
	current = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "source_changed", current.Error.Code)
}
func TestV2SessionMissingCapabilityAndExistingPreparationState(t *testing.T) {
	h, _, cfg, cat, e, source, ctx := v2Fixture(t, "flv")
	session := v2Create(t, h, cfg, e, v2Caps())
	require.Equal(t, "failed", session.Status)
	require.Equal(t, "remux", session.Decision)
	require.Equal(t, "asset_not_ready", session.Error.Code)
	require.True(t, session.Error.Retryable)
	require.Empty(t, session.URL)
	store, err := pipeline.NewSQLiteStore(filepath.Join(cfg.AppDataPath, "tasks.db"))
	require.NoError(t, err)
	defer store.Close()
	pm := pipeline.NewManager(ctx, store, nil, nil)
	defer pm.Close(context.Background())
	instance.GetInstance(ctx).PipelineManager = pm
	// 排队任务尚未进入阶段执行时，也必须让已打开的播放器继续等待。
	queued := pipeline.NewPipelineTask(pipeline.RecordInfo{}, &pipeline.PipelineConfig{Stages: []pipeline.StageConfig{{Name: pipeline.StageNamePreparePlayback}}}, []pipeline.FileInfo{{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: e.SourceVersion}})
	require.NoError(t, store.CreateTask(ctx, queued))
	queuedSession := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "processing", queuedSession.Status, "准备阶段还未开始时不能显示未准备并终止网页轮询")
	require.Empty(t, queuedSession.URL)
	queued.MarkCancelled()
	require.NoError(t, store.UpdateTask(ctx, queued))
	queuedSession = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "asset_not_ready", queuedSession.Error.Code, "取消的排队任务不能冒称仍在准备")
	key := pipeline.PreparationKey{RecordingID: e.RecordingID, SourceVersion: e.SourceVersion, ProfileVersion: stages.PreparationProfileVersion}
	_, err = store.QueuePreparation(ctx, key)
	require.NoError(t, err)
	waiting := v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil)
	current := v2ReadSession(t, waiting)
	require.Equal(t, "processing", current.Status)
	require.Equal(t, "2", waiting.Header().Get("Retry-After"))
	require.Equal(t, 2, current.RetryAfter)
	require.NoError(t, store.ClaimPreparation(ctx, key))
	pausedReason := "转换期间空间不足 /private/server/media，源文件已保留"
	require.NoError(t, store.FinishPreparation(ctx, key, pipeline.PreparationPaused, "", pausedReason))
	current = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "paused", current.Status)
	require.True(t, current.Error.Retryable)
	require.NotContains(t, current.Error.Message, "/private/server")
	require.Contains(t, current.Error.Message, "暂停")
	require.NoError(t, store.ClaimPreparation(ctx, key))
	failedReason := "准备媒体读取错误: [flv @ 0x5a199e91c580] Invalid NAL unit size /private/server/media"
	require.NoError(t, store.FinishPreparation(ctx, key, pipeline.PreparationFailed, "", failedReason))
	current = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "failed", current.Status)
	require.Equal(t, "preparation_failed", current.Error.Code)
	require.NotContains(t, current.Error.Message, "0x5a199e91c580")
	require.NotContains(t, current.Error.Message, "Invalid NAL")
	require.NotContains(t, current.Error.Message, "/private/server")
	require.Contains(t, current.Error.Message, "重试")
	stored, err := store.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, failedReason, stored.Reason, "任务诊断仍须保留，便于定位失败原因")
	queued.Status = pipeline.PipelineStatusPending
	require.NoError(t, store.UpdateTask(ctx, queued))
	current = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "processing", current.Status, "失败任务重试入队后，不能继续返回上次失败并终止轮询")
	stored, err = store.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationFailed, stored.Status, "会话查询不得改写阶段状态")
	queued.MarkCancelled()
	require.NoError(t, store.UpdateTask(ctx, queued))
	stage, err := stages.NewPreparePlaybackStage(pipeline.StageConfig{})
	require.NoError(t, err)
	_, err = stage.Execute(&pipeline.PipelineContext{Ctx: ctx, Catalog: cat, Preparations: store}, []pipeline.FileInfo{{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: e.SourceVersion}})
	require.NoError(t, err)
	current = v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "ready", current.Status)
	require.Equal(t, "remux", current.Decision)
	require.Equal(t, "file", current.Protocol)
	unsupported := v2Caps()
	unsupported.Video[0].Codec = "hevc"
	unsupported.Video[0].Profiles = []string{"main"}
	bad := v2Create(t, h, cfg, e, unsupported)
	require.Equal(t, "failed", bad.Status)
	require.Equal(t, "no_matching_capabilities", bad.Error.Code)
	require.False(t, bad.Error.Retryable)
}
func TestV2PersistentHLSPlaylistUsesSessionTokenForEverySegment(t *testing.T) {
	h, _, cfg, cat, e, source, ctx := v2Fixture(t, "flv")
	store, err := pipeline.NewSQLiteStore(filepath.Join(cfg.AppDataPath, "tasks.db"))
	require.NoError(t, err)
	defer store.Close()
	pc := &pipeline.PipelineContext{Ctx: ctx, Catalog: cat, Preparations: store}
	file := pipeline.FileInfo{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: e.SourceVersion}
	prep, err := stages.NewPreparePlaybackStage(pipeline.StageConfig{})
	require.NoError(t, err)
	_, err = prep.Execute(pc, []pipeline.FileInfo{file})
	require.NoError(t, err)
	stage, err := stages.NewPrepareHLSStage(pipeline.StageConfig{Options: map[string]any{"profiles": []string{"source"}}})
	require.NoError(t, err)
	_, err = stage.Execute(pc, []pipeline.FileInfo{file})
	require.NoError(t, err)
	caps := v2Caps()
	caps.Protocols = []string{"hls"}
	caps.Containers = []string{"hls"}
	session := v2Create(t, h, cfg, e, caps)
	require.Equal(t, "ready", session.Status)
	require.Equal(t, "hls", session.Protocol)
	h.HandleFunc("/api/playback/resolve/{path:.*}", resolvePlayback).Methods("GET")
	legacy := v2Request(t, h, "GET", "/api/playback/resolve/legacy.flv?session_id="+session.ID, cfg.Security.APIKey, nil)
	var legacyData struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(legacy.Body.Bytes(), &legacyData))
	require.Equal(t, "application/vnd.apple.mpegurl", legacyData.Data.MimeType)

	playlist := v2Request(t, h, "GET", session.URL, "", nil)
	require.Equal(t, 200, playlist.Code, playlist.Body.String())
	require.Contains(t, playlist.Body.String(), "#EXT-X-ENDLIST")
	for _, line := range strings.Split(playlist.Body.String(), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		require.NoError(t, err)
		require.Contains(t, u.Path, session.ID)
		require.NotEmpty(t, u.Query().Get("play_token"))
		require.NotContains(t, line, "sig=")
		require.Equal(t, 200, v2Request(t, h, "GET", line, "", nil).Code)
	}
}

func TestV2SessionFollowsVerifiedSourceRenameWithoutChangingAsset(t *testing.T) {
	h, _, cfg, cat, e, source, _ := v2Fixture(t, "mp4")
	session := v2Create(t, h, cfg, e, v2Caps())
	renamed := filepath.Join(filepath.Dir(source), "renamed.mp4")
	require.NoError(t, os.Rename(source, renamed))
	require.NoError(t, os.WriteFile(source, []byte("unrelated replacement"), 0600))
	require.NoError(t, cat.Reconcile(context.Background()))
	refreshed := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
	require.Equal(t, "ready", refreshed.Status)
	require.Equal(t, session.AssetVersion, refreshed.AssetVersion)
	require.Equal(t, 200, v2Request(t, h, "GET", session.URL, "", nil).Code)
}
func TestV2LegacySessionAdapterPreservesExistingEnvelopeAndPolling(t *testing.T) {
	h, _, cfg, _, e, _, _ := v2Fixture(t, "mp4")
	h.HandleFunc("/api/playback/resolve/{path:.*}", resolvePlayback).Methods("GET")
	session := v2Create(t, h, cfg, e, v2Caps())
	res := v2Request(t, h, "GET", "/api/playback/resolve/legacy.mp4?session_id="+session.ID, cfg.Security.APIKey, nil)
	require.Equal(t, 200, res.Code, res.Body.String())
	var result struct {
		ErrNo int                     `json:"err_no"`
		Data  playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Zero(t, result.ErrNo)
	require.Equal(t, "ready", result.Data.Status)
	require.Contains(t, result.Data.URL, "play_token=")
	v2Request(t, h, "DELETE", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil)
	res = v2Request(t, h, "GET", "/api/playback/resolve/legacy.mp4?session_id="+session.ID, cfg.Security.APIKey, nil)
	result.Data = playbackResolveResponse{}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	require.Equal(t, "failed", result.Data.Status)
	require.Empty(t, result.Data.URL)
}

func TestV2LegacySessionErrorsKeepLegacyEnvelope(t *testing.T) {
	h, s, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	h.HandleFunc("/api/playback/resolve/{path:.*}", resolvePlayback).Methods("GET")
	now := time.Now()
	s.now = func() time.Time { return now }
	session := v2Create(t, h, cfg, entry, v2Caps())
	path := "/api/playback/resolve/old.mp4?session_id=" + session.ID
	now = now.Add(16 * time.Minute)
	res := v2Request(t, h, "GET", path, cfg.Security.APIKey, nil)
	require.Equal(t, 410, res.Code, res.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	require.EqualValues(t, 410, payload["err_no"])
	require.Nil(t, payload["data"])
	res = v2Request(t, h, "GET", path, "", nil)
	require.Equal(t, 401, res.Code)
	payload = make(map[string]any)
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	require.EqualValues(t, 401, payload["err_no"])
}

func TestV2SessionUsesRealAudioAndVideoTranscodeAssets(t *testing.T) {
	cases := []struct{ name, video, audio, want string }{{"audio", "libx264", "libmp3lame", "audio-transcode"}, {"video", "mpeg4", "aac", "video-transcode"}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h, _, cfg, cat, entry, source, ctx := v2CodecFixture(t, "mkv", tt.video, tt.audio)
			session := v2Create(t, h, cfg, entry, v2Caps())
			require.Equal(t, "failed", session.Status)
			require.Equal(t, tt.want, session.Decision)
			require.Equal(t, "asset_not_ready", session.Error.Code)
			store, err := pipeline.NewSQLiteStore(filepath.Join(cfg.AppDataPath, "tasks.db"))
			require.NoError(t, err)
			defer store.Close()
			stage, err := stages.NewPreparePlaybackStage(pipeline.StageConfig{})
			require.NoError(t, err)
			_, err = stage.Execute(&pipeline.PipelineContext{Ctx: ctx, Catalog: cat, Preparations: store}, []pipeline.FileInfo{{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: entry.SourceVersion}})
			require.NoError(t, err)
			ready := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
			require.Equal(t, "ready", ready.Status)
			require.Equal(t, tt.want, ready.Decision)
			require.FileExists(t, source)
			require.Equal(t, 200, v2Request(t, h, "GET", ready.URL, "", nil).Code)
			require.NoError(t, cat.Reconcile(context.Background()))
			after := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
			require.Equal(t, "ready", after.Status)
			require.Equal(t, ready.AssetVersion, after.AssetVersion)
		})
	}
}
func TestV2SessionExpiringDuringResponseCannotReturnReadyURL(t *testing.T) {
	h, s, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	now := time.Now()
	s.now = func() time.Time { return now }
	session := v2Create(t, h, cfg, entry, v2Caps())
	calls := 0
	s.now = func() time.Time {
		calls++
		if calls == 1 {
			return now
		}
		return now.Add(16 * time.Minute)
	}
	res := v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil)
	require.Equal(t, 410, res.Code, res.Body.String())
	require.NotContains(t, res.Body.String(), "play_token=")
}
