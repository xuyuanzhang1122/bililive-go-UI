package stages

import (
	"context"
	"errors"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareHLSRealVODProfilesAndReuse(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	data, err := exec.Command(ctx.FFmpegPath, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "7", "-c:v", "libx264", "-g", "20", "-keyint_min", "20", "-sc_threshold", "0", "-c:a", "aac", "-y", input.Path).CombinedOutput()
	require.NoError(t, err, string(data))
	input.PreparationSourceVersion, err = recordingcatalog.SnapshotVersion(input.Path)
	require.NoError(t, err)
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	stage := &PrepareHLSStage{profiles: []string{"source", "720p", "1080p"}}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	for _, profile := range stage.profiles {
		k := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: hlsasset.ProfileVersion(profile)}
		job, err := store.QueuePreparation(ctx.Ctx, k)
		require.NoError(t, err)
		require.Equal(t, pipeline.PreparationReady, job.Status, job.Reason)
		manifest, err := hlsasset.Load(filepath.Dir(job.OutputPath))
		require.NoError(t, err)
		require.NoError(t, manifest.ValidateAll(filepath.Dir(job.OutputPath)))
		content, err := os.ReadFile(job.OutputPath)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(manifest.Files), 3, "档位 %s，清单 %s，命令 %v", profile, string(content), stage.GetCommands())
		require.Contains(t, string(content), "#EXT-X-PLAYLIST-TYPE:VOD")
		require.Contains(t, string(content), "#EXT-X-ENDLIST")
		require.NotContains(t, string(content), "EVENT")
		before, err := os.Stat(job.OutputPath)
		require.NoError(t, err)
		_, err = (&PrepareHLSStage{profiles: []string{profile}}).Execute(ctx, []pipeline.FileInfo{input})
		require.NoError(t, err)
		after, err := os.Stat(job.OutputPath)
		require.NoError(t, err)
		require.True(t, os.SameFile(before, after))
		data, err := os.ReadFile(manifest.FilePath(filepath.Dir(job.OutputPath), "seg_00000.ts"))
		require.NoError(t, err)
		require.Greater(t, len(data), 188)
	}
}

func TestPrepareHLSSharedBudgetCancellationAndPauseResume(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: hlsasset.ProfileVersion("source")}
	var releases []func()
	for i := 0; i < 3; i++ {
		release, err := mediawork.Shared.Acquire(context.Background(), 3)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	t.Cleanup(func() {
		for _, release := range releases {
			release()
		}
	})
	canceled, cancel := context.WithCancel(ctx.Ctx)
	ctx.Ctx = canceled
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&PrepareHLSStage{profiles: []string{"source"}}).Execute(ctx, []pipeline.FileInfo{input})
		done <- err
	}()
	require.Eventually(t, func() bool {
		j, e := store.QueuePreparation(context.Background(), key)
		return e == nil && j.Status == pipeline.PreparationRunning
	}, time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("HLS 预算等待未取消")
	}
	job, err := store.QueuePreparation(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationPaused, job.Status)
	for _, release := range releases {
		release()
	}
	ctx.Ctx = context.Background()
	_, err = (&PrepareHLSStage{profiles: []string{"source"}, runner: &PreparePlaybackStage{freeBytes: func(string) (uint64, error) { return 0, nil }}}).Execute(ctx, []pipeline.FileInfo{input})
	require.Error(t, err)
	_, err = (&PrepareHLSStage{profiles: []string{"source"}}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	job, err = store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationReady, job.Status)
}

func TestPrepareHLSRestartsAfterPublishedBeforeJobReady(t *testing.T) {
	ctx, input, store, root := preparationFixture(t, "libx264", "aac")
	_, err := (&PrepareHLSStage{profiles: []string{"source"}}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: hlsasset.ProfileVersion("source")}
	job, err := store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	before, err := os.Stat(job.OutputPath)
	require.NoError(t, err)
	require.NoError(t, store.InvalidatePreparation(ctx.Ctx, key))
	require.NoError(t, store.ClaimPreparation(ctx.Ctx, key))
	reopened, err := pipeline.NewSQLiteStore(filepath.Join(root, "pipeline.db"))
	require.NoError(t, err)
	defer reopened.Close()
	require.NoError(t, reopened.ResetRunningTasks(ctx.Ctx))
	ctx.Preparations = reopened
	stage := &PrepareHLSStage{profiles: []string{"source"}}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	require.Empty(t, stage.GetCommands(), "重启只核验和登记现有完整集合")
	after, err := os.Stat(job.OutputPath)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
}

func TestPrepareHLSLowDiskBudgetCancelAndSourceChange(t *testing.T) {
	ctx, input, _, _ := preparationFixture(t, "libx264", "aac")
	stage := &PrepareHLSStage{profiles: []string{"source"}, runner: &PreparePlaybackStage{freeBytes: func(string) (uint64, error) { return 0, nil }}}
	_, err := stage.Execute(ctx, []pipeline.FileInfo{input})
	var paused *pipeline.PreparationPausedError
	require.ErrorAs(t, err, &paused)
	require.FileExists(t, input.Path)
	canceled, cancel := context.WithCancel(ctx.Ctx)
	cancel()
	ctx.Ctx = canceled
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.True(t, errors.Is(err, context.Canceled))
	ctx.Ctx = context.Background()
	require.NoError(t, os.WriteFile(input.Path, []byte("replaced"), 0600))
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.Error(t, err)
}
