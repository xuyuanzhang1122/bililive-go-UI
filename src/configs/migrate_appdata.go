package configs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

const migrateSentinel = ".migrated"

// MigrateAppDataDir 将旧版 OutPutPath/.appdata 迁移到独立 AppDataPath。
//
// 策略：
//   - 旧路径无 lives.db → 无操作（新部署或旧数据已迁）
//   - 新路径已有 lives.db → fatal，要求人工介入
//   - 哨兵文件存在 → 跳过（已完成迁移）
//   - 旧有、新空 → copyDir 整个 .appdata 到新位置，写哨兵、删旧目录
func MigrateAppDataDir(outPutPath, appDataPath string) error {
	oldDir := filepath.Join(outPutPath, ".appdata")
	oldDB := filepath.Join(oldDir, "db", "lives.db")
	newDB := filepath.Join(appDataPath, "db", "lives.db")

	// 旧路径不存在 lives.db → 无操作
	if !fileExists(oldDB) {
		return nil
	}

	// 哨兵存在 → 已迁移过，跳过
	if fileExists(filepath.Join(appDataPath, migrateSentinel)) {
		log.Infof("[migrate] 哨兵文件存在，跳过迁移")
		return nil
	}

	// 新旧路径都有 lives.db → 冲突
	if fileExists(newDB) {
		return fmt.Errorf("AppData 迁移冲突: 旧路径 %s 和新路径 %s 同时存在 lives.db; "+
			"请人工处理: 保留其中一个位置的 db/ 目录, 删除另一个后重启", oldDB, newDB)
	}

	// 执行迁移：copyDir oldDir → appDataPath
	log.Infof("[migrate] 开始迁移 AppData: %s → %s", oldDir, appDataPath)
	if err := copyDir(oldDir, appDataPath); err != nil {
		return fmt.Errorf("迁移 AppData 失败 (copyDir %s → %s): %w", oldDir, appDataPath, err)
	}

	// 写哨兵
	if err := os.WriteFile(filepath.Join(appDataPath, migrateSentinel), []byte("ok"), 0644); err != nil {
		return fmt.Errorf("写哨兵文件失败: %w", err)
	}

	// 删旧目录
	if err := os.RemoveAll(oldDir); err != nil {
		log.Warnf("[migrate] 删除旧 AppData 目录失败 (数据已迁，可手动删除): %s: %v", oldDir, err)
	} else {
		log.Infof("[migrate] 已删除旧 AppData 目录: %s", oldDir)
	}

	log.Infof("[migrate] 迁移完成: %s → %s", oldDir, appDataPath)
	return nil
}

// copyDir 递归复制目录。跨设备时 os.Rename 会报 EXDEV，用此函数兜底。
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}
