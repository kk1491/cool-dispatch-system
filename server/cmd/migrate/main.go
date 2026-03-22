package main

import (
	"log"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/database"
)

// main 提供单独的数据库迁移入口，便于在部署或运维阶段显式执行表结构更新。
func main() {
	cfg := config.Load()
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("migration completed")
}
