package servers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func persistentHLSFixture(t *testing.T) (*configs.Config, *recordingcatalog.Manager, recordingcatalog.Entry, hlsasset.Manifest, string) {
	t.Helper()
	cfg := configs.NewConfig()
	cfg.OutPutPath = filepath.Join(t.TempDir(), "recordings")
	cfg.AppDataPath = t.TempDir()
	source := filepath.Join(cfg.OutPutPath, "平台", "主播", "clip.flv")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0755))
	require.NoError(t, os.WriteFile(source, []byte("original source"), 0600))
	store, err := recordingcatalog.Open(filepath.Join(cfg.AppDataPath, "catalog.db"), cfg.OutPutPath, filepath.Join(cfg.AppDataPath, "media-assets"))
	require.NoError(t, err)
	cat := recordingcatalog.NewManager(store, 0)
	t.Cleanup(func() { require.NoError(t, cat.Close()) })
	require.NoError(t, cat.Reconcile(context.Background()))
	version, err := recordingcatalog.SnapshotVersion(source)
	require.NoError(t, err)
	entry, err := cat.PreparationEntry(context.Background(), source, version)
	require.NoError(t, err)
	root := filepath.Join(cat.AssetRoot(), entry.RecordingID, version+"-hls-source-v1")
	require.NoError(t, os.MkdirAll(root, 0755))
	data, err := os.MkdirTemp(root, ".hls-build-")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(data, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nseg_00000.ts\n#EXT-X-ENDLIST\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(data, "seg_00000.ts"), []byte("persistent segment payload"), 0600))
	manifest, err := hlsasset.Seal(data, hlsasset.Manifest{Version: 1, RecordingID: entry.RecordingID, SourcePath: source, SourceVersion: version, Profile: "source", Duration: 1})
	require.NoError(t, err)
	require.NoError(t, hlsasset.Publish(root, manifest, nil))
	proofs := make(map[string]string)
	for _, file := range manifest.Files {
		path := manifest.FilePath(root, file.Name)
		proofs[path], err = recordingcatalog.SnapshotVersion(path)
		require.NoError(t, err)
	}
	require.NoError(t, cat.RegisterHLS(context.Background(), entry, "hls-source-v1", manifest.Digest(), proofs))
	return cfg, cat, entry, manifest, root
}

func TestPersistentHLSHTTPDoesNotEnterJITAndSurvivesJanitor(t *testing.T) {
	cfg, cat, entry, manifest, root := persistentHLSFixture(t)
	cfg.Security.EnableAPIKey = true
	cfg.Security.APIKey = "fixture-secret"
	oldCfg, oldManager := configs.GetCurrentConfig(), globalHLSJobManager
	configs.SetCurrentConfig(cfg)
	globalHLSJobManager = &hlsJobManager{}
	t.Cleanup(func() {
		globalHLSJobManager.Close()
		globalHLSJobManager = oldManager
		configs.SetCurrentConfig(oldCfg)
	})
	ctx := context.WithValue(context.Background(), instance.Key, &instance.Instance{RecordingCatalog: cat})
	req := mux.SetURLVars(httptest.NewRequest("GET", "/api/stream/hls/平台/主播/clip.flv/playlist.m3u8", nil).WithContext(ctx), map[string]string{"path": "平台/主播/clip.flv/playlist.m3u8"})
	req.Header.Set("Authorization", "Bearer fixture-secret")
	res := httptest.NewRecorder()
	getHLSPlaylist(res, req)
	require.Equal(t, 200, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), "#EXT-X-ENDLIST")
	require.Contains(t, res.Body.String(), "/api/stream/hls-asset/")
	require.Empty(t, globalHLSJobManager.jobs)
	require.NoError(t, resetHLSCache(hlsCacheRoot(cfg)))
	_, err := runHLSCacheJanitor(hlsCacheRoot(cfg), 1, 0.000001, time.Now().Add(48*time.Hour))
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "index.m3u8"))
	require.FileExists(t, manifest.FilePath(root, "seg_00000.ts"))
	assetVersion := entry.SourceVersion + "-hls-source-v1"
	segmentURL := ""
	for _, line := range strings.Split(res.Body.String(), "\n") {
		if strings.HasPrefix(line, "/api/stream/hls-asset/") {
			segmentURL = line
			break
		}
	}
	require.NotEmpty(t, segmentURL)
	vars := map[string]string{"recording_id": entry.RecordingID, "asset_version": assetVersion, "file": "seg_00000.ts"}
	segReq := mux.SetURLVars(httptest.NewRequest("GET", segmentURL, nil).WithContext(ctx), vars)
	segReq.Header.Set("Range", "bytes=0-9")
	segment := httptest.NewRecorder()
	apiAuthMiddleware(http.HandlerFunc(getPersistentHLSFile)).ServeHTTP(segment, segReq)
	require.Equal(t, 206, segment.Code, segment.Body.String())
	require.Equal(t, "persistent", segment.Body.String())
	unsigned := httptest.NewRecorder()
	apiAuthMiddleware(http.HandlerFunc(getPersistentHLSFile)).ServeHTTP(unsigned, mux.SetURLVars(httptest.NewRequest("GET", strings.Split(segmentURL, "?")[0], nil).WithContext(ctx), vars))
	require.Equal(t, 401, unsigned.Code)
	require.NoError(t, os.WriteFile(manifest.FilePath(root, "seg_00000.ts"), []byte("tampered segment payload!!"), 0600))
	broken := httptest.NewRecorder()
	getPersistentHLSFile(broken, segReq)
	require.Equal(t, 503, broken.Code)
}

