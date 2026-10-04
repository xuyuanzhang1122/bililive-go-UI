package pipeline

import (
	"context"
	"database/sql"
)

// PreparationStatus 只读已有状态，媒体 API 不因轮询创建准备任务。
func (s *SQLiteStore) PreparationStatus(ctx context.Context, k PreparationKey) (PreparationJob, error) {
	j := PreparationJob{PreparationKey: k}
	err := s.db.QueryRowContext(ctx, `SELECT status,output_path,reason,output_version FROM playback_preparations WHERE recording_id=? AND source_version=? AND profile_version=?`, k.RecordingID, k.SourceVersion, k.ProfileVersion).Scan(&j.Status, &j.OutputPath, &j.Reason, &j.OutputVersion)
	return j, err
}
func (m *Manager) PreparationStatus(ctx context.Context, k PreparationKey) (PreparationJob, error) {
	if store, ok := m.store.(interface {
		PreparationStatus(context.Context, PreparationKey) (PreparationJob, error)
	}); ok {
		return store.PreparationStatus(ctx, k)
	}
	return PreparationJob{}, sql.ErrNoRows
}
