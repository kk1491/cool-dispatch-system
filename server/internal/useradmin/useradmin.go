package useradmin

import (
	"errors"
	"fmt"
	"strings"

	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"gorm.io/gorm"
)

// 包级错误变量统一复用在 CLI 与服务层，避免同类账号操作错误文案分散定义。
var (
	// ErrPhoneRequired 表示 CLI 未提供手机号，无法定位或创建账号。
	ErrPhoneRequired = errors.New("phone is required")
	// ErrPasswordTooShort 表示新密码不满足最小长度要求。
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", security.PasswordMinLength)
	// ErrNameRequiredForCreate 表示创建新用户时缺少展示名称。
	ErrNameRequiredForCreate = errors.New("name is required when creating a new user")
	// ErrRoleRequiredForCreate 表示创建新用户时缺少角色。
	ErrRoleRequiredForCreate = errors.New("role is required when creating a new user")
	// ErrInvalidRole 表示输入角色不在系统允许范围内。
	ErrInvalidRole = errors.New("role must be admin or technician")
	// ErrUserNotFound 表示按手机号定位用户失败，通常用于撤销指定账号会话。
	ErrUserNotFound = errors.New("user not found")
)

// UpsertUserInput 描述用户初始化/重置密码命令所需的最小输入。
// 已存在用户会按手机号更新密码和可选资料；不存在时则创建新用户。
type UpsertUserInput struct {
	// Phone 是用户手机号，也是 upsert 查找键。
	Phone string
	// Password 是需要写入的新明文密码。
	Password string
	// Name 是创建用户时必填、更新时可选覆盖的显示名称。
	Name string
	// Role 是创建用户时必填、更新时可选覆盖的角色。
	Role string
	// UserID 允许创建用户时显式指定主键，便于迁移旧数据。
	UserID *uint
}

// UpsertUserResult 统一返回命令执行结果，便于 CLI 输出“创建”还是“更新”。
type UpsertUserResult struct {
	// User 是写入后的完整用户记录。
	User models.User
	// Created 表示本次操作是新建还是更新。
	Created bool
}

// UserSummary 为账号巡检输出的最小字段集合，避免 CLI 把密码哈希暴露到日志。
type UserSummary struct {
	// ID 是用户主键。
	ID uint
	// Name 是用户显示名称。
	Name string
	// Role 是用户角色。
	Role string
	// Phone 是用户手机号。
	Phone string
	// CreatedAt 是 ISO8601 格式的创建时间。
	CreatedAt string
	// UpdatedAt 是 ISO8601 格式的最近更新时间。
	UpdatedAt string
}

// UpsertUserPassword 按手机号创建账号或重置密码。
// 该方法只更新明确传入的名称/角色，避免误把既有资料清空。
func UpsertUserPassword(db *gorm.DB, input UpsertUserInput) (*UpsertUserResult, error) {
	phone := strings.TrimSpace(input.Phone)
	password := strings.TrimSpace(input.Password)
	name := strings.TrimSpace(input.Name)
	role := normalizeRole(input.Role)

	if phone == "" {
		return nil, ErrPhoneRequired
	}
	if len(password) < security.PasswordMinLength {
		return nil, ErrPasswordTooShort
	}

	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	var user models.User
	err = db.First(&user, "phone = ?", phone).Error
	if err == nil {
		updates := map[string]any{
			"password_hash": passwordHash,
		}
		if name != "" {
			updates["name"] = name
		}
		if role != "" {
			if role != "admin" && role != "technician" {
				return nil, ErrInvalidRole
			}
			updates["role"] = role
		}
		if err := db.Model(&user).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update user password: %w", err)
		}
		if err := db.First(&user, "id = ?", user.ID).Error; err != nil {
			return nil, fmt.Errorf("reload updated user: %w", err)
		}
		return &UpsertUserResult{User: user, Created: false}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query user by phone: %w", err)
	}

	if name == "" {
		return nil, ErrNameRequiredForCreate
	}
	if role == "" {
		return nil, ErrRoleRequiredForCreate
	}
	if role != "admin" && role != "technician" {
		return nil, ErrInvalidRole
	}

	userID := uint(0)
	if input.UserID != nil {
		userID = *input.UserID
	} else {
		userID, err = nextUserID(db)
		if err != nil {
			return nil, err
		}
	}

	user = models.User{
		ID:           userID,
		Name:         name,
		Role:         role,
		Phone:        phone,
		PasswordHash: passwordHash,
	}
	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &UpsertUserResult{User: user, Created: true}, nil
}

// ListUsers 返回用于 CLI 输出的账号摘要列表，默认按 ID 升序。
func ListUsers(db *gorm.DB) ([]UserSummary, error) {
	var users []models.User
	if err := db.Order("id asc").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	result := make([]UserSummary, 0, len(users))
	for _, user := range users {
		result = append(result, UserSummary{
			ID:        user.ID,
			Name:      user.Name,
			Role:      user.Role,
			Phone:     user.Phone,
			CreatedAt: user.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt: user.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

// RevokeTokensByPhone 撤销指定手机号用户的所有持久化登录令牌，用于紧急下线单个账号。
func RevokeTokensByPhone(db *gorm.DB, phone string) (int64, *models.User, error) {
	normalizedPhone := strings.TrimSpace(phone)
	if normalizedPhone == "" {
		return 0, nil, ErrPhoneRequired
	}

	var user models.User
	if err := db.First(&user, "phone = ?", normalizedPhone).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil, ErrUserNotFound
		}
		return 0, nil, fmt.Errorf("query user by phone: %w", err)
	}

	result := db.Where("user_id = ?", user.ID).Delete(&models.AuthToken{})
	if result.Error != nil {
		return 0, nil, fmt.Errorf("revoke user tokens: %w", result.Error)
	}
	return result.RowsAffected, &user, nil
}

// RevokeAllTokens 撤销系统内全部持久化登录令牌，用于全局强制下线。
func RevokeAllTokens(db *gorm.DB) (int64, error) {
	result := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.AuthToken{})
	if result.Error != nil {
		return 0, fmt.Errorf("revoke all tokens: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// nextUserID 在未显式提供 user id 时自动分配下一个可用整数主键。
func nextUserID(db *gorm.DB) (uint, error) {
	var currentMax uint
	if err := db.Model(&models.User{}).Select("COALESCE(MAX(id), 0)").Scan(&currentMax).Error; err != nil {
		return 0, fmt.Errorf("query next user id: %w", err)
	}
	return currentMax + 1, nil
}

// normalizeRole 统一把外部输入角色收敛为小写，便于 CLI 与数据校验共用。
func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}
