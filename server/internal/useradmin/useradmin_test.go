package useradmin

import (
	"testing"
	"time"

	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newUserAdminTestDB 使用内存 SQLite 构建最小测试库，便于验证账号初始化与密码重置逻辑。
func newUserAdminTestDB(t *testing.T) *gorm.DB {
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

// TestUpsertUserPasswordCreatesUserWithNextID 验证新建用户时会按当前最大 ID 自动分配下一个主键。
func TestUpsertUserPasswordCreatesUserWithNextID(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	if err := db.Create(&models.User{
		ID:           7,
		Name:         "已有用户",
		Role:         "admin",
		Phone:        "0911000007",
		PasswordHash: "$2a$10$existing",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	result, err := UpsertUserPassword(db, UpsertUserInput{
		Phone:    "0912000008",
		Password: "password-123",
		Name:     "新管理员",
		Role:     "admin",
	})
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.User.ID != 8 {
		t.Fatalf("expected next user id 8, got %d", result.User.ID)
	}
	if !security.VerifyPassword("password-123", result.User.PasswordHash) {
		t.Fatalf("expected stored hash to match input password")
	}
}

// TestUpsertUserPasswordUpdatesExistingUserPassword 验证按手机号匹配到既有用户时只更新密码和显式传入资料。
func TestUpsertUserPasswordUpdatesExistingUserPassword(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	oldHash, err := security.HashPassword("old-password-123")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	if err := db.Create(&models.User{
		ID:           1,
		Name:         "管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: oldHash,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	result, err := UpsertUserPassword(db, UpsertUserInput{
		Phone:    "0912345678",
		Password: "new-password-123",
		Name:     "新管理员名称",
	})
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	if result.Created {
		t.Fatalf("expected existing user to be updated")
	}
	if result.User.Name != "新管理员名称" {
		t.Fatalf("expected name updated, got %s", result.User.Name)
	}
	if !security.VerifyPassword("new-password-123", result.User.PasswordHash) {
		t.Fatalf("expected updated hash to match new password")
	}
}

// TestUpsertUserPasswordRejectsCreateWithoutRole 验证创建新用户时缺少角色会被拒绝。
func TestUpsertUserPasswordRejectsCreateWithoutRole(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	_, err := UpsertUserPassword(db, UpsertUserInput{
		Phone:    "0912000008",
		Password: "password-123",
		Name:     "新管理员",
	})
	if err != ErrRoleRequiredForCreate {
		t.Fatalf("expected ErrRoleRequiredForCreate, got %v", err)
	}
}

// TestListUsersReturnsOrderedSummaries 验证账号巡检输出会按用户 ID 升序返回摘要列表。
func TestListUsersReturnsOrderedSummaries(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	now := time.Date(2026, 3, 21, 11, 0, 0, 0, time.UTC)
	seeds := []models.User{
		{ID: 2, Name: "技师乙", Role: "technician", Phone: "0911000002", PasswordHash: "$2a$10$tech", CreatedAt: now, UpdatedAt: now},
		{ID: 1, Name: "管理员", Role: "admin", Phone: "0911000001", PasswordHash: "$2a$10$admin", CreatedAt: now, UpdatedAt: now},
	}
	for _, seed := range seeds {
		if err := db.Create(&seed).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}

	items, err := ListUsers(db)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 users, got %d", len(items))
	}
	if items[0].ID != 1 || items[1].ID != 2 {
		t.Fatalf("expected ordered ids [1,2], got %+v", items)
	}
}

// TestRevokeTokensByPhoneDeletesOnlyTargetUserTokens 验证按手机号强制下线时只删除目标用户令牌。
func TestRevokeTokensByPhoneDeletesOnlyTargetUserTokens(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	if err := db.Create(&models.User{ID: 1, Name: "管理员", Role: "admin", Phone: "0911000001", PasswordHash: "$2a$10$admin"}).Error; err != nil {
		t.Fatalf("seed user 1: %v", err)
	}
	if err := db.Create(&models.User{ID: 2, Name: "技师", Role: "technician", Phone: "0911000002", PasswordHash: "$2a$10$tech"}).Error; err != nil {
		t.Fatalf("seed user 2: %v", err)
	}
	tokens := []models.AuthToken{
		{UserID: 1, Token: "token-admin-1", ExpiresAt: time.Now().Add(24 * time.Hour)},
		{UserID: 1, Token: "token-admin-2", ExpiresAt: time.Now().Add(24 * time.Hour)},
		{UserID: 2, Token: "token-tech-1", ExpiresAt: time.Now().Add(24 * time.Hour)},
	}
	for _, token := range tokens {
		if err := db.Create(&token).Error; err != nil {
			t.Fatalf("seed token: %v", err)
		}
	}

	revoked, user, err := RevokeTokensByPhone(db, "0911000001")
	if err != nil {
		t.Fatalf("revoke tokens: %v", err)
	}
	if user == nil || user.ID != 1 {
		t.Fatalf("expected revoked user id 1, got %+v", user)
	}
	if revoked != 2 {
		t.Fatalf("expected 2 revoked tokens, got %d", revoked)
	}

	var remaining int64
	if err := db.Model(&models.AuthToken{}).Where("user_id = ?", 2).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining tokens: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected other user token preserved, got %d", remaining)
	}
}

// TestRevokeAllTokensDeletesEverything 验证全局强制下线会删除所有持久化认证令牌。
func TestRevokeAllTokensDeletesEverything(t *testing.T) {
	t.Parallel()

	db := newUserAdminTestDB(t)
	tokens := []models.AuthToken{
		{UserID: 1, Token: "token-1", ExpiresAt: time.Now().Add(24 * time.Hour)},
		{UserID: 2, Token: "token-2", ExpiresAt: time.Now().Add(24 * time.Hour)},
	}
	for _, token := range tokens {
		if err := db.Create(&token).Error; err != nil {
			t.Fatalf("seed token: %v", err)
		}
	}

	revoked, err := RevokeAllTokens(db)
	if err != nil {
		t.Fatalf("revoke all tokens: %v", err)
	}
	if revoked != 2 {
		t.Fatalf("expected 2 revoked tokens, got %d", revoked)
	}
}
