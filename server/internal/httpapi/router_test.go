package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cool-dispatch/internal/config"
	"cool-dispatch/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newRouterTestDB 为路由测试创建一套内存数据库并完成模型迁移。
func newRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.AutoMigrateModels()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// attachAuthCookie 为测试请求注入真实持久化 token，确保路由层认证逻辑按生产路径执行。
func attachAuthCookie(t *testing.T, db *gorm.DB, request *http.Request, userID uint) {
	t.Helper()

	token, err := createAuthToken(db, userID)
	if err != nil {
		t.Fatalf("create auth token: %v", err)
	}

	request.AddCookie(&http.Cookie{
		Name:  tokenCookieName,
		Value: token.Token,
		Path:  "/",
	})
}

// TestNewRouterServesStaticIndexAndPreservesAPI404 验证静态页面回退与 API 404 不互相污染。
func TestNewRouterServesStaticIndexAndPreservesAPI404(t *testing.T) {
	t.Parallel()

	distDir := t.TempDir()
	indexPath := filepath.Join(distDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>cool-dispatch</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.Mkdir(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}

	router := NewRouter(config.Config{
		AppEnv:       "production",
		EnableStatic: true,
		FrontendDist: distDir,
	}, newRouterTestDB(t))

	staticRecorder := httptest.NewRecorder()
	staticRequest := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	router.ServeHTTP(staticRecorder, staticRequest)
	if staticRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", staticRecorder.Code)
	}
	if !strings.Contains(staticRecorder.Body.String(), "cool-dispatch") {
		t.Fatalf("expected index.html body, got %s", staticRecorder.Body.String())
	}

	apiRecorder := httptest.NewRecorder()
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	router.ServeHTTP(apiRecorder, apiRequest)
	if apiRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", apiRecorder.Code)
	}
	if !strings.Contains(apiRecorder.Body.String(), "找不到請求的資源") {
		t.Fatalf("expected JSON 404 body, got %s", apiRecorder.Body.String())
	}
}

// TestNewRouterHandlesCORSPreflightForAllowedOrigin 验证允许来源的预检请求会返回正确跨域头。
func TestNewRouterHandlesCORSPreflightForAllowedOrigin(t *testing.T) {
	t.Parallel()

	router := NewRouter(config.Config{
		AppEnv:         "development",
		FrontendOrigin: "https://dispatch.example.com",
	}, newRouterTestDB(t))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	request.Header.Set("Origin", "https://dispatch.example.com")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://dispatch.example.com" {
		t.Fatalf("expected allow origin header, got %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestNewRouterRejectsOversizedWebhookBody 验证公开 webhook 会应用更严格的请求体大小限制。
func TestNewRouterRejectsOversizedWebhookBody(t *testing.T) {
	t.Parallel()

	router := NewRouter(config.Config{
		AppEnv:              "development",
		MaxJSONBodyBytes:    1024,
		MaxWebhookBodyBytes: 64,
	}, newRouterTestDB(t))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/webhook/line", strings.NewReader(strings.Repeat("x", 128)))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "請求體過大") {
		t.Fatalf("expected request body too large message, got %s", recorder.Body.String())
	}
}

// TestCORSAllowsAllOrigins 验证 CORS 中间件允许任意来源的跨域请求。
func TestCORSAllowsAllOrigins(t *testing.T) {
	t.Parallel()

	router := NewRouter(config.Config{
		AppEnv: "development",
	}, newRouterTestDB(t))

	// 测试任意外部来源均被允许
	origins := []string{
		"https://dispatch.example.com",
		"http://localhost:5173",
		"https://evil.example.com",
		"https://another-site.com",
	}

	for _, origin := range origins {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
		request.Header.Set("Origin", origin)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("origin %s: expected 204, got %d", origin, recorder.Code)
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("origin %s: expected allow origin %q, got %q", origin, origin, got)
		}
	}
}

