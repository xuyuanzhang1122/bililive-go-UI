package servers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func operationAsset(t *testing.T, keepSource bool) (source, target string) {
	t.Helper()
	cfg := &configs.Config{OutPutPath: t.TempDir()}
	previous := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	t.Cleanup(func() { configs.SetCurrentConfig(previous) })
	source = filepath.Join(cfg.OutPutPath, "clip.flv")
	target = filepath.Join(cfg.OutPutPath, "clip.mp4")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	require.NoError(t, os.WriteFile(target, []byte("prepared"), 0600))
	info, err := os.Stat(source)
	require.NoError(t, err)
	require.NoError(t, playbackasset.Save(source, target, info))
	if !keepSource {
		require.NoError(t, os.Remove(source))
	}
	return
}
func TestPreparedAssetDeleteOriginalAlias(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			_, target := operationAsset(t, false)
			body := []byte(`{"paths":["clip.flv"]}`)
			req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/file", bytes.NewReader(body)), map[string]string{"path": "clip.flv"})
			res := httptest.NewRecorder()
			if batch {
				batchDeleteFiles(res, req)
			} else {
				deleteFile(res, req)
			}
			_, err := os.Stat(target)
			require.True(t, os.IsNotExist(err), "别名删除应删除对应产物：%s", res.Body.String())
		})
	}
}
func TestPreparedAssetRenameOriginalAlias(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			_, target := operationAsset(t, false)
			body := []byte(`{"new_name":"renamed","paths":["clip.flv"],"find":"clip","replace":"renamed"}`)
			req := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/file", bytes.NewReader(body)), map[string]string{"path": "clip.flv"})
			res := httptest.NewRecorder()
			if batch {
				batchRenameFiles(res, req)
			} else {
				renameFile(res, req)
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(target), "renamed.mp4"))
			require.NoError(t, err, res.Body.String())
			require.Equal(t, "prepared", string(data))
		})
	}
}
func TestPreparedAssetDeletePreservesOtherVersion(t *testing.T) {
	source, target := operationAsset(t, true)
	req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/file", nil), map[string]string{"path": "clip.mp4"})
	deleteFile(httptest.NewRecorder(), req)
	_, err := os.Stat(source)
	require.NoError(t, err, "删除产物不能额外删除源文件")
	_, err = os.Stat(target)
	require.True(t, os.IsNotExist(err))
}

func TestPreparedAssetDeleteSourceDetachesIdentity(t *testing.T) {
	source, target := operationAsset(t, true)
	req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/file", nil), map[string]string{"path": "clip.flv"})
	deleteFile(httptest.NewRecorder(), req)
	_, err := os.Stat(source)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(target)
	require.NoError(t, err, "只删除所请求的源文件")
	_, _, ok := preparedPlayback(configs.GetCurrentConfig(), "clip.flv")
	require.False(t, ok, "删除原身份后应保留独立MP4，不继续显示旧身份")
}
func TestPreparedAssetRenameSourceDetachesIdentity(t *testing.T) {
	_, target := operationAsset(t, true)
	req := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/file", bytes.NewBufferString(`{"new_name":"renamed"}`)), map[string]string{"path": "clip.flv"})
	renameFile(httptest.NewRecorder(), req)
	_, err := os.Stat(filepath.Join(filepath.Dir(target), "renamed.flv"))
	require.NoError(t, err)
	_, err = os.Stat(target)
	require.NoError(t, err, "重命名源文件不能删除产物")
	_, _, ok := preparedPlayback(configs.GetCurrentConfig(), "clip.flv")
	require.False(t, ok, "重命名应解除旧身份")
}
