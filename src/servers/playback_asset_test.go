package servers

import (
	"encoding/json"
	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedMP4ResolvesOriginalPathAfterSourceRemoved(t *testing.T) {
	cfg := &configs.Config{OutPutPath: t.TempDir(), AppDataPath: t.TempDir()}
	old := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	defer configs.SetCurrentConfig(old)
	source := filepath.Join(cfg.OutPutPath, "clip.flv")
	target := filepath.Join(cfg.OutPutPath, "clip.mp4")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0644))
	require.NoError(t, os.WriteFile(target, []byte("validated mp4"), 0644))
	info, _ := os.Stat(source)
	require.NoError(t, playbackasset.Save(source, target, info))
	require.NoError(t, os.Remove(source))
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve", nil), map[string]string{"path": "clip.flv"})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	require.Equal(t, 200, res.Code, res.Body.String())
	require.Contains(t, res.Body.String(), `"protocol":"file"`)
	require.Contains(t, res.Body.String(), "clip.mp4")
}
func TestPreparedMP4ListKeepsOriginalIdentity(t *testing.T) {
	cfg := &configs.Config{OutPutPath: t.TempDir(), AppDataPath: t.TempDir()}
	old := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	defer configs.SetCurrentConfig(old)
	source := filepath.Join(cfg.OutPutPath, "clip.flv")
	target := filepath.Join(cfg.OutPutPath, "clip.mp4")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0644))
	require.NoError(t, os.WriteFile(target, []byte("prepared"), 0644))
	info, _ := os.Stat(source)
	require.NoError(t, playbackasset.Save(source, target, info))
	req := mux.SetURLVars(httptest.NewRequest("GET", "/videos", nil), map[string]string{"path": ""})
	res := httptest.NewRecorder()
	getVideoFiles(res, req)
	var files []VideoFileInfo
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &files))
	require.Len(t, files, 1)
	require.Equal(t, "clip.flv", files[0].RelPath)
	require.Empty(t, files[0].HLSURL)
	require.Contains(t, files[0].FileURL, "clip.mp4")
}
