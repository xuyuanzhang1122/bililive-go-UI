package pipeline

import (
	"context"
	"database/sql"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestPreparationStatusReadDoesNotCreateOrChangeJobs(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	defer s.Close()
	key := PreparationKey{"id", "source", "profile"}
	_, err = s.PreparationStatus(ctx, key)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	j, err := s.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationQueued, j.Status)
	require.NoError(t, s.ClaimPreparation(ctx, key))
	require.NoError(t, s.FinishPreparation(ctx, key, PreparationFailed, "", "actual failure"))
	j, err = s.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationFailed, j.Status)
	require.Equal(t, "actual failure", j.Reason)
}

func TestScheduledPreparationStatusBindsSourceRuleAndActiveTask(t *testing.T) {
	for _, tc := range []struct {
		name, path, source, profile string
		stage                       StageConfig
		status                      PipelineStatus
		want                        PreparationStatus
	}{
		{"auto-queued", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusPending, PreparationQueued},
		{"running", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusRunning, PreparationQueued},
		{"paused", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusPaused, PreparationPaused},
		{"cancelled", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusCancelled, ""},
		{"failed", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusFailed, ""},
		{"changed-source", "/recording.flv", "source-v2", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusPending, ""},
		{"different-path", "/other.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback}, PipelineStatusPending, ""},
		{"disabled", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback, Enabled: EnabledPtr(false)}, PipelineStatusPending, ""},
		{"old-rule", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback, Options: map[string]any{"expected_profile_version": "old"}}, PipelineStatusPending, ""},
		{"pinned-rule", "/recording.flv", "source-v1", "current", StageConfig{Name: StageNamePreparePlayback, Options: map[string]any{"expected_profile_version": "current"}}, PipelineStatusPending, PreparationQueued},
		{"hls-match", "/recording.flv", "source-v1", "hls-720p-v1", StageConfig{Name: StageNamePrepareHLS, Options: map[string]any{"profiles": []string{"720p"}}}, PipelineStatusPending, PreparationQueued},
		{"hls-mismatch", "/recording.flv", "source-v1", "hls-1080p-v1", StageConfig{Name: StageNamePrepareHLS, Options: map[string]any{"profiles": []string{"720p"}}}, PipelineStatusPending, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
			require.NoError(t, err)
			defer s.Close()
			task := NewPipelineTask(RecordInfo{}, &PipelineConfig{Stages: []StageConfig{tc.stage}}, []FileInfo{{Path: tc.path, Type: FileTypeVideo, PreparationSourceVersion: tc.source}})
			task.Status = tc.status
			require.NoError(t, s.CreateTask(ctx, task))
			status, err := s.ScheduledPreparationStatus(ctx, "/recording.flv", "source-v1", tc.stage.Name, tc.profile)
			if tc.want == "" {
				require.ErrorIs(t, err, sql.ErrNoRows)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, status)
			}
			var count int
			require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM playback_preparations").Scan(&count))
			require.Zero(t, count, "播放状态查询不能创建准备任务")
		})
	}
}
