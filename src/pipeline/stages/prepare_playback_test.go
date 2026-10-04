package stages

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
)

func preparationFixture(t *testing.T, video, audio string) (*pipeline.PipelineContext, pipeline.FileInfo, *pipeline.SQLiteStore, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 ffmpeg")
	}
	root := t.TempDir()
	source := filepath.Join(root, "recordings", "平台", "主播", "clip.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0755))
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "1", "-c:v", video, "-c:a", audio, source}
	data, err := exec.Command(ffmpeg, args...).CombinedOutput()
	require.NoError(t, err, string(data))
	catalog, err := recordingcatalog.Open(filepath.Join(root, "catalog.db"), filepath.Join(root, "recordings"))
	require.NoError(t, err)
	require.NoError(t, catalog.Reconcile(context.Background()))
	manager := recordingcatalog.NewManager(catalog, 0)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	store, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	input := pipeline.NewVideoFileInfo(source)
	input.PreparationSourceVersion, err = recordingcatalog.SnapshotVersion(source)
	require.NoError(t, err)
	ctx := mp4TestContext(ffmpeg)
	ctx.Catalog = manager
	ctx.Preparations = store
	return ctx, input, store, root
}

func TestPreparePlaybackRealProfilesAndReuse(t *testing.T) {
	for _, tc := range []struct {
		name, video, audio, mode string
		derived                  bool
	}{
		{"copy", "libx264", "aac", "copy", false},
		{"audio", "libx264", "pcm_s16le", "audio-transcode", true},
		{"video", "mpeg4", "aac", "video-transcode", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, input, store, _ := preparationFixture(t, tc.video, tc.audio)
			stage := &PreparePlaybackStage{}
			output, err := stage.Execute(ctx, []pipeline.FileInfo{input})
			require.NoError(t, err)
			require.FileExists(t, input.Path)
			require.Len(t, output, 1, "准备阶段给旧管道继续传递源文件")
			entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
			require.NoError(t, err)
			job, err := store.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
			require.NoError(t, err)
			require.Equal(t, pipeline.PreparationReady, job.Status)
			require.FileExists(t, job.OutputPath)
			require.NoError(t, checkMP4Faststart(job.OutputPath))
			_, err = inspectMP4Media(ctx.Ctx, ctx.FFmpegPath, job.OutputPath)
			require.NoError(t, err)
			require.Contains(t, stage.logs, tc.mode)
			require.Equal(t, tc.derived, filepath.Dir(job.OutputPath) != filepath.Dir(input.Path))
			if tc.mode != "video-transcode" {
				sourceHash, e := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "packet=data_hash", "-of", "csv=p=0", input.Path).Output()
				require.NoError(t, e)
				targetHash, e := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "packet=data_hash", "-of", "csv=p=0", job.OutputPath).Output()
				require.NoError(t, e)
				require.Equal(t, string(sourceHash), string(targetHash), "复制视频不得重新编码")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, job.OutputPath) }))
			defer server.Close()
			req, e := http.NewRequest(http.MethodGet, server.URL, nil)
			require.NoError(t, e)
			req.Header.Set("Range", "bytes=0-31")
			response, e := http.DefaultClient.Do(req)
			require.NoError(t, e)
			body, e := io.ReadAll(response.Body)
			require.NoError(t, e)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusPartialContent, response.StatusCode)
			require.Len(t, body, 32)
			before, err := os.Stat(job.OutputPath)
			require.NoError(t, err)
			_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
			require.NoError(t, err)
			after, err := os.Stat(job.OutputPath)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after))
		})
	}
}

