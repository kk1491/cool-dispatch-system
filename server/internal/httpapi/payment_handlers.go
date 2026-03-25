package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cool-dispatch/internal/logger"
	"cool-dispatch/internal/models"
	"cool-dispatch/internal/payuni"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ============================================================================
// PAYUNi 支付相关 HTTP 处理函数
//
// 整体流程：
//   管理员创建支付订单 → 生成 PaymentToken → 拼成 URL 发给客户
//   → 客户凭 Token 无需登录查看订单信息 → 填写卡号/选择ATM → 完成支付
//   → PAYUNi 异步通知回调 → 更新 PaymentOrder 状态
//
// 安全设计：
//   - Token 使用 32 字节 crypto/rand + URL-safe base64，不可遍历
//   - 信用卡支付成功后 Token 状态变 paid，不可重复支付
//   - 信用卡号不落库，仅存 Card6No/Card4No 脱敏信息
//   - 异步通知验签后通过 MerTradeNo 关联回 PaymentOrder
// ============================================================================

// paymentTokenByteLength 是支付令牌的随机字节长度（32字节 → base64约43字符）。
const paymentTokenByteLength = 32

// generatePaymentToken 生成随机支付令牌（URL-safe base64 编码）。
func generatePaymentToken() (string, error) {
	b := make([]byte, paymentTokenByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成支付令牌失败: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// generateMerTradeNo 生成 PAYUNi 商店订单编号。
// 格式：P + 时间戳(秒) + "_" + 4位随机数 = 总长 ≤ 25 字符。
func generateMerTradeNo() string {
	b := make([]byte, 2)
	rand.Read(b)
	randomPart := int(b[0])<<8 | int(b[1])
	return fmt.Sprintf("P%d_%04d", time.Now().Unix(), randomPart%10000)
}

// ==================== 管理员：创建支付订单 ====================

// createPaymentOrderRequest 管理员创建支付订单的请求体。
type createPaymentOrderRequest struct {
	// TradeAmt 订单金额（必填，正整数）
	TradeAmt int `json:"trade_amt" binding:"required,gt=0"`
	// ProdDesc 商品说明（必填）
	ProdDesc string `json:"prod_desc" binding:"required"`
	// PaymentMethod 允许的支付方式：credit / atm / both（默认 both）
	PaymentMethod string `json:"payment_method"`
	// CustomerName 消费者名称（必填）
	CustomerName string `json:"customer_name" binding:"required"`
	// CustomerEmail 消费者信箱（选填）
	CustomerEmail string `json:"customer_email"`
	// CustomerPhone 消费者电话（选填）
	CustomerPhone string `json:"customer_phone"`
	// AppointmentID 可选关联预约ID
	AppointmentID *uint `json:"appointment_id"`
}

// CreatePaymentOrder 管理员创建支付订单，生成支付链接。
// 路由: POST /api/payment/orders（需要管理员登录）
func (h *Handler) CreatePaymentOrder(c *gin.Context) {
	// 检查 PAYUNi 配置
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未配置")
		return
	}

	var body createPaymentOrderRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "invalid payment order request") {
		return
	}

	// 通过认证中间件获取当前管理员用户（中间件存储 key 为 "user"）
	user, err := currentUser(c)
	if err != nil {
		respondMessage(c, http.StatusUnauthorized, "未获取到用户信息")
		return
	}

	// 生成支付令牌和商店订单编号
	paymentToken, err := generatePaymentToken()
	if err != nil {
		logger.Errorf("生成支付令牌失败: %v", err)
		respondMessage(c, http.StatusInternalServerError, "创建支付订单失败")
		return
	}

	merTradeNo := generateMerTradeNo()

	// 规范化支付方式默认值
	paymentMethod := strings.TrimSpace(body.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = "both"
	}
	if paymentMethod != "credit" && paymentMethod != "atm" && paymentMethod != "both" {
		respondMessage(c, http.StatusBadRequest, "payment_method 仅允许 credit / atm / both")
		return
	}

	// 创建支付订单
	order := models.PaymentOrder{
		PaymentToken:  paymentToken,
		MerTradeNo:    merTradeNo,
		TradeAmt:      body.TradeAmt,
		ProdDesc:      body.ProdDesc,
		PaymentMethod: paymentMethod,
		CustomerName:  body.CustomerName,
		CustomerEmail: body.CustomerEmail,
		CustomerPhone: body.CustomerPhone,
		AppointmentID: body.AppointmentID,
		CreatedByID:   user.ID,
		Status:        "pending",
	}

	if err := h.db.Create(&order).Error; err != nil {
		logger.Errorf("创建支付订单数据库写入失败: %v", err)
		respondMessage(c, http.StatusInternalServerError, "创建支付订单失败")
		return
	}

	logger.Infof("管理员 %d 创建支付订单: ID=%d, MerTradeNo=%s, 金额=%d",
		order.CreatedByID, order.ID, order.MerTradeNo, order.TradeAmt)

	c.JSON(http.StatusCreated, gin.H{
		"order":         order,
		"payment_token": paymentToken,
		"payment_url":   "/pay/" + paymentToken,
	})
}