// TestNewRouterProvidesResourceReadEndpoints 验证资源读取接口在认证后可返回完整读模型。
func TestNewRouterProvidesResourceReadEndpoints(t *testing.T) {
	t.Parallel()

	db := newRouterTestDB(t)
	now := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)
	color := "#1677ff"
	description := "客戶完工後幾天提醒回訪"

	seeds := []any{
		&models.User{ID: 1, Name: "管理员", Role: "admin", Phone: "0900000001", CreatedAt: now, UpdatedAt: now},
		&models.User{ID: 7, Name: "王師傅", Role: "technician", Phone: "0911222333", Color: &color, CreatedAt: now, UpdatedAt: now},
		&models.Customer{ID: "cust-1", Name: "王小明", Phone: "0911222333", Address: "台北市中山區南京東路 1 號", CreatedAt: now, UpdatedAt: now},
		&models.Appointment{ID: 11, CustomerName: "王小明", Address: "台北市中山區南京東路 1 號", Phone: "0911222333", PaymentMethod: "現金", TotalAmount: 1800, ScheduledAt: now, Status: "pending", Items: []byte(`[]`), ExtraItems: []byte(`[]`), Photos: []byte(`[]`), CreatedAt: now, UpdatedAt: now},
		&models.LineFriend{LineUID: "Urouter-1", LineName: "好友一號", LinePicture: "https://example.com/line.png", JoinedAt: now, LinkedCustomerID: stringPtr("cust-1"), Status: "followed", LastPayload: []byte(`{"source":"router-test"}`), CreatedAt: now, UpdatedAt: now},
		&models.ServiceZone{ID: "zone-1", Name: "台北一區", Districts: []byte(`["中山區"]`), AssignedTechnicianIDs: []byte(`[7]`), CreatedAt: now, UpdatedAt: now},
		&models.ServiceItem{ID: "svc-1", Name: "分離式", DefaultPrice: 1800, CreatedAt: now, UpdatedAt: now},
		&models.ExtraItem{ID: "extra-1", Name: "防霉", Price: 300, CreatedAt: now, UpdatedAt: now},
		&models.CashLedgerEntry{ID: "cl-1", TechnicianID: 7, Type: "return", Amount: 200, Note: "回缴", CreatedAt: now, UpdatedAt: now},
		&models.Review{ID: "rev-1", AppointmentID: 11, CustomerName: "王小明", Rating: 5, Misconducts: []byte(`[]`), Comment: "很好", CreatedAt: now, UpdatedAt: now},
		&models.NotificationLog{ID: "notif-1", AppointmentID: 11, Type: "line", Message: "提醒", SentAt: now, CreatedAt: now, UpdatedAt: now},
		&models.AppSetting{Key: "reminder_days", Value: "45", Description: &description, CreatedAt: now, UpdatedAt: now},
	}
	for _, seed := range seeds {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed data: %v", err)
		}
	}

	router := NewRouter(config.Config{AppEnv: "development"}, db)

	// settingsPayload 表示设置页相关接口在测试中的最小设置载荷。
	type settingsPayload struct {
		// ReminderDays 是测试中断言的回访提醒天数。
		ReminderDays int `json:"reminder_days"`
	}

	// dashboardPayload 表示首页仪表盘聚合接口在测试中的返回载荷。
	type dashboardPayload struct {
		// Appointments 是首页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// Technicians 是首页技师列表。
		Technicians []models.User `json:"technicians"`
		// Customers 是首页客户列表。
		Customers []models.Customer `json:"customers"`
		// Reviews 是首页评价列表。
		Reviews []models.Review `json:"reviews"`
	}

	// customerPagePayload 表示客户页聚合接口在测试中的返回载荷。
	type customerPagePayload struct {
		// Customers 是客户页客户列表。
		Customers []models.Customer `json:"customers"`
		// Appointments 是客户页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// Reviews 是客户页评价列表。
		Reviews []models.Review `json:"reviews"`
	}

	// settingsPagePayload 表示系统设置页聚合接口在测试中的返回载荷。
	type settingsPagePayload struct {
		// ServiceItems 是设置页服务项目列表。
		ServiceItems []models.ServiceItem `json:"service_items"`
		// ExtraFeeProducts 是设置页额外收费项列表。
		ExtraFeeProducts []models.ExtraItem `json:"extra_fee_products"`
		// Settings 是设置页基础系统配置。
		Settings settingsPayload `json:"settings"`
	}

	// linePagePayload 表示 LINE 管理页聚合接口在测试中的返回载荷。
	type linePagePayload struct {
		// LineFriends 是 LINE 好友列表。
		LineFriends []lineFriendResponse `json:"line_friends"`
		// Customers 是可供绑定的客户列表。
		Customers []models.Customer `json:"customers"`
	}

	// technicianPagePayload 表示技师页聚合接口在测试中的返回载荷。
	type technicianPagePayload struct {
		// Technicians 是技师页技师列表。
		Technicians []models.User `json:"technicians"`
		// Appointments 是技师页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// Reviews 是技师页评价列表。
		Reviews []models.Review `json:"reviews"`
		// Zones 是技师页服务区域列表。
		Zones []models.ServiceZone `json:"zones"`
	}

	// reminderPagePayload 表示回访提醒页聚合接口在测试中的返回载荷。
	type reminderPagePayload struct {
		// Customers 是回访提醒页客户列表。
		Customers []models.Customer `json:"customers"`
		// Appointments 是回访提醒页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// Settings 是回访提醒页需要的提醒配置。
		Settings settingsPayload `json:"settings"`
	}

	// zonePagePayload 表示服务区域页聚合接口在测试中的返回载荷。
	type zonePagePayload struct {
		// Zones 是服务区域页区域列表。
		Zones []models.ServiceZone `json:"zones"`
		// Technicians 是服务区域页技师列表。
		Technicians []models.User `json:"technicians"`
	}

	// financialReportPagePayload 表示财务报表页聚合接口在测试中的返回载荷。
	type financialReportPagePayload struct {
		// Appointments 是财务页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// Technicians 是财务页技师列表。
		Technicians []models.User `json:"technicians"`
	}

	// reviewDashboardPagePayload 表示评价看板页聚合接口在测试中的返回载荷。
	type reviewDashboardPagePayload struct {
		// Reviews 是评价看板评价列表。
		Reviews []models.Review `json:"reviews"`
		// Technicians 是评价看板技师列表。
		Technicians []models.User `json:"technicians"`
		// Appointments 是评价看板预约列表。
		Appointments []models.Appointment `json:"appointments"`
	}

	// cashLedgerPagePayload 表示现金账页聚合接口在测试中的返回载荷。
	type cashLedgerPagePayload struct {
		// Technicians 是现金账页技师列表。
		Technicians []models.User `json:"technicians"`
		// Appointments 是现金账页预约列表。
		Appointments []models.Appointment `json:"appointments"`
		// CashLedgerEntries 是现金账流水列表。
		CashLedgerEntries []models.CashLedgerEntry `json:"cash_ledger_entries"`
	}

	testCases := []struct {
		name   string
		path   string
		assert func(t *testing.T, body []byte)
	}{
		{
			name: "dashboard page data",
			path: "/api/dashboard-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload dashboardPayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal dashboard payload: %v", err)
				}
				if len(payload.Appointments) != 1 || len(payload.Technicians) != 1 || len(payload.Customers) != 1 || len(payload.Reviews) != 1 {
					t.Fatalf("unexpected dashboard payload: %s", string(body))
				}
			},
		},
		{
			name: "customer page data",
			path: "/api/customer-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload customerPagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal customer page payload: %v", err)
				}
				if len(payload.Customers) != 1 || len(payload.Appointments) != 1 || len(payload.Reviews) != 1 {
					t.Fatalf("unexpected customer page payload: %s", string(body))
				}
			},
		},
		{
			name: "appointments",
			path: "/api/appointments",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.Appointment
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal appointments: %v", err)
				}
				if len(items) != 1 || items[0].ID != 11 {
					t.Fatalf("unexpected appointments payload: %s", string(body))
				}
			},
		},
		{
			name: "technicians",
			path: "/api/technicians",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.User
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal technicians: %v", err)
				}
				if len(items) != 1 || items[0].Name != "王師傅" {
					t.Fatalf("unexpected technicians payload: %s", string(body))
				}
			},
		},
		{
			name: "customers",
			path: "/api/customers",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.Customer
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal customers: %v", err)
				}
				if len(items) != 1 || items[0].ID != "cust-1" {
					t.Fatalf("unexpected customers payload: %s", string(body))
				}
			},
		},
		{
			name: "zones",
			path: "/api/zones",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.ServiceZone
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal zones: %v", err)
				}
				if len(items) != 1 || items[0].ID != "zone-1" {
					t.Fatalf("unexpected zones payload: %s", string(body))
				}
			},
		},
		{
			name: "service items",
			path: "/api/service-items",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.ServiceItem
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal service items: %v", err)
				}
				if len(items) != 1 || items[0].ID != "svc-1" {
					t.Fatalf("unexpected service items payload: %s", string(body))
				}
			},
		},
		{
			name: "extra items",
			path: "/api/extra-items",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.ExtraItem
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal extra items: %v", err)
				}
				if len(items) != 1 || items[0].ID != "extra-1" {
					t.Fatalf("unexpected extra items payload: %s", string(body))
				}
			},
		},
		{
			name: "cash ledger",
			path: "/api/cash-ledger",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.CashLedgerEntry
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal cash ledger: %v", err)
				}
				if len(items) != 1 || items[0].ID != "cl-1" {
					t.Fatalf("unexpected cash ledger payload: %s", string(body))
				}
			},
		},
		{
			name: "reviews",
			path: "/api/reviews",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.Review
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal reviews: %v", err)
				}
				if len(items) != 1 || items[0].ID != "rev-1" {
					t.Fatalf("unexpected reviews payload: %s", string(body))
				}
			},
		},
		{
			name: "notifications",
			path: "/api/notifications",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var items []models.NotificationLog
				if err := json.Unmarshal(body, &items); err != nil {
					t.Fatalf("unmarshal notifications: %v", err)
				}
				if len(items) != 1 || items[0].ID != "notif-1" {
					t.Fatalf("unexpected notifications payload: %s", string(body))
				}
			},
		},
		{
			name: "settings page data",
			path: "/api/settings-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload settingsPagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal settings page payload: %v", err)
				}
				if len(payload.ServiceItems) != 1 || len(payload.ExtraFeeProducts) != 1 || payload.Settings.ReminderDays != 45 {
					t.Fatalf("unexpected settings page payload: %s", string(body))
				}
			},
		},
		{
			name: "settings",
			path: "/api/settings",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var item settingsPayload
				if err := json.Unmarshal(body, &item); err != nil {
					t.Fatalf("unmarshal settings: %v", err)
				}
				if item.ReminderDays != 45 {
					t.Fatalf("unexpected settings payload: %s", string(body))
				}
			},
		},
		{
			name: "line page data",
			path: "/api/line-page-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload linePagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal line page payload: %v", err)
				}
				if len(payload.LineFriends) != 1 || len(payload.Customers) != 1 || payload.LineFriends[0].LineUID != "Urouter-1" {
					t.Fatalf("unexpected line page payload: %s", string(body))
				}
			},
		},
		{
			name: "technician page data alias",
			path: "/api/technician-page-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload technicianPagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal technician page payload: %v", err)
				}
				if len(payload.Technicians) != 1 || len(payload.Appointments) != 1 || len(payload.Reviews) != 1 || len(payload.Zones) != 1 {
					t.Fatalf("unexpected technician page payload: %s", string(body))
				}
			},
		},
		{
			name: "financial report page data alias",
			path: "/api/financial-report-page-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload financialReportPagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal financial report payload: %v", err)
				}
				if len(payload.Appointments) != 1 || len(payload.Technicians) != 1 {
					t.Fatalf("unexpected financial report payload: %s", string(body))
				}
			},
		},
		{
			name: "cash ledger page data alias",
			path: "/api/cash-ledger-page-data",
			assert: func(t *testing.T, body []byte) {
				t.Helper()
				var payload cashLedgerPagePayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("unmarshal cash ledger payload: %v", err)
				}
				if len(payload.Technicians) != 1 || len(payload.Appointments) != 1 || len(payload.CashLedgerEntries) != 1 {
					t.Fatalf("unexpected cash ledger payload: %s", string(body))
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			attachAuthCookie(t, db, request, 1)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d body=%s", tc.path, recorder.Code, recorder.Body.String())
			}
			tc.assert(t, recorder.Body.Bytes())
		})
	}
}

