package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"cool-dispatch/internal/cloudflare"
	"cool-dispatch/internal/logger"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// bootstrapResponse 聚合前端首页和管理页当前需要的全部资源，避免开发态发起大量串行请求。
type bootstrapResponse struct {
	// Users 是系统用户列表。
	Users []models.User `json:"users"`
	// Customers 是客户主档列表。
	Customers []models.Customer `json:"customers"`
	// Appointments 是预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// LineFriends 是 LINE 好友列表。
	LineFriends []models.LineFriend `json:"line_friends"`
	// ExtraFeeProducts 是额外收费项目列表，保持与旧前端字段兼容。
	ExtraFeeProducts []models.ExtraItem `json:"extra_fee_products"`
	// CashLedger 是现金账流水列表。
	CashLedger []models.CashLedgerEntry `json:"cash_ledger_entries"`
	// Zones 是服务区域列表。
	Zones []models.ServiceZone `json:"zones"`
	// Reviews 是评价列表。
	Reviews []models.Review `json:"reviews"`
	// NotificationLogs 是通知日志列表。
	NotificationLogs []models.NotificationLog `json:"notification_logs"`
	// ServiceItems 是标准服务项目列表。
	ServiceItems []models.ServiceItem `json:"service_items"`
	// Settings 是首页和管理页共用的轻量系统设置。
	Settings bootstrapSettings `json:"settings"`
}

// bootstrapSettings 表示 bootstrap 接口中聚合返回的轻量设置。
type bootstrapSettings struct {
	// ReminderDays 是回访提醒天数。
	ReminderDays int `json:"reminder_days"`
}

// loginPayload 表示登录接口接受的最小凭证载荷。
type loginPayload struct {
	// Phone 是登录手机号。
	Phone string `json:"phone"`
	// Password 是登录明文密码。
	Password string `json:"password"`
}

// appointmentCreatePayload 仅允许前端提交创建预约所需的业务字段。
// 主键、创建时间、师傅名称、作业时间戳等服务端字段一律不接受，避免脏数据写入。
type appointmentCreatePayload struct {
	// CustomerName 是预约时填写的客户名称。
	CustomerName string `json:"customer_name"`
	// Address 是本次预约服务地址。
	Address string `json:"address"`
	// Phone 是本次预约联系电话。
	Phone string `json:"phone"`
	// Items 是服务项目 JSON 数组。
	Items json.RawMessage `json:"items"`
	// ExtraItems 是额外收费项 JSON 数组。
	ExtraItems json.RawMessage `json:"extra_items"`
	// PaymentMethod 是预约初始付款方式。
	PaymentMethod string `json:"payment_method"`
	// DiscountAmount 是折扣金额。
	DiscountAmount int `json:"discount_amount"`
	// ScheduledAt 是预约开始时间字符串。
	ScheduledAt string `json:"scheduled_at"`
	// ScheduledEnd 是预约结束时间字符串。
	ScheduledEnd *string `json:"scheduled_end"`
	// TechnicianID 是可选的预分配技师 ID。
	TechnicianID *uint `json:"technician_id"`
	// LineUID 是可选的 LINE 用户关联 UID。
	LineUID *string `json:"line_uid"`
}

// appointmentChargePayload 仅提取预约收费明细里的金额字段，用于后端重算总额，避免继续信任前端上传的 total_amount。
type appointmentChargePayload struct {
	// Price 是单个收费项金额。
	Price int `json:"price"`
}

// appointmentItemPayload 约束预约服务项的最小合法结构，避免脏对象在严格 JSON 只校验顶层时混入数据库。
type appointmentItemPayload struct {
	// ID 是服务项主键或前端临时标识。
	ID string `json:"id"`
	// Type 是服务项类型名称。
	Type string `json:"type"`
	// Note 是服务项备注。
	Note string `json:"note"`
	// Price 是服务项金额。
	Price int `json:"price"`
}

// appointmentExtraItemPayload 约束预约额外费用项的最小合法结构，避免后续编辑页拿到缺字段对象。
type appointmentExtraItemPayload struct {
	// ID 是额外收费项主键。
	ID string `json:"id"`
	// Name 是额外收费项名称。
	Name string `json:"name"`
	// Price 是额外收费项金额。
	Price int `json:"price"`
}

// appointmentPhotoPayload 只允许照片数组内出现字符串，避免对象或空字符串污染预约回传结构。
type appointmentPhotoPayload string

// serviceZoneDistrictPayload 用于解析服务区域中的行政区列表，以便后端根据地址自动回填 zone_id。
type serviceZoneDistrictPayload struct {
	// Districts 是服务区域包含的行政区 JSON 数组。
	Districts json.RawMessage `json:"districts"`
}

// appointmentUpdatePayload 用于预约更新，允许提交排程与作业进度字段，但仍不信任主键、创建时间和师傅名称。
type appointmentUpdatePayload struct {
	// CustomerName 是更新后的客户名称。
	CustomerName string `json:"customer_name"`
	// Address 是更新后的服务地址。
	Address string `json:"address"`
	// Phone 是更新后的联系电话。
	Phone string `json:"phone"`
	// Items 是更新后的服务项 JSON 数组。
	Items json.RawMessage `json:"items"`
	// ExtraItems 是更新后的额外收费项 JSON 数组。
	ExtraItems json.RawMessage `json:"extra_items"`
	// 更新路径允许按场景省略 payment_* 字段。
	// 普通编辑 legacy 旧资料时，前端只提交非支付字段，后端沿用既有支付状态；
	// 显式补录付款方式或确认收款时，再由前端提交完整支付写模型。
	// PaymentMethod 是可选的付款方式更新值。
	PaymentMethod *string `json:"payment_method"`
	// DiscountAmount 是更新后的折扣金额。
	DiscountAmount int `json:"discount_amount"`
	// PaidAmount 是可选的已收金额。
	PaidAmount *int `json:"paid_amount"`
	// ScheduledAt 是更新后的开始时间字符串。
	ScheduledAt string `json:"scheduled_at"`
	// ScheduledEnd 是更新后的结束时间字符串。
	ScheduledEnd *string `json:"scheduled_end"`
	// Status 是更新后的预约状态。
	Status string `json:"status"`
	// CancelReason 是取消原因。
	CancelReason *string `json:"cancel_reason"`
	// TechnicianID 是更新后的技师 ID。
	TechnicianID *uint `json:"technician_id"`
	// Lat 是更新后的纬度。
	Lat *float64 `json:"lat"`
	// Lng 是更新后的经度。
	Lng *float64 `json:"lng"`
	// CheckinTime 是签到时间字符串。
	CheckinTime *string `json:"checkin_time"`
	// CheckoutTime 是签退时间字符串。
	CheckoutTime *string `json:"checkout_time"`
	// DepartedTime 是出发时间字符串。
	DepartedTime *string `json:"departed_time"`
	// CompletedTime 是完成时间字符串。
	CompletedTime *string `json:"completed_time"`
	// Photos 是作业照片 JSON 数组。
	Photos json.RawMessage `json:"photos"`
	// PaymentReceived 表示是否确认收款。
	PaymentReceived *bool `json:"payment_received"`
	// SignatureData 是客户签名数据。
	SignatureData *string `json:"signature_data"`
	// LineUID 是更新后的 LINE 用户关联 UID。
	LineUID *string `json:"line_uid"`
}

// userPayload 是技师管理接口用于批量替换用户的写模型。
type userPayload struct {
	// ID 是用户主键。
	ID uint `json:"id"`
	// Name 是用户名称。
	Name string `json:"name"`
	// Role 是用户角色，占位保留给前端回传。
	Role string `json:"role"`
	// Phone 是联系电话。
	Phone string `json:"phone"`
	// Password 是明文密码，新增师傅时必填，编辑时可选（为空则保留原密码）。
	Password *string `json:"password,omitempty"`
	// Color 是技师展示色。
	Color *string `json:"color"`
	// Skills 是技能 JSON 数组。
	Skills json.RawMessage `json:"skills"`
	// ZoneID 是默认服务区域 ID。
	ZoneID *string `json:"zone_id"`
	// Availability 是可用时段 JSON 数组。
	Availability json.RawMessage `json:"availability"`
}

// technicianPasswordPayload 是修改技师密码的输入结构。
type technicianPasswordPayload struct {
	// Password 是新密码明文，至少 8 位。
	Password string `json:"password"`
}

// zonePayload 是区域管理接口的写模型。
type zonePayload struct {
	// ID 是区域主键。
	ID string `json:"id"`
	// Name 是区域名称。
	Name string `json:"name"`
	// Districts 是行政区 JSON 数组。
	Districts json.RawMessage `json:"districts"`
	// AssignedTechnicianIDs 是分配技师 ID JSON 数组。
	AssignedTechnicianIDs json.RawMessage `json:"assigned_technician_ids"`
}

// serviceItemPayload 是服务项目管理接口的写模型。
type serviceItemPayload struct {
	// ID 是服务项目主键。
	ID string `json:"id"`
	// Name 是服务项目名称。
	Name string `json:"name"`
	// DefaultPrice 是默认报价。
	DefaultPrice int `json:"default_price"`
	// Description 是项目描述。
	Description *string `json:"description"`
}

// extraItemPayload 是额外收费项管理接口的写模型。
type extraItemPayload struct {
	// ID 是额外收费项主键。
	ID string `json:"id"`
	// Name 是额外收费项名称。
	Name string `json:"name"`
	// Price 是额外收费项金额。
	Price int `json:"price"`
}

// customerPayload 前端客户管理页提交的客户资料结构。
type customerPayload struct {
	// ID 是客户主键。
	ID string `json:"id"`
	// Name 是客户名称。
	Name string `json:"name"`
	// Phone 是客户手机号。
	Phone string `json:"phone"`
	// Address 是客户地址。
	Address string `json:"address"`
	// LineID 是兼容旧数据的 LINE 标识。
	LineID *string `json:"line_id"`
	// LineName 是 LINE 昵称。
	LineName *string `json:"line_name"`
	// LinePicture 是 LINE 头像地址。
	LinePicture *string `json:"line_picture"`
	// LineUID 是 LINE 用户 UID。
	LineUID *string `json:"line_uid"`
	// LineJoinedAt 是关注时间字符串。
	LineJoinedAt *string `json:"line_joined_at"`
	// LineData 是 LINE 扩展资料 JSON。
	LineData json.RawMessage `json:"line_data"`
	// CreatedAt 是创建时间字符串。
	CreatedAt string `json:"created_at"`
}

// cashLedgerPayload 是现金账写接口的输入结构。
type cashLedgerPayload struct {
	// ID 是前端回传的临时标识，后端不会直接信任。
	ID string `json:"id"`
	// TechnicianID 是流水所属技师 ID。
	TechnicianID uint `json:"technician_id"`
	// AppointmentID 是可选关联预约 ID。
	AppointmentID *uint `json:"appointment_id"`
	// Type 是流水类型。
	Type string `json:"type"`
	// Amount 是流水金额。
	Amount int `json:"amount"`
	// Note 是流水备注。
	Note string `json:"note"`
	// CreatedAt 是流水时间字符串。
	CreatedAt string `json:"created_at"`
}

// reviewPayload 是评价写接口的输入结构。
type reviewPayload struct {
	// ID 是评价主键或前端临时标识。
	ID string `json:"id"`
	// Rating 是客户评分。
	Rating int `json:"rating"`
	// Misconducts 是异常行为 JSON 数组。
	Misconducts json.RawMessage `json:"misconducts"`
	// Comment 是评价内容。
	Comment string `json:"comment"`
	// SharedLine 表示是否已分享到 LINE。
	SharedLine bool `json:"shared_line"`
	// CreatedAt 是评价时间字符串。
	CreatedAt string `json:"created_at"`
}

// notificationPayload 是通知日志写接口的输入结构。
type notificationPayload struct {
	// ID 是通知日志主键或前端临时标识。
	ID string `json:"id"`
	// AppointmentID 是关联预约 ID。
	AppointmentID uint `json:"appointment_id"`
	// Type 是通知类型。
	Type string `json:"type"`
	// Message 是通知正文。
	Message string `json:"message"`
	// SentAt 是通知发送时间字符串。
	SentAt string `json:"sent_at"`
}

// reminderDaysPayload 是更新回访提醒天数时的输入结构。
type reminderDaysPayload struct {
	// ReminderDays 是回访提醒天数。
	ReminderDays int `json:"reminder_days"`
}

// webhookEnabledPayload 是更新 webhook 开关时的输入结构。
type webhookEnabledPayload struct {
	// Enabled 表示是否启用 webhook。
	Enabled bool `json:"enabled"`
}

// reviewContextResponse 是公开评价页需要的预约与评价上下文。
type reviewContextResponse struct {
	// Appointment 是被评价的预约。
	Appointment *models.Appointment `json:"appointment"`
	// Review 是已存在的评价记录，若未评价则为空。
	Review *models.Review `json:"review,omitempty"`
}

// webhookSettingsResponse 是系统设置页展示的 webhook 配置读模型。
type webhookSettingsResponse struct {
	// Enabled 是管理员配置的 webhook 开关。
	Enabled bool `json:"enabled"`
	// EffectiveEnabled 是综合依赖检查后的最终可用状态。
	EffectiveEnabled bool `json:"effective_enabled"`
	// URL 是展示给管理员的 webhook 地址。
	URL string `json:"url"`
	// URLSource 标识 URL 推导来源。
	URLSource string `json:"url_source"`
	// URLIsPublic 表示该 URL 是否为公网可访问地址。
	URLIsPublic bool `json:"url_is_public"`
	// HasLineChannelSecret 表示是否已配置 LINE channel secret。
	HasLineChannelSecret bool `json:"has_line_channel_secret"`
	// StatusMessage 是配置状态提示文案。
	StatusMessage string `json:"status_message"`
	// DependencySummary 是依赖项检查摘要。
	DependencySummary string `json:"dependency_summary"`
}