// ==================== 管理员：查看支付订单列表 ====================

// ListPaymentOrders 查看所有支付订单记录。
// 路由: GET /api/payment/orders（需要管理员登录）
func (h *Handler) ListPaymentOrders(c *gin.Context) {
	var orders []models.PaymentOrder
	if err := h.db.Order("created_at desc").Find(&orders).Error; err != nil {
		respondMessage(c, http.StatusInternalServerError, "查询支付订单失败")
		return
	}
	c.JSON(http.StatusOK, orders)
}

// ==================== 客户公开接口：凭 Token 查看订单 ====================

// GetPaymentOrderByToken 客户凭支付令牌查看订单信息（无需登录）。
// 路由: GET /api/payment/token/:payToken
//
// 返回订单基本信息（金额、商品说明、允许的支付方式、当前状态），
// 不返回敏感的管理信息。
func (h *Handler) GetPaymentOrderByToken(c *gin.Context) {
	order, err := h.findPaymentOrderByToken(c)
	if err != nil {
		return // findPaymentOrderByToken 已写入错误响应
	}

	// 仅返回客户可见的信息
	c.JSON(http.StatusOK, gin.H{
		"trade_amt":      order.TradeAmt,
		"prod_desc":      order.ProdDesc,
		"payment_method": order.PaymentMethod,
		"customer_name":  order.CustomerName,
		"status":         order.Status,
		"mer_trade_no":   order.MerTradeNo,
		// ATM 已取号时返回虚拟帐号信息
		"pay_no":          order.PayNo,
		"atm_expire_date": order.ATMExpireDate,
	})
}

// ==================== 客户公开接口：凭 Token 信用卡支付 ====================

// tokenCreditPayRequest 客户凭 Token 发起信用卡支付的请求体。
type tokenCreditPayRequest struct {
	// CardNo 信用卡号码
	CardNo string `json:"card_no" binding:"required"`
	// CardExpired 有效日期（MMYY）
	CardExpired string `json:"card_expired" binding:"required"`
	// CardCVC 安全码
	CardCVC string `json:"card_cvc" binding:"required"`
	// CardInst 分期数：1=一次付清（默认），3/6/9/12/18/24/30
	CardInst string `json:"card_inst"`
}

