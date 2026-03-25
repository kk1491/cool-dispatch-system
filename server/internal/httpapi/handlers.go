package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cool-dispatch/internal/cloudflare"
	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/payuni"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Handler 聚合 HTTP 处理器依赖，负责资源接口、鉴权接口和 webhook 接口共享状态。
type Handler struct {
	// db 是全部处理器复用的数据库连接。
	db *gorm.DB
	// lineChannelSecret 用于校验 LINE webhook 请求签名。
	lineChannelSecret string
	// webhookBaseURL 是设置页展示给管理员的 webhook 基址。
	webhookBaseURL string
	// webhookBaseURLSource 标识 webhook 基址的配置来源。
	webhookBaseURLSource string
	// hasPublicWebhookBaseURL 表示当前展示地址是否可被外部 LINE 平台访问。
	hasPublicWebhookBaseURL bool
	// cookieSecure 控制认证 Cookie 是否只允许在 HTTPS 链路上传输。
	cookieSecure bool
	// cookieSameSite 控制认证 Cookie 的 SameSite 策略。
	cookieSameSite http.SameSite
	// cfClient 是 Cloudflare Images 图床客户端，未配置时为 nil。
	cfClient *cloudflare.Client
	// payuniClient 是 PAYUNi 支付客户端，未配置时为 nil（MerID/HashKey/HashIV 全部配置才创建）。
	payuniClient *payuni.Client
}

// handlers.go 负责健康检查与 LINE webhook/好友相关接口；资源 CRUD 统一放在 resource_handlers.go。
func NewHandler(db *gorm.DB, cfg config.Config) *Handler {
	webhookBaseURL, webhookBaseURLSource, hasPublicWebhookBaseURL := resolveWebhookBaseURL(cfg)

	// 初始化 Cloudflare Images 客户端，未配置 account_id 或 api_token 时仍创建实例，
	// 但 IsConfigured() 会返回 false，上传/删除接口会返回配置缺失错误。
	cfClient := cloudflare.NewClient(cfg.CloudflareAccountID, cfg.CloudflareAPIToken)

	// 初始化 PAYUNi 支付客户端：仅在 MerID + HashKey + HashIV 全部配置时才创建，
	// 未配置时 payuniClient 为 nil，支付接口会返回 503。
	var payuniClient *payuni.Client
	if cfg.PayuniMerID != "" && cfg.PayuniHashKey != "" && cfg.PayuniHashIV != "" {
		payuniClient = &payuni.Client{
			BaseURL:   cfg.PayuniAPIBaseURL,
			MerID:     cfg.PayuniMerID,
			HashKey:   cfg.PayuniHashKey,
			HashIV:    cfg.PayuniHashIV,
			NotifyURL: cfg.PayuniNotifyURL,
		}
	}

	return &Handler{
		db:                      db,
		lineChannelSecret:       strings.TrimSpace(cfg.LineChannelSecret),
		webhookBaseURL:          webhookBaseURL,
		webhookBaseURLSource:    webhookBaseURLSource,
		hasPublicWebhookBaseURL: hasPublicWebhookBaseURL,
		cookieSecure:            cfg.CookieSecure,
		cookieSameSite:          cookieSameSiteFromConfig(cfg.CookieSameSite),
		cfClient:                cfClient,
		payuniClient:            payuniClient,
	}
}

// resolveWebhookBaseURL 统一生成设置页展示的 webhook 基址，并区分公网来源与本地调试回退。
func resolveWebhookBaseURL(cfg config.Config) (string, string, bool) {
	if baseURL := strings.TrimRight(strings.TrimSpace(cfg.WebhookPublicBaseURL), "/"); baseURL != "" {
		return baseURL, "WEBHOOK_PUBLIC_BASE_URL", true
	}

	frontendOrigin := strings.TrimRight(strings.TrimSpace(cfg.FrontendOrigin), "/")
	if frontendOrigin != "" && !isLocalWebhookURL(frontendOrigin) {
		return frontendOrigin, "FRONTEND_ORIGIN", true
	}

	if strings.TrimSpace(cfg.Port) != "" {
		return "http://localhost:" + strings.TrimSpace(cfg.Port), "LOCAL_SERVER_FALLBACK", false
	}

	return "", "UNAVAILABLE", false
}

// isLocalWebhookURL 判断 URL 是否属于本机地址，供管理员页提示“仅本机可访问”与“缺公网域名”使用。
func isLocalWebhookURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return true
	}

	host := strings.ToLower(parsed.Hostname())
	return host == "" || host == "localhost" || host == "127.0.0.1"
}

// Health 返回最小健康检查结果，供负载均衡和运维探针使用。
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	})
}