// settingsResponse 是设置接口对外返回的轻量系统配置。
type settingsResponse struct {
	// ReminderDays 是回访提醒天数。
	ReminderDays int `json:"reminder_days"`
	// Webhook 是 webhook 配置状态与依赖检查结果。
	Webhook webhookSettingsResponse `json:"webhook"`
}

// dashboardPageResponse 为首页总览页返回最小必要读取集合，避免该页继续强耦合 bootstrap 聚合包。
type dashboardPageResponse struct {
	// Appointments 是首页展示的预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// Technicians 是首页统计和看板需要的技师列表。
	Technicians []models.User `json:"technicians"`
	// Customers 是首页统计需要的客户列表。
	Customers []models.Customer `json:"customers"`
	// Reviews 是首页评价统计使用的评价列表。
	Reviews []models.Review `json:"reviews"`
}

// customerPageResponse 为顾客管理页返回客户主档、服务历史与评价明细所需数据。
type customerPageResponse struct {
	// Customers 是客户主档列表。
	Customers []models.Customer `json:"customers"`
	// Appointments 是客户历史预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// Reviews 是客户相关评价列表。
	Reviews []models.Review `json:"reviews"`
}

// settingsPageResponse 为系统设置页返回服务项目、额外费用与轻量设置。
type settingsPageResponse struct {
	// ServiceItems 是标准服务项目列表。
	ServiceItems []models.ServiceItem `json:"service_items"`
	// ExtraFeeProducts 是额外收费项目列表。
	ExtraFeeProducts []models.ExtraItem `json:"extra_fee_products"`
	// Settings 是系统轻量设置集合。
	Settings settingsResponse `json:"settings"`
}

// linePageResponse 为 LINE 管理页返回好友与客户绑定所需最小读取集合。
type linePageResponse struct {
	// LineFriends 是 LINE 好友列表。
	LineFriends []lineFriendResponse `json:"line_friends"`
	// Customers 是可绑定的客户主档列表。
	Customers []models.Customer `json:"customers"`
}

// technicianPageResponse 为师傅管理页返回师傅、工单、评价与区域的组合读模型。
type technicianPageResponse struct {
	// Technicians 是技师列表。
	Technicians []models.User `json:"technicians"`
	// Appointments 是技师关联预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// Reviews 是技师评价列表。
	Reviews []models.Review `json:"reviews"`
	// Zones 是区域配置列表。
	Zones []models.ServiceZone `json:"zones"`
}

// reminderPageResponse 为回访页返回客户、工单和提醒设置。
type reminderPageResponse struct {
	// Customers 是回访页候选客户列表。
	Customers []models.Customer `json:"customers"`
	// Appointments 是回访筛选用的预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// Settings 是回访页依赖的提醒设置。
	Settings settingsResponse `json:"settings"`
}

// zonePageResponse 为区域页返回区域与技师读模型。
type zonePageResponse struct {
	// Zones 是区域列表。
	Zones []models.ServiceZone `json:"zones"`
	// Technicians 是可分配技师列表。
	Technicians []models.User `json:"technicians"`
}

// zonePagePayload 保持测试与旧调用方兼容，避免页面级接口类型名改动扩大影响面。
type zonePagePayload = zonePageResponse

// financialReportPageResponse 为财务报表页返回工单与技师数据。
type financialReportPageResponse struct {
	// Appointments 是财务核算所需预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// Technicians 是财务页展示所需技师列表。
	Technicians []models.User `json:"technicians"`
}

// financialReportPagePayload 保持旧类型名兼容。
type financialReportPagePayload = financialReportPageResponse

// reviewDashboardPageResponse 为评价看板返回评价、技师和工单数据。
type reviewDashboardPageResponse struct {
	// Reviews 是评价列表。
	Reviews []models.Review `json:"reviews"`
	// Technicians 是评价归属技师列表。
	Technicians []models.User `json:"technicians"`
	// Appointments 是评价关联预约列表。
	Appointments []models.Appointment `json:"appointments"`
}

// reviewDashboardPagePayload 保持旧类型名兼容。
type reviewDashboardPagePayload = reviewDashboardPageResponse

// cashLedgerPageResponse 为现金账页返回技师、工单和现金流水数据。
type cashLedgerPageResponse struct {
	// Technicians 是现金账所属技师列表。
	Technicians []models.User `json:"technicians"`
	// Appointments 是现金账关联预约列表。
	Appointments []models.Appointment `json:"appointments"`
	// CashLedgerEntries 是现金流水列表。
	CashLedgerEntries []models.CashLedgerEntry `json:"cash_ledger_entries"`
}

// cashLedgerPagePayload 保持旧类型名兼容。
type cashLedgerPagePayload = cashLedgerPageResponse

// reviewShareLinePayload 是更新评价分享状态时的输入结构。
type reviewShareLinePayload struct {
	// SharedLine 表示评价是否已经分享至 LINE。
	SharedLine bool `json:"shared_line"`
}

// linkLineFriendCustomerPayload 是绑定或解绑 LINE 好友与客户时的输入结构。
type linkLineFriendCustomerPayload struct {
	// CustomerID 为空表示解绑，非空表示绑定到指定客户。
	CustomerID *string `json:"customer_id"`
}

const reviewTokenByteLength = 24

// Login 使用数据库中的密码哈希校验账号，通过后签发持久化 token 并设置 HttpOnly cookie。
// 同一用户同时只保留一个有效 token，旧 token 在新登录时自动失效。
func (h *Handler) Login(c *gin.Context) {
	var payload loginPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid login payload") {
		return
	}

	var user models.User
	if err := h.db.First(&user, "phone = ?", strings.TrimSpace(payload.Phone)).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusUnauthorized
		}
		respondMessage(c, status, "invalid credentials")
		return
	}

	if !security.VerifyPassword(strings.TrimSpace(payload.Password), user.PasswordHash) {
		respondMessage(c, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// 登录成功，签发新 token（自动删除该用户旧 token，确保同一用户同时只有一个有效 token）。
	authToken, err := createAuthToken(h.db, user.ID)
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to create auth token")
		return
	}

	// 将 token 写入 HttpOnly cookie，有效期 30 天。
	setAuthCookie(c, authToken.Token, int(tokenDuration.Seconds()), h.cookieSecure, h.cookieSameSite)

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// AuthMe 通过 cookie 中的 token 恢复登录态，前端页面刷新/服务重启后自动恢复用户身份。
// token 剩余有效期不足 29 天时，中间件已自动续期并刷新 cookie。
func (h *Handler) AuthMe(c *gin.Context) {
	tokenStr, err := c.Cookie(tokenCookieName)
	if err != nil || tokenStr == "" {
		respondMessage(c, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, renewed, err := validateAuthToken(h.db, tokenStr)
	if err != nil || user == nil {
		clearAuthCookie(c, h.cookieSecure, h.cookieSameSite)
		respondMessage(c, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// token 续期后刷新 cookie 有效期。
	if renewed {
		setAuthCookie(c, tokenStr, int(tokenDuration.Seconds()), h.cookieSecure, h.cookieSameSite)
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// Logout 注销当前用户：删除数据库中的 token 并清除 cookie。
func (h *Handler) Logout(c *gin.Context) {
	tokenStr, err := c.Cookie(tokenCookieName)
	if err == nil && tokenStr != "" {
		// 从数据库中删除该 token，使其立即失效。
		_ = h.db.Where("token = ?", tokenStr).Delete(&models.AuthToken{})
	}
	clearAuthCookie(c, h.cookieSecure, h.cookieSameSite)
	respondMessage(c, http.StatusOK, "logged out")
}

// Bootstrap 返回首页和多页历史兼容所需的聚合数据。
func (h *Handler) Bootstrap(c *gin.Context) {
	payload, err := h.loadBootstrapPayload()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load bootstrap data")
		return
	}

	c.JSON(http.StatusOK, payload)
}

// GetDashboardPageData 返回首页总览所需的最小数据集合，避免该页继续依赖 bootstrap 整包。
func (h *Handler) GetDashboardPageData(c *gin.Context) {
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load dashboard page data")
		return
	}
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load dashboard page data")
		return
	}
	customers, err := h.loadCustomers()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load dashboard page data")
		return
	}
	reviews, err := h.loadReviews()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load dashboard page data")
		return
	}

	c.JSON(http.StatusOK, dashboardPageResponse{
		Appointments: appointments,
		Technicians:  technicians,
		Customers:    customers,
		Reviews:      reviews,
	})
}

// GetCustomerPageData 返回顾客管理页依赖的客户、预约与评价数据。
func (h *Handler) GetCustomerPageData(c *gin.Context) {
	customers, err := h.loadCustomers()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load customer page data")
		return
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load customer page data")
		return
	}
	reviews, err := h.loadReviews()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load customer page data")
		return
	}

	c.JSON(http.StatusOK, customerPageResponse{
		Customers:    customers,
		Appointments: appointments,
		Reviews:      reviews,
	})
}

// GetSettingsPageData 返回系统设置页依赖的服务项目、额外费用与提醒设置。
func (h *Handler) GetSettingsPageData(c *gin.Context) {
	extraItems, err := h.loadExtraItems()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load settings page data")
		return
	}
	serviceItems, err := h.loadServiceItems()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load settings page data")
		return
	}
	settings, err := h.loadSettings()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load settings page data")
		return
	}

	c.JSON(http.StatusOK, settingsPageResponse{
		ServiceItems:     serviceItems,
		ExtraFeeProducts: extraItems,
		Settings:         h.buildSettingsResponse(settings),
	})
}

// GetLinePageData 返回 LINE 管理页依赖的好友与客户主档。
func (h *Handler) GetLinePageData(c *gin.Context) {
	lineFriends, err := h.loadLineFriends()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load line page data")
		return
	}
	customers, err := h.loadCustomers()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load line page data")
		return
	}

	c.JSON(http.StatusOK, linePageResponse{
		LineFriends: buildLineFriendResponses(lineFriends),
		Customers:   customers,
	})
}

// GetTechnicianPageData 返回师傅管理页需要的师傅、工单、评价和区域数据。
func (h *Handler) GetTechnicianPageData(c *gin.Context) {
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technician page data")
		return
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technician page data")
		return
	}
	reviews, err := h.loadReviews()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technician page data")
		return
	}
	zones, err := h.loadZones()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technician page data")
		return
	}

	c.JSON(http.StatusOK, technicianPageResponse{
		Technicians:  technicians,
		Appointments: appointments,
		Reviews:      reviews,
		Zones:        zones,
	})
}

// GetReminderPageData 返回回访页需要的客户、工单与提醒设置。
func (h *Handler) GetReminderPageData(c *gin.Context) {
	customers, err := h.loadCustomers()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load reminder page data")
		return
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load reminder page data")
		return
	}
	settings, err := h.loadSettings()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load reminder page data")
		return
	}

	c.JSON(http.StatusOK, reminderPageResponse{
		Customers:    customers,
		Appointments: appointments,
		Settings:     h.buildSettingsResponse(settings),
	})
}

// GetZonePageData 返回区域页需要的区域与技师数据。
func (h *Handler) GetZonePageData(c *gin.Context) {
	zones, err := h.loadZones()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load zone page data")
		return
	}
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load zone page data")
		return
	}

	c.JSON(http.StatusOK, zonePageResponse{
		Zones:       zones,
		Technicians: technicians,
	})
}

// GetFinancialReportPageData 返回财务页需要的工单与技师数据。
func (h *Handler) GetFinancialReportPageData(c *gin.Context) {
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load financial report page data")
		return
	}
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load financial report page data")
		return
	}

	c.JSON(http.StatusOK, financialReportPageResponse{
		Appointments: appointments,
		Technicians:  technicians,
	})
}

// GetReviewDashboardPageData 返回评价看板需要的评价、技师与工单数据。
func (h *Handler) GetReviewDashboardPageData(c *gin.Context) {
	reviews, err := h.loadReviews()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load review dashboard page data")
		return
	}
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load review dashboard page data")
		return
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load review dashboard page data")
		return
	}

	c.JSON(http.StatusOK, reviewDashboardPageResponse{
		Reviews:      reviews,
		Technicians:  technicians,
		Appointments: appointments,
	})
}

// GetCashLedgerPageData 返回现金账页需要的技师、工单和现金流水。
func (h *Handler) GetCashLedgerPageData(c *gin.Context) {
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load cash ledger page data")
		return
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load cash ledger page data")
		return
	}
	entries, err := h.loadCashLedger()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load cash ledger page data")
		return
	}

	c.JSON(http.StatusOK, cashLedgerPageResponse{
		Technicians:       technicians,
		Appointments:      appointments,
		CashLedgerEntries: entries,
	})
}

// ListAppointments 提供预约列表读取接口，供首页、列表、排程、财务等高频页面按资源域真实取数。
func (h *Handler) ListAppointments(c *gin.Context) {
	appointments, err := h.loadAppointments()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load appointments")
		return
	}
	c.JSON(http.StatusOK, appointments)
}

// ListTechnicians 提供师傅列表读取接口，避免前端继续从 bootstrap 中顺带取出师傅数据。
func (h *Handler) ListTechnicians(c *gin.Context) {
	technicians, err := h.loadTechnicians()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technicians")
		return
	}
	c.JSON(http.StatusOK, technicians)
}

// ListCustomers 提供客户列表读取接口，供客户管理、回访提醒、LINE 绑定等页面独立取数。
func (h *Handler) ListCustomers(c *gin.Context) {
	customers, err := h.loadCustomers()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load customers")
		return
	}
	c.JSON(http.StatusOK, customers)
}

// ListZones 提供服务区域读取接口，供创建预约自动派工与区域管理页面复用。
func (h *Handler) ListZones(c *gin.Context) {
	zones, err := h.loadZones()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load zones")
		return
	}
	c.JSON(http.StatusOK, zones)
}

// ListServiceItems 提供服务项目读取接口，供设置页与预约编辑页统一取数。
func (h *Handler) ListServiceItems(c *gin.Context) {
	items, err := h.loadServiceItems()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load service items")
		return
	}
	c.JSON(http.StatusOK, items)
}

