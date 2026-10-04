package recordingcatalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) assetPath(rel string) (string, error) {
	if strings.Contains(rel, "\\") || filepath.ToSlash(filepath.Clean(rel)) != rel || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("非法派生资产路径")
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 3 || parts[0] == ".." || parts[0] == "." {
		return "", fmt.Errorf("非法派生资产路径")
	}
	path := s.assetRoot
	// 所有目录项均拒绝符号链接，防止可信相对路径被链接引出根目录。
	for _, part := range append([]string{""}, parts...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("派生资产路径包含符号链接")
		}
	}
	return path, nil
}

// RegisterDerived 供已验证的准备阶段调用，不是客户端上传入口。
// 来源版本必须仍是当前目录版本；路径必须位于 ID/资产版本目录。
func (s *Store) RegisterDerived(ctx context.Context, id, sourceVersion, assetVersion, rel, kind string) error {
	if (kind != "hls" && kind != "compat") || strings.ContainsAny(assetVersion, "/\\") || assetVersion == "" || assetVersion == "." || assetVersion == ".." || !strings.HasPrefix(rel, id+"/"+assetVersion+"/") {
		return fmt.Errorf("非法派生资产描述")
	}
	path, err := s.assetPath(rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("派生资产不是普通文件")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err = tx.QueryRowContext(ctx, "SELECT source_version FROM recordings WHERE id=?", id).Scan(&current); err != nil {
		return err
	}
	if current != sourceVersion {
		return fmt.Errorf("派生资产来源版本已变化")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO media_assets VALUES(?,?,?,?,?,'verified',?,?)
 ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,asset_version=excluded.asset_version,kind=excluded.kind,status='verified',size=excluded.size,mtime_ns=excluded.mtime_ns`, "@derived/"+rel, id, sourceVersion, assetVersion, kind, info.Size(), info.ModTime().UnixNano())
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) refreshDerived(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT a.path,a.size,a.mtime_ns,a.source_version,r.source_version FROM media_assets a JOIN recordings r ON a.recording_id=r.id WHERE a.kind!='main'")
	if err != nil {
		return err
	}
	type asset struct {
		path                          string
		size, mtime                   int64
		sourceVersion, currentVersion string
	}
	var assets []asset
	for rows.Next() {
		var a asset
		if err = rows.Scan(&a.path, &a.size, &a.mtime, &a.sourceVersion, &a.currentVersion); err != nil {
			rows.Close()
			return err
		}
		assets = append(assets, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, a := range assets {
		path, err := s.assetPath(strings.TrimPrefix(a.path, "@derived/"))
		if os.IsNotExist(err) {
			if _, err = tx.ExecContext(ctx, "UPDATE media_assets SET status='missing',size=0 WHERE path=?", a.path); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("派生资产类型变化")
		}
		status := "unverified"
		if info.Size() == a.size && info.ModTime().UnixNano() == a.mtime && a.sourceVersion == a.currentVersion {
			status = "verified"
		}
		if _, err = tx.ExecContext(ctx, "UPDATE media_assets SET status=?,size=? WHERE path=?", status, info.Size(), a.path); err != nil {
			return err
		}
	}
	return nil
}
