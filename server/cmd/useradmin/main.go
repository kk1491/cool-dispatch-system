package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/database"
	"cool-dispatch/internal/useradmin"
)

// main 提供最小可用的账号巡检、初始化、重置密码与强制下线命令。
// 用法示例：
//
//	go run ./cmd/useradmin --action list
//	go run ./cmd/useradmin --action upsert --phone 0912345678 --password new-password --name 管理员 --role admin
//	go run ./cmd/useradmin --action revoke-user --phone 0912345678
//	go run ./cmd/useradmin --action revoke-all
func main() {
	var (
		action   = flag.String("action", "upsert", "执行动作：list | upsert | revoke-user | revoke-all")
		phone    = flag.String("phone", "", "用户手机号，作为查找或创建的唯一键")
		password = flag.String("password", "", "新密码，至少 8 位")
		name     = flag.String("name", "", "创建新用户时必填；更新已有用户时可选覆盖名称")
		role     = flag.String("role", "", "创建新用户时必填：admin 或 technician；更新时可选覆盖角色")
		userID   = flag.Uint("user-id", 0, "创建新用户时可选指定用户 ID；省略时自动分配")
	)
	flag.Parse()

	cfg := config.Load()
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	var targetUserID *uint
	if *userID > 0 {
		resolvedUserID := uint(*userID)
		targetUserID = &resolvedUserID
	}

	switch strings.ToLower(strings.TrimSpace(*action)) {
	case "list":
		users, err := useradmin.ListUsers(db)
		if err != nil {
			log.Fatalf("useradmin failed: %v", err)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(users); err != nil {
			log.Fatalf("encode user list failed: %v", err)
		}
	case "upsert":
		result, err := useradmin.UpsertUserPassword(db, useradmin.UpsertUserInput{
			Phone:    strings.TrimSpace(*phone),
			Password: strings.TrimSpace(*password),
			Name:     strings.TrimSpace(*name),
			Role:     strings.TrimSpace(*role),
			UserID:   targetUserID,
		})
		if err != nil {
			log.Fatalf("useradmin failed: %v", err)
		}
		if result.Created {
			log.Printf("user created: id=%d phone=%s role=%s name=%s", result.User.ID, result.User.Phone, result.User.Role, result.User.Name)
			return
		}
		log.Printf("user password updated: id=%d phone=%s role=%s name=%s", result.User.ID, result.User.Phone, result.User.Role, result.User.Name)
	case "revoke-user":
		revoked, user, err := useradmin.RevokeTokensByPhone(db, strings.TrimSpace(*phone))
		if err != nil {
			log.Fatalf("useradmin failed: %v", err)
		}
		log.Printf("user tokens revoked: id=%d phone=%s role=%s revoked=%d", user.ID, user.Phone, user.Role, revoked)
	case "revoke-all":
		revoked, err := useradmin.RevokeAllTokens(db)
		if err != nil {
			log.Fatalf("useradmin failed: %v", err)
		}
		log.Printf("all auth tokens revoked: %d", revoked)
	default:
		log.Fatalf("unsupported action: %s", *action)
	}
}