// lineFriendResponse 是 LINE 管理页对外返回的好友读模型。
type lineFriendResponse struct {
	// LineUID 是 LINE 用户 UID。
	LineUID string `json:"line_uid"`
	// LineName 是好友当前昵称。
	LineName string `json:"line_name"`
	// LinePicture 是好友头像地址。
	LinePicture string `json:"line_picture"`
	// JoinedAt 是好友首次关注时间。
	JoinedAt string `json:"joined_at"`
	// LineJoinedAt 为兼容旧前端字段保留的关注时间别名。
	LineJoinedAt string `json:"line_joined_at"`
	// Phone 是 webhook 中同步到的手机号。
	Phone *string `json:"phone,omitempty"`
	// LinkedCustomerID 是当前绑定到的客户主档 ID。
	LinkedCustomerID *string `json:"linked_customer_id,omitempty"`
	// Status 表示好友当前关注状态。
	Status string `json:"status"`
	// LastPayload 是最近一次 webhook 原始负载。
	LastPayload any `json:"last_payload,omitempty"`
	// CreatedAt 是好友记录创建时间。
	CreatedAt string `json:"created_at"`
	// UpdatedAt 是好友记录最近更新时间。
	UpdatedAt string `json:"updated_at"`
}

// ListLineData 返回 LINE 页面需要的好友列表数据。
func (h *Handler) ListLineData(c *gin.Context) {
	var friends []models.LineFriend
	if err := h.db.Order("joined_at desc").Find(&friends).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load line data")
		return
	}

	c.JSON(http.StatusOK, buildLineFriendResponses(friends))
}

// buildLineFriendResponses 统一转换 LINE 好友响应字段，保证独立页面接口和资源接口结构一致。
func buildLineFriendResponses(friends []models.LineFriend) []lineFriendResponse {
	response := make([]lineFriendResponse, 0, len(friends))
	for _, item := range friends {
		joinedAt := item.JoinedAt.UTC().Format(time.RFC3339)
		createdAt := item.CreatedAt.UTC().Format(time.RFC3339)
		updatedAt := item.UpdatedAt.UTC().Format(time.RFC3339)
		response = append(response, lineFriendResponse{
			LineUID:          item.LineUID,
			LineName:         item.LineName,
			LinePicture:      item.LinePicture,
			JoinedAt:         joinedAt,
			LineJoinedAt:     joinedAt,
			Phone:            item.Phone,
			LinkedCustomerID: item.LinkedCustomerID,
			Status:           item.Status,
			LastPayload:      json.RawMessage(item.LastPayload),
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
		})
	}

	return response
}

// webhookPayload 是 LINE webhook 外层事件数组载体。
type webhookPayload struct {
	// Events 是本次 webhook 批次携带的事件列表。
	Events []lineEventPayload `json:"events"`
}

// lineEventPayload 表示单条 LINE webhook 事件。
type lineEventPayload struct {
	// Type 是事件类型。
	Type string `json:"type"`
	// Timestamp 是 LINE 上报的毫秒时间戳。
	Timestamp int64 `json:"timestamp"`
	// Source 描述事件来源账号。
	Source lineEventSourcePayload `json:"source"`
	// Profile 是可选的用户资料补充信息。
	Profile *lineProfilePayload `json:"profile,omitempty"`
}

// lineEventSourcePayload 是 LINE webhook 的事件来源信息。
type lineEventSourcePayload struct {
	// UserID 是触发事件的 LINE 用户 UID。
	UserID string `json:"userId"`
}

// lineProfilePayload 是 webhook 中可能携带的好友资料快照。
type lineProfilePayload struct {
	// DisplayName 是好友昵称。
	DisplayName string `json:"displayName"`
	// PictureURL 是好友头像地址。
	PictureURL string `json:"pictureUrl"`
	// Phone 是扩展资料中同步到的手机号。
	Phone string `json:"phone"`
}