func TestPrepareRecordingManagerPersistsSnapshotAndRestarts(t *testing.T) {
	ctx, input, store, root := preparationFixture(t, "libx264", "aac")
	managerCtx := context.WithValue(ctx.Ctx, instance.Key, &instance.Instance{RecordingCatalog: ctx.Catalog})
	manager := pipeline.NewManager(managerCtx, store, &pipeline.ManagerConfig{MaxConcurrent: 1, PollInterval: 10 * time.Millisecond}, nil)
	RegisterBuiltinStagesToManager(manager)
	require.NoError(t, manager.Start(managerCtx))
	task := pipeline.NewPipelineTask(pipeline.RecordInfo{}, pipeline.GetEffectivePipelineConfig(&configs.OnRecordFinished{PreparePlayback: true}), []pipeline.FileInfo{pipeline.NewVideoFileInfo(input.Path)})
	require.NoError(t, manager.EnqueueTask(task))
	var completed *pipeline.PipelineTask
	require.Eventually(t, func() bool {
		var err error
		completed, err = manager.GetTask(task.ID)
		return err == nil && (completed.Status == pipeline.PipelineStatusCompleted || completed.Status == pipeline.PipelineStatusFailed)
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, pipeline.PipelineStatusCompleted, completed.Status, completed.ErrorMessage)
	require.Equal(t, input.PreparationSourceVersion, completed.InitialFiles[0].PreparationSourceVersion)
	manager.Close(managerCtx)
	reopened, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
	require.NoError(t, err)
	defer reopened.Close()
	persisted, err := reopened.GetTask(managerCtx, task.ID)
	require.NoError(t, err)
	require.Equal(t, pipeline.PipelineStatusCompleted, persisted.Status)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	job, err := reopened.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationReady, job.Status)
}

func TestPrepareSourceChangesBeforePublicationAndReadyReplacement(t *testing.T) {
	t.Run("source-changed", func(t *testing.T) {
		ctx, input, _, _ := preparationFixture(t, "libx264", "aac")
		replacement := filepath.Join(filepath.Dir(input.Path), "replacement.mkv")
		bytes, err := os.ReadFile(input.Path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(replacement, bytes, 0600))
		stage := &PreparePlaybackStage{freeBytes: func(string) (uint64, error) {
			require.NoError(t, os.Rename(replacement, input.Path))
			return 1 << 40, nil
		}}
		_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
		require.Error(t, err)
		require.FileExists(t, input.Path)
		require.NoFileExists(t, filepath.Join(filepath.Dir(input.Path), "clip.mp4"))
	})
	t.Run("ready-target-changed", func(t *testing.T) {
		ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
		_, err := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
		require.NoError(t, err)
		entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
		require.NoError(t, err)
		key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
		job, err := store.QueuePreparation(ctx.Ctx, key)
		require.NoError(t, err)
		info, err := os.Stat(job.OutputPath)
		require.NoError(t, err)
		bytes, err := os.ReadFile(job.OutputPath)
		require.NoError(t, err)
		replacement := job.OutputPath + ".replacement"
		require.NoError(t, os.WriteFile(replacement, bytes, 0600))
		require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
		require.NoError(t, os.Rename(replacement, job.OutputPath))
		_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
		require.Error(t, err)
		job, err = store.QueuePreparation(ctx.Ctx, key)
		require.NoError(t, err)
		require.Equal(t, pipeline.PreparationFailed, job.Status)
		require.FileExists(t, input.Path)
	})
}

func TestPrepareAlreadyFaststartMP4RegistersMainAsset(t *testing.T) {
	ctx, input, store, root := preparationFixture(t, "libx264", "aac")
	path := filepath.Join(filepath.Dir(input.Path), "direct.mp4")
	data, err := exec.Command(ctx.FFmpegPath, "-v", "error", "-i", input.Path, "-c", "copy", "-movflags", "+faststart", path).CombinedOutput()
	require.NoError(t, err, string(data))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	input = pipeline.NewVideoFileInfo(path)
	input.PreparationSourceVersion, err = recordingcatalog.SnapshotVersion(path)
	require.NoError(t, err)
	_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, path, input.PreparationSourceVersion)
	require.NoError(t, err)
	job, err := store.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	require.Equal(t, path, job.OutputPath)
	db, err := sql.Open("sqlite", filepath.Join(root, "catalog.db"))
	require.NoError(t, err)
	defer db.Close()
	for i := 0; i < 2; i++ {
		var count int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM media_assets WHERE recording_id=? AND kind='main' AND status='verified'", entry.RecordingID).Scan(&count))
		require.Equal(t, 1, count, "直接可播文件也必须提交已验证主资产")
		require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	}
}

