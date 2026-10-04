package recordingcatalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrRecordingActive = errors.New("来源仍在录制")
var ErrCatalogPending = errors.New("来源目录尚未核验")
var activeSources = struct {
	sync.Mutex
	paths map[string]int
}{paths: make(map[string]int)}

// BeginRecording 在下载器写入期间保护文件及录播姬的分段路径。
func BeginRecording(path string) func() {
	path, _ = filepath.Abs(path)
	activeSources.Lock()
	activeSources.paths[path]++
	activeSources.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			activeSources.Lock()
			defer activeSources.Unlock()
			activeSources.paths[path]--
			if activeSources.paths[path] == 0 {
				delete(activeSources.paths, path)
			}
		})
	}
}
func IsRecording(path string) bool {
	path, _ = filepath.Abs(path)
	activeSources.Lock()
	defer activeSources.Unlock()
	for source := range activeSources.paths {
		if path == source {
			return true
		}
		ext := filepath.Ext(source)
		if filepath.Dir(path) == filepath.Dir(source) && strings.HasPrefix(path, strings.TrimSuffix(source, ext)+"_PART") && strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
func SnapshotVersion(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("来源不是普通文件")
	}
	identity, err := fileIdentity(path, info)
	if err != nil {
		return "", err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("来源核验期间变化")
	}
	return version(identity, info.Size(), info.ModTime().UnixNano()), nil
}
func (s *Store) PreparationEntry(ctx context.Context, path, expected string) (Entry, error) {
	var e Entry
	if IsRecording(path) {
		return e, ErrRecordingActive
	}
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return e, err
	}
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return e, err
	}
	rel, err := filepath.Rel(root, actual)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return e, fmt.Errorf("来源越过录播根目录")
	}
	current, err := SnapshotVersion(path)
	if err != nil {
		return e, err
	}
	if current != expected || expected == "" {
		return e, fmt.Errorf("准备任务来源版本已变化")
	}
	lexical, err := filepath.Rel(s.root, path)
	if err != nil {
		return e, err
	}
	e, err = s.Lookup(ctx, filepath.ToSlash(lexical))
	if err != nil || !e.Present {
		return Entry{}, ErrCatalogPending
	}
	// 已发布主产物的来源版本与自身文件版本不同；本阶段只接受目录原始来源。
	if e.SourceVersion != expected {
		return Entry{}, ErrCatalogPending
	}
	return e, nil
}
func (m *Manager) PreparationEntry(ctx context.Context, path, expected string) (Entry, error) {
	return m.store.PreparationEntry(ctx, path, expected)
}
func (m *Manager) RegisterDerived(ctx context.Context, id, sourceVersion, assetVersion, rel, kind string) error {
	err := m.store.RegisterDerived(ctx, id, sourceVersion, assetVersion, rel, kind)
	if err == nil {
		m.Notify()
	}
	return err
}
func (m *Manager) AssetRoot() string { return m.store.assetRoot }

func (m *Manager) Reconcile(ctx context.Context) error { return m.store.Reconcile(ctx) }

// RegisterPreparedMain 接受阶段已验证的同房间主文件；复核目录和真实版本后提交资产。
func (m *Manager) RegisterPreparedMain(ctx context.Context, entry Entry, path, assetVersion string) error {
	s := m.store
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	current, err := SnapshotVersion(path)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sourceVersion, identity, id string
	var size, mtime int64
	if err := tx.QueryRowContext(ctx, "SELECT source_version FROM recordings WHERE id=?", entry.RecordingID).Scan(&sourceVersion); err != nil {
		return err
	}
	if sourceVersion != entry.SourceVersion {
		return fmt.Errorf("主资产来源版本变化")
	}
	if err := tx.QueryRowContext(ctx, "SELECT recording_id,file_id,size,mtime_ns FROM recording_aliases WHERE path=? AND present=1", rel).Scan(&id, &identity, &size, &mtime); err != nil {
		return err
	}
	if id != entry.RecordingID || current != version(identity, size, mtime) {
		return fmt.Errorf("主资产目录身份变化")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media_assets VALUES(?,?,?,?,'main','verified',?,?) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,asset_version=excluded.asset_version,kind='main',status='verified',size=excluded.size,mtime_ns=excluded.mtime_ns`, rel, id, sourceVersion, assetVersion, size, mtime)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyPreparedAsset 复用完成任务时同时核验媒体身份、当前来源与目录归属。
func (m *Manager) VerifyPreparedAsset(ctx context.Context, entry Entry, path, expectedVersion, profileVersion string) error {
	s := m.store
	rel, err := s.preparedPath(path)
	if err != nil {
		return err
	}
	if strings.HasPrefix(rel, "@derived/") {
		if _, err = s.assetPath(strings.TrimPrefix(rel, "@derived/")); err != nil {
			return err
		}
	}
	var proofID, proofSource, proofProfile, proofFile string
	var valid int
	if err := s.db.QueryRowContext(ctx, "SELECT recording_id,source_version,profile_version,file_version,valid FROM prepared_verifications WHERE path=?", rel).Scan(&proofID, &proofSource, &proofProfile, &proofFile, &valid); err != nil {
		return err
	}
	if proofID != entry.RecordingID || proofSource != entry.SourceVersion || proofProfile != profileVersion || proofFile != expectedVersion || valid != 1 {
		return fmt.Errorf("准备资产证明已失效")
	}

	var id, sourceVersion, status, assetVersion string
	var size, mtime int64
	err = s.db.QueryRowContext(ctx, "SELECT a.recording_id,a.source_version,a.status,a.size,a.mtime_ns,a.asset_version FROM media_assets a JOIN recordings r ON a.recording_id=r.id WHERE a.path=? AND a.source_version=r.source_version", rel).Scan(&id, &sourceVersion, &status, &size, &mtime, &assetVersion)
	if err != nil {
		return err
	}
	if id != entry.RecordingID || sourceVersion != entry.SourceVersion || status != "verified" {
		return fmt.Errorf("准备产物目录关联已变化")
	}
	if strings.HasPrefix(rel, "@derived/") && assetVersion != entry.SourceVersion+"-"+profileVersion {
		return fmt.Errorf("准备产物规则版本变化")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	current, err := SnapshotVersion(path)
	if err != nil {
		return err
	}
	if current != expectedVersion || info.Size() != size || info.ModTime().UnixNano() != mtime {
		return fmt.Errorf("准备产物版本已变化")
	}
	return nil
}

// InvalidatePreparedAsset 失败只撤销本来源的可信标记，不删除产物或触碰其他录播关联。
func (m *Manager) InvalidatePreparedAsset(ctx context.Context, entry Entry, path string) error {
	s := m.store
	rel, err := s.preparedPath(path)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE prepared_verifications SET valid=0 WHERE path=? AND recording_id=? AND source_version=?", rel, entry.RecordingID, entry.SourceVersion); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, "UPDATE media_assets SET status='unverified' WHERE path=? AND recording_id=? AND source_version=?", rel, entry.RecordingID, entry.SourceVersion)
	if err != nil {
		return err
	}
	return tx.Commit()
}