// ListExtraItems 提供额外费用项目读取接口，避免设置页继续依赖 bootstrap 附带数组。
func (h *Handler) ListExtraItems(c *gin.Context) {
	items, err := h.loadExtraItems()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load extra items")
		return
	}
	c.JSON(http.StatusOK, items)
}

// ListCashLedgerEntries 提供现金账流水读取接口，供现金账页独立刷新数据库结果。
func (h *Handler) ListCashLedgerEntries(c *gin.Context) {
	entries, err := h.loadCashLedger()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load cash ledger entries")
		return
	}
	c.JSON(http.StatusOK, entries)
}

// ListReviews 提供评价列表读取接口，供评价看板与客户页统计独立读取。
func (h *Handler) ListReviews(c *gin.Context) {
	reviews, err := h.loadReviews()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load reviews")
		return
	}
	c.JSON(http.StatusOK, reviews)
}

// ListNotificationLogs 提供通知记录读取接口，供通知抽屉和后续通知中心页面复用。
func (h *Handler) ListNotificationLogs(c *gin.Context) {
	logs, err := h.loadNotificationLogs()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load notification logs")
		return
	}
	c.JSON(http.StatusOK, logs)
}

// GetSettings 返回系统级轻量配置，当前先暴露 reminder_days，便于前端摆脱 bootstrap 绑定。
func (h *Handler) GetSettings(c *gin.Context) {
	settings, err := h.loadSettings()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load settings")
		return
	}
	c.JSON(http.StatusOK, h.buildSettingsResponse(settings))
}

// GetReviewContext 为公开评价页按随机评价令牌返回最小必要数据，避免外链暴露自增预约 ID。
func (h *Handler) GetReviewContext(c *gin.Context) {
	reviewToken := strings.TrimSpace(c.Param("reviewToken"))
	if reviewToken == "" {
		respondMessage(c, http.StatusBadRequest, "invalid review token")
		return
	}

	var appointment models.Appointment
	if err := h.db.First(&appointment, "review_public_token = ?", reviewToken).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "appointment not found")
		return
	}

	var review models.Review
	result := h.db.First(&review, "appointment_id = ?", appointment.ID)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		respondMessage(c, http.StatusInternalServerError, "failed to load review context")
		return
	}

	response := reviewContextResponse{Appointment: &appointment}
	if result.Error == nil {
		response.Review = &review
	}

	c.JSON(http.StatusOK, response)
}

// CreateAppointment 创建预约并同步或更新客户主档。
func (h *Handler) CreateAppointment(c *gin.Context) {
	var payload appointmentCreatePayload
	if err := decodeStrictJSONBody(c, &payload); handleStrictJSONError(c, err) {
		return
	}

	appointment, err := appointmentModelFromCreatePayload(payload)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	// 创建预约时主键与创建时间统一由后端生成，避免前端临时 ID 污染 BIGSERIAL 序列。
	appointment.ID = 0
	appointment.CreatedAt = time.Time{}
	appointment.PaymentReceived = false
	appointment.PaidAmount = 0
	appointment.PaymentTime = nil
	appointment.ReviewToken = nil

	if err := h.applyAppointmentDerivedFields(&appointment); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.hydrateAppointmentTechnicianName(&appointment); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.ensureAppointmentReviewToken(&appointment); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to generate review token")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&appointment).Error; err != nil {
			return err
		}
		return upsertCustomerFromAppointment(tx, appointment)
	}); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to create appointment")
		return
	}

	c.JSON(http.StatusCreated, appointment)
}

// UpdateAppointment 更新预约，并限制技师只能修改自己的工单。
func (h *Handler) UpdateAppointment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid appointment id")
		return
	}

	var existing models.Appointment
	if err := h.db.First(&existing, "id = ?", id).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "appointment not found")
		return
	}

	// 已登录用户才能走到这里；若是技师，只允许修改分配给自己的工单，避免横向越权更新他人任务。
	user, err := currentUser(c)
	if err != nil {
		respondMessage(c, http.StatusUnauthorized, "authentication required")
		return
	}
	if user.Role == "technician" {
		if existing.TechnicianID == nil || *existing.TechnicianID != user.ID {
			respondMessage(c, http.StatusForbidden, "forbidden")
			return
		}
	}

	var payload appointmentUpdatePayload
	if err := decodeStrictJSONBody(c, &payload); handleStrictJSONError(c, err) {
		return
	}
	inheritOmittedAppointmentPaymentFields(&payload, existing)
	// 历史 `未收款` 旧值只允许原记录继续回写，避免新的编辑入口继续把占位值扩散成常态数据。
	if normalizePaymentMethod(*payload.PaymentMethod) == "未收款" && normalizePaymentMethod(existing.PaymentMethod) != "未收款" {
		respondMessage(c, http.StatusBadRequest, "invalid payment_method")
		return
	}
	appointment, err := appointmentModelFromUpdatePayload(payload)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}
	// 技师更新自己的工单时，不允许借机改派 technician_id，避免通过构造请求把工单转移给他人。
	if user.Role == "technician" && (payload.TechnicianID == nil || *payload.TechnicianID != user.ID) {
		respondMessage(c, http.StatusForbidden, "forbidden")
		return
	}
	// 历史旧资料曾残留 `payment_method=未收款 + paid_amount>0 + payment_received=false` 的脏组合。
	// 这类值会在前端按读模型原样回传时命中新的支付一致性校验，导致单纯编辑地址/备注也返回 400。
	// 这里仅在“旧记录本身就是 legacy 脏数据”且本次请求仍未确认收款时兜底清零 paid_amount，
	// 让更新链路可以先保存其它字段，同时不放松新资料与正常资料的支付校验。
	normalizeLegacyOutstandingPaymentFields(&existing, &appointment)
	// 更新路径的主键只信任 URL 参数，避免请求体伪造其它预约 ID。
	appointment.ID = uint(id)
	appointment.CreatedAt = existing.CreatedAt
	appointment.ReviewToken = existing.ReviewToken
	// payment_time 由服务端在确认收款时生成或沿用旧值，请求体不允许直接覆盖。
	appointment.PaymentTime = existing.PaymentTime

	if err := h.applyAppointmentDerivedFields(&appointment); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.hydrateAppointmentTechnicianName(&appointment); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&existing).Select("*").Updates(&appointment).Error; err != nil {
			return err
		}
		return upsertCustomerFromAppointment(tx, appointment)
	}); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to update appointment")
		return
	}

	c.JSON(http.StatusOK, appointment)
}

// inheritOmittedAppointmentPaymentFields 让更新接口在 payment_* 缺省时沿用数据库既有值。
// 这样前端就能在普通编辑 legacy 旧资料时避开把 `未收款` 强行归一成真实付款方式，
// 同时继续允许显式补录或确认收款场景提交完整支付写模型。
func inheritOmittedAppointmentPaymentFields(payload *appointmentUpdatePayload, existing models.Appointment) {
	if payload == nil {
		return
	}
	if payload.PaymentMethod == nil {
		value := existing.PaymentMethod
		payload.PaymentMethod = &value
	}
	if payload.PaidAmount == nil {
		value := existing.PaidAmount
		payload.PaidAmount = &value
	}
	if payload.PaymentReceived == nil {
		value := existing.PaymentReceived
		payload.PaymentReceived = &value
	}
}

// normalizeLegacyOutstandingPaymentFields 只处理历史遗留的脏支付组合，
// 避免旧资料在“未确认收款”状态下因为残留 paid_amount 被新的统一校验直接挡住。
func normalizeLegacyOutstandingPaymentFields(existing *models.Appointment, incoming *models.Appointment) {
	if existing == nil || incoming == nil {
		return
	}
	if normalizePaymentMethod(existing.PaymentMethod) != "未收款" {
		return
	}
	if existing.PaymentReceived || existing.PaidAmount <= 0 {
		return
	}
	if incoming.PaymentReceived {
		return
	}
	incoming.PaidAmount = 0
	incoming.PaymentTime = nil
}

// DeleteAppointment 删除指定预约记录，并异步清理关联的 Cloudflare 图床照片。
func (h *Handler) DeleteAppointment(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid appointment id")
		return
	}

	// 删除前先查出预约记录，提取照片列表用于后续图床清理。
	var appointment models.Appointment
	if err := h.db.First(&appointment, "id = ?", id).Error; err != nil {
		// 记录不存在也视为删除成功（幂等），但无需清理图床。
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"deleted": true})
			return
		}
		respondMessage(c, http.StatusInternalServerError, "failed to find appointment")
		return
	}

	// 提取照片 URL 列表，用于删除数据库记录后异步清理图床。
	var photoURLs []string
	if len(appointment.Photos) > 0 {
		_ = json.Unmarshal(appointment.Photos, &photoURLs)
	}

	if err := h.db.Delete(&models.Appointment{}, "id = ?", id).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to delete appointment")
		return
	}

	// 异步清理 Cloudflare 图床照片，不阻塞删除响应。
	// 清理失败仅打日志，不影响删除结果（图床会有孤儿图片，可定期人工清理）。
	if h.cfClient.IsConfigured() && len(photoURLs) > 0 {
		go func(urls []string) {
			for _, u := range urls {
				imageID := cloudflare.ExtractImageIDFromURL(u)
				if imageID == "" {
					continue // 非 Cloudflare 图片（如旧 Base64 数据），跳过
				}
				if err := h.cfClient.DeleteImage(imageID); err != nil {
					logger.Errorf("[cloudflare] async delete image %s failed: %v", imageID, err)
				}
			}
		}(photoURLs)
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ReplaceTechnicians 批量覆盖技师列表，缺失项会被视为删除。
// 密码字段为可选：新增师傅时如提供则设置，编辑时不提供则保留原密码。
func (h *Handler) ReplaceTechnicians(c *gin.Context) {
	var payload []userPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid technicians payload") {
		return
	}
	if len(payload) == 0 {
		respondMessage(c, http.StatusBadRequest, "technicians payload must not be empty")
		return
	}

	// 预处理：校验必填字段并为有密码的师傅计算哈希。
	type techEntry struct {
		model        models.User
		passwordHash string // 非空表示本次请求要设置/更新密码
	}
	entries := make([]techEntry, 0, len(payload))
	ids := make([]uint, 0, len(payload))
	for _, item := range payload {
		// 技师手机号同时承担登录唯一键与运维查找键，因此这里必须与数据库约束保持一致要求必填。
		if strings.TrimSpace(item.Name) == "" {
			respondMessage(c, http.StatusBadRequest, "technician name is required")
			return
		}
		if strings.TrimSpace(item.Phone) == "" {
			respondMessage(c, http.StatusBadRequest, "technician phone is required")
			return
		}
		var pwHash string
		if item.Password != nil && strings.TrimSpace(*item.Password) != "" {
			pw := strings.TrimSpace(*item.Password)
			if len(pw) < security.PasswordMinLength {
				respondMessage(c, http.StatusBadRequest, fmt.Sprintf("technician password must be at least %d characters", security.PasswordMinLength))
				return
			}
			hashed, err := security.HashPassword(pw)
			if err != nil {
				respondMessage(c, http.StatusInternalServerError, "failed to hash technician password")
				return
			}
			pwHash = hashed
		}
		model := models.User{
			ID:           item.ID,
			Name:         strings.TrimSpace(item.Name),
			Role:         "technician",
			Phone:        strings.TrimSpace(item.Phone),
			Color:        item.Color,
			Skills:       normalizeJSON(item.Skills, []byte("[]")),
			ZoneID:       item.ZoneID,
			Availability: normalizeJSON(item.Availability, []byte("[]")),
		}
		entries = append(entries, techEntry{model: model, passwordHash: pwHash})
		ids = append(ids, model.ID)
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		// 删除不在本次列表中的技师。
		deleteQuery := tx.Where("role = ?", "technician")
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&models.User{}).Error; err != nil {
			return err
		}
		for _, entry := range entries {
			// 先检查该师傅是否已存在。
			var existing models.User
			err := tx.First(&existing, "id = ? AND role = ?", entry.model.ID, "technician").Error
			if err == nil {
				// 已存在：更新非密码字段。
				updates := map[string]any{
					"name":         entry.model.Name,
					"phone":        entry.model.Phone,
					"color":        entry.model.Color,
					"skills":       entry.model.Skills,
					"zone_id":      entry.model.ZoneID,
					"availability": entry.model.Availability,
				}
				// 仅在请求中显式提供了密码时才覆盖，否则保留原密码哈希。
				if entry.passwordHash != "" {
					updates["password_hash"] = entry.passwordHash
				}
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// 新增师傅：设置密码哈希（若有）。
				newUser := entry.model
				if entry.passwordHash != "" {
					newUser.PasswordHash = entry.passwordHash
				}
				if err := tx.Create(&newUser).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	}); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to replace technicians")
		return
	}

	var result []models.User
	if err := h.db.Where("role = ?", "technician").Order("id asc").Find(&result).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load technicians")
		return
	}

	c.JSON(http.StatusOK, result)
}

// UpdateTechnicianPassword 修改指定技师的登录密码，同时吊销该技师的所有旧令牌。
// 仅管理员可操作，用于在技师详情页查看/重置密码。
func (h *Handler) UpdateTechnicianPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid technician id")
		return
	}

	var payload technicianPasswordPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid technician password payload") {
		return
	}

	password := strings.TrimSpace(payload.Password)
	if password == "" {
		respondMessage(c, http.StatusBadRequest, "technician password is required")
		return
	}
	if len(password) < security.PasswordMinLength {
		respondMessage(c, http.StatusBadRequest, fmt.Sprintf("technician password must be at least %d characters", security.PasswordMinLength))
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ? AND role = ?", id, "technician").Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "technician not found")
		return
	}

	hashed, err := security.HashPassword(password)
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to hash technician password")
		return
	}

	if err := h.db.Model(&user).Update("password_hash", hashed).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to update technician password")
		return
	}

	// 修改密码后吊销该技师的所有旧令牌，强制重新登录。
	_ = h.db.Where("user_id = ?", user.ID).Delete(&models.AuthToken{})

	c.JSON(http.StatusOK, gin.H{"message": "technician password updated"})
}

