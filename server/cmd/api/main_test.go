package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveSQLSeedDirPointsToMigrationsDirectory 验证 SQL 种子目录解析会稳定指向迁移目录。
func TestResolveSQLSeedDirPointsToMigrationsDirectory(t *testing.T) {
	t.Parallel()

	seedDir := resolveSQLSeedDir()
	if !strings.HasSuffix(seedDir, filepath.Join("server", "database", "migrations")) {
		t.Fatalf("expected migrations directory suffix, got %s", seedDir)
	}
	if info, err := os.Stat(seedDir); err != nil || !info.IsDir() {
		t.Fatalf("expected migrations directory to exist, err=%v", err)
	}
}
