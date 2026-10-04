package pipeline

import (
	"context"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type cancellationPreparationStage struct{ started chan struct{} }

func (s *cancellationPreparationStage) Name() string { return "waiting" }
func (s *cancellationPreparationStage) Execute(ctx *PipelineContext, files []FileInfo) ([]FileInfo, error) {
	close(s.started)
	<-ctx.Ctx.Done()
	return nil, &PreparationPausedError{"预算等待已取消"}
}

func TestPreparationExplicitCancelIsNotAutomaticallyResumed(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	manager := NewManager(context.Background(), s, &ManagerConfig{MaxConcurrent: 1, PollInterval: time.Hour}, nil)
	started := make(chan struct{})
	manager.RegisterStage("waiting", func(StageConfig) (Stage, error) { return &cancellationPreparationStage{started}, nil })
	require.NoError(t, manager.Start(context.Background()))
	defer manager.Close(context.Background())
	task := NewPipelineTask(RecordInfo{}, &PipelineConfig{Stages: []StageConfig{{Name: "waiting"}}}, nil)
	require.NoError(t, manager.EnqueueTask(task))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("任务未开始")
	}
	require.NoError(t, manager.CancelTask(task.ID))
	require.Eventually(t, func() bool {
		task, err := s.GetTask(context.Background(), task.ID)
		return err == nil && task.Status != PipelineStatusRunning
	}, time.Second, time.Millisecond)
	persisted, err := s.GetTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, PipelineStatusCancelled, persisted.Status)
	pending, err := s.GetPendingTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Empty(t, pending)
}

type countedPreparationStage struct{ count *atomic.Int32 }

func (s *countedPreparationStage) Name() string { return "counted" }
func (s *countedPreparationStage) Execute(ctx *PipelineContext, files []FileInfo) ([]FileInfo, error) {
	s.count.Add(1)
	return files, nil
}

func TestTaskMustPersistRunningBeforeExecuting(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	manager := NewManager(ctx, s, nil, nil)
	defer manager.Close(ctx)
	var count atomic.Int32
	manager.RegisterStage("counted", func(StageConfig) (Stage, error) { return &countedPreparationStage{&count}, nil })
	task := NewPipelineTask(RecordInfo{}, &PipelineConfig{Stages: []StageConfig{{Name: "counted"}}}, nil)
	require.NoError(t, s.CreateTask(ctx, task))
	_, err = s.db.Exec(`CREATE TRIGGER fail_start BEFORE UPDATE ON pipeline_tasks WHEN NEW.status='running' BEGIN SELECT RAISE(FAIL,'injected start failure'); END`)
	require.NoError(t, err)
	manager.scheduleNextTasks()
	manager.wg.Wait()
	require.Zero(t, count.Load(), "执行权未持久化时不能运行媒体任务")
	persisted, err := s.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, PipelineStatusPending, persisted.Status)
}
