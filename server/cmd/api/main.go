package main

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/database"
	"cool-dispatch/internal/httpapi"
	"cool-dispatch/internal/seed"
)

// main 负责串联配置、数据库、种子数据与 HTTP 服务启动流程。
func main() {
	cfg := config.Load()

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	// 无条件执行数据库表结构迁移，确保新增字段、索引等变更在启动时自动同步，
	// 避免因表结构不一致导致运行时查询或写入失败。
	database.MustMigrate(db)

	// 无条件同步 config.yaml 中的管理员账号配置到数据库
	// 管理员更新不依赖 seed_demo_data 开关，确保配置变更后重启即生效
	if err := seed.SyncAdminFromConfig(db, cfg); err != nil {
		log.Fatalf("sync admin from config failed: %v", err)
	}

	// 无条件同步 config.yaml 中的开发默认师傅账号到数据库
	// 保证开发人员始终有一个可直接登录的师傅端账号用于调试
	if err := seed.SyncDevTechnicianFromConfig(db, cfg); err != nil {
		log.Fatalf("sync dev technician from config failed: %v", err)
	}

	if cfg.SeedDemoData {
		// 先执行 Go 代码内置的种子数据
		if err := seed.SeedDemoData(db, cfg); err != nil {
			log.Fatalf("seed failed: %v", err)
		}

		// 再执行 SQL 种子文件，支持通过 .sql 文件写入追加的默认数据
		sqlSeedDir := resolveSQLSeedDir()
		if err := seed.RunSQLSeedFiles(db, sqlSeedDir); err != nil {
			log.Fatalf("sql seed failed: %v", err)
		}
	}

	router := httpapi.NewRouter(cfg, db)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: time.Duration(cfg.HTTPReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(cfg.HTTPIdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
	}
	log.Printf("cool-dispatch api listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

// resolveSQLSeedDir 基于当前源文件位置定位 SQL 种子目录，
// 兼容从项目根目录或 cmd/api/ 下直接运行的场景。
func resolveSQLSeedDir() string {
	// 尝试从编译时源文件路径推导
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		// filename = .../server/cmd/api/main.go → .../server/database/migrations
		serverRoot := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
		return filepath.Join(serverRoot, "database", "migrations")
	}
	// 回退：假设从 server/ 目录运行
	return "database/migrations"
}
