package pipeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPreparationDeduplicatesAndRecovers(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tasks.db")
	s, err := NewSQLiteStore(path)
	require.NoError(t, err)
	key := PreparationKey{"recording", "source-v1", "profile-v1"}
	job, err := s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationQueued, job.Status)
	require.NoError(t, s.ClaimPreparation(ctx, key))
	duplicate, err := s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationRunning, duplicate.Status)
	require.Error(t, s.ClaimPreparation(ctx, key))
	require.NoError(t, s.Close())
	s, err = NewSQLiteStore(path)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.ResetRunningTasks(ctx))
	job, err = s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationQueued, job.Status)
	require.NoError(t, s.ClaimPreparation(ctx, key))
	require.NoError(t, s.FinishPreparation(ctx, key, PreparationReady, "asset.mp4", ""))
	job, err = s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "asset.mp4", job.OutputPath)
	require.Equal(t, PreparationReady, job.Status)
	require.Error(t, s.ClaimPreparation(ctx, key))
	next, err := s.QueuePreparation(ctx, PreparationKey{"recording", "source-v2", "profile-v1"})
	require.NoError(t, err)
	require.Equal(t, PreparationQueued, next.Status)
}

func TestPausedPreparationTaskCanResume(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	defer s.Close()
	task := NewPipelineTask(RecordInfo{}, &PipelineConfig{}, nil)
	require.NoError(t, s.CreateTask(ctx, task))
	task.Status = PipelineStatusPaused
	require.NoError(t, s.UpdateTask(ctx, task))
	tasks, err := s.GetPendingTasks(ctx, 1)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, task.ID, tasks[0].ID)
}

func TestPausedPreparationDoesNotStarveNewTasks(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	defer s.Close()
	paused := NewPipelineTask(RecordInfo{}, &PipelineConfig{}, nil)
	paused.Status = PipelineStatusPaused
	require.NoError(t, s.CreateTask(ctx, paused))
	ready := NewPipelineTask(RecordInfo{}, &PipelineConfig{}, nil)
	require.NoError(t, s.CreateTask(ctx, ready))
	tasks, err := s.GetPendingTasks(ctx, 1)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, ready.ID, tasks[0].ID, "暂停的老任务不能反复占据所有槽位")
	ready.Status = PipelineStatusPaused
	require.NoError(t, s.UpdateTask(ctx, ready))
	now := time.Now()
	paused.StartedAt = &now
	require.NoError(t, s.UpdateTask(ctx, paused))
	tasks, err = s.GetPendingTasks(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, ready.ID, tasks[0].ID, "多个暂停任务按上次尝试时间轮转")
}
