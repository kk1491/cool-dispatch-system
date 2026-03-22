package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/security"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestHandler 使用内存 SQLite 构建最小 Handler，便于覆盖预约派生字段与校验逻辑。
func newTestHandler(t *testing.T) *Handler {
	return newTestHandlerWithConfig(t, config.Config{})
}

// newTestHandlerWithConfig 允许测试按需注入环境配置，覆盖 webhook 签名等依赖配置的场景。
func newTestHandlerWithConfig(t *testing.T, cfg config.Config) *Handler {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.AutoMigrateModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return NewHandler(db, cfg)
}

// setAuthenticatedUser 在直接调用 handler 的测试里模拟已通过中间件认证的上下文。
// 这样测试仍能聚焦业务校验本身，而不是在进入 handler 前被认证门禁拦截。
func setAuthenticatedUser(ctx *gin.Context, user *models.User) {
	ctx.Set("user", user)
}

// signLineWebhookBody 生成与 LINE 平台一致的 HMAC-SHA256 base64 签名，供 webhook 验签测试复用。
func signLineWebhookBody(secret string, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// TestLoginAcceptsHashedPassword 验证登录接口会按 bcrypt 哈希校验并签发 cookie。
func TestLoginAcceptsHashedPassword(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	passwordHash, err := security.HashPassword("admin-pass-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := handler.db.Create(&models.User{
		ID:           1,
		Name:         "管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: passwordHash,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"phone":"0912345678","password":"admin-pass-123"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Login(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if cookie := recorder.Header().Get("Set-Cookie"); !strings.Contains(cookie, tokenCookieName+"=") {
		t.Fatalf("expected auth cookie, got %q", cookie)
	}
}

// TestLoginRespectsConfiguredCookieSecurityAttributes 验证登录接口会按配置写入 Secure 和 SameSite 属性。
func TestLoginRespectsConfiguredCookieSecurityAttributes(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{
		CookieSecure:   true,
		CookieSameSite: "strict",
	})
	passwordHash, err := security.HashPassword("admin-pass-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := handler.db.Create(&models.User{
		ID:           1,
		Name:         "管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: passwordHash,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"phone":"0912345678","password":"admin-pass-123"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Login(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	cookie := recorder.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "Secure") {
		t.Fatalf("expected secure cookie, got %q", cookie)
	}
	if !strings.Contains(cookie, "SameSite=Strict") {
		t.Fatalf("expected strict same-site cookie, got %q", cookie)
	}
}

