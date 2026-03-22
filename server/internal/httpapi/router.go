package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cool-dispatch/internal/config"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter 组装全部 HTTP 路由、中间件与静态资源托管规则。
func NewRouter(cfg config.Config, db *gorm.DB) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(corsMiddleware(cfg))
	router.Use(requestBodyLimitMiddleware(cfg))

	handler := NewHandler(db, cfg)

	api := router.Group("/api")
	{
		api.GET("/health", handler.Health)
		api.POST("/auth/login", handler.Login)
		api.GET("/auth/me", handler.AuthMe)
		api.POST("/auth/logout", handler.Logout)
		api.GET("/reviews/token/:reviewToken/context", handler.GetReviewContext)
		api.PATCH("/reviews/token/:reviewToken/share-line", handler.UpdateReviewShareLine)
		api.POST("/webhook/line", handler.ReceiveLineWebhook)
		api.POST("/reviews/token/:reviewToken", handler.CreateReview)
	}

	// 需要登录后才能访问的业务接口统一挂载认证中间件，避免客户、工单和财务数据继续匿名暴露。
	authenticated := api.Group("")
	authenticated.Use(authMiddleware(db, cfg.CookieSecure, cookieSameSiteFromConfig(cfg.CookieSameSite)))
	{
		authenticated.GET("/bootstrap", handler.Bootstrap)
		authenticated.GET("/appointments", handler.ListAppointments)
		authenticated.PATCH("/appointments/:id", handler.UpdateAppointment)
		authenticated.GET("/technicians", handler.ListTechnicians)
		authenticated.GET("/customers", handler.ListCustomers)
		authenticated.GET("/zones", handler.ListZones)
		authenticated.GET("/service-items", handler.ListServiceItems)
		authenticated.GET("/extra-items", handler.ListExtraItems)
		authenticated.GET("/cash-ledger", handler.ListCashLedgerEntries)
		authenticated.GET("/reviews", handler.ListReviews)
		authenticated.GET("/notifications", handler.ListNotificationLogs)
		authenticated.GET("/settings", handler.GetSettings)
		// 页面级读接口同时保留旧的 `/api/pages/*` 路径与当前前端使用的简短路径，
		// 避免测试、旧页面和新页面在迁移期出现路由不一致。
		authenticated.GET("/dashboard-data", handler.GetDashboardPageData)
		authenticated.GET("/dashboard-page-data", handler.GetDashboardPageData)
		authenticated.GET("/pages/dashboard", handler.GetDashboardPageData)
		authenticated.GET("/customer-data", handler.GetCustomerPageData)
		authenticated.GET("/customer-page-data", handler.GetCustomerPageData)
		authenticated.GET("/pages/customers", handler.GetCustomerPageData)
		authenticated.GET("/settings-data", handler.GetSettingsPageData)
		authenticated.GET("/settings-page-data", handler.GetSettingsPageData)
		authenticated.GET("/pages/settings", handler.GetSettingsPageData)
		authenticated.GET("/line-data", handler.ListLineData)
		authenticated.GET("/line-page-data", handler.GetLinePageData)
		authenticated.GET("/pages/line", handler.GetLinePageData)
		authenticated.GET("/technician-data", handler.GetTechnicianPageData)
		authenticated.GET("/technician-page-data", handler.GetTechnicianPageData)
		authenticated.GET("/pages/technicians", handler.GetTechnicianPageData)
		authenticated.GET("/reminder-data", handler.GetReminderPageData)
		authenticated.GET("/pages/reminders", handler.GetReminderPageData)
		authenticated.GET("/zone-data", handler.GetZonePageData)
		authenticated.GET("/pages/zones", handler.GetZonePageData)
		authenticated.GET("/financial-report-data", handler.GetFinancialReportPageData)
		authenticated.GET("/financial-report-page-data", handler.GetFinancialReportPageData)
		authenticated.GET("/pages/financial-report", handler.GetFinancialReportPageData)
		authenticated.GET("/review-dashboard-data", handler.GetReviewDashboardPageData)
		authenticated.GET("/pages/reviews", handler.GetReviewDashboardPageData)
		authenticated.GET("/cash-ledger-data", handler.GetCashLedgerPageData)
		authenticated.GET("/cash-ledger-page-data", handler.GetCashLedgerPageData)
		authenticated.GET("/pages/cash-ledger", handler.GetCashLedgerPageData)
	}

	// 管理员接口统一在路由层做第一道授权门禁，避免仅靠前端隐藏按钮形成“假权限”。
	adminOnly := authenticated.Group("")
	adminOnly.Use(requireRoles("admin"))
	{
		adminOnly.POST("/appointments", handler.CreateAppointment)
		adminOnly.DELETE("/appointments/:id", handler.DeleteAppointment)
		adminOnly.PUT("/technicians", handler.ReplaceTechnicians)
		adminOnly.PUT("/technicians/:id/password", handler.UpdateTechnicianPassword)
		adminOnly.PUT("/zones", handler.ReplaceZones)
		adminOnly.PUT("/service-items", handler.ReplaceServiceItems)
		adminOnly.PUT("/extra-items", handler.ReplaceExtraItems)
		adminOnly.POST("/cash-ledger", handler.CreateCashLedgerEntry)
		adminOnly.POST("/notifications", handler.CreateNotificationLog)
		adminOnly.PUT("/settings/reminder-days", handler.UpdateReminderDays)
		adminOnly.PUT("/settings/webhook-enabled", handler.UpdateWebhookEnabled)
		adminOnly.PUT("/customers", handler.ReplaceCustomers)
		adminOnly.DELETE("/customers/:id", handler.DeleteCustomer)
		adminOnly.PUT("/line-friends/:lineUid/customer", handler.LinkLineFriendCustomer)
	}

	if cfg.EnableStatic {
		registerStatic(router, cfg.FrontendDist)
	}

	return router
}