// HandleTokenCreditPay 客户凭支付令牌发起信用卡支付（无需登录）。
// 路由: POST /api/payment/token/:payToken/credit
//
// 交易流程：
//   1. 验证 Token → 检查订单状态和支付方式
//   2. 组装请求 → PAYUNi 加密 → 发送
//   3. 解密返回 → 更新 PaymentOrder 记录 → 返回结果给客户
func (h *Handler) HandleTokenCreditPay(c *gin.Context) {
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未配置")
		return
	}

	// 查找并验证订单
	order, err := h.findPaymentOrderByToken(c)
	if err != nil {
		return
	}

	// 检查订单是否可以进行信用卡支付
	if order.Status != "pending" {
		respondMessage(c, http.StatusConflict, "订单状态不允许支付: "+order.Status)
		return
	}
	if order.PaymentMethod != "credit" && order.PaymentMethod != "both" {
		respondMessage(c, http.StatusBadRequest, "此订单不支持信用卡支付")
		return
	}

	var body tokenCreditPayRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "请填写完整信用卡信息") {
		return
	}

	// 分期数默认一次付清
	cardInst := strings.TrimSpace(body.CardInst)
	if cardInst == "" {
		cardInst = "1"
	}

	// 组装 PAYUNi 信用卡支付请求
	req := payuni.CreditPayRequest{
		MerTradeNo: order.MerTradeNo,
		TradeAmt:   order.TradeAmt,
		CardNo:     body.CardNo,
		CardExpired: body.CardExpired,
		CardCVC:    body.CardCVC,
		CardInst:   cardInst,
		ProdDesc:   order.ProdDesc,
		UsrMail:    order.CustomerEmail,
	}

	// 发起信用卡支付
	resp, err := h.payuniClient.CreditPay(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("PAYUNi 信用卡支付请求失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusBadGateway, "信用卡支付请求失败")
		return
	}

	// 如果外层 Status 不是 SUCCESS 也不是 UNKNOWN
	if resp.Status != "SUCCESS" && resp.Status != "UNKNOWN" {
		// 更新订单状态为失败
		h.db.Model(&order).Updates(map[string]any{
			"status":      "failed",
			"res_code_msg": resp.Status,
		})
		c.JSON(http.StatusOK, gin.H{
			"status":  resp.Status,
			"message": "支付请求失败: " + resp.Status,
		})
		return
	}

	// 解密返回数据并验签
	detail, err := h.payuniClient.DecryptResponse(resp.EncryptInfo, resp.HashInfo)
	if err != nil {
		logger.Errorf("PAYUNi 信用卡返回解密失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "支付返回数据解密失败")
		return
	}

	// 将解密后的完整返回存入 RawResponse（用于对帐）
	rawJSON, _ := json.Marshal(mapFromValues(detail))
	creditDetail := payuni.ParseCreditPayDetail(detail)

	// 根据交易状态更新 PaymentOrder
	updates := map[string]any{
		"trade_no":     creditDetail.TradeNo,
		"trade_status": creditDetail.TradeStatus,
		"card_6_no":    creditDetail.Card6No,
		"card_4_no":    creditDetail.Card4No,
		"auth_code":    creditDetail.AuthCode,
		"res_code":     creditDetail.ResCode,
		"res_code_msg": creditDetail.ResCodeMsg,
		"raw_response": rawJSON,
	}

	switch creditDetail.TradeStatus {
	case "1": // 已付款
		now := time.Now()
		updates["status"] = "paid"
		updates["paid_at"] = now
	case "2": // 付款失败
		updates["status"] = "failed"
	case "3": // 付款取消
		updates["status"] = "cancelled"
	default: // 待确认（UNKNOWN）
		updates["status"] = "paying"
	}

	h.db.Model(&order).Updates(updates)

	logger.Infof("信用卡支付完成 OrderID=%d, TradeStatus=%s, TradeNo=%s",
		order.ID, creditDetail.TradeStatus, creditDetail.TradeNo)

	c.JSON(http.StatusOK, gin.H{
		"status":         creditDetail.Status,
		"message":        creditDetail.Message,
		"trade_status":   creditDetail.TradeStatus,
		"trade_no":       creditDetail.TradeNo,
		"auth_code":      creditDetail.AuthCode,
		"card_6_no":      creditDetail.Card6No,
		"card_4_no":      creditDetail.Card4No,
		"res_code":       creditDetail.ResCode,
		"res_code_msg":   creditDetail.ResCodeMsg,
		"auth_bank_name": creditDetail.AuthBankName,
	})
}

// ==================== 客户公开接口：凭 Token ATM 取号 ====================

// tokenATMPayRequest 客户凭 Token 发起 ATM 取号的请求体。
type tokenATMPayRequest struct {
	// BankType 银行代码（必填）
	BankType string `json:"bank_type" binding:"required"`
}

// HandleTokenATMPay 客户凭支付令牌发起 ATM 虚拟帐号取号（无需登录）。
// 路由: POST /api/payment/token/:payToken/atm
//
// 交易流程：
//   1. 验证 Token → 检查订单状态和支付方式
//   2. PAYUNi 取号 → 返回虚拟帐号
//   3. 更新 PaymentOrder（存入 PayNo）→ 返回帐号信息给客户展示
//   4. 等待客户去 ATM 转账 → PAYUNi 异步通知 → 更新订单状态
func (h *Handler) HandleTokenATMPay(c *gin.Context) {
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未配置")
		return
	}

	// 查找并验证订单
	order, err := h.findPaymentOrderByToken(c)
	if err != nil {
		return
	}

	// ATM 已取号的订单允许再次查看帐号信息
	if order.Status != "pending" {
		if order.Status == "paying" && order.PayNo != "" {
			// 已取号，返回现有帐号信息
			c.JSON(http.StatusOK, gin.H{
				"status":          "SUCCESS",
				"message":         "虚拟帐号已生成",
				"pay_no":          order.PayNo,
				"trade_amt":       fmt.Sprintf("%d", order.TradeAmt),
				"atm_expire_date": order.ATMExpireDate,
			})
			return
		}
		respondMessage(c, http.StatusConflict, "订单状态不允许取号: "+order.Status)
		return
	}
	if order.PaymentMethod != "atm" && order.PaymentMethod != "both" {
		respondMessage(c, http.StatusBadRequest, "此订单不支持 ATM 转账")
		return
	}

	var body tokenATMPayRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "请选择转账银行") {
		return
	}

	// 组装 PAYUNi ATM 取号请求
	req := payuni.ATMPayRequest{
		MerTradeNo: order.MerTradeNo,
		TradeAmt:   order.TradeAmt,
		BankType:   body.BankType,
		ProdDesc:   order.ProdDesc,
		UsrMail:    order.CustomerEmail,
	}

	// 发起 ATM 取号
	resp, err := h.payuniClient.ATMPay(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("PAYUNi ATM 取号请求失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusBadGateway, "ATM 取号请求失败")
		return
	}

	if resp.Status != "SUCCESS" {
		h.db.Model(&order).Updates(map[string]any{
			"status":      "failed",
			"res_code_msg": resp.Status,
		})
		c.JSON(http.StatusOK, gin.H{
			"status":  resp.Status,
			"message": "ATM 取号失败: " + resp.Status,
		})
		return
	}

	// 解密返回数据
	detail, err := h.payuniClient.DecryptResponse(resp.EncryptInfo, resp.HashInfo)
	if err != nil {
		logger.Errorf("PAYUNi ATM 返回解密失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "ATM 返回数据解密失败")
		return
	}

	rawJSON, _ := json.Marshal(mapFromValues(detail))
	atmDetail := payuni.ParseATMPayDetail(detail)

	// 更新订单：ATM 取号成功后状态变为 paying（等待客户缴费）
	h.db.Model(&order).Updates(map[string]any{
		"status":          "paying",
		"trade_no":        atmDetail.TradeNo,
		"trade_status":    atmDetail.TradeStatus,
		"pay_no":          atmDetail.PayNo,
		"atm_expire_date": atmDetail.ExpireDate,
		"raw_response":    rawJSON,
	})

	logger.Infof("ATM 取号成功 OrderID=%d, PayNo=%s, ExpireDate=%s",
		order.ID, atmDetail.PayNo, atmDetail.ExpireDate)

	c.JSON(http.StatusOK, gin.H{
		"status":          atmDetail.Status,
		"message":         atmDetail.Message,
		"trade_no":        atmDetail.TradeNo,
		"trade_amt":       atmDetail.TradeAmt,
		"pay_no":          atmDetail.PayNo,
		"atm_expire_date": atmDetail.ExpireDate,
	})
}

// ==================== PAYUNi 异步通知回调（信用卡 + ATM 共用） ====================

// HandlePayuniNotify 处理 PAYUNi 异步通知回调（公开接口，无需认证）。
// 路由: POST /api/webhook/payuni
//
// 功能：
//   - 验证 HashInfo 防篡改
//   - 通过 MerTradeNo 关联到 PaymentOrder
//   - 根据 PaymentType（1=信用卡 / 2=ATM）和 TradeStatus 更新订单状态
func (h *Handler) HandlePayuniNotify(c *gin.Context) {
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未配置")
		return
	}

	// 解析 POST form 数据
	if err := c.Request.ParseForm(); err != nil {
		respondMessage(c, http.StatusBadRequest, "invalid notify data")
		return
	}

	encryptInfo := c.PostForm("EncryptInfo")
	hashInfo := c.PostForm("HashInfo")

	if encryptInfo == "" || hashInfo == "" {
		respondMessage(c, http.StatusBadRequest, "missing EncryptInfo or HashInfo")
		return
	}

	// 解密并验签
	detail, err := h.payuniClient.DecryptResponse(encryptInfo, hashInfo)
	if err != nil {
		logger.Errorf("PAYUNi 异步通知解密/验签失败: %v", err)
		respondMessage(c, http.StatusBadRequest, "notify decrypt failed")
		return
	}

	paymentType := detail.Get("PaymentType")
	merTradeNo := detail.Get("MerTradeNo")
	tradeStatus := detail.Get("TradeStatus")

	logger.Infof("PAYUNi 异步通知: PaymentType=%s, MerTradeNo=%s, TradeStatus=%s",
		paymentType, merTradeNo, tradeStatus)

	// 通过 MerTradeNo 查找对应的 PaymentOrder
	var order models.PaymentOrder
	if err := h.db.First(&order, "mer_trade_no = ?", merTradeNo).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Warnf("PAYUNi 异步通知: 找不到订单 MerTradeNo=%s", merTradeNo)
		} else {
			logger.Errorf("PAYUNi 异步通知: 查询订单失败: %v", err)
		}
		// 即使找不到订单也回应 SUCCESS，避免 PAYUNi 重复推送
		c.String(http.StatusOK, "SUCCESS")
		return
	}

	// 将完整回调数据存入 RawResponse
	rawJSON, _ := json.Marshal(mapFromValues(detail))
	updates := map[string]any{
		"trade_status": tradeStatus,
		"raw_response": rawJSON,
	}

	switch paymentType {
	case "1": // 信用卡
		creditDetail := payuni.ParseCreditPayDetail(detail)
		updates["trade_no"] = creditDetail.TradeNo
		updates["auth_code"] = creditDetail.AuthCode
		updates["card_6_no"] = creditDetail.Card6No
		updates["card_4_no"] = creditDetail.Card4No
		updates["res_code"] = creditDetail.ResCode
		updates["res_code_msg"] = creditDetail.ResCodeMsg

		switch tradeStatus {
		case "1":
			now := time.Now()
			updates["status"] = "paid"
			updates["paid_at"] = now
		case "2":
			updates["status"] = "failed"
		case "3":
			updates["status"] = "cancelled"
		}

		logger.Infof("信用卡异步通知更新 OrderID=%d, TradeStatus=%s", order.ID, tradeStatus)

	case "2": // ATM
		atmDetail := payuni.ParseATMPayDetail(detail)
		updates["trade_no"] = atmDetail.TradeNo

		if tradeStatus == "1" {
			now := time.Now()
			updates["status"] = "paid"
			updates["paid_at"] = now
		}

		logger.Infof("ATM 缴费通知更新 OrderID=%d, TradeStatus=%s", order.ID, tradeStatus)

	default:
		logger.Warnf("PAYUNi 异步通知: 未知 PaymentType=%s", paymentType)
	}

	h.db.Model(&order).Updates(updates)

	// PAYUNi 要求回应 "SUCCESS" 表示已收到通知
	c.String(http.StatusOK, "SUCCESS")
}

// ==================== 内部辅助函数 ====================

// findPaymentOrderByToken 根据 URL 路径中的 payToken 查找支付订单。
// 找不到或 Token 无效时直接写入错误响应并返回 error。
func (h *Handler) findPaymentOrderByToken(c *gin.Context) (*models.PaymentOrder, error) {
	payToken := strings.TrimSpace(c.Param("payToken"))
	if payToken == "" {
		respondMessage(c, http.StatusBadRequest, "缺少支付令牌")
		return nil, fmt.Errorf("missing payToken")
	}

	var order models.PaymentOrder
	if err := h.db.First(&order, "payment_token = ?", payToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			respondMessage(c, http.StatusNotFound, "支付订单不存在或链接无效")
		} else {
			respondMessage(c, http.StatusInternalServerError, "查询支付订单失败")
		}
		return nil, err
	}

	return &order, nil
}

// mapFromValues 将 url.Values 转为 map[string]string 便于 JSON 序列化。
func mapFromValues(v url.Values) map[string]string {
	result := make(map[string]string, len(v))
	for key := range v {
		result[key] = v.Get(key)
	}
	return result
}
