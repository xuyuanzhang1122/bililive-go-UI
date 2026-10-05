package stages

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
)

// 模拟录制进程被中止：不写容器时长，并在最后一个视频包中途结束。
// 使用真实 FFmpeg/FFprobe 验证录后自动队列，避免仅用探测桩掩盖恢复问题。
func TestStoppedFLVAutoPreparationProducesVerifiedPlayback(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	flv := strings.TrimSuffix(input.Path, filepath.Ext(input.Path)) + ".flv"
	output, err := exec.Command(ctx.FFmpegPath, "-v", "error", "-i", input.Path, "-c", "copy", "-flvflags", "no_duration_filesize", "-y", flv).CombinedOutput()
	require.NoError(t, err, string(output))
	data, err := os.ReadFile(flv)
	require.NoError(t, err)
	lastVideo, lastSize := 0, 0
	for offset := int(binary.BigEndian.Uint32(data[5:9])) + 4; offset+11 <= len(data); {
		size := int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		if offset+11+size+4 > len(data) {
			break
		}
		if data[offset] == 9 && size > 16 {
			lastVideo, lastSize = offset, size
		}
		offset += 11 + size + 4
	}
	require.Positive(t, lastVideo)
	data = data[:lastVideo+11+lastSize/2]
	require.NoError(t, os.WriteFile(flv, data, 0600))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	version, err := recordingcatalog.SnapshotVersion(flv)
	require.NoError(t, err)
	media, err := probePreparationSource(ctx.Ctx, ctx.FFmpegPath, flv)
	require.NoError(t, err)
	require.True(t, media.recoveryRequired)
	managerCtx := context.WithValue(ctx.Ctx, instance.Key, &instance.Instance{RecordingCatalog: ctx.Catalog})
	manager := pipeline.NewManager(managerCtx, store, &pipeline.ManagerConfig{MaxConcurrent: 1, PollInterval: 10 * time.Millisecond}, nil)
	RegisterBuiltinStagesToManager(manager)
	require.NoError(t, manager.Start(managerCtx))
	defer manager.Close(managerCtx)
	task := pipeline.NewPipelineTask(pipeline.RecordInfo{}, pipeline.GetEffectivePipelineConfig(&configs.OnRecordFinished{PreparePlayback: true, FixFlvAtFirst: true}), []pipeline.FileInfo{pipeline.NewVideoFileInfo(flv)})
	require.NoError(t, manager.EnqueueTask(task))
	var completed *pipeline.PipelineTask
	require.Eventually(t, func() bool {
		var e error
		completed, e = manager.GetTask(task.ID)
		return e == nil && (completed.Status == pipeline.PipelineStatusCompleted || completed.Status == pipeline.PipelineStatusFailed)
	}, 15*time.Second, 20*time.Millisecond)
	require.Equal(t, pipeline.PipelineStatusCompleted, completed.Status, completed.ErrorMessage)
	after, err := os.ReadFile(flv)
	require.NoError(t, err)
	require.Equal(t, data, after, "自动准备不得改写或删除原始录像")
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, flv, version)
	require.NoError(t, err)
	job, err := store.PreparationStatus(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: version, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationReady, job.Status)
	require.NoError(t, ctx.Catalog.VerifyPreparedAsset(ctx.Ctx, entry, job.OutputPath, job.OutputVersion, PreparationProfileVersion))
	prepared, err := probePreparation(ctx.Ctx, ctx.FFmpegPath, job.OutputPath)
	require.NoError(t, err)
	require.InDelta(t, media.duration, prepared.duration, preparationDurationTolerance(media.duration))
	require.Equal(t, "copy", prepared.mode())
	require.NoError(t, checkMP4Faststart(job.OutputPath))
}

func preparationFLVProbeFixture(t *testing.T, videoEnd, audioEnd float64, warning, packetOutput string) (*pipeline.PipelineContext, pipeline.FileInfo, *pipeline.SQLiteStore) {
	t.Helper()
	ctx, input, store, root := preparationFixture(t, "libx264", "aac")
	flv := strings.TrimSuffix(input.Path, filepath.Ext(input.Path)) + ".flv"
	output, err := exec.Command(ctx.FFmpegPath, "-v", "error", "-i", input.Path, "-c:v", "copy", "-c:a", "copy", "-y", flv).CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	input = pipeline.NewVideoFileInfo(flv)
	input.PreparationSourceVersion, err = recordingcatalog.SnapshotVersion(flv)
	require.NoError(t, err)
	ffprobe, err := exec.LookPath("ffprobe")
	require.NoError(t, err)
	dir := filepath.Join(root, "tools")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.Symlink(ctx.FFmpegPath, filepath.Join(dir, "ffmpeg")))
	if packetOutput == "" {
		packetOutput = fmt.Sprintf("stream_index=0|pts_time=0.000|dts_time=0.000|duration_time=0.100\nstream_index=1|pts_time=0.000|dts_time=0.000|duration_time=0.023\nstream_index=0|pts_time=%.3f|dts_time=%.3f|duration_time=0.100\nstream_index=1|pts_time=%.3f|dts_time=%.3f|duration_time=0.023\n", videoEnd, videoEnd, audioEnd, audioEnd)
	}
	packetPath := filepath.Join(dir, "packets.txt")
	require.NoError(t, os.WriteFile(packetPath, []byte(packetOutput), 0600))
	metadata := `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"High","level":10,"pix_fmt":"yuv420p","width":64,"height":64,"avg_frame_rate":"10/1","nb_read_packets":"2"},{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":1,"sample_rate":"44100","nb_read_packets":"2"}],"format":{"duration":"0.000000"}}`
	metadataPath := filepath.Join(dir, "metadata.json")
	require.NoError(t, os.WriteFile(metadataPath, []byte(metadata), 0600))
	script := fmt.Sprintf("#!/bin/sh\nlast=\nfor arg do last=$arg; done\ncase \"$last\" in\n*.flv) case \" $* \" in\n  *-show_packets*) cat %q;;\n  *) cat %q; printf '%%s\\n' %q >&2;;\n  esac;;\n*) exec %q \"$@\";;\nesac\n", packetPath, metadataPath, warning, ffprobe)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ffprobe"), []byte(script), 0700))
	ctx.FFmpegPath = filepath.Join(dir, "ffmpeg")
	return ctx, input, store
}