// ReceiveLineWebhook 接收并校验 LINE webhook，持久化事件与好友资料。
func (h *Handler) ReceiveLineWebhook(c *gin.Context) {
	settings, err := h.loadSettings()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load webhook settings")
		return
	}
	if !lineWebhookEnabledFromSettings(settings) {
		respondMessage(c, http.StatusServiceUnavailable, "line webhook is disabled by admin setting")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid webhook payload")
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := h.validateLineWebhookSignature(c.GetHeader("X-Line-Signature"), bodyBytes); err != nil {
		status := http.StatusUnauthorized
		if err == errLineWebhookSecretNotConfigured {
			status = http.StatusInternalServerError
		}
		respondMessage(c, status, err.Error())
		return
	}

	var body webhookPayload
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "invalid webhook payload") {
		return
	}

	if len(body.Events) == 0 {
		c.String(http.StatusOK, "OK")
		return
	}

	for _, event := range body.Events {
		payloadBytes, _ := json.Marshal(event)
		lineUID := normalizeString(event.Source.UserID)
		receivedAt := time.Now().UTC()
		if event.Timestamp > 0 {
			receivedAt = time.UnixMilli(event.Timestamp).UTC()
		}

		record := models.LineEvent{
			EventType:  fallbackString(event.Type, "unknown"),
			LineUID:    lineUID,
			ReceivedAt: receivedAt,
			Payload:    datatypes.JSON(payloadBytes),
		}
		if err := h.db.Create(&record).Error; err != nil {
			respondMessage(c, http.StatusInternalServerError, "failed to persist webhook")
			return
		}

		if lineUID == nil {
			continue
		}

		friend := models.LineFriend{
			LineUID:     *lineUID,
			LineName:    deriveDisplayName(event),
			LinePicture: derivePictureURL(event, *lineUID),
			JoinedAt:    receivedAt,
			Status:      deriveLineFriendStatus(event.Type),
			LastPayload: datatypes.JSON(payloadBytes),
		}
		if event.Profile != nil && strings.TrimSpace(event.Profile.Phone) != "" {
			phone := strings.TrimSpace(event.Profile.Phone)
			friend.Phone = &phone
		}

		err := h.db.Transaction(func(tx *gorm.DB) error {
			var existing models.LineFriend
			if err := tx.First(&existing, "line_uid = ?", *lineUID).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					if err := tx.Create(&friend).Error; err != nil {
						return err
					}
					// webhook 首次落库后，如果客户主档已提前持有相同 line_uid，也同步刷新资料避免三方状态漂移。
					return syncCustomersFromLineFriend(tx, friend, nil)
				}
				return err
			}

			updates := map[string]any{
				"line_name":    friend.LineName,
				"line_picture": friend.LinePicture,
				"status":       friend.Status,
				"last_payload": friend.LastPayload,
			}
			// 既有好友收到非 follow 事件时，joined_at 应保持首次关注时间，避免客户档案被事件时间污染。
			friend.JoinedAt = existing.JoinedAt
			if event.Type == "follow" && receivedAt.Before(existing.JoinedAt) {
				friend.JoinedAt = receivedAt
				updates["joined_at"] = receivedAt
			}
			if friend.Phone != nil {
				updates["phone"] = friend.Phone
			}

			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}

			// 已绑定好友收到 webhook 后，客户主档应立即继承最新昵称、头像与 payload。
			friend.LinkedCustomerID = existing.LinkedCustomerID
			return syncCustomersFromLineFriend(tx, friend, existing.LinkedCustomerID)
		})
		if err != nil {
			respondMessage(c, http.StatusInternalServerError, "failed to update line friend")
			return
		}
	}

	c.String(http.StatusOK, "OK")
}

// errLineWebhookSecretNotConfigured 表示服务端未配置 LINE webhook 密钥，签名校验无法执行。
var errLineWebhookSecretNotConfigured = &lineWebhookValidationError{message: "line webhook secret is not configured"}

// lineWebhookValidationError 统一承载 webhook 校验失败原因，便于上层区分配置问题与非法请求。
type lineWebhookValidationError struct {
	// message 是返回给调用方的签名校验失败原因。
	message string
}

// Error 返回 webhook 校验错误文本。
func (e *lineWebhookValidationError) Error() string {
	return e.message
}

// validateLineWebhookSignature 按 LINE 官方规则校验请求体签名，避免伪造 webhook 绕过业务入口。
func (h *Handler) validateLineWebhookSignature(signature string, body []byte) error {
	if h.lineChannelSecret == "" {
		return errLineWebhookSecretNotConfigured
	}
	if strings.TrimSpace(signature) == "" {
		return &lineWebhookValidationError{message: "missing line webhook signature"}
	}

	mac := hmac.New(sha256.New, []byte(h.lineChannelSecret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return &lineWebhookValidationError{message: "invalid line webhook signature"}
	}

	return nil
}

// deriveLineFriendStatus 根据 webhook 事件类型维护好友状态，避免 unfollow 事件仍旧把状态写成 followed。
func deriveLineFriendStatus(eventType string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "unfollow":
		return "unfollowed"
	default:
		return "followed"
	}
}

// deriveDisplayName 从 webhook 事件中推导可展示的好友名称。
func deriveDisplayName(event lineEventPayload) string {
	if event.Profile != nil && strings.TrimSpace(event.Profile.DisplayName) != "" {
		return strings.TrimSpace(event.Profile.DisplayName)
	}
	if event.Source.UserID != "" {
		return "LINE User " + lastN(event.Source.UserID, 6)
	}
	return "LINE User"
}

// derivePictureURL 从 webhook 事件中提取头像地址，缺省时回退到占位头像。
func derivePictureURL(event lineEventPayload, uid string) string {
	if event.Profile != nil && strings.TrimSpace(event.Profile.PictureURL) != "" {
		return strings.TrimSpace(event.Profile.PictureURL)
	}
	return "https://api.dicebear.com/7.x/avataaars/svg?seed=" + uid
}

// normalizeString 去除首尾空白，空字符串统一收敛为 nil。
func normalizeString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// fallbackString 在主值为空白时返回指定回退值。
func fallbackString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// lastN 返回字符串尾部指定长度，常用于脱敏展示。
func lastN(value string, count int) string {
	if len(value) <= count {
		return value
	}
	return value[len(value)-count:]
}
