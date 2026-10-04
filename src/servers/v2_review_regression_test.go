package servers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/stretchr/testify/require"
)

func TestV2LateCancelledRefreshCannotOverwriteReadyAsset(t *testing.T) {
	for _, protocol := range []string{"file", "hls"} {
		for _, legacy := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "-v2", true: "-legacy"}[legacy], func(t *testing.T) {
				h, s, cfg, cat, entry, source, ctx := v2Fixture(t, "flv")
				h.HandleFunc("/api/playback/resolve/{path:.*}", resolvePlayback).Methods("GET")
				caps := v2Caps()
				caps.Protocols = []string{protocol}
				if protocol == "hls" {
					caps.Containers = []string{"hls"}
				}
				session := v2Create(t, h, cfg, entry, caps)
				require.Equal(t, "failed", session.Status)
				store, err := pipeline.NewSQLiteStore(filepath.Join(cfg.AppDataPath, "tasks.db"))
				require.NoError(t, err)
				defer store.Close()
				pc := &pipeline.PipelineContext{Ctx: ctx, Catalog: cat, Preparations: store}
				files := []pipeline.FileInfo{{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: entry.SourceVersion}}
				prep, err := stages.NewPreparePlaybackStage(pipeline.StageConfig{})
				require.NoError(t, err)
				_, err = prep.Execute(pc, files)
				require.NoError(t, err)
				if protocol == "hls" {
					stage, err := stages.NewPrepareHLSStage(pipeline.StageConfig{Options: map[string]any{"profiles": []string{"source"}}})
					require.NoError(t, err)
					_, err = stage.Execute(pc, files)
					require.NoError(t, err)
				}
				entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var calls atomic.Int32
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				now := time.Now()
				s.now = func() time.Time {
					if calls.Add(1) == 1 {
						close(entered)
						<-release
					}
					return now
				}
				path := "/api/v2/playback-sessions/" + session.ID
				if legacy {
					path = "/api/playback/resolve/old.flv?session_id=" + session.ID
				}
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("Authorization", "Bearer "+cfg.Security.APIKey)
				requestCtx, cancel := context.WithCancel(req.Context())
				defer cancel()
				go func() { defer close(done); h.ServeHTTP(httptest.NewRecorder(), req.WithContext(requestCtx)) }()
				<-entered
				newer := v2ReadSession(t, v2Request(t, h, "GET", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil))
				require.Equal(t, "ready", newer.Status)
				require.Equal(t, protocol, newer.Protocol)
				cancel()
				unblock()
				<-done
				s.mu.Lock()
				saved := s.sessions[session.ID]
				s.mu.Unlock()
				require.Equal(t, "ready", saved.DTO.Status)
				require.Equal(t, newer.AssetVersion, saved.DTO.AssetVersion)
				require.Equal(t, http.StatusOK, v2Request(t, h, "GET", newer.URL, "", nil).Code)
			})
		}
	}
}

func TestV2MOVCapabilityUsesActualContainer(t *testing.T) {
	h, _, cfg, _, entry, _, _ := v2Fixture(t, "mov")
	caps := v2Caps()
	caps.Containers = []string{"mov"}
	caps.Protocols = []string{"file"}
	session := v2Create(t, h, cfg, entry, caps)
	require.Equal(t, "ready", session.Status)
	require.Equal(t, "direct", session.Decision)
}

func TestV2LegacyFileMIMEMatchesSelectedMOV(t *testing.T) {
	h, _, cfg, _, entry, _, _ := v2Fixture(t, "mov")
	h.HandleFunc("/api/playback/resolve/{path:.*}", resolvePlayback).Methods("GET")
	caps := v2Caps()
	caps.Containers = []string{"mov"}
	session := v2Create(t, h, cfg, entry, caps)
	require.Equal(t, "ready", session.Status)
	response := v2Request(t, h, "GET", "/api/playback/resolve/unused.mp4?session_id="+session.ID, cfg.Security.APIKey, nil)
	require.Contains(t, response.Body.String(), "video/quicktime")
	require.Equal(t, "video/quicktime", v2Request(t, h, "GET", session.URL, "", nil).Header().Get("Content-Type"))
}

func TestV2CancelledSessionsReleasePlaybackQuota(t *testing.T) {
	h, _, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	var oldURL string
	for i := 0; i < 33; i++ {
		session := v2Create(t, h, cfg, entry, v2Caps())
		if i == 0 {
			oldURL = session.URL
		}
		response := v2Request(t, h, "DELETE", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "cancelled", v2ReadSession(t, response).Status)
	}
	require.Equal(t, http.StatusGone, v2Request(t, h, "GET", oldURL, "", nil).Code)
}

func TestV2ProbeIdentifiesWebMDocTypeAndRenamedMOVBrand(t *testing.T) {
	for _, ext := range []string{"webm", "mkv"} {
		t.Run(ext, func(t *testing.T) {
			_, _, cfg, _, _, source, ctx := v2CodecFixture(t, ext, "libvpx-vp9", "libopus")
			media, err := v2Probe(httptest.NewRequest("GET", "/", nil).WithContext(ctx), cfg, source, "file", "")
			require.NoError(t, err)
			require.Equal(t, ext, media.Container)
		})
	}
	h, _, cfg, cat, entry, source, _ := v2Fixture(t, "mov")
	require.NoError(t, os.Rename(source, source+".mp4"))
	require.NoError(t, cat.Reconcile(context.Background()))
	caps := v2Caps()
	caps.Containers = []string{"mov"}
	require.Equal(t, "ready", v2Create(t, h, cfg, entry, caps).Status)
	caps.Containers = []string{"mp4"}
	wrong := v2Create(t, h, cfg, entry, caps)
	require.Equal(t, "failed", wrong.Status)
	require.Equal(t, "remux", wrong.Decision)
}

func TestV2OpenedHandleCannotServeReplacementAfterPathRestored(t *testing.T) {
	_, _, _, cat, entry, path, ctx := v2Fixture(t, "mp4")
	files, err := cat.PlaybackFiles(ctx, entry.RecordingID, entry.SourceVersion)
	require.NoError(t, err)
	selected, err := cat.RebindPlaybackFile(ctx, files[0])
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, path+".original"))
	require.NoError(t, os.WriteFile(path, []byte("unrelated replacement file"), 0600))
	before, err := os.Lstat(path)
	require.NoError(t, err)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, os.Rename(path, path+".replacement"))
	require.NoError(t, os.Rename(path+".original", path))
	require.Error(t, v2VerifyOpenedPlaybackFile(ctx, cat, selected, before, f))
}