// ReplaceZones 批量覆盖服务区域列表。
func (h *Handler) ReplaceZones(c *gin.Context) {
	var payload []zonePayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid zones payload") {
		return
	}
	if len(payload) == 0 {
		respondMessage(c, http.StatusBadRequest, "zones payload must not be empty")
		return
	}

	zones := make([]models.ServiceZone, 0, len(payload))
	ids := make([]string, 0, len(payload))
	for _, item := range payload {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
			respondMessage(c, http.StatusBadRequest, "zone id and name are required")
			return
		}
		zones = append(zones, models.ServiceZone{
			ID:                    strings.TrimSpace(item.ID),
			Name:                  strings.TrimSpace(item.Name),
			Districts:             normalizeJSON(item.Districts, []byte("[]")),
			AssignedTechnicianIDs: normalizeJSON(item.AssignedTechnicianIDs, []byte("[]")),
		})
		ids = append(ids, strings.TrimSpace(item.ID))
	}

	if err := replaceZones(h.db, zones, ids); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to replace zones")
		return
	}

	var result []models.ServiceZone
	if err := h.db.Order("id asc").Find(&result).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load zones")
		return
	}

	c.JSON(http.StatusOK, result)
}

// ReplaceServiceItems 批量覆盖标准服务项目列表。
func (h *Handler) ReplaceServiceItems(c *gin.Context) {
	var payload []serviceItemPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid service items payload") {
		return
	}
	if len(payload) == 0 {
		respondMessage(c, http.StatusBadRequest, "service items payload must not be empty")
		return
	}

	items := make([]models.ServiceItem, 0, len(payload))
	ids := make([]string, 0, len(payload))
	for _, item := range payload {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
			respondMessage(c, http.StatusBadRequest, "service item id and name are required")
			return
		}
		items = append(items, models.ServiceItem{
			ID:           strings.TrimSpace(item.ID),
			Name:         strings.TrimSpace(item.Name),
			DefaultPrice: item.DefaultPrice,
			Description:  trimStringPtr(item.Description),
		})
		ids = append(ids, strings.TrimSpace(item.ID))
	}

	if err := replaceServiceItems(h.db, items, ids); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to replace service items")
		return
	}

	var result []models.ServiceItem
	if err := h.db.Order("id asc").Find(&result).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load service items")
		return
	}

	c.JSON(http.StatusOK, result)
}

// ReplaceExtraItems 批量覆盖额外收费项列表。
func (h *Handler) ReplaceExtraItems(c *gin.Context) {
	var payload []extraItemPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid extra items payload") {
		return
	}
	if len(payload) == 0 {
		respondMessage(c, http.StatusBadRequest, "extra items payload must not be empty")
		return
	}

	items := make([]models.ExtraItem, 0, len(payload))
	ids := make([]string, 0, len(payload))
	for _, item := range payload {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
			respondMessage(c, http.StatusBadRequest, "extra item id and name are required")
			return
		}
		items = append(items, models.ExtraItem{
			ID:    strings.TrimSpace(item.ID),
			Name:  strings.TrimSpace(item.Name),
			Price: item.Price,
		})
		ids = append(ids, strings.TrimSpace(item.ID))
	}

	if err := replaceExtraItems(h.db, items, ids); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to replace extra items")
		return
	}

	var result []models.ExtraItem
	if err := h.db.Order("id asc").Find(&result).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load extra items")
		return
	}

	c.JSON(http.StatusOK, result)
}

// CreateCashLedgerEntry 创建一条现金账流水记录。
func (h *Handler) CreateCashLedgerEntry(c *gin.Context) {
	var payload cashLedgerPayload
	if err := decodeStrictJSONBody(c, &payload); err != nil {
		handleStrictJSONError(c, err)
		return
	}
	if err := validateCashLedgerPayload(payload); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	createdAt, err := parseOptionalTime(payload.CreatedAt)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid created_at")
		return
	}
	if createdAt == nil {
		now := time.Now().UTC()
		createdAt = &now
	}

	entry := models.CashLedgerEntry{
		// 账务流水主键统一由后端生成，避免客户端复用旧 ID 覆盖或碰撞真实记录。
		ID:            "",
		TechnicianID:  payload.TechnicianID,
		AppointmentID: payload.AppointmentID,
		Type:          strings.TrimSpace(payload.Type),
		Amount:        payload.Amount,
		Note:          strings.TrimSpace(payload.Note),
		CreatedAt:     *createdAt,
	}
	entry.ID = "cl-" + strconv.FormatInt(time.Now().UnixMilli(), 10)

	if err := h.validateCashLedgerBusinessRules(payload); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.db.Create(&entry).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to create cash ledger entry")
		return
	}

	c.JSON(http.StatusCreated, entry)
}

// CreateReview 为已完工预约按随机评价令牌创建或覆盖评价记录，保证一张预约单只保留一份最新评价快照。
func (h *Handler) CreateReview(c *gin.Context) {
	reviewToken := strings.TrimSpace(c.Param("reviewToken"))
	if reviewToken == "" {
		respondMessage(c, http.StatusBadRequest, "invalid review token")
		return
	}

	var payload reviewPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid review payload") {
		return
	}
	if err := validateReviewPayload(payload); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	var appointment models.Appointment
	if err := h.db.First(&appointment, "review_public_token = ?", reviewToken).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "appointment not found")
		return
	}
	if appointment.Status != "completed" {
		respondMessage(c, http.StatusBadRequest, "appointment must be completed before review")
		return
	}

	createdAt, err := parseOptionalTime(payload.CreatedAt)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid created_at")
		return
	}
	if createdAt == nil {
		now := time.Now().UTC()
		createdAt = &now
	}

	review := models.Review{
		ID:             strings.TrimSpace(payload.ID),
		AppointmentID:  appointment.ID,
		CustomerName:   strings.TrimSpace(appointment.CustomerName),
		TechnicianID:   appointment.TechnicianID,
		TechnicianName: trimStringPtr(appointment.TechnicianName),
		Rating:         payload.Rating,
		Misconducts:    normalizeJSON(payload.Misconducts, []byte("[]")),
		Comment:        payload.Comment,
		SharedLine:     payload.SharedLine,
		CreatedAt:      *createdAt,
	}
	if review.ID == "" {
		review.ID = "rev-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	}

	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "appointment_id"}},
		UpdateAll: true,
	}).Create(&review).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to create review")
		return
	}

	c.JSON(http.StatusCreated, review)
}

// UpdateReviewShareLine 按随机评价令牌持久化评价页的 LINE 分享状态，确保前端分享操作会真实回写数据库。
func (h *Handler) UpdateReviewShareLine(c *gin.Context) {
	reviewToken := strings.TrimSpace(c.Param("reviewToken"))
	if reviewToken == "" {
		respondMessage(c, http.StatusBadRequest, "invalid review token")
		return
	}

	var payload reviewShareLinePayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid review share payload") {
		return
	}

	var review models.Review
	if err := h.db.Joins("JOIN appointments ON appointments.id = reviews.appointment_id").
		Where("appointments.review_public_token = ?", reviewToken).
		First(&review).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "review not found")
		return
	}

	review.SharedLine = payload.SharedLine
	if err := h.db.Model(&review).Update("shared_line", payload.SharedLine).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to update review share status")
		return
	}

	c.JSON(http.StatusOK, review)
}

// CreateNotificationLog 记录预约消息发送日志，供后台追踪提醒或通知是否已经发出。
func (h *Handler) CreateNotificationLog(c *gin.Context) {
	var payload notificationPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid notification payload") {
		return
	}
	if err := validateNotificationPayload(payload); err != nil {
		respondMessage(c, http.StatusBadRequest, err.Error())
		return
	}

	var appointment models.Appointment
	if err := h.db.First(&appointment, "id = ?", payload.AppointmentID).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "appointment not found")
		return
	}

	sentAt, err := parseOptionalTime(payload.SentAt)
	if err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid sent_at")
		return
	}
	if sentAt == nil {
		now := time.Now().UTC()
		sentAt = &now
	}

	item := models.NotificationLog{
		ID:            strings.TrimSpace(payload.ID),
		AppointmentID: payload.AppointmentID,
		Type:          strings.TrimSpace(payload.Type),
		Message:       strings.TrimSpace(payload.Message),
		SentAt:        *sentAt,
	}
	if item.ID == "" {
		item.ID = "notif-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	}

	if err := h.db.Create(&item).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to create notification log")
		return
	}

	c.JSON(http.StatusCreated, item)
}

// UpdateReminderDays 更新系统回访提醒天数配置，确保运营后台修改后立即持久化生效。
func (h *Handler) UpdateReminderDays(c *gin.Context) {
	var payload reminderDaysPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid reminder settings payload") {
		return
	}
	if payload.ReminderDays <= 0 {
		respondMessage(c, http.StatusBadRequest, "reminder_days must be greater than 0")
		return
	}

	description := "客戶完工後幾天提醒回訪"
	item := models.AppSetting{
		Key:         "reminder_days",
		Value:       strconv.Itoa(payload.ReminderDays),
		Description: &description,
	}

	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&item).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to update reminder_days")
		return
	}

	c.JSON(http.StatusOK, gin.H{"reminder_days": payload.ReminderDays})
}

// UpdateWebhookEnabled 更新管理员可控的 webhook 开关，但不直接修改环境变量依赖项。
func (h *Handler) UpdateWebhookEnabled(c *gin.Context) {
	var payload webhookEnabledPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid webhook settings payload") {
		return
	}

	description := "管理員控制 LINE webhook 處理開關；實際啟用仍取決於 secret 與公開基址"
	item := models.AppSetting{
		Key:         "line_webhook_enabled",
		Value:       strconv.FormatBool(payload.Enabled),
		Description: &description,
	}

	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&item).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to update webhook setting")
		return
	}

	settings, err := h.loadSettings()
	if err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to reload webhook setting")
		return
	}

	c.JSON(http.StatusOK, h.buildSettingsResponse(settings).Webhook)
}

// ReplaceCustomers 批量更新客户资料，用 upsert 逻辑确保前端编辑的客户数据能持久化到数据库。
func (h *Handler) ReplaceCustomers(c *gin.Context) {
	var payload []customerPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid customers payload") {
		return
	}
	if len(payload) == 0 {
		respondMessage(c, http.StatusBadRequest, "customers payload must not be empty")
		return
	}

	customers := make([]models.Customer, 0, len(payload))
	ids := make([]string, 0, len(payload))
	for _, item := range payload {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
			respondMessage(c, http.StatusBadRequest, "customer id and name are required")
			return
		}
		if strings.TrimSpace(item.Phone) == "" || strings.TrimSpace(item.Address) == "" {
			respondMessage(c, http.StatusBadRequest, "customer phone and address are required")
			return
		}
		createdAt, err := parseOptionalTime(item.CreatedAt)
		if err != nil {
			respondMessage(c, http.StatusBadRequest, "invalid created_at")
			return
		}
		if createdAt == nil {
			now := time.Now().UTC()
			createdAt = &now
		}
		var lineJoinedAt *time.Time
		if item.LineJoinedAt != nil {
			lineJoinedAt, err = parseOptionalTime(*item.LineJoinedAt)
			if err != nil {
				respondMessage(c, http.StatusBadRequest, "invalid line_joined_at")
				return
			}
		}
		customers = append(customers, models.Customer{
			ID:           strings.TrimSpace(item.ID),
			Name:         strings.TrimSpace(item.Name),
			Phone:        strings.TrimSpace(item.Phone),
			Address:      strings.TrimSpace(item.Address),
			LineID:       trimStringPtr(item.LineID),
			LineName:     trimStringPtr(item.LineName),
			LinePicture:  trimStringPtr(item.LinePicture),
			LineUID:      trimStringPtr(item.LineUID),
			LineJoinedAt: lineJoinedAt,
			LineData:     normalizeJSON(item.LineData, []byte(`{}`)),
			CreatedAt:    *createdAt,
		})
		ids = append(ids, strings.TrimSpace(item.ID))
	}

	if err := replaceCustomers(h.db, customers, ids); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to replace customers")
		return
	}

	var result []models.Customer
	if err := h.db.Order("created_at desc").Find(&result).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to load customers")
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteCustomer 删除指定客户，前端客户管理页删除操作使用。
func (h *Handler) DeleteCustomer(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		respondMessage(c, http.StatusBadRequest, "invalid customer id")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.LineFriend{}).
			Where("linked_customer_id = ?", id).
			Update("linked_customer_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Customer{}, "id = ?", id).Error
	}); err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to delete customer")
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// LinkLineFriendCustomer 维护 LINE 好友与客户的绑定关系，并同步客户档案中的 LINE 字段。
func (h *Handler) LinkLineFriendCustomer(c *gin.Context) {
	lineUID := strings.TrimSpace(c.Param("lineUid"))
	if lineUID == "" {
		respondMessage(c, http.StatusBadRequest, "invalid line uid")
		return
	}

	var payload linkLineFriendCustomerPayload
	if err := c.ShouldBindJSON(&payload); handleBindJSONError(c, err, "invalid line friend payload") {
		return
	}

	var customerID *string
	if payload.CustomerID != nil {
		trimmed := strings.TrimSpace(*payload.CustomerID)
		if trimmed != "" {
			customerID = &trimmed
		}
	}

	var lineFriend models.LineFriend
	if err := h.db.First(&lineFriend, "line_uid = ?", lineUID).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		respondMessage(c, status, "line friend not found")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		// 记录旧绑定客户，后续若改绑或解绑，需要同步清空旧客户主档中的 LINE 字段。
		previousLinkedCustomerID := normalizeOptionalString(lineFriend.LinkedCustomerID)
		if customerID != nil {
			var customer models.Customer
			if err := tx.First(&customer, "id = ?", *customerID).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.LineFriend{}).
				Where("linked_customer_id = ? AND line_uid <> ?", *customerID, lineUID).
				Update("linked_customer_id", nil).Error; err != nil {
				return err
			}

			if err := clearCustomerLineFieldsByLineUID(tx, lineUID, customerID); err != nil {
				return err
			}

			if err := tx.Model(&customer).Updates(map[string]any{
				"line_id":        lineFriend.LineUID,
				"line_uid":       lineFriend.LineUID,
				"line_name":      lineFriend.LineName,
				"line_picture":   lineFriend.LinePicture,
				"line_joined_at": lineFriend.JoinedAt,
				"line_data":      normalizeDatatypesJSON(lineFriend.LastPayload, []byte(`{}`)),
				"updated_at":     time.Now().UTC(),
			}).Error; err != nil {
				return err
			}

			// 同一个好友改绑到新客户时，必须同步清空旧客户主档，避免列表里残留旧的 line_uid/头像/昵称。
			if previousLinkedCustomerID != nil && *previousLinkedCustomerID != *customerID {
				if err := clearCustomerLineFieldsByID(tx, *previousLinkedCustomerID); err != nil {
					return err
				}
			}
		} else if lineFriend.LinkedCustomerID != nil {
			if err := clearCustomerLineFieldsByID(tx, *lineFriend.LinkedCustomerID); err != nil {
				return err
			}
		}

		if err := tx.Model(&lineFriend).Update("linked_customer_id", customerID).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		status := http.StatusInternalServerError
		message := "failed to link line friend"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			message = "customer not found"
		}
		respondMessage(c, status, message)
		return
	}

	if err := h.db.First(&lineFriend, "line_uid = ?", lineUID).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "failed to reload line friend")
		return
	}

	c.JSON(http.StatusOK, lineFriend)
}

