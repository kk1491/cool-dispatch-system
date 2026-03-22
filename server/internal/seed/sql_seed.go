package seed

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// RunSQLSeedFiles 扫描指定目录下所有 .sql 文件并按文件名排序依次执行，
// 用于通过 SQL 文件写入默认数据。仅执行文件名包含 "seed" 的 SQL 文件，
// 避免误执行 DDL 迁移脚本。
// RunSQLSeedFiles 只执行文件名包含 seed 的 SQL 脚本，避免把 DDL 迁移误当成种子脚本执行。
func RunSQLSeedFiles(db *gorm.DB, dir string) error {
	// 如果目录不存在，静默跳过（向下兼容）
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Printf("[seed] SQL 种子目录不存在或非目录，跳过: %s", dir)
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read seed directory: %w", err)
	}

	// 收集所有符合条件的 .sql 文件
	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		// 仅执行文件名包含 "seed" 的脚本，避免误执行 DDL 建表脚本
		if !strings.Contains(strings.ToLower(name), "seed") {
			continue
		}
		sqlFiles = append(sqlFiles, name)
	}

	if len(sqlFiles) == 0 {
		log.Printf("[seed] 未找到种子 SQL 文件: %s", dir)
		return nil
	}

	// 按文件名排序，保证执行顺序稳定
	sort.Strings(sqlFiles)

	for _, name := range sqlFiles {
		filePath := filepath.Join(dir, name)
		log.Printf("[seed] 正在执行 SQL 种子文件: %s", filePath)

		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read sql file %s: %w", name, err)
		}

		sql := strings.TrimSpace(string(content))
		if sql == "" {
			log.Printf("[seed] SQL 文件为空，跳过: %s", name)
			continue
		}

		// 在单个事务中执行整个 SQL 文件
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("exec sql file %s: %w", name, err)
		}

		log.Printf("[seed] SQL 种子文件执行成功: %s", name)
	}

	return nil
}
