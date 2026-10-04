package pipeline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	uuid "github.com/satori/go.uuid"
	"math"
	"time"
)

const historySchema = `
CREATE TABLE IF NOT EXISTS history_previews(id TEXT PRIMARY KEY,owner TEXT NOT NULL,digest TEXT NOT NULL,body TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'dry_run',paused INTEGER NOT NULL DEFAULT 0,expires_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS history_tasks(task_id INTEGER PRIMARY KEY,preview_id TEXT NOT NULL,recording_id TEXT NOT NULL,source_version TEXT NOT NULL,profile_version TEXT NOT NULL,UNIQUE(recording_id,source_version,profile_version));
CREATE TABLE IF NOT EXISTS history_control(id INTEGER PRIMARY KEY CHECK(id=1),last_started INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO history_control(id,last_started) VALUES(1,0);`

type HistoryItem struct {
	RecordingID   string `json:"recording_id"`
	SourceVersion string `json:"source_version"`
	Path          string `json:"path"`
	SourcePath    string `json:"source_path,omitempty"`
	SizeBytes     uint64 `json:"size_bytes"`
	Reason        string `json:"reason,omitempty"`
}
type HistoryPreview struct {
	ID                       string        `json:"id"`
	Digest                   string        `json:"digest"`
	State                    string        `json:"state"`
	ProfileVersion           string        `json:"profile_version"`
	SourceRoot               string        `json:"source_root"`
	Items                    []HistoryItem `json:"items"`
	Skipped                  []HistoryItem `json:"skipped"`
	EstimatedAdditionalBytes uint64        `json:"estimated_additional_bytes"`
	EstimatedAdditionalText  string        `json:"estimated_additional_text"`
	Strategy                 string        `json:"strategy"`
	NextCursor               string        `json:"next_cursor"`
	ExpiresAt                int64         `json:"expires_at"`
	MinStartIntervalSeconds  int           `json:"min_start_interval_seconds"`
	MaxConcurrent            int           `json:"max_concurrent"`
	Paused                   bool          `json:"paused"`
	PreserveSource           bool          `json:"preserve_source"`
	TaskIDs                  []int64       `json:"task_ids"`
}

func (m *Manager) historyStore() (*SQLiteStore, error) {
	s, ok := m.store.(*SQLiteStore)
	if !ok {
		return nil, fmt.Errorf("历史治理需要持久 SQLite 管道")
	}
	return s, nil
}
func historyCatalog(ctx context.Context) (*recordingcatalog.Manager, error) {
	inst := instance.GetInstance(ctx)
	if inst == nil || inst.RecordingCatalog == nil {
		return nil, fmt.Errorf("录播目录未初始化")
	}
	return inst.RecordingCatalog, nil
}

