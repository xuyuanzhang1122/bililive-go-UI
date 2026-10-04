package recordingcatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveAlias 只读取同一已登记录播的现存版本，并复核一个候选文件的真实身份。
// 不按同名猜测、不扫描目录，绝不覆盖当前真实文件的入口。
func (s *Store) ResolveAlias(ctx context.Context, rel string) (string, error) {
	if strings.Contains(rel, "\\") || strings.HasPrefix(rel, "/") || filepath.ToSlash(filepath.Clean(rel)) != rel || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("非法录播别名")
	}
	if _, err := os.Lstat(filepath.Join(s.root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("真实文件已存在，拒绝别名覆盖")
	}
	var candidate, identity string
	var size, mtime int64
	err := s.db.QueryRowContext(ctx, `SELECT current.path,current.file_id,current.size,current.mtime_ns
 FROM recording_aliases old JOIN recording_aliases current ON old.recording_id=current.recording_id
 WHERE old.path=? AND old.present=0 AND current.present=1
 ORDER BY EXISTS(SELECT 1 FROM media_assets m WHERE m.path=current.path AND m.kind='main' AND m.status='verified') DESC,current.mtime_ns DESC,current.path LIMIT 1`, rel).Scan(&candidate, &identity, &size, &mtime)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.root, filepath.FromSlash(candidate))
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", err
	}
	bounded, err := filepath.Rel(root, actual)
	if err != nil || bounded == ".." || strings.HasPrefix(bounded, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("别名候选越过录播根目录")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != size || info.ModTime().UnixNano() != mtime {
		return "", fmt.Errorf("别名候选版本已变化")
	}
	current, err := fileIdentity(path, info)
	if err != nil {
		return "", err
	}
	if current != identity {
		return "", fmt.Errorf("别名候选身份已变化")
	}
	return candidate, nil
}

func (m *Manager) ResolveAlias(ctx context.Context, rel string) (string, error) {
	return m.store.ResolveAlias(ctx, rel)
}