// requestBodyLimitMiddleware 对写请求统一设置请求体上限，避免 JSON 写接口与公开 webhook 被超大 Body 拖垮。
func requestBodyLimitMiddleware(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}

		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}

		maxBytes := cfg.MaxJSONBodyBytes
		if c.Request.URL.Path == "/api/webhook/line" {
			maxBytes = cfg.MaxWebhookBodyBytes
		}
		if maxBytes <= 0 {
			c.Next()
			return
		}

		// 对已声明 Content-Length 的请求先做快速拒绝，减少无意义的后续解析成本。
		if c.Request.ContentLength > maxBytes {
			abortWithMessage(c, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// registerStatic 在生产模式下托管前端构建产物；若构建产物不存在，则静默跳过静态托管。
func registerStatic(router *gin.Engine, frontendDist string) {
	indexPath := filepath.Join(frontendDist, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	router.Static("/assets", filepath.Join(frontendDist, "assets"))
	router.StaticFile("/favicon.png", filepath.Join(frontendDist, "favicon.png"))
	router.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			respondMessage(c, http.StatusNotFound, "Not Found")
			return
		}
		c.File(indexPath)
	})
}

// corsMiddleware 为前后端分离开发场景提供最小化跨域支持，并明确放行本地开发域名。
func corsMiddleware(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && isAllowedOrigin(origin, cfg.FrontendOrigin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// isAllowedOrigin 只放行显式配置域名和 localhost/127.0.0.1 开发域名，避免把凭据跨域敞开给任意站点。
func isAllowedOrigin(origin string, frontendOrigin string) bool {
	if origin == frontendOrigin {
		return true
	}

	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "https://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "https://127.0.0.1:")
}

// healthResponse 是健康检查接口返回的最小响应结构。
type healthResponse struct {
	// Status 固定返回服务当前状态。
	Status string `json:"status"`
	// Timestamp 返回健康检查响应生成时间。
	Timestamp time.Time `json:"timestamp"`
}
