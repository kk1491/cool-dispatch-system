package database

import (
	"fmt"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/logger"
	"cool-dispatch/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 负责根据配置建立 GORM 连接，并设置基础连接池参数。
func Open(cfg config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
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
	if err := db.AutoMigrate(models.AutoMigrateModels()...); err != nil {
		return err
	}
	return ensureSoftDeleteUniqueIndexes(db)
}

// MustMigrate 在迁移失败时直接终止进程，适用于启动即要求结构一致的场景。
func MustMigrate(db *gorm.DB) {
	if err := Migrate(db); err != nil {
		logger.Fatalf("auto-migrate failed: %v", err)
	}
}