func TestPreparePlaybackProcessRecovery(t *testing.T) {
	if root := os.Getenv("BILIVE_T3_CRASH_ROOT"); root != "" {
		store, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
		require.NoError(t, err)
		task, err := store.GetTask(context.Background(), 1)
		require.NoError(t, err)
		catalog, err := recordingcatalog.Open(filepath.Join(root, "catalog.db"), filepath.Join(root, "recordings"))
		require.NoError(t, err)
		ffmpeg, err := exec.LookPath("ffmpeg")
		require.NoError(t, err)
		ctx := mp4TestContext(ffmpeg)
		ctx.Catalog = recordingcatalog.NewManager(catalog, 0)
		ctx.Preparations = store
		_, err = (&PreparePlaybackStage{}).Execute(ctx, task.InitialFiles)
		require.NoError(t, err)
		// 模拟资产与准备已提交，但外层管道还没提交完成标记时进程退出。
		os.Exit(72)
	}
	ctx, input, store, root := preparationFixture(t, "libx264", "pcm_s16le")
	task := pipeline.NewPipelineTask(pipeline.RecordInfo{}, &pipeline.PipelineConfig{Stages: []pipeline.StageConfig{{Name: pipeline.StageNamePreparePlayback}}}, []pipeline.FileInfo{input})
	task.Status = pipeline.PipelineStatusRunning
	require.NoError(t, store.CreateTask(ctx.Ctx, task))
	require.NoError(t, store.Close())
	command := exec.Command(os.Args[0], "-test.run=^TestPreparePlaybackProcessRecovery$")
	command.Env = append(os.Environ(), "BILIVE_T3_CRASH_ROOT="+root)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, string(output))
	require.Equal(t, 72, exit.ExitCode(), string(output))
	reopened, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
	require.NoError(t, err)
	defer reopened.Close()
	ctx.Preparations = reopened
	require.NoError(t, reopened.ResetRunningTasks(ctx.Ctx))
	persisted, err := reopened.GetTask(ctx.Ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, pipeline.PipelineStatusPending, persisted.Status)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	job, err := reopened.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationReady, job.Status)
	before, err := os.Stat(job.OutputPath)
	require.NoError(t, err)
	stage := &PreparePlaybackStage{}
	_, err = stage.Execute(ctx, persisted.InitialFiles)
	require.NoError(t, err)
	require.Contains(t, stage.logs, "复用")
	after, err := os.Stat(job.OutputPath)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	require.Empty(t, stage.commands)
	require.FileExists(t, input.Path)
}

func TestPreparationProfilesRejectUnsafeCopy(t *testing.T) {
	video := preparationStream{Type: "video", Codec: "h264", Profile: "High", Level: 41, Pixel: "yuv420p", Width: 1920, Height: 1080, FrameRate: "30/1"}
	audio := preparationStream{Type: "audio", Codec: "aac", Profile: "LC", Channels: 2, SampleRate: "48000"}
	for _, tc := range []struct {
		name   string
		mutate func(*preparationMedia)
		want   string
	}{
		{"baseline", func(m *preparationMedia) {}, "copy"},
		{"high10", func(m *preparationMedia) { m.video.Profile = "High 10"; m.video.Pixel = "yuv420p10le" }, "video-transcode"},
		{"hdr", func(m *preparationMedia) { m.video.Transfer = "smpte2084" }, "video-transcode"},
		{"high-level", func(m *preparationMedia) { m.video.Level = 60 }, "video-transcode"},
		{"4k", func(m *preparationMedia) { m.video.Width = 3840; m.video.Height = 2160 }, "video-transcode"},
		{"heaac", func(m *preparationMedia) { m.audioStream.Profile = "HE-AAC" }, "audio-transcode"},
		{"surround", func(m *preparationMedia) { m.audioStream.Channels = 6 }, "audio-transcode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := preparationMedia{video: video, audio: true, audioStream: audio}
			tc.mutate(&media)
			require.Equal(t, tc.want, media.mode())
			if tc.want == "audio-transcode" {
				require.Contains(t, strings.Join(preparationArgs("source", "target", tc.want, media), " "), "-c:v copy")
			}
		})
	}
}

