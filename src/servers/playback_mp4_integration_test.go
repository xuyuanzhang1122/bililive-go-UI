package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

// 真实媒体→后处理→持久化观看记录→旧路径resolve→签名GET→本地反向代理Range。
func TestPreparedMP4RealPipelineSignedRangeAndHistory(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 FFmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("未安装 ffprobe")
	}
	cfg := &configs.Config{OutPutPath: t.TempDir(), AppDataPath: t.TempDir(), Security: configs.Security{EnableAPIKey: true, APIKey: "integration-key", SignedURLTTLSeconds: 3600}}
	old := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	defer configs.SetCurrentConfig(old)
	require.Nil(t, metadata.GetStore(), "测试需要独立元数据实例")
	require.NoError(t, metadata.Init(t.TempDir()))
	defer metadata.Close()
	history := &metadata.WatchHistoryEntry{VideoPath: "录播 clip.flv", VideoName: "录播 clip.flv", PositionSeconds: 0.5, DurationSeconds: 1}
	require.NoError(t, metadata.GetStore().UpsertWatchHistory(context.Background(), history))
	source := filepath.Join(cfg.OutPutPath, history.VideoPath)
	data, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "1", "-c:v", "libx264", "-c:a", "aac", source).CombinedOutput()
	require.NoError(t, err, string(data))
	stage, err := stages.NewConvertMp4Stage(pipeline.StageConfig{Options: map[string]any{pipeline.OptionDeleteSource: true}})
	require.NoError(t, err)
	output, err := stage.Execute(&pipeline.PipelineContext{Ctx: context.Background(), FFmpegPath: ffmpeg, Logger: livelogger.New(10, nil)}, []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	require.NoError(t, err)
	require.NotEmpty(t, output)
	require.NoFileExists(t, source)
	req := mux.SetURLVars(httptest.NewRequest("GET", "/resolve?_key=integration-key", nil), map[string]string{"path": history.VideoPath})
	res := httptest.NewRecorder()
	resolvePlayback(res, req)
	var response struct {
		Data playbackResolveResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, "file", response.Data.Protocol)
	require.Contains(t, response.Data.URL, "sig=")
	saved, err := metadata.GetStore().GetWatchHistoryItemForUser(context.Background(), metadata.DefaultAPIKeyUserID, history.VideoPath)
	require.NoError(t, err)
	require.Equal(t, history.PositionSeconds, saved.PositionSeconds)
	origin := httptest.NewServer(CORSMiddleware(fileAccessMiddleware(http.StripPrefix("/files/", http.FileServer(http.Dir(cfg.OutPutPath))))))
	defer origin.Close()
	originURL, err := url.Parse(origin.URL)
	require.NoError(t, err)
	proxy := httptest.NewServer(httputil.NewSingleHostReverseProxy(originURL))
	defer proxy.Close()

	for _, rawURL := range []string{joinEscapedURLPath("/files", filepath.Base(output[0].Path)), response.Data.URL + "invalid"} {
		received, err := proxy.Client().Get(proxy.URL + rawURL)
		require.NoError(t, err)
		received.Body.Close()
		require.Equal(t, http.StatusUnauthorized, received.StatusCode)
	}
	payload, err := os.ReadFile(output[0].Path)
	require.NoError(t, err)
	for _, start := range []int{0, len(payload) / 2, len(payload) - 64} {
		request, err := http.NewRequest("GET", proxy.URL+response.Data.URL, nil)
		require.NoError(t, err)
		end := start + 31
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		received, err := proxy.Client().Do(request)
		require.NoError(t, err)
		b, err := io.ReadAll(received.Body)
		received.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusPartialContent, received.StatusCode, string(b))
		require.Equal(t, fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)), received.Header.Get("Content-Range"))
		require.Equal(t, payload[start:end+1], b)
	}
}
