package database

import (
	"fmt"
	"log"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 负责根据配置建立 GORM 连接，并设置基础连接池参数。
func Open(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)

	return db, nil
}

// Migrate 执行项目内全部模型的自动迁移。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(models.AutoMigrateModels()...)
}

// MustMigrate 在迁移失败时直接终止进程，适用于启动即要求结构一致的场景。
func MustMigrate(db *gorm.DB) {
	if err := Migrate(db); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}
}
