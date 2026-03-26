package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cool-dispatch/internal/models"
	"cool-dispatch/internal/payuni"

	"github.com/gin-gonic/gin"
)

const (
	// 测试专用 PAYUNi 资料使用固定长度的假密钥，便于构造合法的 EncryptInfo / HashInfo。
	testPayuniMerID   = "MER123456"
	testPayuniHashKey = "12345678901234567890123456789012"
	testPayuniHashIV  = "1234567890123456"
)

// paymentOrderTokenResponse 对应公开 token 查询接口返回体，便于断言订单状态是否已收敛。
type paymentOrderTokenResponse struct {
	Message       string `json:"message"`
	Status        string `json:"status"`
	PayNo         string `json:"pay_no"`
	ATMExpireDate string `json:"atm_expire_date"`
	ResCodeMsg    string `json:"res_code_msg"`
}

// tokenATMPayResponse 对应公开 ATM 取号返回体，便于断言重试时是否直接复用旧 pay_no。
type tokenATMPayResponse struct {
	Status        string `json:"status"`
	PayNo         string `json:"pay_no"`
	ATMExpireDate string `json:"atm_expire_date"`
}

// messageResponse 对应只返回 message 的错误响应，便于断言业务提示是否命中预期。
type messageResponse struct {
	Message string `json:"message"`
}

// newTestPayuniClient 构造测试专用 PAYUNi 客户端。
func newTestPayuniClient(baseURL string) *payuni.Client {
	return &payuni.Client{
		BaseURL: baseURL,
		MerID:   testPayuniMerID,
		HashKey: testPayuniHashKey,
		HashIV:  testPayuniHashIV,
		HTTP:    &http.Client{Timeout: 2 * time.Second},
	}
}

// encodePayuniOuterForm 根据解密明细生成 PAYUNi 外层 form body，便于直接喂给 webhook 或假网关。
func encodePayuniOuterForm(t *testing.T, detail url.Values) string {
	t.Helper()

	encryptInfo, err := payuni.Encrypt(detail.Encode(), testPayuniHashKey, testPayuniHashIV)
	if err != nil {
		t.Fatalf("encrypt payuni detail: %v", err)
	}

	values := url.Values{}
	values.Set("Status", detail.Get("Status"))
	values.Set("MerID", testPayuniMerID)
	values.Set("Version", "1.3")
	values.Set("EncryptInfo", encryptInfo)
	values.Set("HashInfo", payuni.GetHash(encryptInfo, testPayuniHashKey, testPayuniHashIV))
	return values.Encode()
}

// decodeJSONBody 解码测试响应体，避免每个用例重复写 json.Unmarshal 错误处理。
func decodeJSONBody(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response body: %v, body=%s", err, recorder.Body.String())
	}
}

// TestHandlePayuniNotifyPaidCreditBackfillsAppointment 验证支付成功通知会同步回写预约收款主表。
func TestHandlePayuniNotifyPaidCreditBackfillsAppointment(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	handler.payuniClient = newTestPayuniClient("")

	appointment := models.Appointment{
		ID:              41,
		CustomerName:    "王小明",
		Address:         "台北市信義區市府路 1 號",
		Phone:           "0911000001",
		PaymentMethod:   "轉帳",
		TotalAmount:     1800,
		PaidAmount:      0,
		PaymentReceived: false,
		Status:          "completed",
		ScheduledAt:     time.Date(2026, 3, 13, 9, 0, 0, 0, time.UTC),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	order := models.PaymentOrder{
		ID:            91,
		PaymentToken:  "pay-token-credit-paid",
		MerTradeNo:    "P1742900000_0001",
		TradeAmt:      1800,
		ProdDesc:      "冷氣清洗服務費",
		PaymentMethod: "credit",
		CustomerName:  "王小明",
		AppointmentID: &appointment.ID,
		CreatedByID:   1,
		Status:        "paying",
	}
	if err := handler.db.Create(&order).Error; err != nil {
		t.Fatalf("seed payment order: %v", err)
	}

	detail := url.Values{}
	detail.Set("Status", "SUCCESS")
	detail.Set("Message", "付款成功")
	detail.Set("MerID", testPayuniMerID)
	detail.Set("MerTradeNo", order.MerTradeNo)
	detail.Set("TradeNo", "UNI202603250001")
	detail.Set("TradeAmt", "1800")
	detail.Set("TradeStatus", "1")
	detail.Set("PaymentType", "1")
	detail.Set("AuthCode", "654321")
	detail.Set("Card6No", "414763")
	detail.Set("Card4No", "0001")
	detail.Set("ResCode", "00")
	detail.Set("ResCodeMsg", "APPROVED")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/webhook/payuni", strings.NewReader(encodePayuniOuterForm(t, detail)))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	handler.HandlePayuniNotify(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "SUCCESS" {
		t.Fatalf("expected SUCCESS ack, got %q", recorder.Body.String())
	}

	var savedOrder models.PaymentOrder
	if err := handler.db.First(&savedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("reload payment order: %v", err)
	}
	if savedOrder.Status != "paid" {
		t.Fatalf("expected payment order status paid, got %s", savedOrder.Status)
	}
	if savedOrder.PaidAt == nil {
		t.Fatalf("expected paid_at to be populated")
	}

	var savedAppointment models.Appointment
	if err := handler.db.First(&savedAppointment, "id = ?", appointment.ID).Error; err != nil {
		t.Fatalf("reload appointment: %v", err)
	}
	if !savedAppointment.PaymentReceived {
		t.Fatalf("expected appointment payment_received=true after payuni notify")
	}
	if savedAppointment.PaidAmount != order.TradeAmt {
		t.Fatalf("expected appointment paid_amount=%d, got %d", order.TradeAmt, savedAppointment.PaidAmount)
	}
	if savedAppointment.PaymentTime == nil {
		t.Fatalf("expected appointment payment_time to be auto populated")
	}
}

