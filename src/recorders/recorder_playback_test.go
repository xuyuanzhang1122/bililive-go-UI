package recorders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/live"
	livemock "github.com/bililive-go/bililive-go/src/live/mock"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/bililive-go/bililive-go/src/pkg/events"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/bililive-go/bililive-go/src/types"
	"github.com/bluele/gcache"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// 平台接口使用本地直播源，下载器、录制完成入队和准备阶段均执行真实代码。
func TestRecordingEndAutomaticallyPreparesPlayback(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 FFmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("未安装 FFprobe")
	}
	for _, downloader := range []configs.DownloaderType{configs.DownloaderFFmpeg, configs.DownloaderNative} {
		t.Run(string(downloader), func(t *testing.T) {
			root := t.TempDir()
			sample := filepath.Join(root, "直播样片.flv")
			output, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=30", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "2", "-c:v", "libx264", "-c:a", "aac", "-flvflags", "no_duration_filesize", sample).CombinedOutput()
			require.NoError(t, err, string(output))
			data, err := os.ReadFile(sample)
			require.NoError(t, err)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
			defer upstream.Close()
			stream, err := url.Parse(upstream.URL + "/stream.flv")
			require.NoError(t, err)
			cfg := configs.NewConfig()
			cfg.OutPutPath = filepath.Join(root, "录像")
			cfg.AppDataPath = filepath.Join(root, "数据")
			cfg.FfmpegPath = ffmpeg
			cfg.Feature.DownloaderType = downloader
			cfg.OnRecordFinished = configs.OnRecordFinished{PreparePlayback: true}
			previous := configs.GetCurrentConfig()
			configs.SetCurrentConfig(cfg)
			defer configs.SetCurrentConfig(previous)
			catalogStore, err := recordingcatalog.Open(filepath.Join(root, "catalog.db"), cfg.OutPutPath)
			require.NoError(t, err)
			catalog := recordingcatalog.NewManager(catalogStore, 0)
			defer catalog.Close()
			store, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
			require.NoError(t, err)
			inst := &instance.Instance{RecordingCatalog: catalog, Cache: gcache.New(10).Build(), EventDispatcher: events.NewDispatcher(context.Background())}
			ctx := context.WithValue(context.Background(), instance.Key, inst)
			catalog.Start(ctx)
			manager := pipeline.NewManager(ctx, store, &pipeline.ManagerConfig{MaxConcurrent: 1, PollInterval: 10 * time.Millisecond}, nil)
			inst.PipelineManager = manager
			stages.RegisterBuiltinStagesToManager(manager)
			require.NoError(t, manager.Start(ctx))
			defer manager.Close(ctx)
			room := livemock.NewMockLive(gomock.NewController(t))
			room.EXPECT().GetRawUrl().Return(upstream.URL).AnyTimes()
			room.EXPECT().GetStreamInfos().Return([]*live.StreamUrlInfo{{Url: stream}}, nil).AnyTimes()
			room.EXPECT().GetPlatformCNName().Return("测试平台").AnyTimes()
			room.EXPECT().GetOptions().Return(&live.Options{}).AnyTimes()
			room.EXPECT().GetLiveId().Return(types.LiveID("test-auto-playback")).AnyTimes()
			room.EXPECT().GetLogger().Return(livelogger.New(0, nil)).AnyTimes()
			require.NoError(t, inst.Cache.Set(room, &live.Info{Live: room, HostName: "测试主播", RoomName: "自动准备"}))
			rec, err := NewRecorder(ctx, room)
			require.NoError(t, err)
			rec.(*recorder).tryRecord(ctx)
			var completed *pipeline.PipelineTask
			require.Eventually(t, func() bool {
				tasks, e := manager.ListTasks(pipeline.TaskFilter{})
				if e != nil || len(tasks) != 1 {
					return false
				}
				completed = tasks[0]
				return completed.Status == pipeline.PipelineStatusCompleted || completed.Status == pipeline.PipelineStatusFailed
			}, 15*time.Second, 20*time.Millisecond)
			require.Equal(t, pipeline.PipelineStatusCompleted, completed.Status, completed.ErrorMessage)
			require.Len(t, completed.InitialFiles, 1)
			file := completed.InitialFiles[0]
			require.FileExists(t, file.Path)
			entry, err := catalog.PreparationEntry(ctx, file.Path, file.PreparationSourceVersion)
			require.NoError(t, err)
			job, err := store.PreparationStatus(ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: stages.PreparationProfileVersion})
			require.NoError(t, err)
			require.Equal(t, pipeline.PreparationReady, job.Status)
			require.NoError(t, catalog.VerifyPreparedAsset(ctx, entry, job.OutputPath, job.OutputVersion, stages.PreparationProfileVersion))
		})
	}
}
