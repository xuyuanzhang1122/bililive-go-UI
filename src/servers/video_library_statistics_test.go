package servers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func libraryStatisticsConfig(t *testing.T) *configs.Config {
	t.Helper()
	cfg := &configs.Config{OutPutPath: t.TempDir()}
	previous := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	t.Cleanup(func() { configs.SetCurrentConfig(previous) })
	return cfg
}

func statisticsVideo(t *testing.T, path string, size int64) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
}

func requestLibraryStatistics(t *testing.T) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	store, err := recordingcatalog.Open(filepath.Join(t.TempDir(), "catalog.db"), configs.GetCurrentConfig().OutPutPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.Reconcile(context.Background()))
	return requestLibraryStatisticsFromStore(t, store)
}
func requestLibraryStatisticsFromStore(t *testing.T, store *recordingcatalog.Store) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/video-library", nil)
	inst := &instance.Instance{RecordingCatalog: recordingcatalog.NewManager(store, 0)}
	request = request.WithContext(context.WithValue(request.Context(), instance.Key, inst))
	response := httptest.NewRecorder()
	getVideoLibrary(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var rooms []map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &rooms))
	return response, rooms
}

func TestVideoLibraryNeverScansInRequest(t *testing.T) {
	cfg := libraryStatisticsConfig(t)
	statisticsVideo(t, filepath.Join(cfg.OutPutPath, "抖音", "主播", "clip.mp4"), 1000)
	store, err := recordingcatalog.Open(filepath.Join(t.TempDir(), "catalog.db"), cfg.OutPutPath)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Reconcile(context.Background()))
	require.NoError(t, os.Rename(cfg.OutPutPath, cfg.OutPutPath+"-offline"))
	t.Cleanup(func() { _ = os.Rename(cfg.OutPutPath+"-offline", cfg.OutPutPath) })
	_, rooms := requestLibraryStatisticsFromStore(t, store)
	require.Equal(t, float64(1000), rooms[0]["total_size"])
}

func TestVideoLibraryWithoutCatalogDoesNotGuess(t *testing.T) {
	libraryStatisticsConfig(t)
	request := httptest.NewRequest(http.MethodGet, "/api/video-library", nil)
	request = request.WithContext(context.WithValue(request.Context(), instance.Key, &instance.Instance{}))
	response := httptest.NewRecorder()
	getVideoLibrary(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}

func TestCatalogAliasResolverKeepsOldEntryAfterRename(t *testing.T) {
	cfg := libraryStatisticsConfig(t)
	oldPath := filepath.Join(cfg.OutPutPath, "抖音", "主播", "clip.mp4")
	statisticsVideo(t, oldPath, 1000)
	store, err := recordingcatalog.Open(filepath.Join(t.TempDir(), "catalog.db"), cfg.OutPutPath)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Reconcile(context.Background()))
	require.NoError(t, os.Rename(oldPath, filepath.Join(filepath.Dir(oldPath), "renamed.mp4")))
	require.NoError(t, store.Reconcile(context.Background()))
	request := httptest.NewRequest(http.MethodGet, "/api/playback/resolve/old", nil)
	request = mux.SetURLVars(request, map[string]string{"path": "抖音/主播/clip.mp4"})
	request = request.WithContext(context.WithValue(request.Context(), instance.Key, &instance.Instance{RecordingCatalog: recordingcatalog.NewManager(store, 0)}))
	response := httptest.NewRecorder()
	resolvePlayback(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, "ready", body.Data.Status)
	require.Contains(t, body.Data.URL, "renamed.mp4")
}

func TestVideoLibraryStatisticsValidatedSizeAndDisplay(t *testing.T) {
	cfg := libraryStatisticsConfig(t)
	folder := filepath.Join(cfg.OutPutPath, "抖音", "主播")
	statisticsVideo(t, filepath.Join(folder, "large.flv"), 188978561024)
	statisticsVideo(t, filepath.Join(folder, "nested", "small.MP4"), 1000)
	statisticsVideo(t, filepath.Join(folder, ".converting-temp.mp4"), 999999)
	statisticsVideo(t, filepath.Join(folder, ".cache", "cache.ts"), 999999)
	statisticsVideo(t, filepath.Join(folder, "notes.txt"), 999999)
	response, rooms := requestLibraryStatistics(t)
	require.Len(t, rooms, 1)
	require.Equal(t, float64(188978562024), rooms[0]["total_size"])
	require.Equal(t, float64(2), rooms[0]["video_count"])
	require.Equal(t, "189.0 GB", rooms[0]["total_size_text"])
	require.Equal(t, "verified", rooms[0]["statistics_status"])
	require.Greater(t, rooms[0]["statistics_checked_at"], float64(0))
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func TestVideoLibraryStatisticsRechecksGrowthAndDeletion(t *testing.T) {
	cfg := libraryStatisticsConfig(t)
	path := filepath.Join(cfg.OutPutPath, "抖音", "主播", "live.flv")
	statisticsVideo(t, path, 1000)
	_, rooms := requestLibraryStatistics(t)
	require.Equal(t, "1.0 kB", rooms[0]["total_size_text"])
	statisticsVideo(t, path, 2000000000)
	_, rooms = requestLibraryStatistics(t)
	require.Equal(t, float64(2000000000), rooms[0]["total_size"])
	require.Equal(t, "2.0 GB", rooms[0]["total_size_text"])
	require.NoError(t, os.Remove(path))
	_, rooms = requestLibraryStatistics(t)
	require.Empty(t, rooms)
}

func TestVideoLibraryStatisticsExcludesSymlinkAndNonVideo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 符号链接需要额外权限")
	}
	cfg := libraryStatisticsConfig(t)
	folder := filepath.Join(cfg.OutPutPath, "抖音", "主播")
	statisticsVideo(t, filepath.Join(folder, "real.mp4"), 1000)
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	statisticsVideo(t, outside, 5000)
	require.NoError(t, os.Symlink(outside, filepath.Join(folder, "link.mp4")))
	_, rooms := requestLibraryStatistics(t)
	require.Equal(t, float64(1), rooms[0]["video_count"])
	require.Equal(t, float64(1000), rooms[0]["total_size"])
}

func TestVideoLibraryStatisticsCannotClaimVerifiedOnReadFailure(t *testing.T) {
	cfg := libraryStatisticsConfig(t)
	folder := filepath.Join(cfg.OutPutPath, "抖音", "主播")
	statisticsVideo(t, filepath.Join(folder, "readable.mp4"), 1000)
	locked := filepath.Join(folder, "locked")
	statisticsVideo(t, filepath.Join(locked, "hidden.mp4"), 2000)
	store, err := recordingcatalog.Open(filepath.Join(t.TempDir(), "catalog.db"), cfg.OutPutPath)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Reconcile(context.Background()))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("当前运行身份不受目录权限限制")
	}
	require.Error(t, store.Reconcile(context.Background()))
	_, rooms := requestLibraryStatisticsFromStore(t, store)
	require.Len(t, rooms, 1)
	require.Equal(t, "unavailable", rooms[0]["statistics_status"])
	require.Equal(t, "统计暂不可用", rooms[0]["total_size_text"])
	require.Equal(t, float64(0), rooms[0]["total_size"], "不能把部分求和冒充完整统计")
}
