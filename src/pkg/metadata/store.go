// Package metadata 提供程序元数据的持久化存储
// 用于存储设备标识、配置信息、升级状态等关键数据
// 这些数据需要在程序中断后仍能保持完整
package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	// globalStore 全局元数据存储实例
	globalStore *Store
	// storeMu 保护全局存储实例
	storeMu sync.RWMutex
)

// Store 元数据存储
type Store struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

const DefaultAPIKeyUserID = "default"

func ensureAPIKeyUserSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS api_key_users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_suffix TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME,
			revoked_at DATETIME
		)
	`)
	if err != nil {
		return fmt.Errorf("创建 API Key 用户表失败: %w", err)
	}
	return nil
}

func ensureWatchHistorySchema(db *sql.DB) error {
	var tableName string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='watch_history'").Scan(&tableName)
	if err == sql.ErrNoRows {
		_, err = db.Exec(`
			CREATE TABLE watch_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				api_key_user_id TEXT NOT NULL DEFAULT 'default',
				video_path TEXT NOT NULL,
				video_name TEXT DEFAULT '',
				position_seconds REAL DEFAULT 0,
				duration_seconds REAL DEFAULT 0,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(api_key_user_id, video_path)
			)
		`)
		if err != nil {
			return fmt.Errorf("创建观看历史表失败: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查观看历史表失败: %w", err)
	}

	columns, err := tableColumns(db, "watch_history")
	if err != nil {
		return err
	}
	if columns["api_key_user_id"] {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE watch_history_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_key_user_id TEXT NOT NULL DEFAULT 'default',
			video_path TEXT NOT NULL,
			video_name TEXT DEFAULT '',
			position_seconds REAL DEFAULT 0,
			duration_seconds REAL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(api_key_user_id, video_path)
		)
	`); err != nil {
		return fmt.Errorf("创建观看历史迁移表失败: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO watch_history_new (id, api_key_user_id, video_path, video_name, position_seconds, duration_seconds, updated_at)
		SELECT id, 'default', video_path, video_name, position_seconds, duration_seconds, updated_at
		FROM watch_history
	`); err != nil {
		return fmt.Errorf("迁移观看历史失败: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE watch_history`); err != nil {
		return fmt.Errorf("删除旧观看历史表失败: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE watch_history_new RENAME TO watch_history`); err != nil {
		return fmt.Errorf("重命名观看历史表失败: %w", err)
	}
	return tx.Commit()
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, fmt.Errorf("读取表结构失败: %w", err)
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// Init 初始化全局元数据存储
// dbDir 应该是 AppDataPath/db 目录
func Init(dbDir string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	if globalStore != nil {
		return nil // 已经初始化
	}

	dbPath := filepath.Join(dbDir, "metadata.db")

	// 确保目录存在
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 打开数据库
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}

	// 设置 SQLite 优化参数
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	_, _ = db.Exec("PRAGMA synchronous=NORMAL")

	// 创建 key-value 表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			namespace TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			created_at INTEGER DEFAULT (strftime('%s', 'now')),
			updated_at INTEGER DEFAULT (strftime('%s', 'now')),
			PRIMARY KEY (namespace, key)
		)
	`)
	if err != nil {
		db.Close()
		return fmt.Errorf("创建表失败: %w", err)
	}

	if err := ensureAPIKeyUserSchema(db); err != nil {
		db.Close()
		return err
	}

	if err := ensureWatchHistorySchema(db); err != nil {
		db.Close()
		return err
	}

	globalStore = &Store{
		db:     db,
		dbPath: dbPath,
	}

	return nil
}

// GetStore 获取全局元数据存储实例
// 如果未初始化，返回 nil
func GetStore() *Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return globalStore
}

// Close 关闭全局元数据存储
func Close() error {
	storeMu.Lock()
	defer storeMu.Unlock()

	if globalStore == nil {
		return nil
	}

	err := globalStore.db.Close()
	globalStore = nil
	return err
}

// Get 从指定命名空间获取值
func (s *Store) Get(ctx context.Context, namespace, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	err := s.db.QueryRowContext(ctx,
		"SELECT value FROM metadata WHERE namespace = ? AND key = ?",
		namespace, key,
	).Scan(&value)

	if err == sql.ErrNoRows {
		return "", nil // 键不存在返回空字符串
	}
	if err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}
	return value, nil
}

// Set 在指定命名空间设置值
func (s *Store) Set(ctx context.Context, namespace, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metadata (namespace, key, value, updated_at) 
		 VALUES (?, ?, ?, strftime('%s', 'now'))
		 ON CONFLICT(namespace, key) DO UPDATE SET 
		 value = excluded.value, 
		 updated_at = strftime('%s', 'now')`,
		namespace, key, value,
	)
	if err != nil {
		return fmt.Errorf("保存失败: %w", err)
	}
	return nil
}

