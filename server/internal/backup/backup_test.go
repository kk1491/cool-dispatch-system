package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizeConnInfoForDockerExec_MapsLocalhostPort 验证当数据库连接串使用宿主机映射端口时，
// 容器内 pg_dump 会自动切换到容器本身可访问的 5432 端口，避免继续访问宿主机映射端口。
func TestNormalizeConnInfoForDockerExec_MapsLocalhostPort(t *testing.T) {
	t.Setenv("POSTGRES_CONTAINER_PORT", "5432")

	info := &pgConnInfo{
		host:     "localhost",
		port:     "9101",
		user:     "postgres",
		password: "postgres",
		dbName:   "cool_dispatch",
	}

	got := normalizeConnInfoForDockerExec(info)

	if got.host != "127.0.0.1" {
		t.Fatalf("预期容器内 host 为 127.0.0.1，实际为 %q", got.host)
	}
	if got.port != "5432" {
		t.Fatalf("预期容器内 port 为 5432，实际为 %q", got.port)
	}
	if got.user != info.user || got.password != info.password || got.dbName != info.dbName {
		t.Fatalf("预期用户、密码与数据库名保持不变，实际=%+v", got)
	}
}

// TestNormalizeConnInfoForDockerExec_KeepRemoteHost 验证非本机数据库地址不会被误改写，
// 避免回退到 Docker 执行时破坏远端数据库备份配置。
func TestNormalizeConnInfoForDockerExec_KeepRemoteHost(t *testing.T) {
	info := &pgConnInfo{
		host:     "db.internal",
		port:     "5433",
		user:     "postgres",
		password: "postgres",
		dbName:   "cool_dispatch",
	}

	got := normalizeConnInfoForDockerExec(info)

	if got.host != "db.internal" {
		t.Fatalf("预期远端 host 保持不变，实际为 %q", got.host)
	}
	if got.port != "5433" {
		t.Fatalf("预期远端 port 保持不变，实际为 %q", got.port)
	}
}

// TestFindLatestBackupTime_SkipEmptyFiles 验证扫描最近备份时会跳过 0-byte 空文件，
// 避免失败残留文件阻断当天后续补偿备份。
func TestFindLatestBackupTime_SkipEmptyFiles(t *testing.T) {
	backupDir := t.TempDir()
	dbName := "cool_dispatch"

	emptyBackupPath := filepath.Join(backupDir, "cool_dispatch_20260326_193748.sql.gz")
	if err := os.WriteFile(emptyBackupPath, nil, 0644); err != nil {
		t.Fatalf("创建空备份文件失败: %v", err)
	}

	latest, found := findLatestBackupTime(backupDir, dbName)
	if found {
		t.Fatalf("预期空备份文件不应被识别为有效备份，实际 latest=%v", latest)
	}

	validBackupPath := filepath.Join(backupDir, "cool_dispatch_20260326_194337.sql.gz")
	if err := os.WriteFile(validBackupPath, []byte("valid backup"), 0644); err != nil {
		t.Fatalf("创建有效备份文件失败: %v", err)
	}

	latest, found = findLatestBackupTime(backupDir, dbName)
	if !found {
		t.Fatalf("预期有效备份文件应被识别出来")
	}
	if latest.IsZero() {
		t.Fatalf("预期返回有效备份时间，实际为零值")
	}
}
