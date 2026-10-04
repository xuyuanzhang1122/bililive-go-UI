package recordingcatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type deletionFile struct{ rel, path, version string }

// DeleteRecording 只删除客户端明确指定版本的已登记普通文件，不递归清理未知文件。
// 返回实际删除数；遇到后续 I/O 故障可能部分成功，调用者必须逐项报告。
func (m *Manager) DeleteRecording(ctx context.Context, id, source string) (deleted int, resultErr error) {
	s := m.store
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := m.PlaybackRecording(ctx, id)
	if err != nil {
		return 0, err
	}
	if record.SourceVersion != source {
		return 0, ErrPlaybackChanged
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path,file_id,size,mtime_ns FROM recording_aliases WHERE recording_id=? AND present=1 ORDER BY path`, id)
	if err != nil {
		return 0, err
	}
	files := make(map[string]deletionFile)
	var sources []string
	for rows.Next() {
		var rel, identity string
		var size, mtime int64
		if err := rows.Scan(&rel, &identity, &size, &mtime); err != nil {
			rows.Close()
			return 0, err
		}
		path := filepath.Join(s.root, filepath.FromSlash(rel))
		files[rel] = deletionFile{rel, path, version(identity, size, mtime)}
		sources = append(sources, path)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT a.path,a.asset_version,COALESCE(p.file_version,'') FROM media_assets a LEFT JOIN prepared_verifications p ON p.path=a.path AND p.recording_id=a.recording_id AND p.source_version=a.source_version AND p.valid=1 WHERE a.recording_id=? AND a.source_version=? AND a.kind!='main' AND a.status!='missing' ORDER BY a.path`, id, source)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var rel, asset, fv string
		if err := rows.Scan(&rel, &asset, &fv); err != nil {
			rows.Close()
			return 0, err
		}
		if fv == "" || !strings.HasPrefix(rel, "@derived/"+id+"/"+asset+"/") {
			rows.Close()
			return 0, ErrPlaybackChanged
		}
		files[rel] = deletionFile{rel, filepath.Join(s.assetRoot, filepath.FromSlash(strings.TrimPrefix(rel, "@derived/"))), fv}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(files) > 30064 {
		return 0, fmt.Errorf("删除文件数量超限")
	}
	check := func(f deletionFile) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, path := range sources {
			if IsRecording(path) {
				return ErrRecordingActive
			}
		}
		root, rel := s.root, f.rel
		if strings.HasPrefix(rel, "@derived/") {
			root = s.assetRoot
			rel = strings.TrimPrefix(rel, "@derived/")
		}
		path, err := boundedPlaybackPath(root, rel)
		if err != nil || path != f.path {
			return ErrPlaybackChanged
		}
		actual, err := SnapshotVersion(path)
		if err != nil || actual != f.version {
			return ErrPlaybackChanged
		}
		return nil
	}
	ordered := make([]deletionFile, 0, len(files))
	for _, f := range files {
		if err := check(f); err != nil {
			return 0, err
		}
		ordered = append(ordered, f)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].rel < ordered[j].rel })
	defer func() {
		if deleted > 0 {
			_, err := s.db.ExecContext(context.WithoutCancel(ctx), `UPDATE catalog_meta SET value='pending' WHERE key='status'`)
			if err != nil && resultErr == nil {
				resultErr = err
			}
			m.Notify()
		}
	}()
	for _, f := range ordered {
		if err := check(f); err != nil {
			return deleted, err
		}
		root, rel := s.root, f.rel
		if strings.HasPrefix(rel, "@derived/") {
			root, rel = s.assetRoot, strings.TrimPrefix(rel, "@derived/")
		}
		parent, err := openDeletionParent(root, rel)
		if err != nil {
			return deleted, err
		}
		err = withInactiveSources(sources, func() error {
			return removePinnedFile(parent, filepath.Base(f.path), f.version)
		})
		parent.Close()
		if err != nil {
			return deleted, err
		}
		deleted++
		tx, err := s.db.BeginTx(context.WithoutCancel(ctx), nil)
		if err != nil {
			return deleted, err
		}
		_, err = tx.Exec(`UPDATE recording_aliases SET present=0 WHERE path=? AND recording_id=?`, f.rel, id)
		if err == nil {
			_, err = tx.Exec(`UPDATE media_assets SET status='missing',size=0 WHERE path=? AND recording_id=? AND source_version=?`, f.rel, id, source)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE prepared_verifications SET valid=0 WHERE path=? AND recording_id=? AND source_version=?`, f.rel, id, source)
		}
		if err != nil {
			tx.Rollback()
			return deleted, err
		}
		if err := tx.Commit(); err != nil {
			return deleted, err
		}
	}
	return deleted, nil
}

func withInactiveSources(paths []string, action func() error) error {
	activeSources.Lock()
	defer activeSources.Unlock()
	for _, path := range paths {
		if isRecordingLocked(path) {
			return ErrRecordingActive
		}
	}
	return action()
}

func openDeletionParent(root, rel string) (*os.File, error) {
	path, err := boundedPlaybackPath(root, rel)
	if err != nil {
		return nil, err
	}
	rootBefore, err := os.Lstat(root)
	if err != nil || !rootBefore.IsDir() {
		return nil, ErrPlaybackChanged
	}
	parentBefore, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parentBefore.IsDir() {
		return nil, ErrPlaybackChanged
	}
	anchor, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer anchor.Close()
	rootInfo, err := anchor.Stat(".")
	if err != nil || !os.SameFile(rootBefore, rootInfo) {
		return nil, ErrPlaybackChanged
	}
	parent, err := anchor.Open(filepath.Dir(filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	info, err := parent.Stat()
	if err != nil || !os.SameFile(parentBefore, info) {
		parent.Close()
		return nil, ErrPlaybackChanged
	}
	return parent, nil
}
