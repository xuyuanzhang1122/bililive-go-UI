package stages

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparationReadyRejectsReassociatedManifest(t *testing.T) {
	ctx, input, store, root := preparationFixture(t, "libx264", "aac")
	_, err := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	job, err := store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	info, err := os.Stat(input.Path)
	require.NoError(t, err)
	content, err := os.ReadFile(input.Path)
	require.NoError(t, err)
	other := filepath.Join(filepath.Dir(input.Path), "other.mkv")
	require.NoError(t, os.WriteFile(other, content, 0600))
	require.NoError(t, os.Chtimes(other, info.ModTime(), info.ModTime()))
	manifest, err := playbackasset.Load(job.OutputPath)
	require.NoError(t, err)
	manifest.SourceName = filepath.Base(other)
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(job.OutputPath+playbackasset.Suffix, data, 0600))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	db, err := sql.Open("sqlite", filepath.Join(root, "catalog.db"))
	require.NoError(t, err)
	defer db.Close()
	var assetID string
	require.NoError(t, db.QueryRow("SELECT recording_id FROM media_assets WHERE path=?", "平台/主播/clip.mp4").Scan(&assetID))
	require.NotEqual(t, entry.RecordingID, assetID)
	stage := &PreparePlaybackStage{}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	t.Logf("catalog output belongs other ID=%v; stage err=%v; logs=%s", assetID != entry.RecordingID, err, stage.logs)
	require.Error(t, err, "关联已指向另一录播后，完成任务不得继续成功复用")
}

type reviewFailFinishStore struct{ *pipeline.SQLiteStore }

func (s reviewFailFinishStore) FinishPreparation(context.Context, pipeline.PreparationKey, pipeline.PreparationStatus, string, string, ...string) error {
	return fmt.Errorf("注入准备终态提交失败")
}
func reviewQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'" }
func TestPreparationRecoveryRejectsTargetChangedAfterProbe(t *testing.T) {
	ctx, input, store, root := preparationFixture(t, "libx264", "pcm_s16le")
	ctx.Preparations = reviewFailFinishStore{store}
	_, err := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.Error(t, err)
	ctx.Preparations = store
	require.NoError(t, store.ResetRunningTasks(ctx.Ctx))
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	job, err := store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, pipeline.PreparationQueued, job.Status) // 终态提交失败后重启恢复
	job.OutputPath = filepath.Join(ctx.Catalog.AssetRoot(), entry.RecordingID, entry.SourceVersion+"-"+PreparationProfileVersion, "compat.mp4")
	info, err := os.Stat(job.OutputPath)
	require.NoError(t, err)
	content, err := os.ReadFile(job.OutputPath)
	require.NoError(t, err)
	mdat := bytes.Index(content, []byte("mdat"))
	require.Greater(t, mdat, 0)
	for i := mdat + 4; i < len(content); i++ {
		content[i] = 0
	}
	replacement := job.OutputPath + ".replacement"
	require.NoError(t, os.WriteFile(replacement, content, 0600))
	require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
	replacementVersion, err := recordingcatalog.SnapshotVersion(replacement)
	require.NoError(t, err)
	toolsDir := t.TempDir()
	originalProbe := filepath.Join(filepath.Dir(ctx.FFmpegPath), "ffprobe")
	require.NoError(t, os.Symlink(ctx.FFmpegPath, filepath.Join(toolsDir, "ffmpeg")))
	wrapper := "#!/bin/sh\n" + reviewQuote(originalProbe) + " \"$@\"\nrc=$?\nfor last; do :; done\nif [ \"$last\" = " + reviewQuote(job.OutputPath) + " ] && [ -f " + reviewQuote(replacement) + " ]; then mv " + reviewQuote(replacement) + " " + reviewQuote(job.OutputPath) + "; fi\nexit $rc\n"
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "ffprobe"), []byte(wrapper), 0700))
	ctx.FFmpegPath = filepath.Join(toolsDir, "ffmpeg")
	_, err = (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	after, e := store.QueuePreparation(ctx.Ctx, key)
	require.NoError(t, e)
	t.Logf("stage err=%v; ready=%s; ready version is replacement=%v", err, after.Status, after.OutputVersion == replacementVersion)
	require.Error(t, err, "媒体探测后目标替换，不能将替换文件版本作为已核验产物完成标记")
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	db, e := sql.Open("sqlite", filepath.Join(root, "catalog.db"))
	require.NoError(t, e)
	defer db.Close()
	var status string
	require.NoError(t, db.QueryRow("SELECT status FROM media_assets WHERE kind='compat' AND recording_id=?", entry.RecordingID).Scan(&status))
	require.NotEqual(t, "verified", status, "后台核验不能只凭相同大小mtime把失效产物再次标为可信")
}

func TestHistoryPreparationRejectsChangedApprovedProfile(t *testing.T) {
	_, err := NewPreparePlaybackStage(pipeline.StageConfig{Options: map[string]any{"expected_profile_version": "obsolete-profile"}})
	require.Error(t, err)
}