// 默认预览只读取已核验目录和候选来源快照，不执行转换或删除。
func (m *Manager) PreviewHistory(ctx context.Context, owner, after string, limit int, profile string) (HistoryPreview, error) {
	s, err := m.historyStore()
	if err != nil {
		return HistoryPreview{}, err
	}
	cat, err := historyCatalog(ctx)
	if err != nil {
		return HistoryPreview{}, err
	}
	if owner == "" || profile == "" || limit < 1 || limit > 100 {
		return HistoryPreview{}, fmt.Errorf("预览参数无效")
	}
	status, err := cat.Status(ctx)
	if err != nil || status != "verified" {
		return HistoryPreview{}, fmt.Errorf("录播目录尚未核验")
	}
	records, err := cat.ListRecordings(ctx, after, limit)
	if err != nil {
		return HistoryPreview{}, err
	}
	id, err := uuid.NewV4()
	if err != nil {
		return HistoryPreview{}, err
	}
	p := HistoryPreview{ID: id.String(), State: "dry_run", ProfileVersion: profile, SourceRoot: cat.RootPath(), ExpiresAt: time.Now().Add(30 * time.Minute).Unix(), MinStartIntervalSeconds: 60, MaxConcurrent: 1, PreserveSource: true, Items: []HistoryItem{}, Skipped: []HistoryItem{}, TaskIDs: []int64{}}
	for _, r := range records {
		p.NextCursor = r.RecordingID
		item := HistoryItem{RecordingID: r.RecordingID, SourceVersion: r.SourceVersion, Path: r.Path}
		job, e := s.PreparationStatus(ctx, PreparationKey{r.RecordingID, r.SourceVersion, profile})
		if e == nil && (job.Status == PreparationReady || job.Status == PreparationRunning) {
			item.Reason = "已有准备资产或正在准备"
			p.Skipped = append(p.Skipped, item)
			continue
		}
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return HistoryPreview{}, e
		}
		files, e := cat.PlaybackFiles(ctx, r.RecordingID, r.SourceVersion)
		if e != nil {
			return HistoryPreview{}, e
		}
		for _, file := range files {
			if file.Kind == "source" && file.FileVersion == r.SourceVersion {
				item.SourcePath = file.Path
				item.Path = file.RelativePath
				break
			}
		}
		if item.SourcePath == "" {
			item.Reason = "没有可核验来源"
			p.Skipped = append(p.Skipped, item)
			continue
		}
		if _, e = cat.PreparationEntry(ctx, item.SourcePath, item.SourceVersion); e != nil {
			item.Reason = e.Error()
			p.Skipped = append(p.Skipped, item)
			continue
		}
		if r.Size < 0 || uint64(r.Size) > (math.MaxUint64-1_000_000_000)/3 {
			return HistoryPreview{}, fmt.Errorf("预计空间溢出")
		}
		item.SizeBytes = uint64(r.Size)
		estimate := item.SizeBytes*3 + 1_000_000_000
		if p.EstimatedAdditionalBytes > math.MaxUint64-estimate {
			return HistoryPreview{}, fmt.Errorf("预计总空间溢出")
		}
		p.EstimatedAdditionalBytes += estimate
		p.Items = append(p.Items, item)
	}
	if len(records) < limit {
		p.NextCursor = ""
	}
	p.EstimatedAdditionalText = fmt.Sprintf("%d 字节（保守预估）", p.EstimatedAdditionalBytes)
	p.Strategy = "复用录后 SDR H264/AAC 准备规则；探测后决定复制或转码，保留来源；不上传、不删源"
	data, err := json.Marshal(p)
	if err != nil {
		return HistoryPreview{}, err
	}
	hash := sha256.Sum256(data)
	p.Digest = hex.EncodeToString(hash[:])
	data, err = json.Marshal(p)
	if err != nil {
		return HistoryPreview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var drafts int
	if err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM history_previews WHERE owner=? AND state='dry_run' AND expires_at>?", owner, time.Now().Unix()).Scan(&drafts); err != nil {
		return HistoryPreview{}, err
	}
	if drafts >= 32 {
		return HistoryPreview{}, fmt.Errorf("待确认预览已达到32个，请等待旧预览过期")
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO history_previews(id,owner,digest,body,expires_at) VALUES(?,?,?,?,?)", p.ID, owner, p.Digest, string(data), p.ExpiresAt)
	return p, err
}
func (s *SQLiteStore) readHistory(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, owner, id string) (HistoryPreview, error) {
	var p HistoryPreview
	var body, state, digest string
	var paused int
	var expires int64
	err := q.QueryRowContext(ctx, "SELECT body,state,digest,paused,expires_at FROM history_previews WHERE id=? AND owner=?", id, owner).Scan(&body, &state, &digest, &paused, &expires)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal([]byte(body), &p); err != nil {
		return p, err
	}
	p.State = state
	p.Digest = digest
	p.Paused = paused != 0
	p.ExpiresAt = expires
	return p, nil
}
func (m *Manager) GetHistoryPreview(ctx context.Context, owner, id string) (HistoryPreview, error) {
	s, err := m.historyStore()
	if err != nil {
		return HistoryPreview{}, err
	}
	return s.readHistory(ctx, s.db, owner, id)
}

