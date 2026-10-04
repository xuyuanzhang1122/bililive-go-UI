package servers

import (
	"context"
	"encoding/json"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryAPIRequiresExplicitConfirmationAndRunsOriginalPreparation(t *testing.T) {
	router, _, _, _, _, source, ctx := v2CodecFixture(t, "mkv", "mpeg4", "pcm_s16le")
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	store, err := pipeline.NewSQLiteStore(filepath.Join(t.TempDir(), "pipeline.db"))
	require.NoError(t, err)
	pm := pipeline.NewManager(ctx, store, &pipeline.ManagerConfig{MaxConcurrent: 3, PollInterval: 20 * time.Millisecond}, nil)
	defer pm.Close(context.Background())
	instance.GetInstance(ctx).PipelineManager = pm
	pm.RegisterStage(pipeline.StageNamePreparePlayback, stages.NewPreparePlaybackStage)
	require.Equal(t, 401, v2Request(t, router, "POST", "/api/v2/history-previews?_key=v2-test-key", "", map[string]any{}).Code)
	preview := v2Request(t, router, "POST", "/api/v2/history-previews", "v2-test-key", map[string]any{})
	require.Equal(t, 200, preview.Code, preview.Body.String())
	var envelope struct {
		Data pipeline.HistoryPreview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &envelope))
	p := envelope.Data
	previewSnapshot, err := json.Marshal(p)
	require.NoError(t, err)
	t.Logf("临时样片预览（非真实库）: %s", previewSnapshot)
	require.Len(t, p.Items, 1)
	tasks, err := store.ListTasks(ctx, pipeline.TaskFilter{})
	require.NoError(t, err)
	require.Empty(t, tasks)
	endpoint := "/api/v2/history-previews/" + p.ID + "/confirm"
	require.Equal(t, 400, v2Request(t, router, "POST", endpoint, "v2-test-key", map[string]any{"digest": p.Digest}).Code)
	confirmation := v2Request(t, router, "POST", endpoint, "v2-test-key", map[string]any{"digest": p.Digest, "confirm": true})
	require.Equal(t, 200, confirmation.Code, confirmation.Body.String())
	require.NoError(t, json.Unmarshal(confirmation.Body.Bytes(), &envelope))
	require.NoError(t, pm.Start(ctx))
	require.Eventually(t, func() bool {
		task, e := store.GetTask(ctx, envelope.Data.TaskIDs[0])
		return e == nil && task.Status == pipeline.PipelineStatusCompleted
	}, 15*time.Second, 20*time.Millisecond)
	task, err := store.GetTask(ctx, envelope.Data.TaskIDs[0])
	require.NoError(t, err)
	require.Len(t, task.StageResults, 1)
	require.Equal(t, pipeline.StageNamePreparePlayback, task.StageResults[0].StageName)
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, data, after)
	status := v2Request(t, router, http.MethodGet, "/api/v2/history-previews/"+p.ID, "v2-test-key", nil)
	require.Equal(t, 200, status.Code)
}

func TestHistoryLowSpacePausesAndPreservesOriginal(t *testing.T) {
	_, _, cfg, _, _, source, ctx := v2CodecFixture(t, "mkv", "mpeg4", "pcm_s16le")
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	cfg.HLSCache.MinFreeSpaceGB = 1_000_000
	store, err := pipeline.NewSQLiteStore(filepath.Join(t.TempDir(), "pipeline.db"))
	require.NoError(t, err)
	pm := pipeline.NewManager(ctx, store, &pipeline.ManagerConfig{MaxConcurrent: 3, PollInterval: 20 * time.Millisecond}, nil)
	defer pm.Close(ctx)
	instance.GetInstance(ctx).PipelineManager = pm
	pm.RegisterStage(pipeline.StageNamePreparePlayback, stages.NewPreparePlaybackStage)
	p, err := pm.PreviewHistory(ctx, "u", "", 100, stages.PreparationProfileVersion)
	require.NoError(t, err)
	p, err = pm.ConfirmHistory(ctx, "u", p.ID, p.Digest)
	require.NoError(t, err)
	require.NoError(t, pm.Start(ctx))
	require.Eventually(t, func() bool {
		task, e := store.GetTask(ctx, p.TaskIDs[0])
		return e == nil && task.Status == pipeline.PipelineStatusPaused
	}, 10*time.Second, 20*time.Millisecond)
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
	task, err := store.GetTask(ctx, p.TaskIDs[0])
	require.NoError(t, err)
	require.Contains(t, task.ErrorMessage, "空间")
}
