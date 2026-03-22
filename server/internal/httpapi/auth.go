package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"cool-dispatch/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// token 有效期与续期阈值常量
const (
	// tokenDuration token 有效期为 30 天。
	tokenDuration = 30 * 24 * time.Hour
	// tokenRenewThreshold 当 token 剩余有效期小于 29 天时自动续期。
	tokenRenewThreshold = 29 * 24 * time.Hour
	// tokenCookieName 存放认证 token 的 cookie 名称。
	tokenCookieName = "cd_auth_token"
	// tokenLength token 随机字节长度（64 字节 = 128 位十六进制字符串）。
	tokenLength = 64
)

// generateToken 生成 128 字符的加密安全随机 hex token。
func generateToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// createAuthToken 为指定用户创建新的认证 token。
// 同一用户同时只保留一个有效 token：先删除旧 token，再创建新 token。
func createAuthToken(db *gorm.DB, userID uint) (*models.AuthToken, error) {
	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(tokenDuration)

	authToken := &models.AuthToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// 删除该用户所有已有 token，确保同时只有一个有效 token。
		if err := tx.Where("user_id = ?", userID).Delete(&models.AuthToken{}).Error; err != nil {
			return err
		}
		return tx.Create(authToken).Error
	})
	if err != nil {
		return nil, err
	}

	return authToken, nil
}

// validateAuthToken 校验 token 是否存在且未过期，返回对应用户。
// 如果 token 剩余有效期小于 tokenRenewThreshold（29 天），自动续期到完整 tokenDuration（30 天）。
func validateAuthToken(db *gorm.DB, tokenStr string) (*models.User, bool, error) {
	var authToken models.AuthToken
	if err := db.Where("token = ?", tokenStr).First(&authToken).Error; err != nil {
		return nil, false, err
	}

	now := time.Now().UTC()

	// token 已过期，删除并返回无效。
	if now.After(authToken.ExpiresAt) {
		_ = db.Delete(&authToken)
		return nil, false, nil
	}

	// 查询对应的用户。
	var user models.User
	if err := db.First(&user, "id = ?", authToken.UserID).Error; err != nil {
		return nil, false, err
	}

	// 判断是否需要自动续期：剩余有效期 < 29 天时续期到完整 30 天。
	renewed := false
	remaining := authToken.ExpiresAt.Sub(now)
	if remaining < tokenRenewThreshold {
		authToken.ExpiresAt = now.Add(tokenDuration)
		_ = db.Model(&authToken).Update("expires_at", authToken.ExpiresAt)
		renewed = true
	}

	return &user, renewed, nil
}

// setAuthCookie 将 token 写入 HttpOnly cookie，有效期与 token 数据库记录一致。
func setAuthCookie(c *gin.Context, token string, maxAge int, secure bool, sameSite http.SameSite) {
	c.SetSameSite(sameSite)
	c.SetCookie(
		tokenCookieName, // cookie 名称
		token,           // cookie 值
		maxAge,          // 秒数
		"/",             // 路径
		"",              // domain（空 = 当前域名）
		secure,          // secure 由环境配置控制，HTTPS 部署时必须开启。
		true,            // httpOnly
	)
}

// clearAuthCookie 清除认证 cookie。
func clearAuthCookie(c *gin.Context, secure bool, sameSite http.SameSite) {
	setAuthCookie(c, "", -1, secure, sameSite)
}

// cookieSameSiteFromConfig 把字符串配置统一转换为标准库 SameSite 枚举。
func cookieSameSiteFromConfig(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// authMiddleware 认证中间件：从 cookie 读取 token 校验身份。
// 校验通过后将用户信息注入 gin context，供后续 handler 使用。
// 如果 token 触发自动续期，同步刷新 cookie 有效期。
func authMiddleware(db *gorm.DB, cookieSecure bool, cookieSameSite http.SameSite) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := c.Cookie(tokenCookieName)
		if err != nil || tokenStr == "" {
			abortWithMessage(c, http.StatusUnauthorized, "authentication required")
			return
		}

		user, renewed, err := validateAuthToken(db, tokenStr)
		if err != nil || user == nil {
			clearAuthCookie(c, cookieSecure, cookieSameSite)
			abortWithMessage(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// token 续期后刷新 cookie 有效期，让浏览器保持最新的过期时间。
		if renewed {
			setAuthCookie(c, tokenStr, int(tokenDuration.Seconds()), cookieSecure, cookieSameSite)
		}

		// 把用户信息注入 context，后续 handler 可通过 c.MustGet("user") 获取。
		c.Set("user", user)
		c.Next()
	}
}

// currentUser 从 gin context 读取认证后的当前用户。
// 若中间件未正确注入或类型异常，则返回错误，避免后续授权逻辑在空用户上继续执行。
func currentUser(c *gin.Context) (*models.User, error) {
	raw, ok := c.Get("user")
	if !ok {
		return nil, errors.New("authenticated user missing from context")
	}

	user, ok := raw.(*models.User)
	if !ok || user == nil {
		return nil, errors.New("authenticated user has invalid type")
	}

	return user, nil
}

// requireRoles 要求当前登录用户必须命中指定角色之一。
// 该中间件只负责角色门禁；前置的登录态校验仍由 authMiddleware 负责。
func requireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		user, err := currentUser(c)
		if err != nil {
			abortWithMessage(c, http.StatusUnauthorized, "authentication required")
			return
		}

		if _, ok := allowed[user.Role]; !ok {
			abortWithMessage(c, http.StatusForbidden, "forbidden")
			return
		}

		c.Next()
	}
}
