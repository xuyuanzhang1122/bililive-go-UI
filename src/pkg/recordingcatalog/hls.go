package recordingcatalog

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type HLSAsset struct {
	Entry             Entry
	Path, FileVersion string
	ManifestDigest    string
}

// RegisterHLS 在一个事务内提交整组已核验文件及完整身份，不把分片字节混入主录播大小。
func (m *Manager) RegisterHLS(ctx context.Context, entry Entry, profileVersion, digest string, proofs map[string]string) error {
	s := m.store
	if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != 32 {
		return fmt.Errorf("HLS 关联摘要无效")
	}
	if len(proofs) < 2 || len(proofs) > 30000 || (profileVersion != "hls-source-v1" && profileVersion != "hls-720p-v1" && profileVersion != "hls-1080p-v1") {
		return fmt.Errorf("非法持久 HLS 集合")
	}
	assetVersion := entry.SourceVersion + "-" + profileVersion
	prefix := entry.RecordingID + "/" + assetVersion + "/"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, "SELECT source_version FROM recordings WHERE id=?", entry.RecordingID).Scan(&current); err != nil {
		return err
	}
	if current != entry.SourceVersion {
		return fmt.Errorf("HLS 来源版本变化")
	}
	for path, expected := range proofs {
		rel, err := s.preparedPath(path)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(rel, "@derived/"+prefix) {
			return fmt.Errorf("HLS 文件越过版本目录")
		}
		if _, err := s.assetPath(strings.TrimPrefix(rel, "@derived/")); err != nil {
			return err
		}
		version, err := SnapshotVersion(path)
		if err != nil {
			return err
		}
		if version != expected {
			return fmt.Errorf("HLS 证明提交期间文件变化")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO media_assets VALUES(?,?,?,?,'hls','verified',?,?) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,asset_version=excluded.asset_version,kind='hls',status='verified',size=excluded.size,mtime_ns=excluded.mtime_ns`, rel, entry.RecordingID, entry.SourceVersion, assetVersion, info.Size(), info.ModTime().UnixNano()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO prepared_verifications VALUES(?,?,?,?,?,1) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,profile_version=excluded.profile_version,file_version=excluded.file_version,valid=1`, rel, entry.RecordingID, entry.SourceVersion, profileVersion+":"+digest, expected); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.Notify()
	return nil
}

func (m *Manager) FindHLS(ctx context.Context, id, assetVersion string) (HLSAsset, error) {
	var asset HLSAsset
	var rel string
	var proofProfile string
	err := m.store.db.QueryRowContext(ctx, `SELECT a.path,a.recording_id,a.source_version,p.file_version,p.profile_version FROM media_assets a JOIN recordings r ON a.recording_id=r.id JOIN prepared_verifications p ON p.path=a.path WHERE a.recording_id=? AND a.asset_version=? AND a.kind='hls' AND a.status='verified' AND a.source_version=r.source_version AND p.valid=1 AND p.recording_id=a.recording_id AND p.source_version=a.source_version AND a.path=?`, id, assetVersion, "@derived/"+id+"/"+assetVersion+"/index.m3u8").Scan(&rel, &asset.Entry.RecordingID, &asset.Entry.SourceVersion, &asset.FileVersion, &proofProfile)
	if err != nil {
		return asset, err
	}
	profile, digest, ok := strings.Cut(proofProfile, ":")
	if !ok || assetVersion != asset.Entry.SourceVersion+"-"+profile || len(digest) != 64 {
		return asset, fmt.Errorf("HLS 关联证明不符")
	}
	asset.ManifestDigest = digest
	asset.Path, err = m.store.assetPath(strings.TrimPrefix(rel, "@derived/"))
	return asset, err
}

func (m *Manager) HLSForPath(ctx context.Context, rel, profile string) (HLSAsset, error) {
	if profile != "source" && profile != "720p" && profile != "1080p" {
		return HLSAsset{}, fmt.Errorf("未知 HLS 档位")
	}
	e, err := m.store.Lookup(ctx, filepath.ToSlash(rel))
	if err != nil {
		return HLSAsset{}, err
	}
	return m.FindHLS(ctx, e.RecordingID, e.SourceVersion+"-hls-"+profile+"-v1")
}

// HLSSource 从该录播的现存目录别名中选择原始来源，不把旧路径或主产物当作源身份。
func (m *Manager) HLSSource(ctx context.Context, entry Entry) (Entry, error) {
	rows, err := m.store.db.QueryContext(ctx, `SELECT a.path,a.file_id,a.size,a.mtime_ns FROM recording_aliases a JOIN recordings r ON a.recording_id=r.id WHERE a.recording_id=? AND a.present=1 AND r.source_version=? ORDER BY a.path`, entry.RecordingID, entry.SourceVersion)
	if err != nil {
		return Entry{}, err
	}
	var candidates []string
	for rows.Next() {
		var path, identity string
		var size, mtime int64
		if err := rows.Scan(&path, &identity, &size, &mtime); err != nil {
			rows.Close()
			return Entry{}, err
		}
		if version(identity, size, mtime) == entry.SourceVersion {
			candidates = append(candidates, path)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Entry{}, err
	}
	for _, rel := range candidates {
		current, err := m.PreparationEntry(ctx, filepath.Join(m.store.root, filepath.FromSlash(rel)), entry.SourceVersion)
		if err == nil && current.RecordingID == entry.RecordingID {
			return current, nil
		}
		if err == ErrRecordingActive {
			return Entry{}, err
		}
	}
	return Entry{}, fmt.Errorf("持久 HLS 没有同身份及版本的现存来源")
}