func TestProcessingRetryAfterMatchesBody(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.MaxConcurrent = 1
	m := &hlsJobManager{}
	oldCfg, old := configs.GetCurrentConfig(), globalHLSJobManager
	configs.SetCurrentConfig(cfg)
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); configs.SetCurrentConfig(oldCfg); globalHLSJobManager = old })
	first := lifecycleJob(t, m, cfg, "hold-retry", "hold")
	_, err := m.WaitPlayable(context.Background(), first)
	require.NoError(t, err)
	lifecycleJob(t, m, cfg, "queued-retry", "short")
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve", nil), map[string]string{"path": "queued-retry.flv"})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	var body struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "processing", body.Data.Status)
	require.Equal(t, strconv.Itoa(body.Data.RetryAfter), res.Header().Get("Retry-After"))
}

func TestQueueFullRetryAfterIsRateLimitedAndMatchesBody(t *testing.T) {
	cfg := lifecycleConfig(t)
	cfg.HLSCache.MaxConcurrent = 1
	cfg.HLSCache.MaxQueued = 1
	m := &hlsJobManager{}
	oldCfg, old := configs.GetCurrentConfig(), globalHLSJobManager
	configs.SetCurrentConfig(cfg)
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); configs.SetCurrentConfig(oldCfg); globalHLSJobManager = old })
	first := lifecycleJob(t, m, cfg, "limit-first", "hold")
	_, err := m.WaitPlayable(context.Background(), first)
	require.NoError(t, err)
	lifecycleJob(t, m, cfg, "limit-second", "short")
	lifecycleJob(t, m, cfg, "limit-third", "short")
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve", nil), map[string]string{"path": "limit-third.flv"})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	var body struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, 429, res.Code, res.Body.String())
	require.Equal(t, strconv.Itoa(body.Data.RetryAfter), res.Header().Get("Retry-After"))
}

func TestPersistentHLSResolverSelectsPreparedProfileWithoutJob(t *testing.T) {
	cfg, cat, _, _, _ := persistentHLSFixture(t)
	oldCfg, old := configs.GetCurrentConfig(), globalHLSJobManager
	configs.SetCurrentConfig(cfg)
	globalHLSJobManager = &hlsJobManager{}
	t.Cleanup(func() { globalHLSJobManager.Close(); globalHLSJobManager = old; configs.SetCurrentConfig(oldCfg) })
	ctx := context.WithValue(context.Background(), instance.Key, &instance.Instance{RecordingCatalog: cat})
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve?hls_profile=source", nil).WithContext(ctx), map[string]string{"path": "平台/主播/clip.flv"})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	var body struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "ready", body.Data.Status, res.Body.String())
	require.Contains(t, body.Data.URL, "/api/stream/hls-asset/")
	require.Empty(t, globalHLSJobManager.jobs)
}

func TestFutureJITSegmentWaitsThenReadsAndCancels(t *testing.T) {
	cfg := lifecycleConfig(t)
	oldCfg, old := configs.GetCurrentConfig(), globalHLSJobManager
	m := &hlsJobManager{}
	configs.SetCurrentConfig(cfg)
	globalHLSJobManager = m
	t.Cleanup(func() { m.Close(); configs.SetCurrentConfig(oldCfg); globalHLSJobManager = old })
	job := lifecycleJob(t, m, cfg, "future", "hold")
	_, err := m.WaitPlayable(context.Background(), job)
	require.NoError(t, err)
	req := mux.SetURLVars(httptest.NewRequest("GET", "/segment", nil), map[string]string{"cache_key": job.key, "segment": "seg_00001.ts"})
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(job.cacheDir, "seg_00001.ts"), []byte("future segment"), 0600)
	}()
	res := httptest.NewRecorder()
	getHLSSegment(res, req)
	require.Equal(t, 200, res.Code, res.Body.String())
	require.Equal(t, "future segment", res.Body.String())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req = mux.SetURLVars(httptest.NewRequest("GET", "/segment", nil).WithContext(ctx), map[string]string{"cache_key": job.key, "segment": "seg_00002.ts"})
	start := time.Now()
	getHLSSegment(httptest.NewRecorder(), req)
	require.Less(t, time.Since(start), time.Second)
}
