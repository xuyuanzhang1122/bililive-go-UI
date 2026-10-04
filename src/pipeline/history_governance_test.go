package pipeline

import (
	"context"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func historyFixture(t *testing.T) (*Manager, *SQLiteStore, *recordingcatalog.Manager, string, context.Context) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "recordings", "平台", "主播", "clip.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0755))
	require.NoError(t, os.WriteFile(source, []byte("source-data"), 0644))
	catStore, err := recordingcatalog.Open(filepath.Join(root, "catalog.db"), filepath.Join(root, "recordings"))
	require.NoError(t, err)
	cat := recordingcatalog.NewManager(catStore, 0)
	t.Cleanup(func() { require.NoError(t, cat.Close()) })
	require.NoError(t, cat.Reconcile(context.Background()))
	store, err := NewSQLiteStore(filepath.Join(root, "pipeline.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	inst := &instance.Instance{RecordingCatalog: cat}
	ctx := context.WithValue(context.Background(), instance.Key, inst)
	manager := NewManager(ctx, store, &ManagerConfig{MaxConcurrent: 3, PollInterval: 10 * time.Millisecond}, nil)
	inst.PipelineManager = manager
	t.Cleanup(func() { manager.Close(context.Background()) })
	return manager, store, cat, source, ctx
}
func TestHistoryPreviewOnlyAndConfirmedQueueIsDurableIdempotent(t *testing.T) {
	m, s, _, source, ctx := historyFixture(t)
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	p, err := m.PreviewHistory(ctx, "owner", "", 100, "test-profile")
	require.NoError(t, err)
	require.Len(t, p.Items, 1)
	require.Equal(t, "dry_run", p.State)
	require.Greater(t, p.EstimatedAdditionalBytes, uint64(len(before)))
	tasks, err := s.ListTasks(ctx, TaskFilter{})
	require.NoError(t, err)
	require.Empty(t, tasks)
	_, err = m.ConfirmHistory(ctx, "other", p.ID, p.Digest)
	require.Error(t, err)
	_, err = m.ConfirmHistory(ctx, "owner", p.ID, "changed")
	require.Error(t, err)
	confirmed, err := m.ConfirmHistory(ctx, "owner", p.ID, p.Digest)
	require.NoError(t, err)
	require.Len(t, confirmed.TaskIDs, 1)
	again, err := m.ConfirmHistory(ctx, "owner", p.ID, p.Digest)
	require.NoError(t, err)
	require.Equal(t, confirmed.TaskIDs, again.TaskIDs)
	tasks, err = s.ListTasks(ctx, TaskFilter{})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, StageNamePreparePlayback, tasks[0].PipelineConfig.Stages[0].Name)
	require.Len(t, tasks[0].PipelineConfig.Stages, 1)
	require.Equal(t, p.Items[0].SourceVersion, tasks[0].InitialFiles[0].PreparationSourceVersion)
	require.NoError(t, m.SetHistoryPaused(ctx, "owner", p.ID, true))
	pending, err := s.GetPendingTasks(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.NoError(t, m.SetHistoryPaused(ctx, "owner", p.ID, false))
	pending, err = s.GetPendingTasks(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
func TestHistoryPreviewActiveChangedAndExpiredDoNotEnqueue(t *testing.T) {
	m, s, cat, source, ctx := historyFixture(t)
	end := recordingcatalog.BeginRecording(source)
	active, err := m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	require.Empty(t, active.Items)
	require.NotEmpty(t, active.Skipped)
	end()
	p, err := m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte("replacement-source"), 0644))
	require.NoError(t, cat.Reconcile(ctx))
	_, err = m.ConfirmHistory(ctx, "u", p.ID, p.Digest)
	require.Error(t, err)
	p, err = m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	_, err = s.db.Exec("UPDATE history_previews SET expires_at=0 WHERE id=?", p.ID)
	require.NoError(t, err)
	_, err = m.ConfirmHistory(ctx, "u", p.ID, p.Digest)
	require.Error(t, err)
	tasks, err := s.ListTasks(ctx, TaskFilter{})
	require.NoError(t, err)
	require.Empty(t, tasks)
}

func TestHistoryDuplicatePreviewCannotControlAnotherQueue(t *testing.T) {
	m, _, _, _, ctx := historyFixture(t)
	first, err := m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	second, err := m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	_, err = m.ConfirmHistory(ctx, "u", first.ID, first.Digest)
	require.NoError(t, err)
	_, err = m.ConfirmHistory(ctx, "u", second.ID, second.Digest)
	require.Error(t, err, "不能把另一个预览的任务控制权移到新预览")
}

type historyWaitingStage struct{ started chan struct{} }

func (s *historyWaitingStage) Name() string { return StageNamePreparePlayback }
func (s *historyWaitingStage) Execute(ctx *PipelineContext, files []FileInfo) ([]FileInfo, error) {
	s.started <- struct{}{}
	<-ctx.Ctx.Done()
	return nil, ctx.Ctx.Err()
}
func TestHistoryPauseRateLimitAndRestartPreserveSource(t *testing.T) {
	m, s, _, source, ctx := historyFixture(t)
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	p, err := m.PreviewHistory(ctx, "u", "", 100, "profile")
	require.NoError(t, err)
	p, err = m.ConfirmHistory(ctx, "u", p.ID, p.Digest)
	require.NoError(t, err)
	started := make(chan struct{}, 2)
	m.RegisterStage(StageNamePreparePlayback, func(StageConfig) (Stage, error) { return &historyWaitingStage{started}, nil })
	require.NoError(t, m.Start(ctx))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("治理任务未启动")
	}
	require.NoError(t, m.SetHistoryPaused(ctx, "u", p.ID, true))
	require.Eventually(t, func() bool {
		task, e := s.GetTask(ctx, p.TaskIDs[0])
		return e == nil && task.Status == PipelineStatusPaused
	}, time.Second, time.Millisecond)
	require.NoError(t, m.SetHistoryPaused(ctx, "u", p.ID, false))
	allowed, err := s.startHistoryTask(ctx, p.TaskIDs[0])
	require.NoError(t, err)
	require.False(t, allowed, "跨恢复仍遵守60秒限速")
	require.NoError(t, m.SetHistoryPaused(ctx, "u", p.ID, true))
	var dbPath string
	require.NoError(t, s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&dbPath))
	m.Close(ctx)
	restartedStore, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	restarted := NewManager(ctx, restartedStore, &ManagerConfig{MaxConcurrent: 3, PollInterval: 10 * time.Millisecond}, nil)
	defer restarted.Close(ctx)
	restarted.RegisterStage(StageNamePreparePlayback, func(StageConfig) (Stage, error) { return &historyWaitingStage{started}, nil })
	require.NoError(t, restarted.Start(ctx))
	saved, err := restarted.GetHistoryPreview(ctx, "u", p.ID)
	require.NoError(t, err)
	require.True(t, saved.Paused)
	pending, err := restartedStore.GetPendingTasks(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, pending)
	// 只在测试时推进持久节流时钟，生产恢复不会清零预算。
	_, err = restartedStore.db.Exec("UPDATE history_control SET last_started=0")
	require.NoError(t, err)
	require.NoError(t, restarted.SetHistoryPaused(ctx, "u", p.ID, false))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("恢复后未执行")
	}
	require.NoError(t, restarted.CancelTask(p.TaskIDs[0]))
	require.Eventually(t, func() bool {
		task, e := restartedStore.GetTask(ctx, p.TaskIDs[0])
		return e == nil && task.Status == PipelineStatusCancelled
	}, time.Second, time.Millisecond)
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