// normalizeOptionalString 统一把 *string 规整为去空白后的可选值，避免同一事务里多次重复 trim 判空。
func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// loadBootstrapPayload 汇总 bootstrap 接口需要的全部资源，避免调用层散落重复取数逻辑。
func (h *Handler) loadBootstrapPayload() (bootstrapResponse, error) {
	users, err := h.loadUsers()
	if err != nil {
		return bootstrapResponse{}, err
	}
	customers, err := h.loadCustomers()
	if err != nil {
		return bootstrapResponse{}, err
	}
	appointments, err := h.loadAppointments()
	if err != nil {
		return bootstrapResponse{}, err
	}
	lineFriends, err := h.loadLineFriends()
	if err != nil {
		return bootstrapResponse{}, err
	}
	extraItems, err := h.loadExtraItems()
	if err != nil {
		return bootstrapResponse{}, err
	}
	cashLedger, err := h.loadCashLedger()
	if err != nil {
		return bootstrapResponse{}, err
	}
	zones, err := h.loadZones()
	if err != nil {
		return bootstrapResponse{}, err
	}
	reviews, err := h.loadReviews()
	if err != nil {
		return bootstrapResponse{}, err
	}
	notificationLogs, err := h.loadNotificationLogs()
	if err != nil {
		return bootstrapResponse{}, err
	}
	serviceItems, err := h.loadServiceItems()
	if err != nil {
		return bootstrapResponse{}, err
	}
	settings, err := h.loadSettings()
	if err != nil {
		return bootstrapResponse{}, err
	}

	return bootstrapResponse{
		Users:            users,
		Customers:        customers,
		Appointments:     appointments,
		LineFriends:      lineFriends,
		ExtraFeeProducts: extraItems,
		CashLedger:       cashLedger,
		Zones:            zones,
		Reviews:          reviews,
		NotificationLogs: notificationLogs,
		ServiceItems:     serviceItems,
		Settings:         bootstrapSettings{ReminderDays: reminderDaysFromSettings(settings)},
	}, nil
}

// loadUsers 统一封装用户查询，供 bootstrap 与资源级读接口复用。
func (h *Handler) loadUsers() ([]models.User, error) {
	var users []models.User
	if err := h.db.Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(users), nil
}

// loadTechnicians 只返回 role=technician 的用户，避免管理页自行从 users 列表二次裁剪。
func (h *Handler) loadTechnicians() ([]models.User, error) {
	var technicians []models.User
	if err := h.db.Where("role = ?", "technician").Order("id asc").Find(&technicians).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(technicians), nil
}

// loadCustomers 统一读取客户主档列表，默认按创建时间倒序返回。
func (h *Handler) loadCustomers() ([]models.Customer, error) {
	var customers []models.Customer
	if err := h.db.Order("created_at desc").Find(&customers).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(customers), nil
}

// loadAppointments 统一读取预约列表，默认按排程时间倒序返回。
func (h *Handler) loadAppointments() ([]models.Appointment, error) {
	var appointments []models.Appointment
	if err := h.db.Order("scheduled_at desc").Find(&appointments).Error; err != nil {
		return nil, err
	}
	for i := range appointments {
		if err := h.ensureAppointmentReviewToken(&appointments[i]); err != nil {
			return nil, err
		}
	}

	return emptyIfNil(appointments), nil
}

// loadLineFriends 统一读取 LINE 好友列表，默认按关注时间倒序返回。
func (h *Handler) loadLineFriends() ([]models.LineFriend, error) {
	var lineFriends []models.LineFriend
	if err := h.db.Order("joined_at desc").Find(&lineFriends).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(lineFriends), nil
}

// loadExtraItems 统一读取额外收费项列表，默认按 ID 升序返回。
func (h *Handler) loadExtraItems() ([]models.ExtraItem, error) {
	var items []models.ExtraItem
	if err := h.db.Order("id asc").Find(&items).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(items), nil
}

// loadCashLedger 统一读取现金流水列表，默认按创建时间倒序返回。
func (h *Handler) loadCashLedger() ([]models.CashLedgerEntry, error) {
	var entries []models.CashLedgerEntry
	if err := h.db.Order("created_at desc").Find(&entries).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(entries), nil
}

// loadZones 统一读取服务区域列表，默认按 ID 升序返回。
func (h *Handler) loadZones() ([]models.ServiceZone, error) {
	var zones []models.ServiceZone
	if err := h.db.Order("id asc").Find(&zones).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(zones), nil
}

// loadReviews 统一读取评价列表，默认按创建时间倒序返回。
func (h *Handler) loadReviews() ([]models.Review, error) {
	var reviews []models.Review
	if err := h.db.Order("created_at desc").Find(&reviews).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(reviews), nil
}

// loadNotificationLogs 统一读取通知日志列表，默认按发送时间倒序返回。
func (h *Handler) loadNotificationLogs() ([]models.NotificationLog, error) {
	var logs []models.NotificationLog
	if err := h.db.Order("sent_at desc").Find(&logs).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(logs), nil
}

// loadServiceItems 统一读取标准服务项目列表，默认按 ID 升序返回。
func (h *Handler) loadServiceItems() ([]models.ServiceItem, error) {
	var items []models.ServiceItem
	if err := h.db.Order("id asc").Find(&items).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(items), nil
}

// loadSettings 统一读取系统设置项列表，供设置接口和 webhook 判定复用。
func (h *Handler) loadSettings() ([]models.AppSetting, error) {
	var settings []models.AppSetting
	if err := h.db.Find(&settings).Error; err != nil {
		return nil, err
	}

	return emptyIfNil(settings), nil
}

// emptyIfNil 把 nil slice 统一转为空数组，避免前端收到 null 后继续出现 .map/.filter 崩溃。
func emptyIfNil[T any](items []T) []T {
	if items == nil {
		return make([]T, 0)
	}

	return items
}

// reminderDaysFromSettings 从设置列表中解析回访提醒天数，缺失或非法时回退默认值。
func reminderDaysFromSettings(settings []models.AppSetting) int {
	for _, item := range settings {
		if item.Key == "reminder_days" {
			value, err := strconv.Atoi(item.Value)
			if err == nil && value > 0 {
				return value
			}
		}
	}

	return 180
}

// lineWebhookEnabledFromSettings 读取管理员持久化开关；未配置时默认保持启用，避免升级后意外关闭既有 webhook。
func lineWebhookEnabledFromSettings(settings []models.AppSetting) bool {
	for _, item := range settings {
		if item.Key == "line_webhook_enabled" {
			value, err := strconv.ParseBool(strings.TrimSpace(item.Value))
			if err == nil {
				return value
			}
		}
	}

	return true
}

// buildSettingsResponse 统一拼装系统设置读模型，避免 `/api/settings` 与设置页接口各自复制 webhook 判定逻辑。
func (h *Handler) buildSettingsResponse(settings []models.AppSetting) settingsResponse {
	webhookEnabled := lineWebhookEnabledFromSettings(settings)
	hasLineChannelSecret := strings.TrimSpace(h.lineChannelSecret) != ""
	webhookURL := ""
	if strings.TrimSpace(h.webhookBaseURL) != "" {
		webhookURL = strings.TrimRight(strings.TrimSpace(h.webhookBaseURL), "/") + "/api/webhook/line"
	}
	effectiveEnabled := webhookEnabled && hasLineChannelSecret && h.hasPublicWebhookBaseURL

	statusMessage := "管理員已啟用 webhook，且 LINE secret 與公開網址均已就緒。"
	switch {
	case !webhookEnabled:
		statusMessage = "管理員已停用 webhook；即使外部平台持續回呼，服務端也會拒絕處理。"
	case !hasLineChannelSecret && !h.hasPublicWebhookBaseURL:
		statusMessage = "已開啟 webhook 開關，但缺少 LINE_CHANNEL_SECRET 與公開基址，目前無法對外提供可用回呼。"
	case !hasLineChannelSecret:
		statusMessage = "已開啟 webhook 開關，但缺少 LINE_CHANNEL_SECRET，目前無法校驗 LINE webhook 簽章。"
	case !h.hasPublicWebhookBaseURL:
		statusMessage = "已開啟 webhook 開關，但目前僅有本機除錯位址，LINE 平台仍無法從公開網路回呼。"
	}

	dependencySummary := "啟用條件：管理員開關已開啟、已設定 LINE_CHANNEL_SECRET，且能產生公開可存取的 webhook URL。"

	return settingsResponse{
		ReminderDays: reminderDaysFromSettings(settings),
		Webhook: webhookSettingsResponse{
			Enabled:              webhookEnabled,
			EffectiveEnabled:     effectiveEnabled,
			URL:                  webhookURL,
			URLSource:            h.webhookBaseURLSource,
			URLIsPublic:          h.hasPublicWebhookBaseURL,
			HasLineChannelSecret: hasLineChannelSecret,
			StatusMessage:        statusMessage,
			DependencySummary:    dependencySummary,
		},
	}
}

// nilToEmpty* helpers 把 nil slice 固定序列化成 []，避免前端列表页切到资源接口后再次处理 null 分支。
// nilToEmptyUsers 把用户切片中的 nil 固定转为空数组。
func nilToEmptyUsers(items []models.User) []models.User {
	if items == nil {
		return make([]models.User, 0)
	}
	return items
}

// nilToEmptyCustomers 把客户切片中的 nil 固定转为空数组。
func nilToEmptyCustomers(items []models.Customer) []models.Customer {
	if items == nil {
		return make([]models.Customer, 0)
	}
	return items
}

// nilToEmptyAppointments 把预约切片中的 nil 固定转为空数组。
func nilToEmptyAppointments(items []models.Appointment) []models.Appointment {
	if items == nil {
		return make([]models.Appointment, 0)
	}
	return items
}

// nilToEmptyZones 把区域切片中的 nil 固定转为空数组。
func nilToEmptyZones(items []models.ServiceZone) []models.ServiceZone {
	if items == nil {
		return make([]models.ServiceZone, 0)
	}
	return items
}

// nilToEmptyServiceItems 把服务项目切片中的 nil 固定转为空数组。
func nilToEmptyServiceItems(items []models.ServiceItem) []models.ServiceItem {
	if items == nil {
		return make([]models.ServiceItem, 0)
	}
	return items
}

// nilToEmptyExtraItems 把额外收费项切片中的 nil 固定转为空数组。
func nilToEmptyExtraItems(items []models.ExtraItem) []models.ExtraItem {
	if items == nil {
		return make([]models.ExtraItem, 0)
	}
	return items
}

// nilToEmptyCashLedger 把现金流水切片中的 nil 固定转为空数组。
func nilToEmptyCashLedger(items []models.CashLedgerEntry) []models.CashLedgerEntry {
	if items == nil {
		return make([]models.CashLedgerEntry, 0)
	}
	return items
}

// nilToEmptyReviews 把评价切片中的 nil 固定转为空数组。
func nilToEmptyReviews(items []models.Review) []models.Review {
	if items == nil {
		return make([]models.Review, 0)
	}
	return items
}

// nilToEmptyNotificationLogs 把通知日志切片中的 nil 固定转为空数组。
func nilToEmptyNotificationLogs(items []models.NotificationLog) []models.NotificationLog {
	if items == nil {
		return make([]models.NotificationLog, 0)
	}
	return items
}

