package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
)

// PreparationStatus 只读已有状态，媒体 API 不因轮询创建准备任务。
func (s *SQLiteStore) PreparationStatus(ctx context.Context, k PreparationKey) (PreparationJob, error) {
	j := PreparationJob{PreparationKey: k}
	err := s.db.QueryRowContext(ctx, `SELECT status,output_path,reason,output_version FROM playback_preparations WHERE recording_id=? AND source_version=? AND profile_version=?`, k.RecordingID, k.SourceVersion, k.ProfileVersion).Scan(&j.Status, &j.OutputPath, &j.Reason, &j.OutputVersion)
	return j, err
}

// ScheduledPreparationStatus 只读持久队列，覆盖准备阶段尚未执行的等待窗口。
// 来源路径及快照必须同时匹配，取消、失败和旧规则任务不得冒称正在准备。
func (s *SQLiteStore) ScheduledPreparationStatus(ctx context.Context, path, sourceVersion, stageName, profileVersion string) (PreparationStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT CASE WHEN p.paused=1 THEN 'paused' ELSE t.status END,t.pipeline_config_json
	 FROM pipeline_tasks t LEFT JOIN history_tasks h ON h.task_id=t.id LEFT JOIN history_previews p ON p.id=h.preview_id
	 WHERE t.status IN ('pending','running','paused') AND EXISTS (
	 SELECT 1 FROM json_each(t.initial_files_json) f WHERE json_extract(f.value,'$.path')=?
	 AND json_extract(f.value,'$.preparation_source_version')=? AND json_extract(f.value,'$.type')='video')
	 ORDER BY CASE t.status WHEN 'running' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,t.id`, path, sourceVersion)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var status PipelineStatus
		var data string
		if err := rows.Scan(&status, &data); err != nil {
			return "", err
		}
		var config PipelineConfig
		if err := json.Unmarshal([]byte(data), &config); err != nil {
			return "", err
		}
		for _, stage := range config.Stages {
			if stage.Name != stageName || !stage.IsEnabled() {
				continue
			}
			if expected, ok := stage.Options["expected_profile_version"]; ok && expected != profileVersion {
				continue
			}
			if stageName == StageNamePrepareHLS {
				profiles, _ := stage.Options["profiles"].([]any)
				matches := false
				for _, value := range profiles {
					if profile, ok := value.(string); ok && hlsasset.ProfileVersion(profile) == profileVersion {
						matches = true
					}
				}
				if !matches {
					continue
				}
			}
			if status == PipelineStatusPaused {
				return PreparationPaused, nil
			}
			return PreparationQueued, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", sql.ErrNoRows
}

func (m *Manager) ScheduledPreparationStatus(ctx context.Context, path, sourceVersion, stageName, profileVersion string) (PreparationStatus, error) {
	if store, ok := m.store.(interface {
		ScheduledPreparationStatus(context.Context, string, string, string, string) (PreparationStatus, error)
	}); ok {
		return store.ScheduledPreparationStatus(ctx, path, sourceVersion, stageName, profileVersion)
	}
	return "", sql.ErrNoRows
}
func (m *Manager) PreparationStatus(ctx context.Context, k PreparationKey) (PreparationJob, error) {
	if store, ok := m.store.(interface {
		PreparationStatus(context.Context, PreparationKey) (PreparationJob, error)
	}); ok {
		return store.PreparationStatus(ctx, k)
	}
	return PreparationJob{}, sql.ErrNoRows
}
