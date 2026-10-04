package servers

import (
	"context"
	"encoding/json"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2BatchDeleteReportsPartialFailureAndPreservesOtherRecording(t *testing.T) {
	h, _, cfg, cat, e, source, _ := v2Fixture(t, "mp4")
	other := filepath.Join(filepath.Dir(source), "other.flv")
	require.NoError(t, os.WriteFile(other, []byte("different recording"), 0600))
	require.NoError(t, cat.Reconcile(context.Background()))
	records, err := cat.ListRecordings(context.Background(), "", 100)
	require.NoError(t, err)
	var otherID string
	for _, r := range records {
		if r.RecordingID != e.RecordingID {
			otherID = r.RecordingID
		}
	}
	session := v2Create(t, h, cfg, e, v2Caps())
	body := map[string]any{"items": []any{map[string]any{"recording_id": otherID, "source_version": strings.Repeat("b", 64)}, map[string]any{"recording_id": e.RecordingID, "source_version": e.SourceVersion}}}
	res := v2Request(t, h, "POST", "/api/v2/recordings/batch-delete", cfg.Security.APIKey, body)
	require.Equal(t, 200, res.Code, res.Body.String())
	v2AssertResponse(t, res, "BatchEnvelope")
	var envelope struct {
		Data struct {
			Items []struct {
				Status  string   `json:"status"`
				Deleted int      `json:"deleted_files"`
				Error   *v2Error `json:"error"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 2)
	require.Equal(t, "failed", envelope.Data.Items[0].Status)
	require.Equal(t, "source_changed", envelope.Data.Items[0].Error.Code)
	require.Equal(t, "deleted", envelope.Data.Items[1].Status)
	require.Equal(t, 1, envelope.Data.Items[1].Deleted)
	require.NoFileExists(t, source)
	require.FileExists(t, other)
	require.NoError(t, cat.Reconcile(context.Background()))
	require.Equal(t, 409, v2Request(t, h, "GET", session.URL, "", nil).Code)
	duplicate := map[string]any{"items": []any{map[string]any{"recording_id": otherID, "source_version": strings.Repeat("b", 64)}, map[string]any{"recording_id": otherID, "source_version": strings.Repeat("b", 64)}}}
	require.Equal(t, 400, v2Request(t, h, "POST", "/api/v2/recordings/batch-delete", cfg.Security.APIKey, duplicate).Code)
	require.FileExists(t, other)
}

func TestV2BatchDeletePreparedMediaSetPreservesUnknownSidecars(t *testing.T) {
	h, _, cfg, cat, entry, source, ctx := v2Fixture(t, "flv")
	store, err := pipeline.NewSQLiteStore(filepath.Join(cfg.AppDataPath, "tasks.db"))
	require.NoError(t, err)
	defer store.Close()
	pc := &pipeline.PipelineContext{Ctx: ctx, Catalog: cat, Preparations: store}
	files := []pipeline.FileInfo{{Path: source, Type: pipeline.FileTypeVideo, PreparationSourceVersion: entry.SourceVersion}}
	prep, err := stages.NewPreparePlaybackStage(pipeline.StageConfig{})
	require.NoError(t, err)
	_, err = prep.Execute(pc, files)
	require.NoError(t, err)
	stage, err := stages.NewPrepareHLSStage(pipeline.StageConfig{Options: map[string]any{"profiles": []string{"source"}}})
	require.NoError(t, err)
	_, err = stage.Execute(pc, files)
	require.NoError(t, err)
	asset, err := cat.FindHLS(ctx, entry.RecordingID, entry.SourceVersion+"-"+hlsasset.ProfileVersion("source"))
	require.NoError(t, err)
	manifest, err := hlsasset.Load(filepath.Dir(asset.Path))
	require.NoError(t, err)
	paths := []string{}
	candidates, err := cat.PlaybackFiles(ctx, entry.RecordingID, entry.SourceVersion)
	require.NoError(t, err)
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}
	for _, member := range manifest.Files {
		paths = append(paths, manifest.FilePath(filepath.Dir(asset.Path), member.Name))
	}
	sidecar := filepath.Join(filepath.Dir(asset.Path), "unregistered-note.txt")
	require.NoError(t, os.WriteFile(sidecar, []byte("保留未知旁文件"), 0600))
	response := v2Request(t, h, "POST", "/api/v2/recordings/batch-delete", cfg.Security.APIKey, map[string]any{"items": []any{map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion}}})
	require.Equal(t, 200, response.Code, response.Body.String())
	v2AssertResponse(t, response, "BatchEnvelope")
	var envelope struct {
		Data struct {
			Items []v2DeleteResult `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "deleted", envelope.Data.Items[0].Status, response.Body.String())
	require.Equal(t, len(paths), envelope.Data.Items[0].Deleted)
	for _, path := range paths {
		require.NoFileExists(t, path)
	}
	require.FileExists(t, sidecar)
}