// appointmentModelFromCreatePayload 把创建预约写模型转换为数据库模型，并在转换阶段完成基础校验。
func appointmentModelFromCreatePayload(payload appointmentCreatePayload) (models.Appointment, error) {
	if err := validateAppointmentCreatePayload(payload); err != nil {
		return models.Appointment{}, err
	}

	scheduledAt, err := parseRequiredTime(payload.ScheduledAt, "scheduled_at")
	if err != nil {
		return models.Appointment{}, err
	}
	scheduledEnd, err := parseOptionalTimePtr(payload.ScheduledEnd)
	if err != nil {
		return models.Appointment{}, err
	}
	if scheduledEnd != nil && !scheduledEnd.After(*scheduledAt) {
		return models.Appointment{}, errors.New("scheduled_end must be later than scheduled_at")
	}
	itemsJSON, itemsTotal, err := normalizeAppointmentItemsJSON(payload.Items)
	if err != nil {
		return models.Appointment{}, err
	}
	extraItemsJSON, extraItemsTotal, err := normalizeAppointmentExtraItemsJSON(payload.ExtraItems)
	if err != nil {
		return models.Appointment{}, err
	}
	totalAmount := itemsTotal + extraItemsTotal - payload.DiscountAmount
	if totalAmount < 0 {
		return models.Appointment{}, errors.New("discount_amount cannot be greater than subtotal")
	}

	model := models.Appointment{
		CustomerName:   strings.TrimSpace(payload.CustomerName),
		Address:        strings.TrimSpace(payload.Address),
		Phone:          strings.TrimSpace(payload.Phone),
		Items:          itemsJSON,
		ExtraItems:     extraItemsJSON,
		PaymentMethod:  normalizePaymentMethod(payload.PaymentMethod),
		TotalAmount:    totalAmount,
		DiscountAmount: payload.DiscountAmount,
		ScheduledAt:    scheduledAt.UTC(),
		ScheduledEnd:   normalizeTimePtrUTC(scheduledEnd),
		Status:         deriveAppointmentCreateStatus(payload.TechnicianID),
		TechnicianID:   payload.TechnicianID,
		LineUID:        trimStringPtr(payload.LineUID),
	}

	return model, nil
}

// appointmentModelFromUpdatePayload 把更新预约写模型转换为数据库模型，并统一标准化时间和 JSON 字段。
func appointmentModelFromUpdatePayload(payload appointmentUpdatePayload) (models.Appointment, error) {
	if err := validateAppointmentUpdatePayload(payload); err != nil {
		return models.Appointment{}, err
	}

	scheduledAt, err := parseRequiredTime(payload.ScheduledAt, "scheduled_at")
	if err != nil {
		return models.Appointment{}, err
	}
	scheduledEnd, err := parseOptionalTimePtr(payload.ScheduledEnd)
	if err != nil {
		return models.Appointment{}, err
	}
	if scheduledEnd != nil && !scheduledEnd.After(*scheduledAt) {
		return models.Appointment{}, errors.New("scheduled_end must be later than scheduled_at")
	}
	checkinTime, err := parseOptionalTimePtr(payload.CheckinTime)
	if err != nil {
		return models.Appointment{}, err
	}
	checkoutTime, err := parseOptionalTimePtr(payload.CheckoutTime)
	if err != nil {
		return models.Appointment{}, err
	}
	departedTime, err := parseOptionalTimePtr(payload.DepartedTime)
	if err != nil {
		return models.Appointment{}, err
	}
	completedTime, err := parseOptionalTimePtr(payload.CompletedTime)
	if err != nil {
		return models.Appointment{}, err
	}
	itemsJSON, itemsTotal, err := normalizeAppointmentItemsJSON(payload.Items)
	if err != nil {
		return models.Appointment{}, err
	}
	extraItemsJSON, extraItemsTotal, err := normalizeAppointmentExtraItemsJSON(payload.ExtraItems)
	if err != nil {
		return models.Appointment{}, err
	}
	photosJSON, err := normalizeAppointmentPhotosJSON(payload.Photos)
	if err != nil {
		return models.Appointment{}, err
	}
	totalAmount := itemsTotal + extraItemsTotal - payload.DiscountAmount
	if totalAmount < 0 {
		return models.Appointment{}, errors.New("discount_amount cannot be greater than subtotal")
	}
	model := models.Appointment{
		CustomerName:    strings.TrimSpace(payload.CustomerName),
		Address:         strings.TrimSpace(payload.Address),
		Phone:           strings.TrimSpace(payload.Phone),
		Items:           itemsJSON,
		ExtraItems:      extraItemsJSON,
		PaymentMethod:   normalizePaymentMethod(*payload.PaymentMethod),
		TotalAmount:     totalAmount,
		DiscountAmount:  payload.DiscountAmount,
		PaidAmount:      *payload.PaidAmount,
		ScheduledAt:     scheduledAt.UTC(),
		ScheduledEnd:    normalizeTimePtrUTC(scheduledEnd),
		Status:          normalizeAppointmentStatus(payload.Status),
		CancelReason:    trimStringPtr(payload.CancelReason),
		TechnicianID:    payload.TechnicianID,
		Lat:             payload.Lat,
		Lng:             payload.Lng,
		CheckinTime:     normalizeTimePtrUTC(checkinTime),
		CheckoutTime:    normalizeTimePtrUTC(checkoutTime),
		DepartedTime:    normalizeTimePtrUTC(departedTime),
		CompletedTime:   normalizeTimePtrUTC(completedTime),
		Photos:          photosJSON,
		PaymentReceived: *payload.PaymentReceived,
		SignatureData:   trimStringPtr(payload.SignatureData),
		LineUID:         trimStringPtr(payload.LineUID),
	}

	return model, nil
}

// decodeStrictJSONBytes 对任意 JSON 字节做严格解码，统一拒绝未知字段与多余尾随数据。
func decodeStrictJSONBytes(rawBody []byte, target any) error {
	if len(bytes.TrimSpace(rawBody)) == 0 {
		return errors.New("empty request body")
	}

	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid json payload")
	}
	return nil
}

// isRequestBodyTooLarge 统一识别 MaxBytesReader 触发的超限错误，便于所有写接口返回一致的 413。
func isRequestBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// handleStrictJSONError 统一处理严格 JSON 解码错误。
// 对请求体过大返回 413，其余错误继续保留原始校验信息，方便前端和测试定位具体字段问题。
func handleStrictJSONError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if isRequestBodyTooLarge(err) {
		respondMessage(c, http.StatusRequestEntityTooLarge, "request body too large")
		return true
	}
	respondMessage(c, http.StatusBadRequest, err.Error())
	return true
}

// handleBindJSONError 统一处理 ShouldBindJSON 一类宽松 JSON 绑定错误。
// 对于超限请求体返回 413，其余错误保持业务侧自定义的 400 提示。
func handleBindJSONError(c *gin.Context, err error, message string) bool {
	if err == nil {
		return false
	}
	if isRequestBodyTooLarge(err) {
		respondMessage(c, http.StatusRequestEntityTooLarge, "request body too large")
		return true
	}
	respondMessage(c, http.StatusBadRequest, message)
	return true
}

// decodeStrictJSONBody 对写接口启用严格 JSON 解码，拒绝未知字段，避免旧前端或脏请求把服务端字段混入写模型。
func decodeStrictJSONBody(c *gin.Context, target any) error {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return decodeStrictJSONBytes(rawBody, target)
}

// hydrateAppointmentTechnicianName 在写入前统一根据 technician_id 回填真实师傅名称，避免客户端伪造 technician_name。
func (h *Handler) hydrateAppointmentTechnicianName(appointment *models.Appointment) error {
	if appointment.TechnicianID == nil {
		appointment.TechnicianName = nil
		return nil
	}

	var technician models.User
	if err := h.db.Select("id", "name", "role").First(&technician, "id = ?", *appointment.TechnicianID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("technician not found")
		}
		return err
	}
	if strings.TrimSpace(technician.Role) != "technician" {
		return errors.New("technician_id must reference a technician user")
	}

	name := strings.TrimSpace(technician.Name)
	if name == "" {
		appointment.TechnicianName = nil
		return nil
	}
	appointment.TechnicianName = &name
	return nil
}

// applyAppointmentDerivedFields 在落库前补齐服务端派生字段，避免总额、区域、收款状态继续信任前端。
func (h *Handler) applyAppointmentDerivedFields(appointment *models.Appointment) error {
	appointment.PaymentMethod = normalizePaymentMethod(appointment.PaymentMethod)
	appointment.Status = normalizeAppointmentStatus(appointment.Status)
	appointment.ZoneID = trimStringPtr(appointment.ZoneID)
	appointment.LineUID = trimStringPtr(appointment.LineUID)
	appointment.CancelReason = trimStringPtr(appointment.CancelReason)
	appointment.SignatureData = trimStringPtr(appointment.SignatureData)
	appointment.ScheduledAt = appointment.ScheduledAt.UTC()
	appointment.ScheduledEnd = normalizeTimePtrUTC(appointment.ScheduledEnd)
	appointment.CheckinTime = normalizeTimePtrUTC(appointment.CheckinTime)
	appointment.CheckoutTime = normalizeTimePtrUTC(appointment.CheckoutTime)
	appointment.DepartedTime = normalizeTimePtrUTC(appointment.DepartedTime)
	appointment.CompletedTime = normalizeTimePtrUTC(appointment.CompletedTime)
	appointment.PaymentTime = normalizeTimePtrUTC(appointment.PaymentTime)

	// ---------- 服务端时间自动填充 ----------
	// 当状态驱动的时间字段为空时，用服务器当前 UTC 时间自动补充，
	// 确保所有关键业务时间戳来自后端，不依赖客户端本地时钟。
	now := time.Now().UTC()

	// 状态为 arrived 或更后阶段时，自动补充出发时间
	if isOneOf(appointment.Status, "arrived", "completed") && appointment.DepartedTime == nil {
		appointment.DepartedTime = &now
	}
	// 状态为 arrived 时，自动补充签到时间（如果前端没传）
	if isOneOf(appointment.Status, "arrived", "completed") && appointment.CheckinTime == nil {
		appointment.CheckinTime = &now
	}
	// 状态为 completed 时，自动补充完工时间
	if appointment.Status == "completed" && appointment.CompletedTime == nil {
		appointment.CompletedTime = &now
	}


	if appointment.TechnicianID != nil && appointment.Status == "pending" {
		appointment.Status = "assigned"
	}
	if appointment.TechnicianID == nil && isOneOf(appointment.Status, "assigned", "arrived", "completed") {
		return errors.New("technician_id is required for the current status")
	}
	// 经纬度必须成对出现，并且坐标范围必须落在真实地理范围内，避免前端半写入或脏值污染地图数据。
	if (appointment.Lat == nil) != (appointment.Lng == nil) {
		return errors.New("lat and lng must either both be set or both be empty")
	}
	if appointment.Lat != nil && (*appointment.Lat < -90 || *appointment.Lat > 90) {
		return errors.New("lat must be between -90 and 90")
	}
	if appointment.Lng != nil && (*appointment.Lng < -180 || *appointment.Lng > 180) {
		return errors.New("lng must be between -180 and 180")
	}
	if appointment.Status == "cancelled" {
		if appointment.CancelReason == nil {
			return errors.New("cancel_reason is required when status is cancelled")
		}
	} else {
		appointment.CancelReason = nil
	}
	// 作业时间戳与支付时间属于状态驱动字段，后端至少兜住明显矛盾的组合，避免伪造进度。
	if appointment.DepartedTime != nil && appointment.TechnicianID == nil {
		return errors.New("departed_time requires technician_id")
	}
	if appointment.CheckinTime != nil && appointment.TechnicianID == nil {
		return errors.New("checkin_time requires technician_id")
	}
	if appointment.CompletedTime != nil && appointment.Status != "completed" {
		return errors.New("completed_time requires completed status")
	}
	if appointment.CheckoutTime != nil && !isOneOf(appointment.Status, "completed", "cancelled") {
		return errors.New("checkout_time requires completed or cancelled status")
	}
	if appointment.CheckinTime != nil && appointment.DepartedTime != nil && appointment.CheckinTime.Before(*appointment.DepartedTime) {
		return errors.New("checkin_time cannot be earlier than departed_time")
	}
	if appointment.CompletedTime != nil && appointment.CheckinTime != nil && appointment.CompletedTime.Before(*appointment.CheckinTime) {
		return errors.New("completed_time cannot be earlier than checkin_time")
	}
	if appointment.CheckoutTime != nil && appointment.CompletedTime != nil && appointment.CheckoutTime.Before(*appointment.CompletedTime) {
		return errors.New("checkout_time cannot be earlier than completed_time")
	}

	matchedZoneID, err := h.matchAppointmentZoneID(appointment.Address)
	if err != nil {
		return err
	}
	// zone_id 每次都由最新地址重新推导，避免地址改走后仍残留旧区域。
	appointment.ZoneID = matchedZoneID

	if appointment.PaidAmount < 0 {
		return errors.New("paid_amount must be greater than or equal to 0")
	}
	// 「無收款」屬於服務模式而非前端可信輸入；一旦命中就先收斂支付欄位，再做其餘支付一致性檢查。
	if appointment.PaymentMethod == "無收款" {
		appointment.PaidAmount = 0
		appointment.PaymentReceived = false
		appointment.PaymentTime = nil
	}
	if appointment.PaidAmount > appointment.TotalAmount {
		return errors.New("paid_amount cannot be greater than total_amount")
	}
	if appointment.PaymentReceived && !isOneOf(appointment.Status, "completed", "cancelled") {
		return errors.New("payment_received requires completed or cancelled status")
	}
	if appointment.PaymentReceived && appointment.PaidAmount == 0 && appointment.TotalAmount > 0 {
		return errors.New("paid_amount is required when payment_received is true")
	}
	if !appointment.PaymentReceived && appointment.PaidAmount > 0 {
		return errors.New("payment_received must be true when paid_amount is greater than 0")
	}
	if appointment.PaymentTime != nil && !appointment.PaymentReceived {
		return errors.New("payment_time requires payment_received to be true")
	}
	if appointment.PaymentReceived && appointment.PaymentTime == nil {
		now := time.Now().UTC()
		appointment.PaymentTime = &now
	}
	if !appointment.PaymentReceived {
		appointment.PaymentTime = nil
	}

	return nil
}

// matchAppointmentZoneID 按地址关键字匹配服务区域，确保创建预约与编辑预约都能由后端补齐 zone_id。
func (h *Handler) matchAppointmentZoneID(address string) (*string, error) {
	trimmedAddress := strings.TrimSpace(address)
	if trimmedAddress == "" {
		return nil, nil
	}

	var zones []models.ServiceZone
	if err := h.db.Select("id", "districts").Order("id asc").Find(&zones).Error; err != nil {
		return nil, err
	}

	for _, zone := range zones {
		var districts []string
		if err := json.Unmarshal(zone.Districts, &districts); err != nil {
			continue
		}
		for _, district := range districts {
			if district != "" && strings.Contains(trimmedAddress, strings.TrimSpace(district)) {
				return stringPtr(zone.ID), nil
			}
		}
	}

	return nil, nil
}