// TestNewRouterRejectsAnonymousSensitiveEndpoints 验证敏感接口在匿名访问时会被统一拦截。
func TestNewRouterRejectsAnonymousSensitiveEndpoints(t *testing.T) {
	t.Parallel()

	db := newRouterTestDB(t)
	router := NewRouter(config.Config{AppEnv: "development"}, db)

	testCases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{name: "bootstrap", method: http.MethodGet, path: "/api/bootstrap"},
		{name: "appointments", method: http.MethodGet, path: "/api/appointments"},
		{name: "settings", method: http.MethodGet, path: "/api/settings"},
		{name: "create appointment", method: http.MethodPost, path: "/api/appointments", body: []byte(`{}`)},
		{name: "replace technicians", method: http.MethodPut, path: "/api/technicians", body: []byte(`[]`)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var bodyReader *bytes.Reader
			if tc.body != nil {
				bodyReader = bytes.NewReader(tc.body)
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			if tc.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestNewRouterRejectsTechnicianAccessToAdminOnlyEndpoints 验证技师账号无法访问管理员专属写接口。
func TestNewRouterRejectsTechnicianAccessToAdminOnlyEndpoints(t *testing.T) {
	t.Parallel()

	db := newRouterTestDB(t)
	if err := db.Create(&models.User{ID: 2, Name: "王師傅", Role: "technician", Phone: "0911222333"}).Error; err != nil {
		t.Fatalf("seed technician: %v", err)
	}

	router := NewRouter(config.Config{AppEnv: "development"}, db)
	req := httptest.NewRequest(http.MethodPut, "/api/settings/reminder-days", bytes.NewReader([]byte(`{"reminder_days":30}`)))
	req.Header.Set("Content-Type", "application/json")
	attachAuthCookie(t, db, req, 2)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestNewRouterAllowsTechnicianToUpdateOwnAppointmentButRejectsOthers 验证技师只能更新分配给自己的工单。
func TestNewRouterAllowsTechnicianToUpdateOwnAppointmentButRejectsOthers(t *testing.T) {
	t.Parallel()

	db := newRouterTestDB(t)
	now := time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC)
	if err := db.Create(&models.User{ID: 2, Name: "王師傅", Role: "technician", Phone: "0911222333"}).Error; err != nil {
		t.Fatalf("seed technician 2: %v", err)
	}
	if err := db.Create(&models.User{ID: 3, Name: "李師傅", Role: "technician", Phone: "0911333444"}).Error; err != nil {
		t.Fatalf("seed technician 3: %v", err)
	}
	if err := db.Create(&models.Appointment{
		ID:             101,
		CustomerName:   "王小明",
		Address:        "台北市信義區市府路 1 號",
		Phone:          "0911000001",
		Items:          []byte(`[{"id":"svc-1","type":"分離式","note":"","price":1800}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1800,
		ScheduledAt:    now,
		Status:         "assigned",
		TechnicianID:   uintPtr(2),
		TechnicianName: stringPtr("王師傅"),
		Photos:         []byte(`[]`),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("seed own appointment: %v", err)
	}
	if err := db.Create(&models.Appointment{
		ID:             102,
		CustomerName:   "李小美",
		Address:        "台北市大安區忠孝東路 1 號",
		Phone:          "0911000002",
		Items:          []byte(`[{"id":"svc-1","type":"分離式","note":"","price":1800}]`),
		ExtraItems:     []byte(`[]`),
		PaymentMethod:  "現金",
		TotalAmount:    1800,
		ScheduledAt:    now,
		Status:         "assigned",
		TechnicianID:   uintPtr(3),
		TechnicianName: stringPtr("李師傅"),
		Photos:         []byte(`[]`),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("seed other appointment: %v", err)
	}

	router := NewRouter(config.Config{AppEnv: "development"}, db)
	validPayload := []byte(`{"customer_name":"王小明","address":"台北市信義區市府路 1 號","phone":"0911000001","items":[{"id":"svc-1","type":"分離式","note":"","price":1800}],"extra_items":[],"payment_method":"現金","discount_amount":0,"paid_amount":0,"scheduled_at":"2026-03-13T09:00:00Z","status":"assigned","technician_id":2,"photos":[],"payment_received":false}`)

	ownReq := httptest.NewRequest(http.MethodPatch, "/api/appointments/101", bytes.NewReader(validPayload))
	ownReq.Header.Set("Content-Type", "application/json")
	attachAuthCookie(t, db, ownReq, 2)
	ownRecorder := httptest.NewRecorder()
	router.ServeHTTP(ownRecorder, ownReq)
	if ownRecorder.Code != http.StatusOK {
		t.Fatalf("expected own appointment update success, got %d body=%s", ownRecorder.Code, ownRecorder.Body.String())
	}

	otherReq := httptest.NewRequest(http.MethodPatch, "/api/appointments/102", bytes.NewReader(validPayload))
	otherReq.Header.Set("Content-Type", "application/json")
	attachAuthCookie(t, db, otherReq, 2)
	otherRecorder := httptest.NewRecorder()
	router.ServeHTTP(otherRecorder, otherReq)
	if otherRecorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for other technician appointment, got %d body=%s", otherRecorder.Code, otherRecorder.Body.String())
	}
}