// 确认绑定预览摘要与用户，事务内创建原管道任务，重复确认不会重复转换。
func (m *Manager) ConfirmHistory(ctx context.Context, owner, id, digest string) (HistoryPreview, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	s, err := m.historyStore()
	if err != nil {
		return HistoryPreview{}, err
	}
	cat, err := historyCatalog(ctx)
	if err != nil {
		return HistoryPreview{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HistoryPreview{}, err
	}
	defer tx.Rollback()
	p, err := s.readHistory(ctx, tx, owner, id)
	if err != nil {
		return p, err
	}
	if p.Digest != digest || digest == "" {
		return p, fmt.Errorf("预览摘要不匹配")
	}
	if p.State == "confirmed" {
		return p, nil
	}
	if p.ExpiresAt <= time.Now().Unix() || p.SourceRoot != cat.RootPath() {
		return p, fmt.Errorf("预览已过期或来源根目录变化")
	}
	if len(p.Items) == 0 {
		return p, fmt.Errorf("预览没有可执行候选")
	}
	// 先核验全部来源，再创建任务；任何一项变化整批拒绝。
	for _, item := range p.Items {
		entry, e := cat.PreparationEntry(ctx, item.SourcePath, item.SourceVersion)
		if e != nil || entry.RecordingID != item.RecordingID {
			return p, fmt.Errorf("来源失效: %s", item.Path)
		}
	}
	for _, item := range p.Items {
		var existing int64
		e := tx.QueryRowContext(ctx, "SELECT task_id FROM history_tasks WHERE recording_id=? AND source_version=? AND profile_version=?", item.RecordingID, item.SourceVersion, p.ProfileVersion).Scan(&existing)
		if e == nil {
			return p, fmt.Errorf("候选已由其他预览入队，请使用原任务和预览控制")
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return p, e
		}
		input := NewVideoFileInfo(item.SourcePath)
		input.PreparationSourceVersion = item.SourceVersion
		task := NewPipelineTask(RecordInfo{}, &PipelineConfig{Stages: []StageConfig{{Name: StageNamePreparePlayback, Options: map[string]any{"expected_profile_version": p.ProfileVersion}}}}, []FileInfo{input})
		if e = s.createTask(ctx, tx, task); e != nil {
			return p, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO history_tasks(task_id,preview_id,recording_id,source_version,profile_version) VALUES(?,?,?,?,?)", task.ID, p.ID, item.RecordingID, item.SourceVersion, p.ProfileVersion); e != nil {
			return p, e
		}
		p.TaskIDs = append(p.TaskIDs, task.ID)
	}
	p.State = "confirmed"
	data, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE history_previews SET body=?,state='confirmed' WHERE id=?", string(data), p.ID); err != nil {
		return p, err
	}
	return p, tx.Commit()
}
func (m *Manager) SetHistoryPaused(ctx context.Context, owner, id string, paused bool) error {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	s, err := m.historyStore()
	if err != nil {
		return err
	}
	p, err := s.readHistory(ctx, s.db, owner, id)
	if err != nil {
		return err
	}
	if p.State != "confirmed" {
		return fmt.Errorf("预览尚未确认")
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE history_previews SET paused=? WHERE id=? AND owner=?", boolToInt(paused), id, owner); err != nil {
		return err
	}
	if paused {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, taskID := range p.TaskIDs {
			if cancel := m.runningTasks[taskID]; cancel != nil {
				m.historyPausedTasks[taskID] = true
				cancel()
			}
		}
	}
	return nil
}
func (s *SQLiteStore) startHistoryTask(ctx context.Context, id int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM history_tasks WHERE task_id=?", id).Scan(&count)
	if err != nil || count == 0 {
		return err == nil, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE history_control SET last_started=? WHERE id=1 AND last_started<=? AND EXISTS(SELECT 1 FROM history_tasks h JOIN history_previews p ON p.id=h.preview_id WHERE h.task_id=? AND p.paused=0 AND p.state='confirmed') AND NOT EXISTS(SELECT 1 FROM history_tasks h JOIN pipeline_tasks t ON t.id=h.task_id WHERE t.status='running')`, time.Now().Unix(), time.Now().Unix()-60, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
