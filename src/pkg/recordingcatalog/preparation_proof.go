package recordingcatalog

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// v1 原表不改写；v2 新增准备阶段的完整文件身份及规则证明。
const preparationSchemaV2 = `CREATE TABLE IF NOT EXISTS prepared_verifications (
 path TEXT PRIMARY KEY, recording_id TEXT NOT NULL, source_version TEXT NOT NULL,
 profile_version TEXT NOT NULL, file_version TEXT NOT NULL, valid INTEGER NOT NULL
); PRAGMA user_version=2;`

func (s *Store) preparedPath(path string) (string, error) {
	for _, candidate := range []struct{ root, prefix string }{{s.assetRoot, "@derived/"}, {s.root, ""}} {
		rel, err := filepath.Rel(candidate.root, path)
		if err != nil {
			return "", err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return candidate.prefix + filepath.ToSlash(rel), nil
	}
	return "", fmt.Errorf("准备产物越过资产目录")
}

func (m *Manager) RecordPreparedVerification(ctx context.Context, entry Entry, path, fileVersion, profileVersion string) error {
	s := m.store
	rel, err := s.preparedPath(path)
	if err != nil {
		return err
	}
	current, err := SnapshotVersion(path)
	if err != nil {
		return err
	}
	if current != fileVersion {
		return fmt.Errorf("记录准备证明前产物变化")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_assets a JOIN recordings r ON a.recording_id=r.id WHERE a.path=? AND a.recording_id=? AND a.source_version=? AND r.source_version=a.source_version", rel, entry.RecordingID, entry.SourceVersion).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("准备证明没有对应当前来源的资产")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO prepared_verifications VALUES(?,?,?,?,?,1) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,profile_version=excluded.profile_version,file_version=excluded.file_version,valid=1`, rel, entry.RecordingID, entry.SourceVersion, profileVersion, fileVersion)
	if err != nil {
		return err
	}
	// 此次媒体核验已通过，失败后撤销的旧标记只有重新核验成功才恢复。
	if _, err = tx.ExecContext(ctx, "UPDATE media_assets SET status='verified' WHERE path=?", rel); err != nil {
		return err
	}
	current, err = SnapshotVersion(path)
	if err != nil {
		return err
	}
	if current != fileVersion {
		return fmt.Errorf("记录准备证明期间产物变化")
	}
	return tx.Commit()
}

func (s *Store) refreshPreparedVerifications(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT p.path,p.recording_id,p.source_version,p.file_version,p.valid,a.recording_id,a.source_version FROM prepared_verifications p JOIN media_assets a ON p.path=a.path`)
	if err != nil {
		return err
	}
	type proof struct {
		path, id, sourceVersion, fileVersion, assetID, assetSourceVersion string
		valid                                                             int
	}
	var proofs []proof
	for rows.Next() {
		var p proof
		if err := rows.Scan(&p.path, &p.id, &p.sourceVersion, &p.fileVersion, &p.valid, &p.assetID, &p.assetSourceVersion); err != nil {
			rows.Close()
			return err
		}
		proofs = append(proofs, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range proofs {
		path := filepath.Join(s.root, filepath.FromSlash(p.path))
		if strings.HasPrefix(p.path, "@derived/") {
			path = filepath.Join(s.assetRoot, filepath.FromSlash(strings.TrimPrefix(p.path, "@derived/")))
		}
		current, e := SnapshotVersion(path)
		if e != nil || p.valid != 1 || current != p.fileVersion || p.id != p.assetID || p.sourceVersion != p.assetSourceVersion {
			if _, err := tx.ExecContext(ctx, "UPDATE media_assets SET status='unverified' WHERE path=? AND status!='missing'", p.path); err != nil {
				return err
			}
		}
	}
	return nil
}