func TestPrepareHDRToneMapOrKeepsSourceIfFilterUnavailable(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	path := filepath.Join(filepath.Dir(input.Path), "hdr.mp4")
	data, err := exec.Command(ctx.FFmpegPath, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-x264-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc", "-c:a", "aac", path).CombinedOutput()
	require.NoError(t, err, string(data))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	input = pipeline.NewVideoFileInfo(path)
	input.PreparationSourceVersion, err = recordingcatalog.SnapshotVersion(path)
	require.NoError(t, err)
	sourceProbe, probeErr := probePreparation(ctx.Ctx, ctx.FFmpegPath, path)
	require.NoError(t, probeErr)
	require.Equal(t, "smpte2084", sourceProbe.video.Transfer)
	filters, filterErr := exec.Command(ctx.FFmpegPath, "-hide_banner", "-filters").Output()
	require.NoError(t, filterErr)
	_, prepareErr := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, path, input.PreparationSourceVersion)
	require.NoError(t, err)
	job, err := store.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	if !strings.Contains(string(filters), "zscale") {
		require.Error(t, prepareErr)
		require.Contains(t, prepareErr.Error(), "zscale")
		require.Equal(t, pipeline.PreparationFailed, job.Status)
		require.Contains(t, job.Reason, "zscale")
		require.FileExists(t, path)
		return
	}
	require.NoError(t, prepareErr)
	media, err := probePreparation(ctx.Ctx, ctx.FFmpegPath, job.OutputPath)
	require.NoError(t, err)
	require.Equal(t, "copy", media.mode())
	require.Equal(t, "bt709", media.video.Transfer, "%+v", media)
	require.FileExists(t, path)
}

func TestPreparePlaybackPausesAndCancellationKeepsSource(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	stage := &PreparePlaybackStage{freeBytes: func(string) (uint64, error) { return 0, nil }}
	_, err := stage.Execute(ctx, []pipeline.FileInfo{input})
	var paused *pipeline.PreparationPausedError
	require.ErrorAs(t, err, &paused)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	job, err := store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationPaused, job.Status)
	release := recordingcatalog.BeginRecording(input.Path)
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.ErrorAs(t, err, &paused)
	release()
	cancelled, cancel := context.WithCancel(ctx.Ctx)
	cancel()
	ctx.Ctx = cancelled
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.True(t, errors.Is(err, context.Canceled))
	require.FileExists(t, input.Path)
	ctx.Ctx = context.Background()
	_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
}

func TestPrepareCancellationWhileSharedBudgetOccupied(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	for i := 0; i < 3; i++ {
		release, err := mediawork.Shared.Acquire(context.Background(), 3)
		require.NoError(t, err)
		defer release()
	}
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	cancelled, cancel := context.WithCancel(ctx.Ctx)
	ctx.Ctx = cancelled
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input}); done <- err }()
	require.Eventually(t, func() bool {
		job, err := store.QueuePreparation(context.Background(), key)
		return err == nil && job.Status == pipeline.PreparationRunning
	}, time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("预算等待未响应取消")
	}
	job, err := store.QueuePreparation(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationPaused, job.Status)
	require.FileExists(t, input.Path)
	require.NoFileExists(t, filepath.Join(filepath.Dir(input.Path), "clip.mp4"))
}
