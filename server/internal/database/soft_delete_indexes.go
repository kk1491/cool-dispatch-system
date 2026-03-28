package database

import (
	"fmt"

	"gorm.io/gorm"
)

// ensureSoftDeleteUniqueIndexes 为启用软删除的表补齐“仅对未删除记录生效”的唯一索引。
// 这样技师手机号、客户 LINE UID 在旧记录进入回收站后仍可被新的有效记录复用。
func ensureSoftDeleteUniqueIndexes(db *gorm.DB) error {
	statements := []string{
		`DROP INDEX IF EXISTS idx_users_phone`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone_active ON users (phone) WHERE deleted_at IS NULL`,
		`DROP INDEX IF EXISTS idx_customers_line_uid`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_line_uid_active ON customers (line_uid) WHERE deleted_at IS NULL AND line_uid IS NOT NULL`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("ensure soft-delete indexes: %w", err)
		}
	}

	return nil
}
