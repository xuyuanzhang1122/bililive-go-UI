package pipeline

import (
	"context"
	"fmt"
)

type PreparationStatus string

const (
	PreparationQueued  PreparationStatus = "queued"
	PreparationRunning PreparationStatus = "running"
	PreparationReady   PreparationStatus = "ready"
	PreparationFailed  PreparationStatus = "failed"
	PreparationPaused  PreparationStatus = "paused"
)

type PreparationKey struct{ RecordingID, SourceVersion, ProfileVersion string }
type PreparationJob struct {
	PreparationKey
	Status        PreparationStatus
	OutputPath    string
	OutputVersion string
	Reason        string
}
type PreparationStore interface {
	QueuePreparation(context.Context, PreparationKey) (PreparationJob, error)
	ClaimPreparation(context.Context, PreparationKey) error
	FinishPreparation(context.Context, PreparationKey, PreparationStatus, string, string, ...string) error
	InvalidatePreparation(context.Context, PreparationKey) error
}

// PreparationPausedError 允许调度器持久暂停并在后续轮询重新核验。
type PreparationPausedError struct{ Reason string }

func (e *PreparationPausedError) Error() string { return e.Reason }

const preparationSchema = `CREATE TABLE IF NOT EXISTS playback_preparations (
 recording_id TEXT NOT NULL, source_version TEXT NOT NULL, profile_version TEXT NOT NULL,
 status TEXT NOT NULL, output_path TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', output_version TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(recording_id,source_version,profile_version)
);`

func (s *SQLiteStore) QueuePreparation(ctx context.Context, k PreparationKey) (PreparationJob, error) {
	j := PreparationJob{PreparationKey: k}
	if k.RecordingID == "" || k.SourceVersion == "" || k.ProfileVersion == "" {
		return j, fmt.Errorf("准备任务缺少身份或版本")
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO playback_preparations(recording_id,source_version,profile_version,status) VALUES(?,?,?,'queued')`, k.RecordingID, k.SourceVersion, k.ProfileVersion)
	if err != nil {
		return j, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT status,output_path,reason,output_version FROM playback_preparations WHERE recording_id=? AND source_version=? AND profile_version=?`, k.RecordingID, k.SourceVersion, k.ProfileVersion).Scan(&j.Status, &j.OutputPath, &j.Reason, &j.OutputVersion)
	return j, err
}
func (s *SQLiteStore) ClaimPreparation(ctx context.Context, k PreparationKey) error {
	result, err := s.db.ExecContext(ctx, `UPDATE playback_preparations SET status='running',reason='' WHERE recording_id=? AND source_version=? AND profile_version=? AND status IN ('queued','paused','failed')`, k.RecordingID, k.SourceVersion, k.ProfileVersion)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return &PreparationPausedError{"同一准备任务已在运行或完成"}
	}
	return nil
}
func (s *SQLiteStore) FinishPreparation(ctx context.Context, k PreparationKey, status PreparationStatus, output, reason string, versions ...string) error {
	outputVersion := ""
	if len(versions) > 0 {
		outputVersion = versions[0]
	}
	if status != PreparationReady && status != PreparationFailed && status != PreparationPaused {
		return fmt.Errorf("非法准备终态")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE playback_preparations SET status=?,output_path=?,reason=?,output_version=? WHERE recording_id=? AND source_version=? AND profile_version=? AND status='running'`, status, output, reason, outputVersion, k.RecordingID, k.SourceVersion, k.ProfileVersion)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("准备任务未持有执行权")
	}
	return nil
}

func (s *SQLiteStore) InvalidatePreparation(ctx context.Context, k PreparationKey) error {
	_, err := s.db.ExecContext(ctx, "UPDATE playback_preparations SET status='failed',reason='已完成产物失效' WHERE recording_id=? AND source_version=? AND profile_version=? AND status='ready'", k.RecordingID, k.SourceVersion, k.ProfileVersion)
	return err
}
