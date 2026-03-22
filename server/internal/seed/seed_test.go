package seed

import (
	"testing"
	"time"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newSeedTestDB 使用内存 SQLite 构建最小测试库，便于覆盖 demo seed 的管理员同步逻辑。
func newSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.AutoMigrateModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// TestSeedUsersCreatesConfiguredAdminAndTechnicians 验证空库 seed 时会按配置创建管理员，并补齐 demo 技师。
func TestSeedUsersCreatesConfiguredAdminAndTechnicians(t *testing.T) {
	t.Parallel()

	db := newSeedTestDB(t)
	cfg := config.Config{
		SeedAdminName:         "配置管理员",
		SeedAdminPhone:        "0900000001",
		SeedAdminPassword:     "config-admin-123",
		SeedTechnicianPassword: "config-tech-123",
	}

	if err := seedUsers(db, cfg); err != nil {
		t.Fatalf("seed users: %v", err)
	}

	var admin models.User
	if err := db.First(&admin, "role = ?", "admin").Error; err != nil {
		t.Fatalf("query admin: %v", err)
	}
	if admin.Name != "配置管理员" || admin.Phone != "0900000001" {
		t.Fatalf("expected configured admin identity, got %+v", admin)
	}
	if !security.VerifyPassword("config-admin-123", admin.PasswordHash) {
		t.Fatalf("expected admin password hash updated from config")
	}

	var technicianCount int64
	if err := db.Model(&models.User{}).Where("role = ?", "technician").Count(&technicianCount).Error; err != nil {
		t.Fatalf("count technicians: %v", err)
	}
	if technicianCount != 3 {
		t.Fatalf("expected 3 demo technicians, got %d", technicianCount)
	}
}

// TestSeedUsersOverridesExistingAdminFromConfig 验证库内已有管理员时，seed 仍会用当前配置覆盖管理员信息并撤销旧令牌。
func TestSeedUsersOverridesExistingAdminFromConfig(t *testing.T) {
	t.Parallel()

	db := newSeedTestDB(t)
	oldHash, err := security.HashPassword("old-admin-123")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	if err := db.Create(&models.User{
		ID:           1,
		Name:         "旧管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: oldHash,
	}).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&models.AuthToken{
		UserID:    1,
		Token:     "legacy-admin-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed auth token: %v", err)
	}

	cfg := config.Config{
		SeedAdminName:         "新管理员",
		SeedAdminPhone:        "0999888777",
		SeedAdminPassword:     "new-admin-123",
		SeedTechnicianPassword: "config-tech-123",
	}

	if err := seedUsers(db, cfg); err != nil {
		t.Fatalf("seed users: %v", err)
	}

	var admin models.User
	if err := db.First(&admin, "id = ?", 1).Error; err != nil {
		t.Fatalf("reload admin: %v", err)
	}
	if admin.Name != "新管理员" || admin.Phone != "0999888777" || admin.Role != "admin" {
		t.Fatalf("expected admin overridden by config, got %+v", admin)
	}
	if !security.VerifyPassword("new-admin-123", admin.PasswordHash) {
		t.Fatalf("expected admin password hash overwritten from config")
	}

	var tokenCount int64
	if err := db.Model(&models.AuthToken{}).Where("user_id = ?", admin.ID).Count(&tokenCount).Error; err != nil {
		t.Fatalf("count auth tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("expected admin tokens revoked after config override, got %d", tokenCount)
	}
}

// TestSeedUsersRejectsConfigPhoneCollision 验证配置中的管理员手机号若被其它账号占用，会直接报错而不是误覆盖非管理员。
func TestSeedUsersRejectsConfigPhoneCollision(t *testing.T) {
	t.Parallel()

	db := newSeedTestDB(t)
	oldHash, err := security.HashPassword("old-admin-123")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	if err := db.Create(&models.User{
		ID:           1,
		Name:         "旧管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: oldHash,
	}).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&models.User{
		ID:           2,
		Name:         "王師傅",
		Role:         "technician",
		Phone:        "0900000001",
		PasswordHash: oldHash,
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}

	cfg := config.Config{
		SeedAdminName:         "新管理员",
		SeedAdminPhone:        "0900000001",
		SeedAdminPassword:     "new-admin-123",
		SeedTechnicianPassword: "config-tech-123",
	}

	if err := seedUsers(db, cfg); err == nil {
		t.Fatalf("expected phone collision error")
	}
}