// Delete 从指定命名空间删除键
func (s *Store) Delete(ctx context.Context, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		"DELETE FROM metadata WHERE namespace = ? AND key = ?",
		namespace, key,
	)
	if err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

// GetAll 获取指定命名空间的所有键值对
func (s *Store) GetAll(ctx context.Context, namespace string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx,
		"SELECT key, value FROM metadata WHERE namespace = ?",
		namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("读取行失败: %w", err)
		}
		result[key] = value
	}

	return result, rows.Err()
}

// DeleteNamespace 删除整个命名空间
func (s *Store) DeleteNamespace(ctx context.Context, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		"DELETE FROM metadata WHERE namespace = ?",
		namespace,
	)
	if err != nil {
		return fmt.Errorf("删除命名空间失败: %w", err)
	}
	return nil
}

// 预定义的命名空间常量
const (
	// NamespaceDevice 设备相关信息（如 Sentry 设备 ID）
	NamespaceDevice = "device"
	// NamespaceConfig 配置信息
	NamespaceConfig = "config"
	// NamespaceUpdate 升级相关状态
	NamespaceUpdate = "update"
	// NamespaceMigration 数据库迁移状态
	NamespaceMigration = "migration"
)

// 预定义的键常量
const (
	// KeyDeviceID Sentry 设备标识
	KeyDeviceID = "sentry_device_id"
)

// WatchHistoryEntry 观看历史记录
type WatchHistoryEntry struct {
	ID              int64   `json:"id"`
	APIKeyUserID    string  `json:"api_key_user_id,omitempty"`
	VideoPath       string  `json:"video_path"`
	VideoName       string  `json:"video_name"`
	PositionSeconds float64 `json:"position_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
	UpdatedAt       string  `json:"updated_at"`
}

// UpsertWatchHistory 插入或更新观看历史
func (s *Store) UpsertWatchHistory(ctx context.Context, e *WatchHistoryEntry) error {
	return s.UpsertWatchHistoryForUser(ctx, DefaultAPIKeyUserID, e)
}

func (s *Store) UpsertWatchHistoryForUser(ctx context.Context, userID string, e *WatchHistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(userID) == "" {
		userID = DefaultAPIKeyUserID
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO watch_history (api_key_user_id, video_path, video_name, position_seconds, duration_seconds, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(api_key_user_id, video_path) DO UPDATE SET
			video_name = excluded.video_name,
			position_seconds = excluded.position_seconds,
			duration_seconds = excluded.duration_seconds,
			updated_at = CURRENT_TIMESTAMP
	`, userID, e.VideoPath, e.VideoName, e.PositionSeconds, e.DurationSeconds)
	return err
}

// GetWatchHistory 获取所有观看历史（按更新时间倒序）
func (s *Store) GetWatchHistory(ctx context.Context) ([]WatchHistoryEntry, error) {
	return s.GetWatchHistoryForUser(ctx, DefaultAPIKeyUserID)
}

func (s *Store) GetWatchHistoryForUser(ctx context.Context, userID string) ([]WatchHistoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(userID) == "" {
		userID = DefaultAPIKeyUserID
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, api_key_user_id, video_path, video_name, position_seconds, duration_seconds, updated_at
		FROM watch_history
		WHERE api_key_user_id = ?
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []WatchHistoryEntry
	for rows.Next() {
		var e WatchHistoryEntry
		if err := rows.Scan(&e.ID, &e.APIKeyUserID, &e.VideoPath, &e.VideoName, &e.PositionSeconds, &e.DurationSeconds, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) GetWatchHistoryItemForUser(ctx context.Context, userID, videoPath string) (*WatchHistoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(userID) == "" {
		userID = DefaultAPIKeyUserID
	}

	var e WatchHistoryEntry
	err := s.db.QueryRowContext(ctx, `
		SELECT id, api_key_user_id, video_path, video_name, position_seconds, duration_seconds, updated_at
		FROM watch_history
		WHERE api_key_user_id = ? AND video_path = ?
	`, userID, videoPath).Scan(&e.ID, &e.APIKeyUserID, &e.VideoPath, &e.VideoName, &e.PositionSeconds, &e.DurationSeconds, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteWatchHistory 删除单条观看历史
func (s *Store) DeleteWatchHistory(ctx context.Context, videoPath string) error {
	return s.DeleteWatchHistoryForUser(ctx, DefaultAPIKeyUserID, videoPath)
}

func (s *Store) DeleteWatchHistoryForUser(ctx context.Context, userID, videoPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(userID) == "" {
		userID = DefaultAPIKeyUserID
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM watch_history WHERE api_key_user_id = ? AND video_path = ?", userID, videoPath)
	return err
}