// upsertCustomerFromAppointment 用预约单中的客户资料反向维护客户主档，避免前端创建预约后客户列表不同步。
func upsertCustomerFromAppointment(tx *gorm.DB, appointment models.Appointment) error {
	customerID := strings.TrimSpace(appointment.Phone)
	if customerID == "" && appointment.LineUID != nil {
		customerID = "line_" + *appointment.LineUID
	}
	if customerID == "" {
		return nil
	}

	var existing models.Customer
	err := tx.Where("id = ? OR phone = ?", customerID, appointment.Phone).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	model := models.Customer{
		ID:        customerID,
		Name:      appointment.CustomerName,
		Phone:     appointment.Phone,
		Address:   appointment.Address,
		LineUID:   appointment.LineUID,
		CreatedAt: time.Now().UTC(),
		LineData:  datatypes.JSON([]byte(`{}`)),
	}
	if appointment.LineUID != nil {
		model.LineID = appointment.LineUID
		var lineFriend models.LineFriend
		if err := tx.First(&lineFriend, "line_uid = ?", *appointment.LineUID).Error; err == nil {
			model.LineName = stringPtr(lineFriend.LineName)
			model.LinePicture = stringPtr(lineFriend.LinePicture)
			model.LineJoinedAt = &lineFriend.JoinedAt
			model.LineData = normalizeDatatypesJSON(lineFriend.LastPayload, []byte(`{}`))
		}

		// 预约改绑同一个 LINE UID 到另一位客户前，先释放旧占用者，避免 customers.line_uid 唯一约束直接报错。
		targetCustomerID := model.ID
		if err == nil {
			targetCustomerID = existing.ID
		}
		if err := clearCustomerLineFieldsByLineUID(tx, *appointment.LineUID, stringPtr(targetCustomerID)); err != nil {
			return err
		}
	}

	// 预约单一旦带入 LINE UID，就同步回写好友绑定关系，确保 LINE 页面与客户主档状态一致。
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		if appointment.LineUID != nil {
			if err := tx.Model(&models.LineFriend{}).
				Where("linked_customer_id = ? AND line_uid <> ?", model.ID, *appointment.LineUID).
				Update("linked_customer_id", nil).Error; err != nil {
				return err
			}
			return tx.Model(&models.LineFriend{}).
				Where("line_uid = ?", *appointment.LineUID).
				Update("linked_customer_id", model.ID).Error
		}
		return nil
	}

	// 更新既有客户时使用显式 map，确保 line_uid 被清空时能把关联字段一并置空，而不是被 GORM 跳过。
	customerUpdates := map[string]any{
		"name":       model.Name,
		"phone":      model.Phone,
		"address":    model.Address,
		"updated_at": time.Now().UTC(),
	}
	if appointment.LineUID != nil {
		customerUpdates["line_id"] = model.LineID
		customerUpdates["line_uid"] = model.LineUID
		customerUpdates["line_name"] = model.LineName
		customerUpdates["line_picture"] = model.LinePicture
		customerUpdates["line_joined_at"] = model.LineJoinedAt
		customerUpdates["line_data"] = model.LineData
	} else {
		customerUpdates["line_id"] = nil
		customerUpdates["line_uid"] = nil
		customerUpdates["line_name"] = nil
		customerUpdates["line_picture"] = nil
		customerUpdates["line_joined_at"] = nil
		customerUpdates["line_data"] = datatypes.JSON([]byte(`{}`))
	}
	if err := tx.Model(&existing).Updates(customerUpdates).Error; err != nil {
		return err
	}

	if appointment.LineUID != nil {
		if err := tx.Model(&models.LineFriend{}).
			Where("linked_customer_id = ? AND line_uid <> ?", existing.ID, *appointment.LineUID).
			Update("linked_customer_id", nil).Error; err != nil {
			return err
		}
		return tx.Model(&models.LineFriend{}).
			Where("line_uid = ?", *appointment.LineUID).
			Update("linked_customer_id", existing.ID).Error
	}

	// 预约编辑移除 line_uid 时，也要把旧好友反向解绑，避免 line_friends 与 customers 残留旧关系。
	return tx.Model(&models.LineFriend{}).
		Where("linked_customer_id = ?", existing.ID).
		Update("linked_customer_id", nil).Error
}

// syncCustomersFromLineFriend 把 LINE 好友最新资料同步回客户主档，保证 webhook/好友表/客户表一致。
func syncCustomersFromLineFriend(tx *gorm.DB, friend models.LineFriend, linkedCustomerID *string) error {
	updates := map[string]any{
		"line_id":        friend.LineUID,
		"line_uid":       friend.LineUID,
		"line_name":      friend.LineName,
		"line_picture":   friend.LinePicture,
		"line_joined_at": friend.JoinedAt,
		"line_data":      normalizeDatatypesJSON(friend.LastPayload, []byte(`{}`)),
		"updated_at":     time.Now().UTC(),
	}

	query := tx.Model(&models.Customer{}).Where("line_uid = ?", friend.LineUID)
	if linkedCustomerID != nil && strings.TrimSpace(*linkedCustomerID) != "" {
		query = query.Or("id = ?", strings.TrimSpace(*linkedCustomerID))
	}
	return query.Updates(updates).Error
}

// clearCustomerLineFieldsByID 统一按客户 ID 清空 LINE 字段，供手动解绑、改绑和预约改绑复用。
func clearCustomerLineFieldsByID(tx *gorm.DB, customerID string) error {
	trimmedID := strings.TrimSpace(customerID)
	if trimmedID == "" {
		return nil
	}
	return clearCustomerLineFields(tx, tx.Model(&models.Customer{}).Where("id = ?", trimmedID))
}

// clearCustomerLineFieldsByLineUID 清空占用同一 line_uid 的旧客户资料，并允许排除当前目标客户，避免唯一键冲突与脏绑定残留。
func clearCustomerLineFieldsByLineUID(tx *gorm.DB, lineUID string, exceptCustomerID *string) error {
	trimmedUID := strings.TrimSpace(lineUID)
	if trimmedUID == "" {
		return nil
	}
	query := tx.Model(&models.Customer{}).Where("line_uid = ?", trimmedUID)
	if exceptCustomerID != nil && strings.TrimSpace(*exceptCustomerID) != "" {
		query = query.Where("id <> ?", strings.TrimSpace(*exceptCustomerID))
	}
	return clearCustomerLineFields(tx, query)
}

// clearCustomerLineFields 统一清空客户主档中的 LINE 相关字段，避免各条解绑链路出现遗漏或字段不一致。
func clearCustomerLineFields(tx *gorm.DB, query *gorm.DB) error {
	return query.Updates(map[string]any{
		"line_id":        nil,
		"line_uid":       nil,
		"line_name":      nil,
		"line_picture":   nil,
		"line_joined_at": nil,
		"line_data":      datatypes.JSON([]byte(`{}`)),
		"updated_at":     time.Now().UTC(),
	}).Error
}

// replaceZones 批量覆盖区域数据并删除未出现在本次写入中的旧区域。
func replaceZones(db *gorm.DB, items []models.ServiceZone, ids []string) error {
	if len(ids) == 0 {
		return errors.New("replace payload must not be empty")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		deleteQuery := tx.Model(&models.ServiceZone{})
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&models.ServiceZone{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				UpdateAll: true,
			}).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// replaceServiceItems 批量覆盖服务项目数据并删除未出现在本次写入中的旧项目。
func replaceServiceItems(db *gorm.DB, items []models.ServiceItem, ids []string) error {
	if len(ids) == 0 {
		return errors.New("replace payload must not be empty")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		deleteQuery := tx.Model(&models.ServiceItem{})
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&models.ServiceItem{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				UpdateAll: true,
			}).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// replaceExtraItems 批量覆盖额外收费项数据并删除未出现在本次写入中的旧项目。
func replaceExtraItems(db *gorm.DB, items []models.ExtraItem, ids []string) error {
	if len(ids) == 0 {
		return errors.New("replace payload must not be empty")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		deleteQuery := tx.Model(&models.ExtraItem{})
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&models.ExtraItem{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				UpdateAll: true,
			}).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// replaceCustomers 批量 upsert 客户记录，删除不在 ids 列表中的旧数据，与其它 replace 函数保持一致。
func replaceCustomers(db *gorm.DB, items []models.Customer, ids []string) error {
	if len(ids) == 0 {
		return errors.New("replace payload must not be empty")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var existingCustomers []models.Customer
		if err := tx.Find(&existingCustomers).Error; err != nil {
			return err
		}
		existingByID := make(map[string]models.Customer, len(existingCustomers))
		incomingIDs := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			incomingIDs[id] = struct{}{}
		}
		for _, item := range existingCustomers {
			existingByID[item.ID] = item
		}

		var removedCustomerIDs []string
		for _, item := range existingCustomers {
			if _, ok := incomingIDs[item.ID]; !ok {
				removedCustomerIDs = append(removedCustomerIDs, item.ID)
			}
		}
		if len(removedCustomerIDs) > 0 {
			if err := tx.Model(&models.LineFriend{}).
				Where("linked_customer_id IN ?", removedCustomerIDs).
				Update("linked_customer_id", nil).Error; err != nil {
				return err
			}
		}

		deleteQuery := tx.Model(&models.Customer{})
		if len(ids) > 0 {
			deleteQuery = deleteQuery.Where("id NOT IN ?", ids)
		}
		if err := deleteQuery.Delete(&models.Customer{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if existing, ok := existingByID[item.ID]; ok {
				previousLineUID := normalizeOptionalString(existing.LineUID)
				currentLineUID := normalizeOptionalString(item.LineUID)
				if previousLineUID != nil && (currentLineUID == nil || *previousLineUID != *currentLineUID) {
					if err := tx.Model(&models.LineFriend{}).
						Where("line_uid = ?", *previousLineUID).
						Update("linked_customer_id", nil).Error; err != nil {
						return err
					}
				}
			}
			if item.LineUID != nil {
				if err := clearCustomerLineFieldsByLineUID(tx, *item.LineUID, stringPtr(item.ID)); err != nil {
					return err
				}
				if err := tx.Model(&models.LineFriend{}).
					Where("linked_customer_id = ? AND line_uid <> ?", item.ID, *item.LineUID).
					Update("linked_customer_id", nil).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&models.LineFriend{}).
					Where("linked_customer_id = ?", item.ID).
					Update("linked_customer_id", nil).Error; err != nil {
					return err
				}
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				UpdateAll: true,
			}).Create(&item).Error; err != nil {
				return err
			}
			if item.LineUID != nil {
				if err := tx.Model(&models.LineFriend{}).
					Where("line_uid = ?", *item.LineUID).
					Update("linked_customer_id", item.ID).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// normalizeJSON 把前端 JSON 字段统一标准化为 datatypes.JSON，并在空值时回退默认内容。
func normalizeJSON(raw json.RawMessage, fallback []byte) datatypes.JSON {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return datatypes.JSON(slices.Clone(fallback))
	}
	return datatypes.JSON(raw)
}

// normalizeDatatypesJSON 把 datatypes.JSON 复用 normalizeJSON 的空值与默认值逻辑。
func normalizeDatatypesJSON(raw datatypes.JSON, fallback []byte) datatypes.JSON {
	return normalizeJSON(json.RawMessage(raw), fallback)
}

// parseRequiredTime 解析必填 RFC3339 时间字段，并在缺失或非法时返回可读错误。
func parseRequiredTime(raw string, field string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New(field + " is required")
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, errors.New("invalid " + field)
	}
	return &parsed, nil
}

// parseOptionalTime 解析可选 RFC3339 时间字段，空字符串时返回 nil。
func parseOptionalTime(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// parseOptionalTimePtr 解析可选字符串指针时间字段，供写模型中的可选时间统一复用。
func parseOptionalTimePtr(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	return parseOptionalTime(*raw)
}

// trimStringPtr 把可选字符串统一裁剪空白并把空值收敛为 nil。
func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// normalizeTimePtrUTC 把可选时间统一标准化为 UTC，避免不同端回写后同一字段出现混杂时区。
func normalizeTimePtrUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

// generateReviewToken 生成公开评价页使用的随机令牌，避免在外链中暴露自增预约 ID。
func generateReviewToken() (string, error) {
	randomBytes := make([]byte, reviewTokenByteLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}

// ensureAppointmentReviewToken 保证预约记录始终拥有公开评价令牌，供管理员复制评价链接和公开页按令牌读取。
func (h *Handler) ensureAppointmentReviewToken(appointment *models.Appointment) error {
	if appointment == nil {
		return errors.New("appointment is required")
	}
	if trimStringPtr(appointment.ReviewToken) != nil {
		appointment.ReviewToken = trimStringPtr(appointment.ReviewToken)
		return nil
	}

	reviewToken, err := generateReviewToken()
	if err != nil {
		return err
	}
	if appointment.ID == 0 {
		appointment.ReviewToken = &reviewToken
		return nil
	}
	if err := h.db.Model(&models.Appointment{}).
		Where("id = ? AND (review_public_token IS NULL OR review_public_token = '')", appointment.ID).
		Update("review_public_token", reviewToken).Error; err != nil {
		return err
	}
	appointment.ReviewToken = &reviewToken
	return nil
}

// stringPtr 返回字符串指针，供内部派生逻辑快速构造可选字符串字段。
func stringPtr(value string) *string {
	return &value
}

// validateAppointmentCreatePayload 校验创建预约写模型中的公共字段、金额和 JSON 结构。
func validateAppointmentCreatePayload(payload appointmentCreatePayload) error {
	if err := validateAppointmentCommonFields(payload.CustomerName, payload.Address, payload.Phone, payload.LineUID, payload.PaymentMethod); err != nil {
		return err
	}
	// `未收款` 只作为历史读模型兼容值保留；新建预约必须填写真实付款方式或 `無收款`。
	if normalizePaymentMethod(payload.PaymentMethod) == "未收款" {
		return errors.New("invalid payment_method")
	}
	if strings.TrimSpace(payload.ScheduledAt) == "" {
		return errors.New("scheduled_at is required")
	}
	if payload.DiscountAmount < 0 {
		return errors.New("discount_amount must be greater than or equal to 0")
	}
	if _, _, err := normalizeAppointmentItemsJSON(payload.Items); err != nil {
		return err
	}
	if _, _, err := normalizeAppointmentExtraItemsJSON(payload.ExtraItems); err != nil {
		return err
	}
	return nil
}

// validateAppointmentUpdatePayload 校验更新预约写模型中的必填支付字段、状态与 JSON 结构。
func validateAppointmentUpdatePayload(payload appointmentUpdatePayload) error {
	if payload.PaymentMethod == nil {
		return errors.New("payment_method is required")
	}
	if payload.PaidAmount == nil {
		return errors.New("paid_amount is required")
	}
	if payload.PaymentReceived == nil {
		return errors.New("payment_received is required")
	}
	if err := validateAppointmentCommonFields(payload.CustomerName, payload.Address, payload.Phone, payload.LineUID, *payload.PaymentMethod); err != nil {
		return err
	}
	if !isOneOf(normalizeAppointmentStatus(payload.Status), "pending", "assigned", "arrived", "completed", "cancelled") {
		return errors.New("invalid status")
	}
	if strings.TrimSpace(payload.ScheduledAt) == "" {
		return errors.New("scheduled_at is required")
	}
	if normalizeAppointmentStatus(payload.Status) == "assigned" && payload.TechnicianID == nil {
		return errors.New("technician_id is required when status is assigned")
	}
	if payload.DiscountAmount < 0 {
		return errors.New("discount_amount must be greater than or equal to 0")
	}
	if *payload.PaidAmount < 0 {
		return errors.New("paid_amount must be greater than or equal to 0")
	}
	if _, _, err := normalizeAppointmentItemsJSON(payload.Items); err != nil {
		return err
	}
	if _, _, err := normalizeAppointmentExtraItemsJSON(payload.ExtraItems); err != nil {
		return err
	}
	if _, err := normalizeAppointmentPhotosJSON(payload.Photos); err != nil {
		return err
	}
	return nil
}

// validateAppointmentCommonFields 校验预约创建与编辑共用的基础字段，避免多条写入链路出现规则漂移。
func validateAppointmentCommonFields(customerName string, address string, phone string, lineUID *string, paymentMethod string) error {
	if strings.TrimSpace(customerName) == "" {
		return errors.New("customer_name is required")
	}
	if strings.TrimSpace(address) == "" {
		return errors.New("address is required")
	}
	if strings.TrimSpace(phone) == "" && trimStringPtr(lineUID) == nil {
		return errors.New("phone or line_uid is required")
	}
	// 历史数据曾把 `未收款` 写进 payment_method；这里保留兼容，
	// 避免前端编辑旧工单时因为只读脏值而被后端直接拒绝。
	if !isOneOf(normalizePaymentMethod(paymentMethod), "現金", "轉帳", "無收款", "未收款") {
		return errors.New("invalid payment_method")
	}
	return nil
}

// normalizePaymentMethod 统一兼容常见中英文收款方式写法，确保先归一化再校验。
// 注意：`未收款` 是收款状态，不是付款方式；这里绝不能把它折叠成 `無收款`，
// 否则统计页会把本应计入应收/未收的预约误判成免收费工单。
func normalizePaymentMethod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "現金", "现金", "cash":
		return "現金"
	case "轉帳", "转账", "transfer", "bank_transfer", "bank transfer":
		return "轉帳"
	case "無收款", "无需收款", "no_charge", "no charge":
		return "無收款"
	default:
		return strings.TrimSpace(value)
	}
}

// normalizeAppointmentStatus 兼容常见大小写与旧值写法，统一收敛到后端允许的状态集合。
func normalizeAppointmentStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending":
		return "pending"
	case "assigned":
		return "assigned"
	case "arrived", "in_progress", "in progress":
		return "arrived"
	case "completed", "done":
		return "completed"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(value)
	}
}

// deriveAppointmentCreateStatus 统一由后端根据是否已指派师傅决定创建态状态，避免前端继续传 status。
func deriveAppointmentCreateStatus(technicianID *uint) string {
	if technicianID != nil {
		return "assigned"
	}
	return "pending"
}

// decodeStrictJSONArray 复用严格 JSON 解码规则到数组字段，确保数组元素也拒绝未知字段。
func decodeStrictJSONArray[T any](raw json.RawMessage, target *[]T) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		*target = make([]T, 0)
		return nil
	}
	if err := decodeStrictJSONBytes(raw, target); err != nil {
		return err
	}
	if *target == nil {
		*target = make([]T, 0)
	}
	return nil
}