// TestGetPaymentOrderByTokenMarksPastATMOrderExpired 验证公开 token 查询会把过期 ATM 订单收敛成 expired。
func TestGetPaymentOrderByTokenMarksPastATMOrderExpired(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	expiredAt := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	order := models.PaymentOrder{
		ID:            92,
		PaymentToken:  "pay-token-expired-atm",
		MerTradeNo:    "P1742900000_0002",
		TradeAmt:      2200,
		ProdDesc:      "冷氣保養服務費",
		PaymentMethod: "atm",
		CustomerName:  "陳小姐",
		CreatedByID:   1,
		Status:        "paying",
		PayNo:         "98765432101234",
		ATMExpireDate: expiredAt,
		CreatedAt:     time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:     time.Now().UTC().Add(-24 * time.Hour),
	}
	if err := handler.db.Create(&order).Error; err != nil {
		t.Fatalf("seed expired payment order: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "payToken", Value: order.PaymentToken}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/payment/token/"+order.PaymentToken, nil)

	handler.GetPaymentOrderByToken(ctx)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusGone {
		t.Fatalf("expected 200 or 410 for expired order, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Code == http.StatusOK {
		var payload paymentOrderTokenResponse
		decodeJSONBody(t, recorder, &payload)
		if payload.Status != "expired" {
			t.Fatalf("expected expired token query status, got %s", payload.Status)
		}
	}
	if recorder.Code == http.StatusGone && !strings.Contains(recorder.Body.String(), "過期") && !strings.Contains(recorder.Body.String(), "失效") {
		t.Fatalf("expected expired message, got %s", recorder.Body.String())
	}

	var savedOrder models.PaymentOrder
	if err := handler.db.First(&savedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("reload payment order: %v", err)
	}
	if savedOrder.Status != "expired" {
		t.Fatalf("expected payment order status expired after query, got %s", savedOrder.Status)
	}
}

// TestHandleTokenATMPayReusesExistingPayNoWithoutCallingGatewayAgain 验证 ATM 已取号后重复进入不会再次请求 PAYUNi。
func TestHandleTokenATMPayReusesExistingPayNoWithoutCallingGatewayAgain(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	var payuniHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&payuniHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer server.Close()

	handler.payuniClient = newTestPayuniClient(server.URL)

	appointment := models.Appointment{
		ID:              44,
		CustomerName:    "林先生",
		Address:         "台北市松山區南京東路 5 段",
		Phone:           "0911000004",
		PaymentMethod:   "轉帳",
		TotalAmount:     1500,
		PaidAmount:      0,
		PaymentReceived: false,
		Status:          "completed",
		ScheduledAt:     time.Date(2026, 3, 17, 11, 0, 0, 0, time.UTC),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	order := models.PaymentOrder{
		ID:            93,
		PaymentToken:  "pay-token-reuse-payno",
		MerTradeNo:    "P1742900000_0003",
		TradeAmt:      1500,
		ProdDesc:      "冷氣深層清潔費",
		PaymentMethod: "atm",
		CustomerName:  "林先生",
		AppointmentID: &appointment.ID,
		CreatedByID:   1,
		Status:        "paying",
		PayNo:         "11002200330044",
		ATMExpireDate: time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	if err := handler.db.Create(&order).Error; err != nil {
		t.Fatalf("seed atm order: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "payToken", Value: order.PaymentToken}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/token/"+order.PaymentToken+"/atm", strings.NewReader(`{"bank_type":"822"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.HandleTokenATMPay(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload tokenATMPayResponse
	decodeJSONBody(t, recorder, &payload)
	if payload.PayNo != order.PayNo {
		t.Fatalf("expected existing pay_no %s, got %s", order.PayNo, payload.PayNo)
	}
	if got := atomic.LoadInt32(&payuniHits); got != 0 {
		t.Fatalf("expected no outbound payuni request when pay_no already exists, got %d", got)
	}
}

// TestCreatePaymentOrderRequiresAppointmentBinding 验证后端会强制创建支付单必须绑定预约。
// 这样即使旧客户端未升级，也无法再绕过前端直接创建孤儿支付单。
func TestCreatePaymentOrderRequiresAppointmentBinding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	handler.payuniClient = newTestPayuniClient("")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/orders", strings.NewReader(`{
		"trade_amt": 1800,
		"prod_desc": "冷氣清洗服務費",
		"payment_method": "both",
		"customer_name": "王小明"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.CreatePaymentOrder(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload messageResponse
	decodeJSONBody(t, recorder, &payload)
	if !strings.Contains(payload.Message, "綁定預約") {
		t.Fatalf("expected binding appointment error, got %q", payload.Message)
	}

	var count int64
	if err := handler.db.Model(&models.PaymentOrder{}).Count(&count).Error; err != nil {
		t.Fatalf("count payment orders: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no payment order created, got %d", count)
	}
}

// TestCreatePaymentOrderRejectsDuplicateActiveAppointmentOrder 验证同一预约存在活跃支付单时不能重复建单。
// 这样管理员重复点击或多端同时操作时，不会为同一个预约发出多条有效支付链接。
func TestCreatePaymentOrderRejectsDuplicateActiveAppointmentOrder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	handler.payuniClient = newTestPayuniClient("")

	appointment := models.Appointment{
		ID:              42,
		CustomerName:    "王小明",
		Address:         "台北市信義區市府路 2 號",
		Phone:           "0911000002",
		PaymentMethod:   "轉帳",
		TotalAmount:     1800,
		PaidAmount:      0,
		PaymentReceived: false,
		Status:          "completed",
		ScheduledAt:     time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	if err := handler.db.Create(&models.PaymentOrder{
		ID:            94,
		PaymentToken:  "pay-token-duplicate-pending",
		MerTradeNo:    "P1742900000_0004",
		TradeAmt:      appointment.TotalAmount,
		ProdDesc:      "冷氣清洗服務費",
		PaymentMethod: "both",
		CustomerName:  appointment.CustomerName,
		AppointmentID: &appointment.ID,
		CreatedByID:   1,
		Status:        "pending",
	}).Error; err != nil {
		t.Fatalf("seed active payment order: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/orders", strings.NewReader(`{
		"trade_amt": 1800,
		"prod_desc": "冷氣清洗服務費",
		"payment_method": "both",
		"customer_name": "王小明",
		"appointment_id": 42
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAuthenticatedUser(ctx, &models.User{ID: 1, Role: "admin"})

	handler.CreatePaymentOrder(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload messageResponse
	decodeJSONBody(t, recorder, &payload)
	if !strings.Contains(payload.Message, "已有待支付") && !strings.Contains(payload.Message, "已有處理中") {
		t.Fatalf("expected duplicate active order message, got %q", payload.Message)
	}

	var count int64
	if err := handler.db.Model(&models.PaymentOrder{}).Where("appointment_id = ?", appointment.ID).Count(&count).Error; err != nil {
		t.Fatalf("count payment orders by appointment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected still only 1 payment order for appointment, got %d", count)
	}
}

// TestHandleTokenCreditPayCancelsOrderWhenAppointmentAlreadyPaid 验证支付入口会在预约已收款时立刻关闭旧支付单。
// 这样即使客户持有旧链接，也不会再把已收款预约送进外部支付网关。
func TestHandleTokenCreditPayCancelsOrderWhenAppointmentAlreadyPaid(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)
	handler.payuniClient = newTestPayuniClient("")

	paidAt := time.Now().UTC().Add(-30 * time.Minute)
	appointment := models.Appointment{
		ID:              43,
		CustomerName:    "陳小姐",
		Address:         "台北市中正區忠孝西路 1 段",
		Phone:           "0911000003",
		PaymentMethod:   "轉帳",
		TotalAmount:     2200,
		PaidAmount:      2200,
		PaymentReceived: true,
		PaymentTime:     &paidAt,
		Status:          "completed",
		ScheduledAt:     time.Date(2026, 3, 16, 9, 0, 0, 0, time.UTC),
	}
	if err := handler.db.Create(&appointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	order := models.PaymentOrder{
		ID:            95,
		PaymentToken:  "pay-token-paid-appointment",
		MerTradeNo:    "P1742900000_0005",
		TradeAmt:      appointment.TotalAmount,
		ProdDesc:      "冷氣保養服務費",
		PaymentMethod: "credit",
		CustomerName:  appointment.CustomerName,
		AppointmentID: &appointment.ID,
		CreatedByID:   1,
		Status:        "pending",
	}
	if err := handler.db.Create(&order).Error; err != nil {
		t.Fatalf("seed payment order: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "payToken", Value: order.PaymentToken}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/token/"+order.PaymentToken+"/credit", strings.NewReader(`{
		"card_no": "4147630000000001",
		"card_expired": "1228",
		"card_cvc": "123"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.HandleTokenCreditPay(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload messageResponse
	decodeJSONBody(t, recorder, &payload)
	if payload.Message != paymentOrderAlreadyPaidMessage {
		t.Fatalf("expected already-paid close message, got %q", payload.Message)
	}

	var savedOrder models.PaymentOrder
	if err := handler.db.First(&savedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("reload payment order: %v", err)
	}
	if savedOrder.Status != "cancelled" {
		t.Fatalf("expected cancelled order, got %s", savedOrder.Status)
	}
	if savedOrder.ResCodeMsg != paymentOrderAlreadyPaidMessage {
		t.Fatalf("expected already-paid close message, got %q", savedOrder.ResCodeMsg)
	}
}

// TestGetPaymentOrderByTokenMarksStaleConfirmingOrderFailed 验证“确认中”订单超过窗口后会自动收敛成 failed。
// 这样网关异常或回包丢失不会让支付单无限期停在 paying，管理员可以重新建链。
func TestGetPaymentOrderByTokenMarksStaleConfirmingOrderFailed(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := newTestHandler(t)

	order := models.PaymentOrder{
		ID:            96,
		PaymentToken:  "pay-token-stale-confirming",
		MerTradeNo:    "P1742900000_0006",
		TradeAmt:      2600,
		ProdDesc:      "冷氣深度清潔費",
		PaymentMethod: "credit",
		CustomerName:  "林先生",
		CreatedByID:   1,
		Status:        "paying",
		ResCodeMsg:    paymentOrderCreditPendingMessage,
		CreatedAt:     time.Now().UTC().Add(-2 * time.Hour),
		UpdatedAt:     time.Now().UTC().Add(-paymentOrderConfirmationTimeout - time.Minute),
	}
	if err := handler.db.Create(&order).Error; err != nil {
		t.Fatalf("seed stale confirming order: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "payToken", Value: order.PaymentToken}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/payment/token/"+order.PaymentToken, nil)

	handler.GetPaymentOrderByToken(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload paymentOrderTokenResponse
	decodeJSONBody(t, recorder, &payload)
	if payload.Status != "failed" {
		t.Fatalf("expected failed token query status, got %s", payload.Status)
	}
	if payload.ResCodeMsg != paymentOrderTimeoutMessage {
		t.Fatalf("expected timeout message, got %q", payload.ResCodeMsg)
	}

	var savedOrder models.PaymentOrder
	if err := handler.db.First(&savedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatalf("reload payment order: %v", err)
	}
	if savedOrder.Status != "failed" {
		t.Fatalf("expected payment order status failed after query, got %s", savedOrder.Status)
	}
	if savedOrder.ResCodeMsg != paymentOrderTimeoutMessage {
		t.Fatalf("expected persisted timeout message, got %q", savedOrder.ResCodeMsg)
	}
}
