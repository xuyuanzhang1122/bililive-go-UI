package recordingcatalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db        *sql.DB
	dbPath    string
	root      string
	assetRoot string
	mu        sync.Mutex
}

func Open(dbPath, root string, assetRoots ...string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	assetRoot := filepath.Join(filepath.Dir(dbPath), "media-assets")
	if len(assetRoots) > 0 {
		assetRoot = assetRoots[0]
	}
	assetRoot, err = filepath.Abs(assetRoot)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, dbPath: dbPath, root: absolute, assetRoot: assetRoot}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initialize() error {
	var current int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > 2 {
		return fmt.Errorf("不支持的录播目录版本 %d", current)
	}
	if _, err := s.db.Exec("PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;"); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if current == 0 {
		if _, err = tx.Exec(schema); err != nil {
			return err
		}
	}
	if current < 2 {
		if _, err = tx.Exec(preparationSchemaV2); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO catalog_meta VALUES('root',?),('status','pending')", s.root); err != nil {
		return err
	}
	var stored string
	if err = tx.QueryRow("SELECT value FROM catalog_meta WHERE key='root'").Scan(&stored); err != nil {
		return err
	}
	if stored != s.root {
		return fmt.Errorf("录播目录数据库绑定了其他输出路径")
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) MatchesRoot(root string) bool {
	absolute, err := filepath.Abs(root)
	return err == nil && absolute == s.root
}

func (s *Store) Lookup(ctx context.Context, path string) (Entry, error) {
	var e Entry
	e.Path = path
	err := s.db.QueryRowContext(ctx, `SELECT a.recording_id,r.source_version,a.present FROM recording_aliases a JOIN recordings r ON a.recording_id=r.id WHERE path=?`, path).Scan(&e.RecordingID, &e.SourceVersion, &e.Present)
	return e, err
}

func (s *Store) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := scan(ctx, s.root, s.assetRoot)
	if err == nil {
		err = s.apply(ctx, files)
	}
	if err != nil {
		_, statusErr := s.db.ExecContext(context.WithoutCancel(ctx), "UPDATE catalog_meta SET value='unavailable' WHERE key='status'")
		if statusErr != nil {
			return fmt.Errorf("目录核验失败: %w; 状态更新失败: %v", err, statusErr)
		}
	}
	return err
}