func TestPreparationTimelineWithRealFFprobe(t *testing.T) {
	_, input, _ := preparationFLVProbeFixture(t, 1, 1, "", "")
	ffprobe, err := exec.LookPath("ffprobe")
	require.NoError(t, err)
	timeline, warnings, err := probePreparationTimeline(context.Background(), ffprobe, input.Path, 0, 1, true)
	require.NoError(t, err, warnings)
	require.Greater(t, timeline.video.count, int64(5))
	require.Greater(t, timeline.audio.count, int64(5))
	require.InDelta(t, 1, timeline.video.end-timeline.video.start, 0.25)
}

func TestPreparationFLVSourceRecoveryPolicy(t *testing.T) {
	ctx, input, _ := preparationFLVProbeFixture(t, 1, 1, "Invalid NAL unit size", "")
	media, err := probePreparationSource(ctx.Ctx, ctx.FFmpegPath, input.Path)
	require.NoError(t, err)
	require.True(t, media.recoveryRequired)
	require.Equal(t, "recovery-transcode", media.mode())
	require.InDelta(t, 1.1, media.duration, 0.001)
	require.EqualValues(t, 2, media.timelinePackets)
	require.Contains(t, media.probeWarnings, "Invalid NAL unit size")
	args := preparationArgs(input.Path, "output.mp4", media.mode(), media)
	require.Contains(t, args, "+discardcorrupt")
	require.Contains(t, args, "libx264")
	require.Contains(t, args, "aac")

	_, err = probePreparation(ctx.Ctx, ctx.FFmpegPath, input.Path)
	require.ErrorContains(t, err, "准备媒体读取错误")

	mp4 := filepath.Join(t.TempDir(), "output.mp4")
	require.NoError(t, os.WriteFile(mp4, []byte("not a media file"), 0600))
	_, err = probePreparationSource(ctx.Ctx, ctx.FFmpegPath, mp4)
	require.Error(t, err, "非 FLV 不得进入来源恢复策略")
}

func TestPreparationFLVRejectsInvalidTimeline(t *testing.T) {
	for _, tc := range []struct {
		name, packets, errorContains string
	}{
		{"no-timestamps", "stream_index=0|pts_time=N/A|dts_time=N/A\n", "时间戳"},
		{"large-jump", "stream_index=0|pts_time=0|dts_time=0\nstream_index=0|pts_time=61|dts_time=61\n", "间隔"},
		{"audio-missing", "stream_index=0|pts_time=0|dts_time=0\nstream_index=0|pts_time=1|dts_time=1\n", "音视频"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, input, _ := preparationFLVProbeFixture(t, 1, 1, "warning", tc.packets)
			_, err := probePreparationSource(ctx.Ctx, ctx.FFmpegPath, input.Path)
			require.ErrorContains(t, err, tc.errorContains)
		})
	}
}

func TestPreparationRecoveryRejectsShortOutputWithoutPublishing(t *testing.T) {
	ctx, input, store := preparationFLVProbeFixture(t, 30, 30, "Invalid NAL unit size", "")
	before, err := recordingcatalog.SnapshotVersion(input.Path)
	require.NoError(t, err)
	stage := &PreparePlaybackStage{}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.ErrorContains(t, err, "准备产物编码、轨道或时长不符")
	after, err := recordingcatalog.SnapshotVersion(input.Path)
	require.NoError(t, err)
	require.Equal(t, before, after, "来源 FLV 不得被修改")
	entry, err := ctx.Catalog.PreparationEntry(context.Background(), input.Path, before)
	require.NoError(t, err)
	job, err := store.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	require.NotEqual(t, pipeline.PreparationReady, job.Status)
	assetDir := filepath.Join(ctx.Catalog.AssetRoot(), entry.RecordingID, entry.SourceVersion+"-"+PreparationProfileVersion)
	require.NoFileExists(t, filepath.Join(assetDir, "compat.mp4"))
}

func TestPreparationRecoveryAcceptsValidatedOutput(t *testing.T) {
	ctx, input, store := preparationFLVProbeFixture(t, 1, 1, "Invalid NAL unit size", "")
	before, err := recordingcatalog.SnapshotVersion(input.Path)
	require.NoError(t, err)
	stage := &PreparePlaybackStage{}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	require.Contains(t, stage.GetLogs(), "recovery-transcode")
	after, err := recordingcatalog.SnapshotVersion(input.Path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, before)
	require.NoError(t, err)
	job, err := store.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationReady, job.Status)
	require.NoError(t, ctx.Catalog.VerifyPreparedAsset(ctx.Ctx, entry, job.OutputPath, job.OutputVersion, PreparationProfileVersion))
	require.NoError(t, checkMP4Faststart(job.OutputPath))
	_, err = probePreparation(ctx.Ctx, ctx.FFmpegPath, job.OutputPath)
	require.NoError(t, err)
}