// TestLoginRejectsInvalidHashedPassword 验证密码错误时登录接口返回 401。
func TestLoginRejectsInvalidHashedPassword(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	passwordHash, err := security.HashPassword("admin-pass-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := handler.db.Create(&models.User{
		ID:           1,
		Name:         "管理员",
		Role:         "admin",
		Phone:        "0912345678",
		PasswordHash: passwordHash,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"phone":"0912345678","password":"wrong-pass-123"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.Login(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestNormalizeAppointmentItemsJSONRejectsUnknownFields 验证服务项 JSON 开启严格字段校验。
func TestNormalizeAppointmentItemsJSONRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, _, err := normalizeAppointmentItemsJSON([]byte(`[{"id":"svc-1","type":"清洗","price":1000,"unexpected":true}]`))
	if err == nil || err.Error() != "items must be a valid array" {
		t.Fatalf("expected strict items error, got %v", err)
	}
}

// TestNormalizeAppointmentPhotosJSONRejectsEmptyValue 验证照片数组中的空字符串会被拒绝。
func TestNormalizeAppointmentPhotosJSONRejectsEmptyValue(t *testing.T) {
	t.Parallel()

	_, err := normalizeAppointmentPhotosJSON([]byte(`["  "]`))
	if err == nil || err.Error() != "photo value at index 0 must not be empty" {
		t.Fatalf("expected empty photo error, got %v", err)
	}
}

// TestApplyAppointmentDerivedFieldsClearsStaleZoneID 验证地址不再匹配区域时会清理旧 zone_id。
func TestApplyAppointmentDerivedFieldsClearsStaleZoneID(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if err := handler.db.Create(&models.ServiceZone{
		ID:        "zone-north",
		Name:      "北區",
		Districts: []byte(`["北區"]`),
	}).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}

	appointment := models.Appointment{
		Address:       "南區成功路 1 號",
		PaymentMethod: "現金",
		Status:        "pending",
		ScheduledAt:   time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
		ZoneID:        stringPtr("zone-north"),
	}

	if err := handler.applyAppointmentDerivedFields(&appointment); err != nil {
		t.Fatalf("apply derived fields: %v", err)
	}
	if appointment.ZoneID != nil {
		t.Fatalf("expected zone_id to be cleared, got %v", *appointment.ZoneID)
	}
}

// TestHydrateAppointmentTechnicianNameRejectsNonTechnicianUser 验证 technician_id 不能指向管理员账号。
func TestHydrateAppointmentTechnicianNameRejectsNonTechnicianUser(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if err := handler.db.Create(&models.User{
		ID:    99,
		Name:  "管理員",
		Role:  "admin",
		Phone: "0900000000",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	appointment := models.Appointment{TechnicianID: uintPtr(99)}
	err := handler.hydrateAppointmentTechnicianName(&appointment)
	if err == nil || err.Error() != "technician_id must reference a technician user" {
		t.Fatalf("expected role validation error, got %v", err)
	}
}

// TestCreateAppointmentRejectsReadonlyDerivedFields 验证创建预约时拒绝客户端伪造只读派生字段。
func TestCreateAppointmentRejectsReadonlyDerivedFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	body := `{
		"id": 999,
		"created_at": "2026-03-13T09:00:00Z",
		"customer_name": "王小明",
		"address": "北區成功路 1 號",
		"phone": "0911000222",
		"items": [{"id":"svc-1","type":"分離式冷氣","note":"","price":1800}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"scheduled_at": "2026-03-14T10:00:00Z",
		"technician_name": "客戶端偽造",
		"zone_id": "zone-fake"
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/appointments", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "未知欄位：id") {
		t.Fatalf("expected unknown field error, got %s", recorder.Body.String())
	}
}

// TestNewReadAPIEndpoints 覆盖资源读取接口，确保前端按资源域拆请求时有稳定返回。
func TestNewReadAPIEndpoints(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	now := time.Date(2026, 3, 13, 20, 0, 0, 0, time.UTC)
	technicianID := uint(7)
	appointmentID := uint(1001)

	if err := handler.db.Create(&models.User{
		ID:    technicianID,
		Name:  "王師傅",
		Role:  "technician",
		Phone: "0911000007",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	if err := handler.db.Create(&models.User{
		ID:    1,
		Name:  "管理员",
		Role:  "admin",
		Phone: "0900000001",
	}).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := handler.db.Create(&models.Customer{
		ID:        "cust-1",
		Name:      "王小明",
		Phone:     "0911222333",
		Address:   "台北市信義區市府路 1 號",
		CreatedAt: now,
		UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.ServiceZone{
		ID:        "zone-1",
		Name:      "信義區",
		Districts: []byte(`["信義區"]`),
	}).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	if err := handler.db.Create(&models.ServiceItem{
		ID:           "svc-1",
		Name:         "分離式冷氣",
		DefaultPrice: 1800,
	}).Error; err != nil {
		t.Fatalf("seed service item: %v", err)
	}
	if err := handler.db.Create(&models.ExtraItem{
		ID:    "extra-1",
		Name:  "防霉塗層",
		Price: 300,
	}).Error; err != nil {
		t.Fatalf("seed extra item: %v", err)
	}
	if err := handler.db.Create(&models.Appointment{
		ID:              appointmentID,
		CustomerName:    "王小明",
		Address:         "台北市信義區市府路 1 號",
		Phone:           "0911222333",
		Items:           []byte(`[{"id":"svc-1","type":"分離式冷氣","note":"","price":1800}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "現金",
		TotalAmount:     1800,
		PaidAmount:      1800,
		ScheduledAt:     now,
		Status:          "completed",
		TechnicianID:    &technicianID,
		TechnicianName:  stringPtr("王師傅"),
		PaymentReceived: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	if err := handler.db.Create(&models.CashLedgerEntry{
		ID:            "cl-1",
		TechnicianID:  technicianID,
		AppointmentID: &appointmentID,
		Type:          "income",
		Amount:        1800,
		Note:          "完工收現",
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("seed cash ledger: %v", err)
	}
	if err := handler.db.Create(&models.Review{
		ID:             "rev-1",
		AppointmentID:  appointmentID,
		CustomerName:   "王小明",
		TechnicianID:   &technicianID,
		TechnicianName: stringPtr("王師傅"),
		Rating:         5,
		Misconducts:    []byte(`[]`),
		Comment:        "很滿意",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("seed review: %v", err)
	}
	if err := handler.db.Create(&models.NotificationLog{
		ID:            "notif-1",
		AppointmentID: appointmentID,
		Type:          "review-reminder",
		Message:       "請幫我們評價",
		SentAt:        now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("seed notification log: %v", err)
	}
	description := "客戶完工後幾天提醒回訪"
	if err := handler.db.Create(&models.AppSetting{
		Key:         "reminder_days",
		Value:       "30",
		Description: &description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("seed app setting: %v", err)
	}

	router := NewRouter(config.Config{}, handler.db)

	assertList := func(path string, target any) {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		attachAuthCookie(t, handler.db, req, 1)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s expected 200, got %d body=%s", path, recorder.Code, recorder.Body.String())
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
			t.Fatalf("%s decode response: %v", path, err)
		}
	}

	var appointments []models.Appointment
	assertList("/api/appointments", &appointments)
	if len(appointments) != 1 || appointments[0].ID != appointmentID {
		t.Fatalf("unexpected appointments response: %+v", appointments)
	}

	var technicians []models.User
	assertList("/api/technicians", &technicians)
	if len(technicians) != 1 || technicians[0].ID != technicianID {
		t.Fatalf("unexpected technicians response: %+v", technicians)
	}

	var customers []models.Customer
	assertList("/api/customers", &customers)
	if len(customers) != 1 || customers[0].ID != "cust-1" {
		t.Fatalf("unexpected customers response: %+v", customers)
	}

	var zones []models.ServiceZone
	assertList("/api/zones", &zones)
	if len(zones) != 1 || zones[0].ID != "zone-1" {
		t.Fatalf("unexpected zones response: %+v", zones)
	}

	var serviceItems []models.ServiceItem
	assertList("/api/service-items", &serviceItems)
	if len(serviceItems) != 1 || serviceItems[0].ID != "svc-1" {
		t.Fatalf("unexpected service items response: %+v", serviceItems)
	}

	var extraItems []models.ExtraItem
	assertList("/api/extra-items", &extraItems)
	if len(extraItems) != 1 || extraItems[0].ID != "extra-1" {
		t.Fatalf("unexpected extra items response: %+v", extraItems)
	}

	var ledgerEntries []models.CashLedgerEntry
	assertList("/api/cash-ledger", &ledgerEntries)
	if len(ledgerEntries) != 1 || ledgerEntries[0].ID != "cl-1" {
		t.Fatalf("unexpected cash ledger response: %+v", ledgerEntries)
	}

	var reviews []models.Review
	assertList("/api/reviews", &reviews)
	if len(reviews) != 1 || reviews[0].ID != "rev-1" {
		t.Fatalf("unexpected reviews response: %+v", reviews)
	}

	var notificationLogs []models.NotificationLog
	assertList("/api/notifications", &notificationLogs)
	if len(notificationLogs) != 1 || notificationLogs[0].ID != "notif-1" {
		t.Fatalf("unexpected notifications response: %+v", notificationLogs)
	}

	var settings settingsResponse
	assertList("/api/settings", &settings)
	if settings.ReminderDays != 30 {
		t.Fatalf("unexpected settings response: %+v", settings)
	}
}

// TestReceiveLineWebhookSyncsExistingCustomerByLineUID 覆盖客户主档已持有相同 line_uid 时，webhook 仍会直接刷新客户资料。
func TestReceiveLineWebhookSyncsExistingCustomerByLineUID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})

	customerID := "cust-line-1"
	if err := handler.db.Create(&models.Customer{
		ID:        customerID,
		Name:      "旧客户资料",
		Phone:     "0911222333",
		Address:   "台北市中山區南京東路 1 號",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          "Uwebhook123",
		LineName:         "旧昵称",
		LinePicture:      "https://example.com/old.png",
		JoinedAt:         time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		LinkedCustomerID: &customerID,
		Status:           "followed",
		LastPayload:      []byte(`{"old":true}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	body := `{
		"events": [{
			"type": "follow",
			"timestamp": 1773367200000,
			"source": {"userId": "Uwebhook123"},
			"profile": {
				"displayName": "新昵称",
				"pictureUrl": "https://example.com/new.png",
				"phone": "0911222333"
			}
		}]
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var customer models.Customer
	if err := handler.db.First(&customer, "id = ?", customerID).Error; err != nil {
		t.Fatalf("load customer: %v", err)
	}
	if customer.LineUID == nil || *customer.LineUID != "Uwebhook123" {
		t.Fatalf("expected customer line_uid synced, got %+v", customer.LineUID)
	}
	if customer.LineName == nil || *customer.LineName != "新昵称" {
		t.Fatalf("expected customer line_name synced, got %+v", customer.LineName)
	}
	if customer.LinePicture == nil || *customer.LinePicture != "https://example.com/new.png" {
		t.Fatalf("expected customer line_picture synced, got %+v", customer.LinePicture)
	}
	if string(customer.LineData) == "{}" {
		t.Fatalf("expected customer line_data updated, got %s", string(customer.LineData))
	}

	var friend models.LineFriend
	if err := handler.db.First(&friend, "line_uid = ?", "Uwebhook123").Error; err != nil {
		t.Fatalf("load line friend: %v", err)
	}
	if friend.Status != "followed" {
		t.Fatalf("expected line friend status followed, got %s", friend.Status)
	}
	if friend.Phone == nil || *friend.Phone != "0911222333" {
		t.Fatalf("expected line friend phone synced, got %+v", friend.Phone)
	}

	var eventCount int64
	if err := handler.db.Model(&models.LineEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatalf("count line events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected 1 line event, got %d", eventCount)
	}
}

// TestReceiveLineWebhookAcceptsValidSignature 验证合法签名的 webhook 可以正常入库。
func TestReceiveLineWebhookAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})

	body := `{
		"events": [{
			"type": "follow",
			"timestamp": 1773367200000,
			"source": {"userId": "Uvalidsig123"},
			"profile": {
				"displayName": "签名通过",
				"pictureUrl": "https://example.com/signed.png"
			}
		}]
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var eventCount int64
	if err := handler.db.Model(&models.LineEvent{}).Where("line_uid = ?", "Uvalidsig123").Count(&eventCount).Error; err != nil {
		t.Fatalf("count line events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected 1 line event, got %d", eventCount)
	}
}

// TestReceiveLineWebhookIgnoresEmptyEvents 验证空事件数组会被静默接受且不写入数据。
func TestReceiveLineWebhookIgnoresEmptyEvents(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})

	body := `{"events":[]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var eventCount int64
	if err := handler.db.Model(&models.LineEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatalf("count line events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected 0 line events, got %d", eventCount)
	}
}

// TestReceiveLineWebhookRecordsEventWithoutUserID 验证缺少 userId 的事件只记录原始事件，不创建好友。
func TestReceiveLineWebhookRecordsEventWithoutUserID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})

	body := `{"events":[{"type":"follow","timestamp":1773367200000,"source":{},"profile":{"displayName":"匿名好友"}}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var event models.LineEvent
	if err := handler.db.First(&event).Error; err != nil {
		t.Fatalf("load line event: %v", err)
	}
	if event.LineUID != nil {
		t.Fatalf("expected nil line_uid, got %#v", event.LineUID)
	}

	var friendCount int64
	if err := handler.db.Model(&models.LineFriend{}).Count(&friendCount).Error; err != nil {
		t.Fatalf("count line friends: %v", err)
	}
	if friendCount != 0 {
		t.Fatalf("expected 0 line friends, got %d", friendCount)
	}
}

// TestReceiveLineWebhookRejectsMissingSignature 验证缺少签名头时 webhook 会被拒绝。
func TestReceiveLineWebhookRejectsMissingSignature(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: "line-secret-for-test"})

	body := `{"events":[{"type":"follow","source":{"userId":"Umisssig123"}}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "缺少 LINE Webhook 簽章") {
		t.Fatalf("expected missing signature error, got %s", recorder.Body.String())
	}
}

// TestReceiveLineWebhookRejectsInvalidSignature 验证错误签名不会通过 webhook 校验。
func TestReceiveLineWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: "line-secret-for-test"})

	body := `{"events":[{"type":"follow","source":{"userId":"Uinvalidsig123"}}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", "invalid-signature")

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "LINE Webhook 簽章無效") {
		t.Fatalf("expected invalid signature error, got %s", recorder.Body.String())
	}
}

// TestReceiveLineWebhookRejectsWhenSecretNotConfigured 验证未配置 channel secret 时 webhook 直接返回服务端错误。
func TestReceiveLineWebhookRejectsWhenSecretNotConfigured(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{})

	body := `{"events":[{"type":"follow","source":{"userId":"Uno-secret-123"}}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", "any-signature")

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "尚未設定 LINE Webhook 密鑰") {
		t.Fatalf("expected secret missing error, got %s", recorder.Body.String())
	}

	var eventCount int64
	if err := handler.db.Model(&models.LineEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatalf("count line events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected 0 line events when secret missing, got %d", eventCount)
	}

	var friendCount int64
	if err := handler.db.Model(&models.LineFriend{}).Count(&friendCount).Error; err != nil {
		t.Fatalf("count line friends: %v", err)
	}
	if friendCount != 0 {
		t.Fatalf("expected 0 line friends when secret missing, got %d", friendCount)
	}
}

// TestLinkLineFriendCustomerSyncsAndClearsCustomerFields 验证绑定与解绑都会同步客户和好友两侧资料。
func TestLinkLineFriendCustomerSyncsAndClearsCustomerFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	customerID := "cust-bind-1"
	if err := handler.db.Create(&models.Customer{
		ID:        customerID,
		Name:      "绑定客户",
		Phone:     "0922000111",
		Address:   "新北市板橋區文化路 1 號",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:     "Ubind123",
		LineName:    "待绑定好友",
		LinePicture: "https://example.com/bind.png",
		JoinedAt:    time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC),
		Status:      "followed",
		LastPayload: []byte(`{"source":"bind"}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	linkBody := `{"customer_id":"cust-bind-1"}`
	linkRecorder := httptest.NewRecorder()
	linkCtx, _ := gin.CreateTestContext(linkRecorder)
	linkCtx.Params = gin.Params{{Key: "lineUid", Value: "Ubind123"}}
	linkCtx.Request = httptest.NewRequest(http.MethodPut, "/api/line-friends/Ubind123/customer", strings.NewReader(linkBody))
	linkCtx.Request.Header.Set("Content-Type", "application/json")

	handler.LinkLineFriendCustomer(linkCtx)

	if linkRecorder.Code != http.StatusOK {
		t.Fatalf("expected link 200, got %d body=%s", linkRecorder.Code, linkRecorder.Body.String())
	}

	var linkedCustomer models.Customer
	if err := handler.db.First(&linkedCustomer, "id = ?", customerID).Error; err != nil {
		t.Fatalf("load linked customer: %v", err)
	}
	if linkedCustomer.LineUID == nil || *linkedCustomer.LineUID != "Ubind123" {
		t.Fatalf("expected customer line_uid after link, got %+v", linkedCustomer.LineUID)
	}
	if linkedCustomer.LineName == nil || *linkedCustomer.LineName != "待绑定好友" {
		t.Fatalf("expected customer line_name after link, got %+v", linkedCustomer.LineName)
	}

	unlinkRecorder := httptest.NewRecorder()
	unlinkCtx, _ := gin.CreateTestContext(unlinkRecorder)
	unlinkCtx.Params = gin.Params{{Key: "lineUid", Value: "Ubind123"}}
	unlinkCtx.Request = httptest.NewRequest(http.MethodPut, "/api/line-friends/Ubind123/customer", strings.NewReader(`{"customer_id":null}`))
	unlinkCtx.Request.Header.Set("Content-Type", "application/json")

	handler.LinkLineFriendCustomer(unlinkCtx)

	if unlinkRecorder.Code != http.StatusOK {
		t.Fatalf("expected unlink 200, got %d body=%s", unlinkRecorder.Code, unlinkRecorder.Body.String())
	}

	var clearedCustomer models.Customer
	if err := handler.db.First(&clearedCustomer, "id = ?", customerID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if clearedCustomer.LineUID != nil || clearedCustomer.LineName != nil || clearedCustomer.LinePicture != nil {
		t.Fatalf("expected customer line fields cleared after unlink, got %+v", clearedCustomer)
	}

	var clearedFriend models.LineFriend
	if err := handler.db.First(&clearedFriend, "line_uid = ?", "Ubind123").Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if clearedFriend.LinkedCustomerID != nil {
		t.Fatalf("expected linked_customer_id cleared, got %+v", clearedFriend.LinkedCustomerID)
	}
}

// TestLinkLineFriendCustomerRelinkClearsPreviousCustomerFields 验证好友改绑时旧客户的 LINE 资料会被清理。
func TestLinkLineFriendCustomerRelinkClearsPreviousCustomerFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	oldCustomerID := "cust-old-line"
	newCustomerID := "cust-new-line"
	joinedAt := time.Date(2026, 3, 4, 11, 0, 0, 0, time.UTC)
	if err := handler.db.Create(&models.Customer{
		ID:           oldCustomerID,
		Name:         "旧绑定客户",
		Phone:        "0933111000",
		Address:      "台北市大安區仁愛路 1 號",
		LineID:       stringPtr("Urelink123"),
		LineUID:      stringPtr("Urelink123"),
		LineName:     stringPtr("旧好友"),
		LinePicture:  stringPtr("https://example.com/old-customer.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"stale":true}`),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed old customer: %v", err)
	}
	if err := handler.db.Create(&models.Customer{
		ID:        newCustomerID,
		Name:      "新绑定客户",
		Phone:     "0933222000",
		Address:   "台北市信義區松智路 2 號",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed new customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          "Urelink123",
		LineName:         "待改绑好友",
		LinePicture:      "https://example.com/relink.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: &oldCustomerID,
		Status:           "followed",
		LastPayload:      []byte(`{"source":"relink"}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "lineUid", Value: "Urelink123"}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/line-friends/Urelink123/customer", strings.NewReader(`{"customer_id":"cust-new-line"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.LinkLineFriendCustomer(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var oldCustomer models.Customer
	if err := handler.db.First(&oldCustomer, "id = ?", oldCustomerID).Error; err != nil {
		t.Fatalf("reload old customer: %v", err)
	}
	if oldCustomer.LineUID != nil || oldCustomer.LineName != nil || oldCustomer.LinePicture != nil {
		t.Fatalf("expected old customer line fields cleared after relink, got %+v", oldCustomer)
	}
	if strings.TrimSpace(string(oldCustomer.LineData)) != "{}" {
		t.Fatalf("expected old customer line_data reset after relink, got %s", string(oldCustomer.LineData))
	}

	var newCustomer models.Customer
	if err := handler.db.First(&newCustomer, "id = ?", newCustomerID).Error; err != nil {
		t.Fatalf("reload new customer: %v", err)
	}
	if newCustomer.LineUID == nil || *newCustomer.LineUID != "Urelink123" {
		t.Fatalf("expected new customer line_uid after relink, got %+v", newCustomer.LineUID)
	}
}

// TestUpsertCustomerFromAppointmentSyncsAndClearsLineBinding 验证预约同步客户时会建立或清理 LINE 绑定关系。
func TestUpsertCustomerFromAppointmentSyncsAndClearsLineBinding(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if err := handler.db.Create(&models.LineFriend{
		LineUID:     "Uappt123",
		LineName:    "预约好友",
		LinePicture: "https://example.com/appt.png",
		JoinedAt:    time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC),
		Status:      "followed",
		LastPayload: []byte(`{"source":"appointment"}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	appointment := models.Appointment{
		CustomerName: "预约客户",
		Phone:        "0933000222",
		Address:      "桃园市中坜区中正路 88 号",
		LineUID:      stringPtr("Uappt123"),
	}
	if err := handler.db.Transaction(func(tx *gorm.DB) error {
		return upsertCustomerFromAppointment(tx, appointment)
	}); err != nil {
		t.Fatalf("upsert customer from appointment: %v", err)
	}

	var customer models.Customer
	if err := handler.db.First(&customer, "id = ?", "0933000222").Error; err != nil {
		t.Fatalf("load customer: %v", err)
	}
	if customer.LineUID == nil || *customer.LineUID != "Uappt123" {
		t.Fatalf("expected customer line_uid from appointment, got %+v", customer.LineUID)
	}
	if customer.LineName == nil || *customer.LineName != "预约好友" {
		t.Fatalf("expected customer line_name from friend, got %+v", customer.LineName)
	}

	var friend models.LineFriend
	if err := handler.db.First(&friend, "line_uid = ?", "Uappt123").Error; err != nil {
		t.Fatalf("load line friend: %v", err)
	}
	if friend.LinkedCustomerID == nil || *friend.LinkedCustomerID != "0933000222" {
		t.Fatalf("expected line friend linked to customer, got %+v", friend.LinkedCustomerID)
	}

	appointment.LineUID = nil
	appointment.Address = "桃园市中坜区中央西路 99 号"
	if err := handler.db.Transaction(func(tx *gorm.DB) error {
		return upsertCustomerFromAppointment(tx, appointment)
	}); err != nil {
		t.Fatalf("clear customer line binding: %v", err)
	}

	var clearedCustomer models.Customer
	if err := handler.db.First(&clearedCustomer, "id = ?", "0933000222").Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if clearedCustomer.LineUID != nil || clearedCustomer.LineName != nil {
		t.Fatalf("expected customer line fields cleared, got %+v", clearedCustomer)
	}

	var clearedFriend models.LineFriend
	if err := handler.db.First(&clearedFriend, "line_uid = ?", "Uappt123").Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if clearedFriend.LinkedCustomerID != nil {
		t.Fatalf("expected line friend unlink after appointment cleared line_uid, got %+v", clearedFriend.LinkedCustomerID)
	}
}

// TestUpdateAppointmentRejectsReadonlyDerivedFields 验证更新预约时拒绝客户端伪造只读派生字段。
func TestUpdateAppointmentRejectsReadonlyDerivedFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	appointment := models.Appointment{
		CustomerName:   "陳小姐",
		Address:        "北區忠孝路 8 號",
		Phone:          "0922000333",
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "pending",
		Photos:         []byte(`[]`),
		CreatedAt:      time.Date(2026, 3, 13, 7, 0, 0, 0, time.UTC),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "陳小姐",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "pending",
		"photos": [],
		"payment_received": false,
		"created_at": "2026-03-01T00:00:00Z",
		"technician_name": "客戶端偽造",
		"zone_id": "zone-fake"
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "未知欄位：created_at") {
		t.Fatalf("expected unknown field error, got %s", recorder.Body.String())
	}
}

// TestUpdateAppointmentHydratesDerivedFieldsFromServer 验证预约更新成功后由服务端补全区域与技师名称等派生字段。
func TestUpdateAppointmentHydratesDerivedFieldsFromServer(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	if err := handler.db.Create(&models.ServiceZone{
		ID:        "zone-north",
		Name:      "北區",
		Districts: []byte(`["北區"]`),
	}).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	if err := handler.db.Create(&models.User{
		ID:    7,
		Name:  "林師傅",
		Role:  "technician",
		Phone: "0933000444",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	createdAt := time.Date(2026, 3, 13, 7, 0, 0, 0, time.UTC)
	appointment := models.Appointment{
		CustomerName:   "陳小姐",
		Address:        "舊地址",
		Phone:          "0922000333",
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "pending",
		Photos:         []byte(`[]`),
		CreatedAt:      createdAt,
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "陳小姐",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "assigned",
		"technician_id": 7,
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.ZoneID == nil || *saved.ZoneID != "zone-north" {
		t.Fatalf("expected derived zone_id, got %#v", saved.ZoneID)
	}
	if saved.TechnicianName == nil || *saved.TechnicianName != "林師傅" {
		t.Fatalf("expected derived technician_name, got %#v", saved.TechnicianName)
	}
	if !saved.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at %v to be preserved by server, got %v", createdAt, saved.CreatedAt)
	}
}

// TestUpdateAppointmentRejectsReadonlyPaymentTimeField 验证客户端不能直接覆盖 payment_time。
func TestUpdateAppointmentRejectsReadonlyPaymentTimeField(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	appointment := models.Appointment{
		CustomerName:  "陳小姐",
		Address:       "北區忠孝路 8 號",
		Phone:         "0922000333",
		Items:         []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:    []byte(`[]`),
		PaymentMethod: "轉帳",
		TotalAmount:   1500,
		ScheduledAt:   time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:        "completed",
		Photos:        []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "陳小姐",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "轉帳",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"photos": [],
		"payment_received": false,
		"payment_time": "2026-03-13T09:00:00Z"
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "未知欄位：payment_time") {
		t.Fatalf("expected unknown payment_time field error, got %s", recorder.Body.String())
	}
}

// TestUpdateAppointmentNormalizesLegacyOutstandingPaymentFields 验证 legacy 未收款脏数据会在更新时被自动归一。
func TestUpdateAppointmentNormalizesLegacyOutstandingPaymentFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	techID := uint(20)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "王師傅",
		Role:  "technician",
		Phone: "0933444555",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "舊客戶",
		Address:         "北區忠孝路 8 號",
		Phone:           "0922000333",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "未收款",
		TotalAmount:     1500,
		PaidAmount:      800,
		PaymentReceived: false,
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "舊客戶",
		"address": "北區忠孝路 99 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "未收款",
		"discount_amount": 0,
		"paid_amount": 800,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"technician_id": 20,
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.PaidAmount != 0 {
		t.Fatalf("expected legacy paid_amount reset to 0, got %d", saved.PaidAmount)
	}
	if saved.PaymentReceived {
		t.Fatalf("expected payment_received to stay false")
	}
}

// TestUpdateAppointmentLegacyOutstandingPaymentCanStillConfirmReceipt 验证 legacy 未收款记录仍可正常确认收款。
func TestUpdateAppointmentLegacyOutstandingPaymentCanStillConfirmReceipt(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	techID := uint(21)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "陳師傅",
		Role:  "technician",
		Phone: "0933555666",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "舊客戶",
		Address:         "北區忠孝路 8 號",
		Phone:           "0922000333",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "未收款",
		TotalAmount:     1500,
		PaidAmount:      800,
		PaymentReceived: false,
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "舊客戶",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 1500,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"technician_id": 21,
		"photos": [],
		"payment_received": true
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.PaymentMethod != "現金" {
		t.Fatalf("expected payment_method switched to 現金, got %s", saved.PaymentMethod)
	}
	if !saved.PaymentReceived {
		t.Fatalf("expected payment_received=true after confirmation")
	}
	if saved.PaidAmount != 1500 {
		t.Fatalf("expected paid_amount 1500, got %d", saved.PaidAmount)
	}
	if saved.PaymentTime == nil {
		t.Fatalf("expected payment_time to be auto populated")
	}
}

// TestUpdateAppointmentLegacyOutstandingPaymentPreservesOmittedFields 验证省略 payment 字段时会继承旧值并清理脏 paid_amount。
func TestUpdateAppointmentLegacyOutstandingPaymentPreservesOmittedFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	techID := uint(211)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "郭師傅",
		Role:  "technician",
		Phone: "0933999000",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "省略支付欄位客戶",
		Address:         "北區忠孝路 28 號",
		Phone:           "0922555666",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "未收款",
		TotalAmount:     1500,
		PaidAmount:      900,
		PaymentReceived: false,
		ScheduledAt:     time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	// 普通编辑旧资料时允许完全省略 payment_* 字段，
	// 后端应沿用既有值并触发 legacy 脏数据清理，而不是要求前端先补齐真实付款方式。
	body := `{
		"customer_name": "省略支付欄位客戶",
		"address": "北區忠孝路 288 號",
		"phone": "0922555666",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"discount_amount": 0,
		"scheduled_at": "2026-03-13T10:00:00Z",
		"status": "completed",
		"technician_id": 211,
		"photos": []
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.PaymentMethod != "未收款" {
		t.Fatalf("expected omitted payment_method to inherit legacy placeholder, got %s", saved.PaymentMethod)
	}
	if saved.PaymentReceived {
		t.Fatalf("expected payment_received to remain false")
	}
	if saved.PaidAmount != 0 {
		t.Fatalf("expected legacy dirty paid_amount reset to 0, got %d", saved.PaidAmount)
	}
}

// TestReceiveLineWebhookSyncsExistingCustomerProfileData 验证 webhook 会刷新已绑定客户的昵称、头像与 payload。
func TestReceiveLineWebhookSyncsExistingCustomerProfileData(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})
	joinedAt := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	customer := models.Customer{
		ID:           "0911222333",
		Name:         "王小姐",
		Phone:        "0911222333",
		Address:      "台北市信義區松仁路 1 號",
		LineID:       stringPtr("Uline-100"),
		LineUID:      stringPtr("Uline-100"),
		LineName:     stringPtr("舊名稱"),
		LinePicture:  stringPtr("https://example.com/old.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"old":true}`),
	}
	if err := handler.db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          "Uline-100",
		LineName:         "舊名稱",
		LinePicture:      "https://example.com/old.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: stringPtr(customer.ID),
		Status:           "followed",
		LastPayload:      []byte(`{"old":true}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	body := `{
		"events": [
			{
				"type": "message",
				"timestamp": 1773360000000,
				"source": {"userId": "Uline-100"},
				"profile": {
					"displayName": "新名稱",
					"pictureUrl": "https://example.com/new.png",
					"phone": "0911222333"
				}
			}
		]
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var savedCustomer models.Customer
	if err := handler.db.First(&savedCustomer, "id = ?", customer.ID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if savedCustomer.LineName == nil || *savedCustomer.LineName != "新名稱" {
		t.Fatalf("expected customer line_name synced, got %#v", savedCustomer.LineName)
	}
	if savedCustomer.LinePicture == nil || *savedCustomer.LinePicture != "https://example.com/new.png" {
		t.Fatalf("expected customer line_picture synced, got %#v", savedCustomer.LinePicture)
	}
	if savedCustomer.LineJoinedAt == nil || !savedCustomer.LineJoinedAt.Equal(joinedAt) {
		t.Fatalf("expected customer line_joined_at kept original follow time, got %#v", savedCustomer.LineJoinedAt)
	}
	var payload map[string]any
	if err := json.Unmarshal(savedCustomer.LineData, &payload); err != nil {
		t.Fatalf("decode customer line_data: %v", err)
	}
	if payload["type"] != "message" {
		t.Fatalf("expected customer line_data updated from webhook, got %#v", payload)
	}
}

// TestReceiveLineWebhookUnfollowMarksFriendWithoutOverwritingJoinedAt 验证 unfollow 事件只更新状态，不污染首次关注时间。
func TestReceiveLineWebhookUnfollowMarksFriendWithoutOverwritingJoinedAt(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	const lineSecret = "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})
	joinedAt := time.Date(2026, 3, 5, 8, 0, 0, 0, time.UTC)
	customerID := "cust-unfollow-1"
	if err := handler.db.Create(&models.Customer{
		ID:           customerID,
		Name:         "已绑定客户",
		Phone:        "0911555666",
		Address:      "台北市松山區南京東路 3 號",
		LineID:       stringPtr("Uunfollow123"),
		LineUID:      stringPtr("Uunfollow123"),
		LineName:     stringPtr("旧好友名称"),
		LinePicture:  stringPtr("https://example.com/old-unfollow.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"old":true}`),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          "Uunfollow123",
		LineName:         "旧好友名称",
		LinePicture:      "https://example.com/old-unfollow.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: &customerID,
		Status:           "followed",
		LastPayload:      []byte(`{"old":true}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	body := `{"events":[{"type":"unfollow","timestamp":1773600000000,"source":{"userId":"Uunfollow123"}}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var friend models.LineFriend
	if err := handler.db.First(&friend, "line_uid = ?", "Uunfollow123").Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if friend.Status != "unfollowed" {
		t.Fatalf("expected line friend status unfollowed, got %s", friend.Status)
	}
	if !friend.JoinedAt.Equal(joinedAt) {
		t.Fatalf("expected line friend joined_at unchanged, got %s", friend.JoinedAt.UTC().Format(time.RFC3339))
	}

	var customer models.Customer
	if err := handler.db.First(&customer, "id = ?", customerID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if customer.LineJoinedAt == nil || !customer.LineJoinedAt.Equal(joinedAt) {
		t.Fatalf("expected customer line_joined_at unchanged, got %#v", customer.LineJoinedAt)
	}
}

// TestUpdateAppointmentClearsLineBindingWhenLineUIDRemoved 验证预约清空 line_uid 时客户与好友绑定一并解除。
func TestUpdateAppointmentClearsLineBindingWhenLineUIDRemoved(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	joinedAt := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	lineUID := "Uline-200"
	customer := models.Customer{
		ID:           "0922333444",
		Name:         "陳小姐",
		Phone:        "0922333444",
		Address:      "台北市中山區復興北路 2 號",
		LineID:       &lineUID,
		LineUID:      &lineUID,
		LineName:     stringPtr("已綁定好友"),
		LinePicture:  stringPtr("https://example.com/bound.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"before":"bound"}`),
	}
	if err := handler.db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          lineUID,
		LineName:         "已綁定好友",
		LinePicture:      "https://example.com/bound.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: stringPtr(customer.ID),
		Status:           "followed",
		LastPayload:      []byte(`{"before":"bound"}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:   "陳小姐",
		Address:        "台北市中山區復興北路 2 號",
		Phone:          customer.Phone,
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "pending",
		Photos:         []byte(`[]`),
		LineUID:        &lineUID,
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "陳小姐",
		"address": "台北市中山區復興北路 2 號",
		"phone": "0922333444",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "pending",
		"photos": [],
		"payment_received": false,
		"line_uid": ""
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var savedCustomer models.Customer
	if err := handler.db.First(&savedCustomer, "id = ?", customer.ID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if savedCustomer.LineUID != nil || savedCustomer.LineID != nil {
		t.Fatalf("expected customer line ids cleared, got line_uid=%#v line_id=%#v", savedCustomer.LineUID, savedCustomer.LineID)
	}
	if strings.TrimSpace(string(savedCustomer.LineData)) != "{}" {
		t.Fatalf("expected customer line_data reset, got %s", string(savedCustomer.LineData))
	}

	var savedFriend models.LineFriend
	if err := handler.db.First(&savedFriend, "line_uid = ?", lineUID).Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if savedFriend.LinkedCustomerID != nil {
		t.Fatalf("expected line friend unlinked, got %#v", savedFriend.LinkedCustomerID)
	}
}

// TestUpdateAppointmentRebindsLineFriendWhenLineUIDChanges 验证预约改绑新 line_uid 时旧好友解绑、新好友绑定。
func TestUpdateAppointmentRebindsLineFriendWhenLineUIDChanges(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	oldJoinedAt := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)
	newJoinedAt := time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC)
	oldLineUID := "Uline-old-1"
	newLineUID := "Uline-new-1"
	customer := models.Customer{
		ID:           "0922555666",
		Name:         "换绑客户",
		Phone:        "0922555666",
		Address:      "台北市內湖區瑞光路 10 號",
		LineID:       &oldLineUID,
		LineUID:      &oldLineUID,
		LineName:     stringPtr("旧好友"),
		LinePicture:  stringPtr("https://example.com/old-line.png"),
		LineJoinedAt: &oldJoinedAt,
		LineData:     []byte(`{"old":"friend"}`),
	}
	if err := handler.db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          oldLineUID,
		LineName:         "旧好友",
		LinePicture:      "https://example.com/old-line.png",
		JoinedAt:         oldJoinedAt,
		LinkedCustomerID: stringPtr(customer.ID),
		Status:           "followed",
		LastPayload:      []byte(`{"old":"friend"}`),
	}).Error; err != nil {
		t.Fatalf("seed old line friend: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:     newLineUID,
		LineName:    "新好友",
		LinePicture: "https://example.com/new-line.png",
		JoinedAt:    newJoinedAt,
		Status:      "followed",
		LastPayload: []byte(`{"new":"friend"}`),
	}).Error; err != nil {
		t.Fatalf("seed new line friend: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:   customer.Name,
		Address:        customer.Address,
		Phone:          customer.Phone,
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "pending",
		Photos:         []byte(`[]`),
		LineUID:        &oldLineUID,
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "换绑客户",
		"address": "台北市內湖區瑞光路 10 號",
		"phone": "0922555666",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "pending",
		"photos": [],
		"payment_received": false,
		"line_uid": "Uline-new-1"
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var savedCustomer models.Customer
	if err := handler.db.First(&savedCustomer, "id = ?", customer.ID).Error; err != nil {
		t.Fatalf("reload customer: %v", err)
	}
	if savedCustomer.LineUID == nil || *savedCustomer.LineUID != newLineUID {
		t.Fatalf("expected customer rebound to new line_uid, got %#v", savedCustomer.LineUID)
	}
	if savedCustomer.LineName == nil || *savedCustomer.LineName != "新好友" {
		t.Fatalf("expected customer line_name from new friend, got %#v", savedCustomer.LineName)
	}
	if savedCustomer.LineJoinedAt == nil || !savedCustomer.LineJoinedAt.Equal(newJoinedAt) {
		t.Fatalf("expected customer line_joined_at from new friend, got %#v", savedCustomer.LineJoinedAt)
	}

	var oldFriend models.LineFriend
	if err := handler.db.First(&oldFriend, "line_uid = ?", oldLineUID).Error; err != nil {
		t.Fatalf("reload old line friend: %v", err)
	}
	if oldFriend.LinkedCustomerID != nil {
		t.Fatalf("expected old line friend unlinked, got %#v", oldFriend.LinkedCustomerID)
	}

	var newFriend models.LineFriend
	if err := handler.db.First(&newFriend, "line_uid = ?", newLineUID).Error; err != nil {
		t.Fatalf("reload new line friend: %v", err)
	}
	if newFriend.LinkedCustomerID == nil || *newFriend.LinkedCustomerID != customer.ID {
		t.Fatalf("expected new line friend linked to customer, got %#v", newFriend.LinkedCustomerID)
	}
}

// TestUpdateAppointmentLegacyOutstandingPaymentAcceptsNormalizedWriteModel 验证 legacy 记录可接受归一化后的未收款写模型。
func TestUpdateAppointmentLegacyOutstandingPaymentAcceptsNormalizedWriteModel(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	techID := uint(210)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "黃師傅",
		Role:  "technician",
		Phone: "0933777888",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "舊客戶",
		Address:         "北區忠孝路 18 號",
		Phone:           "0922333444",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "未收款",
		TotalAmount:     1500,
		PaidAmount:      500,
		PaymentReceived: false,
		ScheduledAt:     time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "舊客戶",
		"address": "北區忠孝路 188 號",
		"phone": "0922333444",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T09:00:00Z",
		"status": "completed",
		"technician_id": 210,
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.PaymentMethod != "現金" {
		t.Fatalf("expected normalized payment_method 現金, got %s", saved.PaymentMethod)
	}
	if saved.PaymentReceived {
		t.Fatalf("expected payment_received=false for outstanding appointment")
	}
	if saved.PaidAmount != 0 {
		t.Fatalf("expected normalized paid_amount 0, got %d", saved.PaidAmount)
	}
}

// TestUpdateAppointmentRejectsLegacyOutstandingPlaceholderForNonLegacyRecord 验证正常记录不能再写回 legacy 未收款占位值。
func TestUpdateAppointmentRejectsLegacyOutstandingPlaceholderForNonLegacyRecord(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	appointment := models.Appointment{
		CustomerName:   "一般客戶",
		Address:        "北區忠孝路 8 號",
		Phone:          "0922000333",
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "completed",
		Photos:         []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "一般客戶",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "未收款",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "付款方式無效") {
		t.Fatalf("expected invalid payment_method error, got %s", recorder.Body.String())
	}
}

// TestUpdateAppointmentRejectsDirtyPaidAmountForNonLegacyRecord 验证正常记录中 paid_amount 与 payment_received 不一致会被拒绝。
func TestUpdateAppointmentRejectsDirtyPaidAmountForNonLegacyRecord(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	techID := uint(22)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "林師傅",
		Role:  "technician",
		Phone: "0933666777",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:   "一般客戶",
		Address:        "北區忠孝路 8 號",
		Phone:          "0922000333",
		Items:          []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1500,
		DiscountAmount: 0,
		ScheduledAt:    time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:         "completed",
		TechnicianID:   &techID,
		Photos:         []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "一般客戶",
		"address": "北區忠孝路 18 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 800,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"technician_id": 22,
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "實收金額大於 0 時，收款狀態必須為已收款") {
		t.Fatalf("expected dirty paid/payment_received validation error, got %s", recorder.Body.String())
	}
}

// TestUpdateAppointmentPreservesExistingPaymentTimeWhenReceiptAlreadyConfirmed 验证已确认收款记录会保留原 payment_time。
func TestUpdateAppointmentPreservesExistingPaymentTimeWhenReceiptAlreadyConfirmed(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	originalPaymentTime := time.Date(2026, 3, 13, 9, 30, 0, 0, time.UTC)
	techID := uint(23)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "周師傅",
		Role:  "technician",
		Phone: "0933888999",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "已收款客戶",
		Address:         "北區忠孝路 8 號",
		Phone:           "0922000333",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "現金",
		TotalAmount:     1500,
		PaidAmount:      1500,
		PaymentReceived: true,
		PaymentTime:     &originalPaymentTime,
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "已收款客戶",
		"address": "北區忠孝路 99 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 1500,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "completed",
		"technician_id": 23,
		"photos": [],
		"payment_received": true
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: techID, Role: "technician"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var saved models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if saved.PaymentTime == nil {
		t.Fatalf("expected payment_time to be preserved")
	}
	if !saved.PaymentTime.Equal(originalPaymentTime) {
		t.Fatalf("expected payment_time %v, got %v", originalPaymentTime, *saved.PaymentTime)
	}
}

// TestUpdateAppointmentRejectsHalfCoordinates 验证经纬度必须成对出现。
func TestUpdateAppointmentRejectsHalfCoordinates(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	appointment := models.Appointment{
		CustomerName:  "陳小姐",
		Address:       "北區忠孝路 8 號",
		Phone:         "0922000333",
		Items:         []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:    []byte(`[]`),
		PaymentMethod: "現金",
		TotalAmount:   1500,
		ScheduledAt:   time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:        "assigned",
		TechnicianID:  uintPtr(7),
		Photos:        []byte(`[]`),
	}
	if err := handler.db.Create(&models.User{
		ID:    7,
		Name:  "林師傅",
		Role:  "technician",
		Phone: "0933000444",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	body := `{
		"customer_name": "陳小姐",
		"address": "北區忠孝路 8 號",
		"phone": "0922000333",
		"items": [{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"paid_amount": 0,
		"scheduled_at": "2026-03-13T08:00:00Z",
		"status": "assigned",
		"technician_id": 7,
		"lat": 25.0478,
		"photos": [],
		"payment_received": false
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", appointment.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/appointments/%d", appointment.ID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.UpdateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "緯度與經度必須同時填寫或同時留空") {
		t.Fatalf("expected coordinate validation error, got %s", recorder.Body.String())
	}
}

// TestApplyAppointmentDerivedFieldsNormalizesNoChargePaymentFields 验证无收款预约会清空收款相关字段。
func TestApplyAppointmentDerivedFieldsNormalizesNoChargePaymentFields(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	paymentTime := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)
	appointment := models.Appointment{
		Address:         "北區忠孝路 8 號",
		PaymentMethod:   "無收款",
		TotalAmount:     1800,
		PaidAmount:      1200,
		PaymentReceived: true,
		PaymentTime:     &paymentTime,
		Status:          "pending",
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
	}

	if err := handler.applyAppointmentDerivedFields(&appointment); err != nil {
		t.Fatalf("apply derived fields: %v", err)
	}
	if appointment.PaidAmount != 0 {
		t.Fatalf("expected paid_amount reset to 0, got %d", appointment.PaidAmount)
	}
	if appointment.PaymentReceived {
		t.Fatalf("expected payment_received reset to false")
	}
	if appointment.PaymentTime != nil {
		t.Fatalf("expected payment_time cleared, got %v", appointment.PaymentTime)
	}
}

// TestApplyAppointmentDerivedFieldsRejectsPaidAmountWithoutPaymentReceived 验证已收金额大于零时必须显式确认收款。
func TestApplyAppointmentDerivedFieldsRejectsPaidAmountWithoutPaymentReceived(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if err := handler.db.Create(&models.User{
		ID:    11,
		Name:  "王師傅",
		Role:  "technician",
		Phone: "0933111222",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		Address:       "北區忠孝路 8 號",
		PaymentMethod: "現金",
		TotalAmount:   1800,
		PaidAmount:    1200,
		Status:        "completed",
		TechnicianID:  uintPtr(11),
		ScheduledAt:   time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
	}

	err := handler.applyAppointmentDerivedFields(&appointment)
	if err == nil || err.Error() != "payment_received must be true when paid_amount is greater than 0" {
		t.Fatalf("expected paid/payment_received validation error, got %v", err)
	}
}

// TestApplyAppointmentDerivedFieldsAutofillsPaymentTime 验证确认收款时会自动补 payment_time。
func TestApplyAppointmentDerivedFieldsAutofillsPaymentTime(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	if err := handler.db.Create(&models.User{
		ID:    12,
		Name:  "李師傅",
		Role:  "technician",
		Phone: "0933222333",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		Address:         "北區忠孝路 9 號",
		PaymentMethod:   "轉帳",
		TotalAmount:     1800,
		PaidAmount:      1800,
		PaymentReceived: true,
		Status:          "completed",
		TechnicianID:    uintPtr(12),
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
	}

	before := time.Now().UTC()
	if err := handler.applyAppointmentDerivedFields(&appointment); err != nil {
		t.Fatalf("apply derived fields: %v", err)
	}
	if appointment.PaymentTime == nil {
		t.Fatalf("expected payment_time to be auto populated")
	}
	if appointment.PaymentTime.Before(before.Add(-2 * time.Second)) {
		t.Fatalf("expected payment_time to be near now, got %v", appointment.PaymentTime)
	}
}

// TestNormalizePaymentMethodDoesNotCollapseOutstandingIntoNoCharge 验证未收款占位值不会被错误归一到無收款。
func TestNormalizePaymentMethodDoesNotCollapseOutstandingIntoNoCharge(t *testing.T) {
	t.Parallel()

	// `未收款` 属于收款状态而非付款方式；这里必须保留原值，
	// 让上层校验或兼容逻辑继续决定如何处理，不能直接当成 `無收款`。
	if got := normalizePaymentMethod("未收款"); got != "未收款" {
		t.Fatalf("expected 未收款 to stay unchanged, got %q", got)
	}
	if got := normalizePaymentMethod("unpaid"); got != "unpaid" {
		t.Fatalf("expected unpaid to stay unchanged, got %q", got)
	}
	if got := normalizePaymentMethod("no_charge"); got != "無收款" {
		t.Fatalf("expected no_charge normalized to 無收款, got %q", got)
	}
}

// TestValidateAppointmentCommonFieldsAllowsLegacyOutstandingPlaceholder 验证 legacy 未收款占位值仍可通过公共字段校验。
func TestValidateAppointmentCommonFieldsAllowsLegacyOutstandingPlaceholder(t *testing.T) {
	t.Parallel()

	err := validateAppointmentCommonFields("王小明", "北區成功路 1 號", "0911000222", nil, "未收款")
	if err != nil {
		t.Fatalf("expected legacy payment_method to be accepted, got %v", err)
	}
}

// TestCreateAppointmentRejectsPaymentReadonlyFields 验证创建预约时拒绝客户端写入收款只读字段。
func TestCreateAppointmentRejectsPaymentReadonlyFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	body := `{
		"customer_name": "王小明",
		"address": "北區成功路 1 號",
		"phone": "0911000222",
		"items": [{"id":"svc-1","type":"分離式冷氣","note":"","price":1800}],
		"extra_items": [],
		"payment_method": "現金",
		"discount_amount": 0,
		"scheduled_at": "2026-03-14T10:00:00Z",
		"paid_amount": 1800,
		"payment_received": true
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/appointments", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAppointment(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "未知欄位：paid_amount") {
		t.Fatalf("expected unknown paid_amount field error, got %s", recorder.Body.String())
	}
}

// TestValidateCashLedgerBusinessRulesAllowsCashAliases 验证 cash 等别名付款方式仍可通过现金流水校验。
func TestValidateCashLedgerBusinessRulesAllowsCashAliases(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	techID := uint(30)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "現金別名師傅",
		Role:  "technician",
		Phone: "0933000333",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "別名現金客戶",
		Address:         "北區民權路 1 號",
		Phone:           "0911222333",
		Items:           []byte(`[{"id":"svc-1","type":"分離式冷氣","note":"","price":1800}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "cash",
		TotalAmount:     1800,
		PaidAmount:      1800,
		PaymentReceived: true,
		ScheduledAt:     time.Date(2026, 3, 13, 11, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	payload := cashLedgerPayload{
		TechnicianID:  techID,
		AppointmentID: uintPtr(appointment.ID),
		Type:          "collect",
		Amount:        1800,
		Note:          "alias collect",
	}

	if err := handler.validateCashLedgerBusinessRules(payload); err != nil {
		t.Fatalf("expected cash alias appointment to pass collect validation, got %v", err)
	}
}

// TestValidateCashLedgerBusinessRulesRejectsPartialAppointmentCollect 验证预约收现流水金额必须与预约已收金额完全一致。
func TestValidateCashLedgerBusinessRulesRejectsPartialAppointmentCollect(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	techID := uint(301)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "部分收款師傅",
		Role:  "technician",
		Phone: "0933001301",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}
	appointment := models.Appointment{
		CustomerName:    "部分收款客戶",
		Address:         "北區民權路 11 號",
		Phone:           "0911222301",
		Items:           []byte(`[{"id":"svc-1","type":"分離式冷氣","note":"","price":1800}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "現金",
		TotalAmount:     1800,
		PaidAmount:      1800,
		PaymentReceived: true,
		ScheduledAt:     time.Date(2026, 3, 13, 11, 30, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &techID,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	payload := cashLedgerPayload{
		TechnicianID:  techID,
		AppointmentID: uintPtr(appointment.ID),
		Type:          "collect",
		Amount:        1000,
		Note:          "partial collect",
	}

	err := handler.validateCashLedgerBusinessRules(payload)
	if err == nil || err.Error() != "amount must equal appointment paid amount" {
		t.Fatalf("expected exact collect amount validation error, got %v", err)
	}
}

// TestCurrentCashLedgerBalanceCountsCashAliasesOnly 验证现金余额只统计现金别名预约并扣除回缴流水。
func TestCurrentCashLedgerBalanceCountsCashAliasesOnly(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	techID := uint(31)
	if err := handler.db.Create(&models.User{
		ID:    techID,
		Name:  "現金統計師傅",
		Role:  "technician",
		Phone: "0933111444",
	}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}

	appointments := []models.Appointment{
		{
			CustomerName:    "cash 英文",
			Address:         "北區成功路 2 號",
			Phone:           "0911000001",
			Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1200}]`),
			ExtraItems:      []byte(`[]`),
			PaymentMethod:   "cash",
			TotalAmount:     1200,
			PaidAmount:      1200,
			PaymentReceived: true,
			ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
			Status:          "completed",
			TechnicianID:    &techID,
			Photos:          []byte(`[]`),
		},
		{
			CustomerName:    "现金 简体",
			Address:         "北區成功路 3 號",
			Phone:           "0911000002",
			Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":900}]`),
			ExtraItems:      []byte(`[]`),
			PaymentMethod:   "现金",
			TotalAmount:     900,
			PaidAmount:      900,
			PaymentReceived: true,
			ScheduledAt:     time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC),
			Status:          "cancelled",
			TechnicianID:    &techID,
			Photos:          []byte(`[]`),
		},
		{
			CustomerName:    "legacy 未收款",
			Address:         "北區成功路 4 號",
			Phone:           "0911000003",
			Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":600}]`),
			ExtraItems:      []byte(`[]`),
			PaymentMethod:   "未收款",
			TotalAmount:     600,
			PaidAmount:      600,
			PaymentReceived: true,
			ScheduledAt:     time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC),
			Status:          "completed",
			TechnicianID:    &techID,
			Photos:          []byte(`[]`),
		},
		{
			CustomerName:    "轉帳",
			Address:         "北區成功路 5 號",
			Phone:           "0911000004",
			Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":700}]`),
			ExtraItems:      []byte(`[]`),
			PaymentMethod:   "bank_transfer",
			TotalAmount:     700,
			PaidAmount:      700,
			PaymentReceived: true,
			ScheduledAt:     time.Date(2026, 3, 13, 11, 0, 0, 0, time.UTC),
			Status:          "completed",
			TechnicianID:    &techID,
			Photos:          []byte(`[]`),
		},
	}
	for _, appointment := range appointments {
		appointment := appointment
		if err := handler.db.Create(&appointment).Error; err != nil {
			t.Fatalf("seed appointment %s: %v", appointment.CustomerName, err)
		}
	}
	if err := handler.db.Create(&models.CashLedgerEntry{
		ID:           "return-1",
		TechnicianID: techID,
		Type:         "return",
		Amount:       500,
		Note:         "weekly return",
		CreatedAt:    time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatalf("seed return entry: %v", err)
	}

	balance, err := handler.currentCashLedgerBalance(techID)
	if err != nil {
		t.Fatalf("currentCashLedgerBalance: %v", err)
	}
	// 只应统计 cash / 现金 两笔预约的实收，再扣除人工回缴；
	// legacy `未收款` 与转账别名必须被排除在现金余额之外。
	if balance != 1600 {
		t.Fatalf("expected cash alias balance 1600, got %d", balance)
	}
}

// TestCreateReviewUpsertsByAppointmentID 验证同一预约重复评价会按 appointment_id 原地更新。
func TestCreateReviewUpsertsByAppointmentID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	reviewToken := "review-token-1"
	technicianID := uint(9)
	technicianName := "王師傅"
	appointment := models.Appointment{
		ID:              1,
		CustomerName:    "回訪客戶",
		Address:         "北區民權路 8 號",
		Phone:           "0912333444",
		Items:           []byte(`[{"id":"svc-1","type":"窗型冷氣","note":"","price":1500}]`),
		ExtraItems:      []byte(`[]`),
		PaymentMethod:   "現金",
		TotalAmount:     1500,
		PaidAmount:      1500,
		PaymentReceived: true,
		ScheduledAt:     time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:          "completed",
		TechnicianID:    &technicianID,
		TechnicianName:  &technicianName,
		ReviewToken:     &reviewToken,
		Photos:          []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	if err := handler.db.Create(&models.Review{
		ID:            "rev-old",
		AppointmentID: appointment.ID,
		CustomerName:  "回訪客戶",
		Rating:        3,
		Misconducts:   []byte(`[]`),
		Comment:       "舊評論",
		CreatedAt:     time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatalf("seed review: %v", err)
	}

	body := `{
		"customer_name": "前端偽造名稱",
		"technician_id": 777,
		"technician_name": "前端偽造師傅",
		"rating": 5,
		"misconducts": [],
		"comment": "新評論",
		"shared_line": true
	}`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "reviewToken", Value: reviewToken}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/reviews/token/"+reviewToken, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateReview(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var reviews []models.Review
	if err := handler.db.Order("id asc").Find(&reviews).Error; err != nil {
		t.Fatalf("load reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 review after upsert, got %d", len(reviews))
	}
	if reviews[0].Comment != "新評論" || reviews[0].Rating != 5 || !reviews[0].SharedLine {
		t.Fatalf("expected review updated in place, got %+v", reviews[0])
	}
	if reviews[0].CustomerName != appointment.CustomerName {
		t.Fatalf("expected customer snapshot from appointment, got %+v", reviews[0])
	}
	if reviews[0].TechnicianID == nil || *reviews[0].TechnicianID != technicianID {
		t.Fatalf("expected technician snapshot from appointment, got %+v", reviews[0])
	}
	if reviews[0].TechnicianName == nil || *reviews[0].TechnicianName != technicianName {
		t.Fatalf("expected technician name snapshot from appointment, got %+v", reviews[0])
	}
}

// TestGetReviewContextLooksUpAppointmentByReviewToken 验证公开评价页上下文改为按随机 token 读取预约。
func TestGetReviewContextLooksUpAppointmentByReviewToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	reviewToken := "review-token-context"
	appointment := models.Appointment{
		ID:            42,
		CustomerName:  "評價客戶",
		Address:       "台北市中山區民生東路 9 號",
		Phone:         "0912555666",
		Items:         []byte(`[]`),
		ExtraItems:    []byte(`[]`),
		PaymentMethod: "現金",
		TotalAmount:   1500,
		ScheduledAt:   time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:        "completed",
		Photos:        []byte(`[]`),
		ReviewToken:   &reviewToken,
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "reviewToken", Value: reviewToken}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/reviews/token/"+reviewToken+"/context", nil)

	handler.GetReviewContext(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"customer_name":"評價客戶"`) {
		t.Fatalf("expected appointment loaded by review token, got %s", recorder.Body.String())
	}
}

// TestLoadAppointmentsBackfillsMissingReviewToken 验证旧预约记录缺少公开评价令牌时会在读取链路自动补齐。
func TestLoadAppointmentsBackfillsMissingReviewToken(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	appointment := models.Appointment{
		ID:            88,
		CustomerName:  "旧预约",
		Address:       "台北市信義區市府路 1 號",
		Phone:         "0912888999",
		Items:         []byte(`[]`),
		ExtraItems:    []byte(`[]`),
		PaymentMethod: "現金",
		TotalAmount:   1000,
		ScheduledAt:   time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC),
		Status:        "pending",
		Photos:        []byte(`[]`),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	appointments, err := handler.loadAppointments()
	if err != nil {
		t.Fatalf("loadAppointments: %v", err)
	}
	if len(appointments) != 1 || appointments[0].ReviewToken == nil || strings.TrimSpace(*appointments[0].ReviewToken) == "" {
		t.Fatalf("expected review token generated, got %+v", appointments)
	}

	var reloaded models.Appointment
	if err := handler.db.First(&reloaded, "id = ?", appointment.ID).Error; err != nil {
		t.Fatalf("reload appointment: %v", err)
	}
	if reloaded.ReviewToken == nil || strings.TrimSpace(*reloaded.ReviewToken) == "" {
		t.Fatalf("expected review token persisted to database, got %+v", reloaded)
	}
}

// TestReplaceCustomersRebindsLineFriendWhenLineUIDChanges 验证批量替换客户时会同步处理 LINE 改绑。
func TestReplaceCustomersRebindsLineFriendWhenLineUIDChanges(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	oldLineUID := "Ucustomer-old"
	newLineUID := "Ucustomer-new"
	joinedAt := time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC)
	if err := handler.db.Create(&models.Customer{
		ID:           "cust-1",
		Name:         "旧客户",
		Phone:        "0912000001",
		Address:      "台北市中山區民生東路 1 號",
		LineID:       &oldLineUID,
		LineUID:      &oldLineUID,
		LineName:     stringPtr("旧好友"),
		LinePicture:  stringPtr("https://example.com/old.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"old":true}`),
		CreatedAt:    joinedAt,
		UpdatedAt:    joinedAt,
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          oldLineUID,
		LineName:         "旧好友",
		LinePicture:      "https://example.com/old.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: stringPtr("cust-1"),
		Status:           "followed",
		LastPayload:      []byte(`{"old":true}`),
	}).Error; err != nil {
		t.Fatalf("seed old line friend: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:     newLineUID,
		LineName:    "新好友",
		LinePicture: "https://example.com/new.png",
		JoinedAt:    joinedAt.Add(time.Hour),
		Status:      "followed",
		LastPayload: []byte(`{"new":true}`),
	}).Error; err != nil {
		t.Fatalf("seed new line friend: %v", err)
	}

	body := `[
		{
			"id": "cust-1",
			"name": "旧客户",
			"phone": "0912000001",
			"address": "台北市中山區民生東路 99 號",
			"line_id": "Ucustomer-new",
			"line_name": "新好友",
			"line_picture": "https://example.com/new.png",
			"line_uid": "Ucustomer-new",
			"line_joined_at": "2026-03-13T09:00:00Z",
			"line_data": {"new": true},
			"created_at": "2026-03-13T08:00:00Z"
		}
	]`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/customers", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ReplaceCustomers(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var oldFriend models.LineFriend
	if err := handler.db.First(&oldFriend, "line_uid = ?", oldLineUID).Error; err != nil {
		t.Fatalf("reload old line friend: %v", err)
	}
	if oldFriend.LinkedCustomerID != nil {
		t.Fatalf("expected old line friend unlinked, got %#v", oldFriend.LinkedCustomerID)
	}

	var newFriend models.LineFriend
	if err := handler.db.First(&newFriend, "line_uid = ?", newLineUID).Error; err != nil {
		t.Fatalf("reload new line friend: %v", err)
	}
	if newFriend.LinkedCustomerID == nil || *newFriend.LinkedCustomerID != "cust-1" {
		t.Fatalf("expected new line friend linked to customer, got %#v", newFriend.LinkedCustomerID)
	}
}

// TestReplaceCustomersUnlinksRemovedCustomerLineFriend 验证删除客户时会同时解除其好友绑定。
func TestReplaceCustomersUnlinksRemovedCustomerLineFriend(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	lineUID := "Uremoved-customer"
	joinedAt := time.Date(2026, 3, 13, 8, 0, 0, 0, time.UTC)
	if err := handler.db.Create(&models.Customer{
		ID:           "cust-remove",
		Name:         "待删除客户",
		Phone:        "0912000002",
		Address:      "台北市中山區南京東路 1 號",
		LineID:       &lineUID,
		LineUID:      &lineUID,
		LineName:     stringPtr("待删除好友"),
		LinePicture:  stringPtr("https://example.com/remove.png"),
		LineJoinedAt: &joinedAt,
		LineData:     []byte(`{"remove":true}`),
		CreatedAt:    joinedAt,
		UpdatedAt:    joinedAt,
	}).Error; err != nil {
		t.Fatalf("seed old customer: %v", err)
	}
	if err := handler.db.Create(&models.Customer{
		ID:        "cust-keep",
		Name:      "保留客户",
		Phone:     "0912000003",
		Address:   "台北市大安區仁愛路 1 號",
		LineData:  []byte(`{}`),
		CreatedAt: joinedAt,
		UpdatedAt: joinedAt,
	}).Error; err != nil {
		t.Fatalf("seed keep customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          lineUID,
		LineName:         "待删除好友",
		LinePicture:      "https://example.com/remove.png",
		JoinedAt:         joinedAt,
		LinkedCustomerID: stringPtr("cust-remove"),
		Status:           "followed",
		LastPayload:      []byte(`{"remove":true}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	body := `[
		{
			"id": "cust-keep",
			"name": "保留客户",
			"phone": "0912000003",
			"address": "台北市大安區仁愛路 1 號",
			"line_data": {},
			"created_at": "2026-03-13T08:00:00Z"
		}
	]`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/customers", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ReplaceCustomers(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var deletedCustomerCount int64
	if err := handler.db.Model(&models.Customer{}).Where("id = ?", "cust-remove").Count(&deletedCustomerCount).Error; err != nil {
		t.Fatalf("count removed customer: %v", err)
	}
	if deletedCustomerCount != 0 {
		t.Fatalf("expected removed customer deleted, got count=%d", deletedCustomerCount)
	}

	var friend models.LineFriend
	if err := handler.db.First(&friend, "line_uid = ?", lineUID).Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if friend.LinkedCustomerID != nil {
		t.Fatalf("expected removed customer's line friend unlinked, got %#v", friend.LinkedCustomerID)
	}
}

// TestReplaceCustomersRejectsMissingPhoneOrAddress 验证客户批量写接口会拒绝缺手机号或地址的数据。
func TestReplaceCustomersRejectsMissingPhoneOrAddress(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	body := `[
		{
			"id": "cust-invalid",
			"name": "缺资料客户",
			"phone": "",
			"address": "",
			"line_data": {},
			"created_at": "2026-03-13T08:00:00Z"
		}
	]`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/customers", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ReplaceCustomers(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "客戶電話與地址為必填欄位") {
		t.Fatalf("expected missing phone/address error, got %s", recorder.Body.String())
	}
}

// TestReplaceTechniciansRejectsMissingPhone 验证技师批量写接口会拒绝缺少手机号的数据，保持与数据库唯一登录键约束一致。
func TestReplaceTechniciansRejectsMissingPhone(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	body := `[
		{
			"id": 7,
			"name": "未填手機技師",
			"phone": "",
			"skills": [],
			"availability": []
		}
	]`

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/technicians", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ReplaceTechnicians(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "技師手機號碼為必填欄位") {
		t.Fatalf("expected missing technician phone error, got %s", recorder.Body.String())
	}
}

// TestReplaceEndpointsRejectEmptyPayloadToPreventFullDeletion 验证批量覆盖接口会拒绝空数组，避免异常请求误删整张表。
func TestReplaceEndpointsRejectEmptyPayloadToPreventFullDeletion(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name           string
		path           string
		call           func(handler *Handler, ctx *gin.Context)
		seed           func(t *testing.T, handler *Handler)
		assertNotEmpty func(t *testing.T, handler *Handler)
		expectedError  string
	}{
		{
			name: "technicians",
			path: "/api/technicians",
			call: func(handler *Handler, ctx *gin.Context) { handler.ReplaceTechnicians(ctx) },
			seed: func(t *testing.T, handler *Handler) {
				t.Helper()
				if err := handler.db.Create(&models.User{
					ID:           7,
					Name:         "王师傅",
					Role:         "technician",
					Phone:        "0912000007",
					PasswordHash: "hashed",
				}).Error; err != nil {
					t.Fatalf("seed technician: %v", err)
				}
			},
			assertNotEmpty: func(t *testing.T, handler *Handler) {
				t.Helper()
				var count int64
				if err := handler.db.Model(&models.User{}).Where("role = ?", "technician").Count(&count).Error; err != nil {
					t.Fatalf("count technicians: %v", err)
				}
				if count != 1 {
					t.Fatalf("expected technician records preserved, got count=%d", count)
				}
			},
			expectedError: "技師資料不得為空",
		},
		{
			name: "zones",
			path: "/api/zones",
			call: func(handler *Handler, ctx *gin.Context) { handler.ReplaceZones(ctx) },
			seed: func(t *testing.T, handler *Handler) {
				t.Helper()
				if err := handler.db.Create(&models.ServiceZone{
					ID:        "zone-1",
					Name:      "台北一区",
					Districts: []byte(`["中山區"]`),
				}).Error; err != nil {
					t.Fatalf("seed zone: %v", err)
				}
			},
			assertNotEmpty: func(t *testing.T, handler *Handler) {
				t.Helper()
				var count int64
				if err := handler.db.Model(&models.ServiceZone{}).Count(&count).Error; err != nil {
					t.Fatalf("count zones: %v", err)
				}
				if count != 1 {
					t.Fatalf("expected zone records preserved, got count=%d", count)
				}
			},
			expectedError: "區域資料不得為空",
		},
		{
			name: "service items",
			path: "/api/service-items",
			call: func(handler *Handler, ctx *gin.Context) { handler.ReplaceServiceItems(ctx) },
			seed: func(t *testing.T, handler *Handler) {
				t.Helper()
				if err := handler.db.Create(&models.ServiceItem{
					ID:           "svc-1",
					Name:         "分離式",
					DefaultPrice: 1800,
				}).Error; err != nil {
					t.Fatalf("seed service item: %v", err)
				}
			},
			assertNotEmpty: func(t *testing.T, handler *Handler) {
				t.Helper()
				var count int64
				if err := handler.db.Model(&models.ServiceItem{}).Count(&count).Error; err != nil {
					t.Fatalf("count service items: %v", err)
				}
				if count != 1 {
					t.Fatalf("expected service item records preserved, got count=%d", count)
				}
			},
			expectedError: "服務項目資料不得為空",
		},
		{
			name: "extra items",
			path: "/api/extra-items",
			call: func(handler *Handler, ctx *gin.Context) { handler.ReplaceExtraItems(ctx) },
			seed: func(t *testing.T, handler *Handler) {
				t.Helper()
				if err := handler.db.Create(&models.ExtraItem{
					ID:    "extra-1",
					Name:  "高楼层费",
					Price: 500,
				}).Error; err != nil {
					t.Fatalf("seed extra item: %v", err)
				}
			},
			assertNotEmpty: func(t *testing.T, handler *Handler) {
				t.Helper()
				var count int64
				if err := handler.db.Model(&models.ExtraItem{}).Count(&count).Error; err != nil {
					t.Fatalf("count extra items: %v", err)
				}
				if count != 1 {
					t.Fatalf("expected extra item records preserved, got count=%d", count)
				}
			},
			expectedError: "額外項目資料不得為空",
		},
		{
			name: "customers",
			path: "/api/customers",
			call: func(handler *Handler, ctx *gin.Context) { handler.ReplaceCustomers(ctx) },
			seed: func(t *testing.T, handler *Handler) {
				t.Helper()
				if err := handler.db.Create(&models.Customer{
					ID:        "cust-1",
					Name:      "王小明",
					Phone:     "0911222333",
					Address:   "台北市中山區南京東路 1 號",
					LineData:  []byte(`{}`),
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}).Error; err != nil {
					t.Fatalf("seed customer: %v", err)
				}
			},
			assertNotEmpty: func(t *testing.T, handler *Handler) {
				t.Helper()
				var count int64
				if err := handler.db.Model(&models.Customer{}).Count(&count).Error; err != nil {
					t.Fatalf("count customers: %v", err)
				}
				if count != 1 {
					t.Fatalf("expected customer records preserved, got count=%d", count)
				}
			},
			expectedError: "客戶資料不得為空",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := newTestHandler(t)
			tc.seed(t, handler)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(`[]`))
			ctx.Request.Header.Set("Content-Type", "application/json")

			tc.call(handler, ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d, body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.expectedError) {
				t.Fatalf("expected error %q, got %s", tc.expectedError, recorder.Body.String())
			}

			tc.assertNotEmpty(t, handler)
		})
	}
}

// TestDeleteCustomerClearsLinkedLineFriend 验证删除客户会清空关联好友的 linked_customer_id。
func TestDeleteCustomerClearsLinkedLineFriend(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	lineUID := "Udelete-customer"
	if err := handler.db.Create(&models.Customer{
		ID:        "cust-delete",
		Name:      "待删除客户",
		Phone:     "0912000004",
		Address:   "台北市信義區信義路 1 號",
		LineUID:   &lineUID,
		LineData:  []byte(`{}`),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := handler.db.Create(&models.LineFriend{
		LineUID:          lineUID,
		LineName:         "待删除好友",
		LinePicture:      "https://example.com/delete.png",
		JoinedAt:         time.Now().UTC(),
		LinkedCustomerID: stringPtr("cust-delete"),
		Status:           "followed",
		LastPayload:      []byte(`{}`),
	}).Error; err != nil {
		t.Fatalf("seed line friend: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "cust-delete"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/customers/cust-delete", nil)

	handler.DeleteCustomer(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var friend models.LineFriend
	if err := handler.db.First(&friend, "line_uid = ?", lineUID).Error; err != nil {
		t.Fatalf("reload line friend: %v", err)
	}
	if friend.LinkedCustomerID != nil {
		t.Fatalf("expected line friend unlinked after customer delete, got %#v", friend.LinkedCustomerID)
	}
}

// TestGetDashboardPageDataReturnsEmptyArrays 验证空数据场景下首页接口返回空数组而不是 nil。
func TestGetDashboardPageDataReturnsEmptyArrays(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/dashboard-data", nil)

	handler.GetDashboardPageData(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload dashboardPageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal dashboard payload: %v", err)
	}
	if payload.Appointments == nil || payload.Technicians == nil || payload.Customers == nil || payload.Reviews == nil {
		t.Fatalf("expected empty arrays instead of nil, got %s", recorder.Body.String())
	}
}

// TestGetSettingsPageDataAggregatesResources 验证设置页接口会聚合项目、附加项和 webhook 配置。
func TestGetSettingsPageDataAggregatesResources(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{
		LineChannelSecret:    "line-secret-for-test",
		WebhookPublicBaseURL: "https://dispatch.example.com",
	})
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	description := "回访提醒天数"
	if err := handler.db.Create(&models.ServiceItem{
		ID:           "svc-settings-1",
		Name:         "窗型冷氣",
		DefaultPrice: 1500,
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("seed service item: %v", err)
	}
	if err := handler.db.Create(&models.ExtraItem{
		ID:        "extra-settings-1",
		Name:      "抗菌",
		Price:     400,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed extra item: %v", err)
	}
	if err := handler.db.Create(&models.AppSetting{
		Key:         "reminder_days",
		Value:       "60",
		Description: &description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/settings-data", nil)

	handler.GetSettingsPageData(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload settingsPageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal settings page payload: %v", err)
	}
	if len(payload.ServiceItems) != 1 || len(payload.ExtraFeeProducts) != 1 || payload.Settings.ReminderDays != 60 {
		t.Fatalf("unexpected settings page payload: %s", recorder.Body.String())
	}
	if !payload.Settings.Webhook.Enabled || !payload.Settings.Webhook.EffectiveEnabled {
		t.Fatalf("expected webhook enabled in settings page payload, got %s", recorder.Body.String())
	}
	if payload.Settings.Webhook.URL != "https://dispatch.example.com/api/webhook/line" {
		t.Fatalf("unexpected webhook url in settings page payload: %s", recorder.Body.String())
	}
}

// TestListAppointmentsReturnsDatabaseRecords 验证预约列表接口按排程时间倒序返回数据库记录。
func TestListAppointmentsReturnsDatabaseRecords(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	now := time.Now().UTC()
	first := models.Appointment{
		CustomerName:  "王小明",
		Address:       "台北市大安區仁愛路 1 號",
		Phone:         "0911000001",
		Items:         []byte(`[{"id":"svc-1","type":"分離式","note":"","price":1800}]`),
		PaymentMethod: "現金",
		TotalAmount:   1800,
		Status:        "pending",
		ScheduledAt:   now.Add(2 * time.Hour),
	}
	second := models.Appointment{
		CustomerName:  "陳小美",
		Address:       "台北市信義區松仁路 2 號",
		Phone:         "0911000002",
		Items:         []byte(`[{"id":"svc-2","type":"窗型","note":"","price":2200}]`),
		PaymentMethod: "轉帳",
		TotalAmount:   2200,
		Status:        "assigned",
		ScheduledAt:   now.Add(4 * time.Hour),
	}
	if err := handler.db.Create(&first).Error; err != nil {
		t.Fatalf("seed first appointment: %v", err)
	}
	if err := handler.db.Create(&second).Error; err != nil {
		t.Fatalf("seed second appointment: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/appointments", nil)

	handler.ListAppointments(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload []models.Appointment
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal appointments payload: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 appointments, got %d", len(payload))
	}
	if payload[0].CustomerName != "陳小美" || payload[1].CustomerName != "王小明" {
		t.Fatalf("expected appointments sorted by scheduled_at desc, got %+v", payload)
	}
}

// TestListTechniciansOnlyReturnsTechnicians 验证技师列表接口会过滤掉管理员账号。
func TestListTechniciansOnlyReturnsTechnicians(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	if err := handler.db.Create(&models.User{ID: 1, Name: "管理员", Role: "admin", Phone: "0900000001"}).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := handler.db.Create(&models.User{ID: 2, Name: "技师甲", Role: "technician", Phone: "0900000002"}).Error; err != nil {
		t.Fatalf("seed technician 1: %v", err)
	}
	if err := handler.db.Create(&models.User{ID: 3, Name: "技师乙", Role: "technician", Phone: "0900000003"}).Error; err != nil {
		t.Fatalf("seed technician 2: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/technicians", nil)

	handler.ListTechnicians(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload []models.User
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal technicians payload: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 technicians, got %d", len(payload))
	}
	if payload[0].Role != "technician" || payload[1].Role != "technician" {
		t.Fatalf("expected only technicians, got %+v", payload)
	}
}

// TestGetSettingsReturnsReminderDays 验证设置接口会返回提醒天数与 webhook 状态。
func TestGetSettingsReturnsReminderDays(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{
		LineChannelSecret:    "line-secret-for-test",
		WebhookPublicBaseURL: "https://dispatch.example.com",
	})
	description := "提醒天数"
	now := time.Now().UTC()
	if err := handler.db.Create(&models.AppSetting{
		Key:         "reminder_days",
		Value:       "45",
		Description: &description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("seed app setting: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/settings", nil)

	handler.GetSettings(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload settingsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal settings payload: %v", err)
	}
	if payload.ReminderDays != 45 {
		t.Fatalf("expected reminder_days 45, got %+v", payload)
	}
	if !payload.Webhook.Enabled || !payload.Webhook.EffectiveEnabled {
		t.Fatalf("expected webhook enabled, got %+v", payload)
	}
	if payload.Webhook.URL != "https://dispatch.example.com/api/webhook/line" {
		t.Fatalf("unexpected webhook url, got %+v", payload)
	}
}

// TestUpdateWebhookEnabledPersistsAdminSwitch 验证管理员切换 webhook 开关会持久化到设置表。
func TestUpdateWebhookEnabledPersistsAdminSwitch(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandlerWithConfig(t, config.Config{
		LineChannelSecret:    "line-secret-for-test",
		WebhookPublicBaseURL: "https://dispatch.example.com",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/settings/webhook-enabled", strings.NewReader(`{"enabled":false}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateWebhookEnabled(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload webhookSettingsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal webhook settings payload: %v", err)
	}
	if payload.Enabled || payload.EffectiveEnabled {
		t.Fatalf("expected webhook disabled, got %+v", payload)
	}

	var item models.AppSetting
	if err := handler.db.First(&item, "key = ?", "line_webhook_enabled").Error; err != nil {
		t.Fatalf("query webhook setting: %v", err)
	}
	if item.Value != "false" {
		t.Fatalf("expected persisted webhook switch false, got %+v", item)
	}
}

// TestReceiveLineWebhookReturns503WhenAdminDisabled 验证管理员关闭 webhook 后公开入口返回 503。
func TestReceiveLineWebhookReturns503WhenAdminDisabled(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	lineSecret := "line-secret-for-test"
	handler := newTestHandlerWithConfig(t, config.Config{LineChannelSecret: lineSecret})
	description := "管理员控制 LINE webhook 处理开关"
	now := time.Now().UTC()
	if err := handler.db.Create(&models.AppSetting{
		Key:         "line_webhook_enabled",
		Value:       "false",
		Description: &description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("seed webhook setting: %v", err)
	}

	body := `{"events":[]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(body))
	ctx.Request.Header.Set("X-Line-Signature", signLineWebhookBody(lineSecret, body))

	handler.ReceiveLineWebhook(ctx)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "LINE Webhook 已被管理員停用") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

// uintPtr 为测试用例返回 uint 指针，便于内联构造可选字段。
func uintPtr(value uint) *uint {
	return &value
}
