package configs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLegacyAppData(t *testing.T, outPutPath string) {
	t.Helper()
	dbDir := filepath.Join(outPutPath, ".appdata", "db")
	require.NoError(t, os.MkdirAll(dbDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dbDir, "lives.db"), []byte("fake-db"), 0644))
	// 附加文件，确保 copyDir 递归正确
	thumbDir := filepath.Join(outPutPath, ".appdata", "thumbnails")
	require.NoError(t, os.MkdirAll(thumbDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(thumbDir, "cover.jpg"), []byte("fake-jpg"), 0644))
}

// 分支 a：旧路径有 lives.db，新路径空 → 迁移成功
func TestMigrateAppDataDir_Migrate(t *testing.T) {
	outDir := t.TempDir()
	appDir := t.TempDir()

	setupLegacyAppData(t, outDir)

	err := MigrateAppDataDir(outDir, appDir)
	require.NoError(t, err)

	// 新路径应有 lives.db
	assert.True(t, fileExists(filepath.Join(appDir, "db", "lives.db")))
	// 新路径应有 thumbnails
	assert.True(t, fileExists(filepath.Join(appDir, "thumbnails", "cover.jpg")))
	// 哨兵文件应存在
	assert.True(t, fileExists(filepath.Join(appDir, ".migrated")))
	// 旧 .appdata 目录应已删除
	assert.False(t, fileExists(filepath.Join(outDir, ".appdata")))
}

// 分支 b：旧路径无 lives.db → 跳过，无错误
func TestMigrateAppDataDir_NoOldData(t *testing.T) {
	outDir := t.TempDir()
	appDir := t.TempDir()

	err := MigrateAppDataDir(outDir, appDir)
	assert.NoError(t, err)
	// 新路径应无任何文件
	assert.False(t, fileExists(filepath.Join(appDir, ".migrated")))
	assert.False(t, fileExists(filepath.Join(appDir, "db")))
}

// 分支 c：新旧路径都有 lives.db → 返回错误（冲突）
func TestMigrateAppDataDir_Conflict(t *testing.T) {
	outDir := t.TempDir()
	appDir := t.TempDir()

	setupLegacyAppData(t, outDir)
	// 也在新路径放 lives.db
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "db"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "db", "lives.db"), []byte("existing"), 0644))

	err := MigrateAppDataDir(outDir, appDir)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "冲突"),
		"错误信息应包含「冲突」，实际: %v", err)
}

// 分支 d：哨兵文件已存在 → 跳过（幂等）
func TestMigrateAppDataDir_AlreadyMigrated(t *testing.T) {
	outDir := t.TempDir()
	appDir := t.TempDir()

	setupLegacyAppData(t, outDir)
	// 先写哨兵文件
	require.NoError(t, os.WriteFile(filepath.Join(appDir, ".migrated"), []byte("ok"), 0644))

	err := MigrateAppDataDir(outDir, appDir)
	assert.NoError(t, err)
	// 旧路径应保持不变（未删除）
	assert.True(t, fileExists(filepath.Join(outDir, ".appdata", "db", "lives.db")))
}