func (s *Store) apply(ctx context.Context, files []observedFile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE recording_aliases SET present=0"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE media_assets SET status='unverified'"); err != nil {
		return err
	}
	// 必须先保存完整旧身份映射，再写路径；否则交换两个文件名会擦掉后一个身份。
	identityIDs := make(map[string]string)
	oldRows, err := tx.QueryContext(ctx, "SELECT file_id,recording_id FROM recording_aliases ORDER BY path")
	if err != nil {
		return err
	}
	for oldRows.Next() {
		var identity, id string
		if err = oldRows.Scan(&identity, &id); err != nil {
			oldRows.Close()
			return err
		}
		if identity != "" {
			identityIDs[identity] = id
		}
	}
	err = oldRows.Err()
	oldRows.Close()
	if err != nil {
		return err
	}
	ids := make(map[string]string)
	for _, f := range files {
		id := identityIDs[f.identity]
		if id == "" {
			generated, err := uuid.NewV4()
			if err != nil {
				return err
			}
			id = generated.String()
			identityIDs[f.identity] = id
		}

		if _, err = tx.ExecContext(ctx, `INSERT INTO recordings VALUES(?,?) ON CONFLICT(id) DO UPDATE SET source_version=excluded.source_version`, id, version(f.identity, f.size, f.mtime)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO recording_aliases VALUES(?,?,?,?,?,1,?) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,file_id=excluded.file_id,size=excluded.size,mtime_ns=excluded.mtime_ns,present=1,room=excluded.room`, f.path, id, f.identity, f.size, f.mtime, f.room); err != nil {
			return err
		}
		ids[f.path] = id
		// 原文件直接可播的准备结果没有跨文件 manifest；只在同身份、来源版本及字节快照仍匹配时保留验证状态。
		if _, err = tx.ExecContext(ctx, `UPDATE media_assets SET status='verified' WHERE path=? AND kind='main' AND recording_id=? AND source_version=? AND size=? AND mtime_ns=?`, f.path, id, version(f.identity, f.size, f.mtime), f.size, f.mtime); err != nil {
			return err
		}
	}
	// 可信来源关联在同一事务内把主产物并入原录播，缺失来源保留旧别名。
	for _, f := range files {
		if f.manifest == nil {
			continue
		}
		m := f.manifest
		source := filepath.ToSlash(filepath.Join(filepath.Dir(f.path), m.SourceName))
		id := ids[f.path]
		var candidate string
		var sourceIdentity string
		var size, mtime int64
		err = tx.QueryRowContext(ctx, "SELECT recording_id,file_id,size,mtime_ns FROM recording_aliases WHERE path=?", source).Scan(&candidate, &sourceIdentity, &size, &mtime)
		if err == nil && size == m.Source.Size && mtime == m.Source.ModTime {
			id = candidate
		} else if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == sql.ErrNoRows {
			if _, err = tx.ExecContext(ctx, `INSERT INTO recording_aliases VALUES(?,?,?, ?,?,0,?)`, source, id, "", m.Source.Size, m.Source.ModTime, f.room); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE recordings SET source_version=? WHERE id=?", version("", m.Source.Size, m.Source.ModTime), id); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE recording_aliases SET recording_id=? WHERE path=?", id, f.path); err != nil {
			return err
		}
		sourceVersion := version(sourceIdentity, m.Source.Size, m.Source.ModTime)
		if _, err = tx.ExecContext(ctx, "UPDATE recordings SET source_version=? WHERE id=?", sourceVersion, id); err != nil {
			return err
		}
		assetVersion := version(sourceVersion+":faststart-v1", f.size, f.mtime)
		if _, err = tx.ExecContext(ctx, `INSERT INTO media_assets(path,recording_id,source_version,asset_version,kind,status,size,mtime_ns) VALUES(?,?,?,?,'main','verified',?,?) ON CONFLICT(path) DO UPDATE SET recording_id=excluded.recording_id,source_version=excluded.source_version,asset_version=excluded.asset_version,status='verified',size=excluded.size,mtime_ns=excluded.mtime_ns`, f.path, id, sourceVersion, assetVersion, f.size, f.mtime); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE media_assets SET status='missing' WHERE kind='main' AND path NOT IN (SELECT path FROM recording_aliases WHERE present=1)`); err != nil {
		return err
	}
	if err = s.refreshDerived(ctx, tx); err != nil {
		return err
	}
	if err = s.refreshPreparedVerifications(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM room_statistics"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO room_statistics(folder,video_count,total_size,derived_size,latest_video,latest_at,checked_at)
 SELECT a.room,COUNT(DISTINCT a.recording_id),SUM(a.size),
 COALESCE((SELECT SUM(d.size) FROM media_assets d WHERE d.kind!='main' AND d.status!='missing' AND d.recording_id IN (SELECT DISTINCT c.recording_id FROM recording_aliases c WHERE c.room=a.room AND c.present=1)),0),
 (SELECT b.path FROM recording_aliases b WHERE b.room=a.room AND b.present=1 ORDER BY b.mtime_ns DESC,b.path LIMIT 1),
 MAX(a.mtime_ns)/1000000000,? FROM recording_aliases a WHERE a.present=1 GROUP BY a.room`, time.Now().Unix()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE catalog_meta SET value='verified' WHERE key='status'"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Rooms(ctx context.Context) ([]Room, error) {
	// 单个读事务确保状态和统计属于同一已提交快照，完全不访问媒体目录。
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT value FROM catalog_meta WHERE key='status'").Scan(&status); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT folder,video_count,total_size,derived_size,latest_video,latest_at,checked_at FROM room_statistics ORDER BY latest_at DESC,folder")
	if err != nil {
		return nil, err
	}
	rooms := make([]Room, 0)
	for rows.Next() {
		var r Room
		if err = rows.Scan(&r.FolderPath, &r.VideoCount, &r.TotalSize, &r.DerivedSize, &r.LatestVideo, &r.LatestAt, &r.CheckedAt); err != nil {
			rows.Close()
			return nil, err
		}
		parts := strings.SplitN(r.FolderPath, "/", 2)
		r.Platform = parts[0]
		r.HostName = parts[1]
		r.Status = status
		if status != "verified" {
			r.TotalSize = 0
			r.VideoCount = 0
			r.DerivedSize = 0
		}
		rooms = append(rooms, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return rooms, nil
}