// normalizeAppointmentItemsJSON 校验并标准化服务项数组，避免未知字段或缺字段对象进入数据库。
func normalizeAppointmentItemsJSON(raw json.RawMessage) (datatypes.JSON, int, error) {
	var items []appointmentItemPayload
	if err := decodeStrictJSONArray(raw, &items); err != nil {
		return nil, 0, errors.New("items must be a valid array")
	}

	total := 0
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return nil, 0, errors.New("item id is required")
		}
		if strings.TrimSpace(item.Type) == "" {
			return nil, 0, errors.New("item type is required")
		}
		if item.Price < 0 {
			return nil, 0, errors.New("item price must be greater than or equal to 0")
		}
		item.Note = strings.TrimSpace(item.Note)
		total += item.Price
	}

	normalized, err := json.Marshal(items)
	if err != nil {
		return nil, 0, err
	}
	return datatypes.JSON(normalized), total, nil
}

// normalizeAppointmentExtraItemsJSON 校验并标准化额外费用项数组，确保写库结构稳定。
func normalizeAppointmentExtraItemsJSON(raw json.RawMessage) (datatypes.JSON, int, error) {
	var items []appointmentExtraItemPayload
	if err := decodeStrictJSONArray(raw, &items); err != nil {
		return nil, 0, errors.New("extra_items must be a valid array")
	}

	total := 0
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return nil, 0, errors.New("extra_item id is required")
		}
		if strings.TrimSpace(item.Name) == "" {
			return nil, 0, errors.New("extra_item name is required")
		}
		if item.Price < 0 {
			return nil, 0, errors.New("item price must be greater than or equal to 0")
		}
		total += item.Price
	}

	normalized, err := json.Marshal(items)
	if err != nil {
		return nil, 0, err
	}
	return datatypes.JSON(normalized), total, nil
}

// normalizeAppointmentPhotosJSON 校验施工照片数组只能由非空字符串组成，避免对象结构回流到前端时报错。
func normalizeAppointmentPhotosJSON(raw json.RawMessage) (datatypes.JSON, error) {
	var photos []string
	if err := decodeStrictJSONArray(raw, &photos); err != nil {
		return nil, errors.New("photos must be a valid array")
	}

	for index, photo := range photos {
		trimmed := strings.TrimSpace(photo)
		if trimmed == "" {
			return nil, errors.New("photo value at index " + strconv.Itoa(index) + " must not be empty")
		}
		photos[index] = trimmed
	}

	normalized, err := json.Marshal(photos)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(normalized), nil
}

// validateCashLedgerPayload 校验现金流水写模型的基础字段完整性和类型合法性。
func validateCashLedgerPayload(payload cashLedgerPayload) error {
	if !isOneOf(strings.TrimSpace(payload.Type), "collect", "return") {
		return errors.New("invalid cash_ledger type")
	}
	if payload.TechnicianID == 0 {
		return errors.New("technician_id is required")
	}
	if payload.Amount <= 0 {
		return errors.New("amount must be greater than 0")
	}
	if strings.TrimSpace(payload.Note) == "" {
		return errors.New("note is required")
	}
	if strings.TrimSpace(payload.Type) == "collect" && payload.AppointmentID == nil {
		return errors.New("appointment_id is required for collect")
	}
	if strings.TrimSpace(payload.Type) == "return" && payload.AppointmentID != nil {
		return errors.New("appointment_id must be empty for return")
	}
	return nil
}

// validateCashLedgerBusinessRules 校验师傅、预约与现金余额关系，避免 return 金额超过当前可回缴余额。
func (h *Handler) validateCashLedgerBusinessRules(payload cashLedgerPayload) error {
	var technician models.User
	if err := h.db.Select("id", "role").First(&technician, "id = ?", payload.TechnicianID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("technician not found")
		}
		return err
	}
	if technician.Role != "technician" {
		return errors.New("technician_id must reference a technician")
	}

	if payload.AppointmentID != nil {
		var appointment models.Appointment
		if err := h.db.First(&appointment, "id = ?", *payload.AppointmentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("appointment not found")
			}
			return err
		}
		if appointment.TechnicianID == nil || *appointment.TechnicianID != payload.TechnicianID {
			return errors.New("appointment does not belong to technician")
		}
		if strings.TrimSpace(payload.Type) == "collect" {
			var existingCollectCount int64
			if err := h.db.Model(&models.CashLedgerEntry{}).
				Where("appointment_id = ? AND type = ?", *payload.AppointmentID, "collect").
				Count(&existingCollectCount).Error; err != nil {
				return err
			}
			if existingCollectCount > 0 {
				return errors.New("collect entry already exists for appointment")
			}
			// 这里按归一化后的付款方式判定，兼容历史数据里遗留的“现金/cash”等旧写法，
			// 同时继续把 `未收款` 占位值挡在现金账外，避免脏资料被误记成现金实收。
			if normalizePaymentMethod(appointment.PaymentMethod) != "現金" {
				return errors.New("collect entry requires a cash appointment")
			}
			if appointment.Status != "completed" && appointment.Status != "cancelled" {
				return errors.New("collect entry requires a finished appointment")
			}
			collectedAmount := getAppointmentCollectedCashAmount(appointment)
			if collectedAmount == 0 {
				return errors.New("collect entry requires a payment_received appointment")
			}
			// 现金账余额是按预约表里的“现金已收金额”整单重算；
			// 若这里允许关联预约的 collect 只登记部分金额，账务流水与余额口径会永久不一致。
			// 因此前端/脚本只要为预约补记 collect，就必须与该预约的现金实收金额完全一致。
			if payload.Amount != collectedAmount {
				return errors.New("amount must equal appointment paid amount")
			}
		}
	}

	if strings.TrimSpace(payload.Type) != "return" {
		return nil
	}

	balance, err := h.currentCashLedgerBalance(payload.TechnicianID)
	if err != nil {
		return err
	}
	if payload.Amount > balance {
		return errors.New("return amount exceeds current cash balance")
	}

	return nil
}

// getAppointmentCollectedCashAmount 统一给现金账复用“现金且已确认收款”的口径，
// 兼容数据库中的旧付款方式别名，同时明确把 `未收款` 与其它非现金方式排除在现金余额之外。
func getAppointmentCollectedCashAmount(appointment models.Appointment) int {
	if normalizePaymentMethod(appointment.PaymentMethod) != "現金" {
		return 0
	}
	if !isOneOf(appointment.Status, "completed", "cancelled") || !appointment.PaymentReceived {
		return 0
	}
	if appointment.PaidAmount == 0 && appointment.TotalAmount > 0 {
		return appointment.TotalAmount
	}
	return appointment.PaidAmount
}

// currentCashLedgerBalance 按预约现金实收与人工账务流水重算师傅余额，服务端拒绝超额回缴。
func (h *Handler) currentCashLedgerBalance(technicianID uint) (int, error) {
	var appointments []models.Appointment
	if err := h.db.
		Where("technician_id = ? AND status IN ?", technicianID, []string{"completed", "cancelled"}).
		Find(&appointments).Error; err != nil {
		return 0, err
	}

	balance := 0
	for _, appointment := range appointments {
		balance += getAppointmentCollectedCashAmount(appointment)
	}

	var entries []models.CashLedgerEntry
	if err := h.db.Where("technician_id = ?", technicianID).Find(&entries).Error; err != nil {
		return 0, err
	}
	for _, entry := range entries {
		switch entry.Type {
		case "collect":
			// 预约现金实收已经由 appointments 表内 payment_received 统计，
			// 这里只累加无预约关联的手工 collect，避免重复计算余额。
			if entry.AppointmentID == nil {
				balance += entry.Amount
			}
		case "return":
			balance -= entry.Amount
		}
	}

	if balance < 0 {
		return 0, nil
	}
	return balance, nil
}

// validateReviewPayload 校验评价写模型的评分范围和 misconducts 结构。
func validateReviewPayload(payload reviewPayload) error {
	if payload.Rating < 1 || payload.Rating > 5 {
		return errors.New("rating must be between 1 and 5")
	}

	trimmed := strings.TrimSpace(string(payload.Misconducts))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	var misconducts []string
	if err := json.Unmarshal(payload.Misconducts, &misconducts); err != nil {
		return errors.New("invalid misconducts")
	}

	for _, item := range misconducts {
		if !isOneOf(strings.TrimSpace(item), "private_contact", "not_clean", "bad_attitude", "late_arrival", "damage_property", "overcharge", "other") {
			return errors.New("invalid misconducts")
		}
	}
	return nil
}

// validateNotificationPayload 校验通知写模型中的预约引用、类型和正文内容。
func validateNotificationPayload(payload notificationPayload) error {
	if payload.AppointmentID == 0 {
		return errors.New("appointment_id is required")
	}
	if !isOneOf(strings.TrimSpace(payload.Type), "line", "sms") {
		return errors.New("invalid notification type")
	}
	if strings.TrimSpace(payload.Message) == "" {
		return errors.New("message is required")
	}
	return nil
}

// isOneOf 判断值是否命中允许集合，供状态、类型和枚举校验复用。
func isOneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
