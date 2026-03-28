package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"gorm.io/gorm/clause"
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

// paymentOrderExpireLayout 是 PAYUNi ATM 到期时间的固定字符串格式。
const paymentOrderExpireLayout = "2006-01-02 15:04:05"

// paymentOrderConfirmationTimeout 是“结果确认中”订单允许停留在 paying 的最长时间。
// 超过该窗口后，前端与管理端都需要看到可恢复的失败态，避免订单永久卡死。
const paymentOrderConfirmationTimeout = 15 * time.Minute

// 这些文案用于区分“网关确认中”与“已超时可重建”两种状态。
const (
	paymentOrderCreditPendingMessage = "支付結果確認中，請稍後重新整理頁面確認"
	paymentOrderATMPendingMessage    = "取號結果確認中，請稍後重新整理頁面確認"
	paymentOrderTimeoutMessage       = "支付結果確認逾時，請聯絡管理員重新建立支付連結"
	paymentOrderAlreadyPaidMessage   = "關聯預約已完成收款，此支付單已關閉"
)

// paymentOrderActionError 让处理器能把业务校验错误映射成明确的 HTTP 状态码。
// 这样创建支付单时既能保留事务封装，又不会把冲突一律误报成 500。
type paymentOrderActionError struct {
	status  int
	message string
}

func (e *paymentOrderActionError) Error() string {
	return e.message
}

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
	// AppointmentID 必填关联预约ID；保留服务端再次校验，防止旧客户端绕过前端约束。
	AppointmentID uint `json:"appointment_id"`
}

// CreatePaymentOrder 管理员创建支付订单，生成支付链接。
// 路由: POST /api/payment/orders（需要管理员登录）
func (h *Handler) CreatePaymentOrder(c *gin.Context) {
	// 检查 PAYUNi 配置
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未設定")
		return
	}

	var body createPaymentOrderRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "invalid payment order request") {
		return
	}
	if body.AppointmentID == 0 {
		respondMessage(c, http.StatusBadRequest, "支付訂單必須綁定預約")
		return
	}

	// 通过认证中间件获取当前管理员用户（中间件存储 key 为 "user"）
	user, err := currentUser(c)
	if err != nil {
		respondMessage(c, http.StatusUnauthorized, "未取得使用者資訊")
		return
	}

	// 生成支付令牌和商店订单编号
	paymentToken, err := generatePaymentToken()
	if err != nil {
		logger.Errorf("生成支付令牌失败: %v", err)
		respondMessage(c, http.StatusInternalServerError, "建立支付訂單失敗")
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
	appointmentID := body.AppointmentID
	order := models.PaymentOrder{
		PaymentToken:  paymentToken,
		MerTradeNo:    merTradeNo,
		TradeAmt:      body.TradeAmt,
		ProdDesc:      body.ProdDesc,
		PaymentMethod: paymentMethod,
		CustomerName:  body.CustomerName,
		CustomerEmail: body.CustomerEmail,
		CustomerPhone: body.CustomerPhone,
		AppointmentID: &appointmentID,
		CreatedByID:   user.ID,
		Status:        "pending",
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := h.validatePaymentOrderAppointmentWithTx(tx, appointmentID, body.TradeAmt); err != nil {
			return &paymentOrderActionError{status: http.StatusBadRequest, message: err.Error()}
		}
		blockingOrder, err := h.findBlockingPaymentOrderForAppointmentTx(tx, appointmentID)
		if err != nil {
			return err
		}
		if blockingOrder != nil {
			return &paymentOrderActionError{
				status:  http.StatusConflict,
				message: buildBlockingPaymentOrderMessage(*blockingOrder),
			}
		}
		return tx.Create(&order).Error
	}); err != nil {
		var actionErr *paymentOrderActionError
		if errors.As(err, &actionErr) {
			respondMessage(c, actionErr.status, actionErr.message)
			return
		}
		logger.Errorf("创建支付订单数据库写入失败: %v", err)
		respondMessage(c, http.StatusInternalServerError, "建立支付訂單失敗")
		return
	}

	logger.Infof("管理员 %d 创建支付订单: ID=%d, MerTradeNo=%s, 金额=%d",
		order.CreatedByID, order.ID, order.MerTradeNo, order.TradeAmt)

	respondData(c, http.StatusCreated, "success", gin.H{
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
		respondMessage(c, http.StatusInternalServerError, "查詢支付訂單失敗")
		return
	}
	for i := range orders {
		if err := h.refreshPaymentOrderStatus(&orders[i]); err != nil {
			logger.Warnf("刷新支付订单状态失败 (OrderID=%d): %v", orders[i].ID, err)
		}
	}
	respondData(c, http.StatusOK, "success", orders)
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
	respondData(c, http.StatusOK, "success", gin.H{
		"trade_amt":      order.TradeAmt,
		"prod_desc":      order.ProdDesc,
		"payment_method": order.PaymentMethod,
		"customer_name":  order.CustomerName,
		"status":         order.Status,
		"mer_trade_no":   order.MerTradeNo,
		"res_code_msg":   order.ResCodeMsg,
		"appointment_id": order.AppointmentID,
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
//  1. 验证 Token → 检查订单状态和支付方式
//  2. 组装请求 → PAYUNi 加密 → 发送
//  3. 解密返回 → 更新 PaymentOrder 记录 → 返回结果给客户
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
		if strings.TrimSpace(order.ResCodeMsg) != "" {
			respondMessage(c, http.StatusConflict, order.ResCodeMsg)
			return
		}
		if order.Status == "paying" {
			respondMessage(c, http.StatusConflict, "訂單處理中，請稍後重新整理確認結果")
			return
		}
		respondMessage(c, http.StatusConflict, "訂單狀態不允許支付: "+order.Status)
		return
	}
	if order.PaymentMethod != "credit" && order.PaymentMethod != "both" {
		respondMessage(c, http.StatusBadRequest, "此訂單不支援信用卡支付")
		return
	}
	// 先以原子状态迁移占住该订单，阻止双击、并发请求或浏览器重试把同一笔单再次送进 PAYUNi。
	claimed, latest, err := h.tryClaimPaymentOrder(order.ID)
	if err != nil {
		var actionErr *paymentOrderActionError
		if errors.As(err, &actionErr) {
			respondMessage(c, actionErr.status, actionErr.message)
			return
		}
		logger.Errorf("占用支付订单失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "支付訂單狀態更新失敗")
		return
	}
	if !claimed {
		if latest != nil && strings.TrimSpace(latest.ResCodeMsg) != "" {
			respondMessage(c, http.StatusConflict, latest.ResCodeMsg)
			return
		}
		if latest != nil && latest.Status == "paying" {
			respondMessage(c, http.StatusConflict, "訂單處理中，請稍後重新整理確認結果")
			return
		}
		if latest != nil {
			respondMessage(c, http.StatusConflict, "訂單狀態不允許支付: "+latest.Status)
			return
		}
		respondMessage(c, http.StatusConflict, "訂單狀態已變更，請重新整理後再試")
		return
	}
	order.Status = "paying"

	var body tokenCreditPayRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "请填写完整信用卡信息") {
		// 输入校验失败说明请求根本未发往第三方，安全回滚为 pending，保留用户可重试性。
		if revertErr := h.revertClaimedPaymentOrder(order.ID, ""); revertErr != nil {
			logger.Warnf("回滚支付订单占位失败 (OrderID=%d): %v", order.ID, revertErr)
		}
		return
	}

	// 分期数默认一次付清
	cardInst := strings.TrimSpace(body.CardInst)
	if cardInst == "" {
		cardInst = "1"
	}

	// 组装 PAYUNi 信用卡支付请求
	req := payuni.CreditPayRequest{
		MerTradeNo:  order.MerTradeNo,
		TradeAmt:    order.TradeAmt,
		CardNo:      body.CardNo,
		CardExpired: body.CardExpired,
		CardCVC:     body.CardCVC,
		CardInst:    cardInst,
		ProdDesc:    order.ProdDesc,
		UsrMail:     order.CustomerEmail,
	}

	// 发起信用卡支付
	resp, err := h.payuniClient.CreditPay(c.Request.Context(), req)
	if err != nil {
		logger.Errorf("PAYUNi 信用卡支付请求失败 (OrderID=%d): %v", order.ID, err)
		// 请求是否真正到达支付网关无法由网络层错误可靠判断，
		// 这里保持 paying，避免客户立刻重试导致潜在重复扣款。
		if updateErr := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "paying",
			"res_code_msg": paymentOrderCreditPendingMessage,
		}, ""); updateErr != nil {
			logger.Warnf("写入信用卡处理中状态失败 (OrderID=%d): %v", order.ID, updateErr)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  "UNKNOWN",
			"message": paymentOrderCreditPendingMessage,
		})
		return
	}

	// 如果外层 Status 不是 SUCCESS 也不是 UNKNOWN
	if resp.Status != "SUCCESS" && resp.Status != "UNKNOWN" {
		// 更新订单状态为失败
		if err := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "failed",
			"res_code_msg": resp.Status,
		}, ""); err != nil {
			logger.Warnf("写入信用卡失败状态失败 (OrderID=%d): %v", order.ID, err)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  resp.Status,
			"message": "支付請求失敗: " + resp.Status,
		})
		return
	}

	// 解密返回数据并验签
	detail, err := h.payuniClient.DecryptResponse(resp.EncryptInfo, resp.HashInfo)
	if err != nil {
		logger.Errorf("PAYUNi 信用卡返回解密失败 (OrderID=%d): %v", order.ID, err)
		if updateErr := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "paying",
			"res_code_msg": paymentOrderCreditPendingMessage,
		}, ""); updateErr != nil {
			logger.Warnf("写入信用卡解密待确认状态失败 (OrderID=%d): %v", order.ID, updateErr)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  "UNKNOWN",
			"message": paymentOrderCreditPendingMessage,
		})
		return
	}

	// 将解密后的完整返回存入 RawResponse（用于对帐）
	rawJSON, _ := json.Marshal(mapFromValues(detail))
	creditDetail := payuni.ParseCreditPayDetail(detail)

	// 根据交易状态更新 PaymentOrder
	updates := map[string]any{
		"trade_no":     creditDetail.TradeNo,
		"trade_status": creditDetail.TradeStatus,
		"card6_no":     creditDetail.Card6No,
		"card4_no":     creditDetail.Card4No,
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

	if err := h.updatePaymentOrderState(order.ID, updates, "轉帳"); err != nil {
		logger.Errorf("写入信用卡支付结果失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "支付結果寫入失敗")
		return
	}

	logger.Infof("信用卡支付完成 OrderID=%d, TradeStatus=%s, TradeNo=%s",
		order.ID, creditDetail.TradeStatus, creditDetail.TradeNo)

	respondData(c, http.StatusOK, "success", gin.H{
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
//  1. 验证 Token → 检查订单状态和支付方式
//  2. PAYUNi 取号 → 返回虚拟帐号
//  3. 更新 PaymentOrder（存入 PayNo）→ 返回帐号信息给客户展示
//  4. 等待客户去 ATM 转账 → PAYUNi 异步通知 → 更新订单状态
func (h *Handler) HandleTokenATMPay(c *gin.Context) {
	if h.payuniClient == nil {
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未設定")
		return
	}

	// 查找并验证订单
	order, err := h.findPaymentOrderByToken(c)
	if err != nil {
		return
	}

	// ATM 已取号的订单允许再次查看帐号信息
	if order.Status != "pending" {
		if strings.TrimSpace(order.ResCodeMsg) != "" && !(order.Status == "paying" && order.PayNo != "") {
			respondMessage(c, http.StatusConflict, order.ResCodeMsg)
			return
		}
		if order.Status == "paying" && order.PayNo != "" {
			// 已取号，返回现有帐号信息
			respondData(c, http.StatusOK, "success", gin.H{
				"status":          "SUCCESS",
				"message":         "虛擬帳號已產生",
				"pay_no":          order.PayNo,
				"trade_amt":       fmt.Sprintf("%d", order.TradeAmt),
				"atm_expire_date": order.ATMExpireDate,
			})
			return
		}
		if order.Status == "paying" {
			respondMessage(c, http.StatusConflict, "訂單處理中，請稍後重新整理確認結果")
			return
		}
		respondMessage(c, http.StatusConflict, "訂單狀態不允許取號: "+order.Status)
		return
	}
	if order.PaymentMethod != "atm" && order.PaymentMethod != "both" {
		respondMessage(c, http.StatusBadRequest, "此訂單不支援 ATM 轉帳")
		return
	}
	// 与信用卡一致，先原子占位，避免同一链接被并发重复取号。
	claimed, latest, err := h.tryClaimPaymentOrder(order.ID)
	if err != nil {
		var actionErr *paymentOrderActionError
		if errors.As(err, &actionErr) {
			respondMessage(c, actionErr.status, actionErr.message)
			return
		}
		logger.Errorf("占用 ATM 支付订单失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "支付訂單狀態更新失敗")
		return
	}
	if !claimed {
		if latest != nil && strings.TrimSpace(latest.ResCodeMsg) != "" && !(latest.Status == "paying" && latest.PayNo != "") {
			respondMessage(c, http.StatusConflict, latest.ResCodeMsg)
			return
		}
		if latest != nil && latest.Status == "paying" && latest.PayNo != "" {
			respondData(c, http.StatusOK, "success", gin.H{
				"status":          "SUCCESS",
				"message":         "虛擬帳號已產生",
				"pay_no":          latest.PayNo,
				"trade_amt":       fmt.Sprintf("%d", latest.TradeAmt),
				"atm_expire_date": latest.ATMExpireDate,
			})
			return
		}
		if latest != nil && latest.Status == "paying" {
			respondMessage(c, http.StatusConflict, "訂單處理中，請稍後重新整理確認結果")
			return
		}
		if latest != nil {
			respondMessage(c, http.StatusConflict, "訂單狀態不允許取號: "+latest.Status)
			return
		}
		respondMessage(c, http.StatusConflict, "訂單狀態已變更，請重新整理後再試")
		return
	}
	order.Status = "paying"

	var body tokenATMPayRequest
	if err := c.ShouldBindJSON(&body); handleBindJSONError(c, err, "请选择转账银行") {
		if revertErr := h.revertClaimedPaymentOrder(order.ID, ""); revertErr != nil {
			logger.Warnf("回滚 ATM 支付订单占位失败 (OrderID=%d): %v", order.ID, revertErr)
		}
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
		if updateErr := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "paying",
			"res_code_msg": paymentOrderATMPendingMessage,
		}, ""); updateErr != nil {
			logger.Warnf("写入 ATM 处理中状态失败 (OrderID=%d): %v", order.ID, updateErr)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  "UNKNOWN",
			"message": paymentOrderATMPendingMessage,
		})
		return
	}

	if resp.Status != "SUCCESS" {
		if err := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "failed",
			"res_code_msg": resp.Status,
		}, ""); err != nil {
			logger.Warnf("写入 ATM 失败状态失败 (OrderID=%d): %v", order.ID, err)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  resp.Status,
			"message": "ATM 取號失敗: " + resp.Status,
		})
		return
	}

	// 解密返回数据
	detail, err := h.payuniClient.DecryptResponse(resp.EncryptInfo, resp.HashInfo)
	if err != nil {
		logger.Errorf("PAYUNi ATM 返回解密失败 (OrderID=%d): %v", order.ID, err)
		if updateErr := h.updatePaymentOrderState(order.ID, map[string]any{
			"status":       "paying",
			"res_code_msg": paymentOrderATMPendingMessage,
		}, ""); updateErr != nil {
			logger.Warnf("写入 ATM 解密待确认状态失败 (OrderID=%d): %v", order.ID, updateErr)
		}
		respondData(c, http.StatusOK, "success", gin.H{
			"status":  "UNKNOWN",
			"message": paymentOrderATMPendingMessage,
		})
		return
	}

	rawJSON, _ := json.Marshal(mapFromValues(detail))
	atmDetail := payuni.ParseATMPayDetail(detail)

	// 更新订单：ATM 取号成功后状态变为 paying（等待客户缴费）
	if err := h.updatePaymentOrderState(order.ID, map[string]any{
		"status":          "paying",
		"trade_no":        atmDetail.TradeNo,
		"trade_status":    atmDetail.TradeStatus,
		"pay_no":          atmDetail.PayNo,
		"atm_expire_date": atmDetail.ExpireDate,
		"raw_response":    rawJSON,
	}, ""); err != nil {
		logger.Errorf("写入 ATM 取号结果失败 (OrderID=%d): %v", order.ID, err)
		respondMessage(c, http.StatusInternalServerError, "ATM 結果寫入失敗")
		return
	}

	logger.Infof("ATM 取号成功 OrderID=%d, PayNo=%s, ExpireDate=%s",
		order.ID, atmDetail.PayNo, atmDetail.ExpireDate)

	respondData(c, http.StatusOK, "success", gin.H{
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
		respondMessage(c, http.StatusServiceUnavailable, "PAYUNi 支付未設定")
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
		updates["card6_no"] = creditDetail.Card6No
		updates["card4_no"] = creditDetail.Card4No
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

	if err := h.updatePaymentOrderState(order.ID, updates, "轉帳"); err != nil {
		logger.Errorf("写入 PAYUNi 异步通知结果失败 (OrderID=%d): %v", order.ID, err)
	}

	// PAYUNi 要求回应 "SUCCESS" 表示已收到通知
	c.String(http.StatusOK, "SUCCESS")
}

// ==================== 内部辅助函数 ====================

// validatePaymentOrderAppointment 校验绑定预约是否适合走当前这条外部支付链路。
func (h *Handler) validatePaymentOrderAppointment(appointmentID uint, tradeAmt int) error {
	return h.validatePaymentOrderAppointmentWithTx(h.db, appointmentID, tradeAmt)
}

// validatePaymentOrderAppointmentWithTx 在事务内校验预约是否适合创建支付单。
// 创建阶段会锁住预约行，确保“校验通过”和“写入支付单”之间不会被并发请求插入第二张活跃单。
func (h *Handler) validatePaymentOrderAppointmentWithTx(tx *gorm.DB, appointmentID uint, tradeAmt int) error {
	var appointment models.Appointment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&appointment, "id = ?", appointmentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("關聯預約不存在")
		}
		return fmt.Errorf("查詢關聯預約失敗")
	}
	if appointment.PaymentReceived {
		return fmt.Errorf("關聯預約已確認收款")
	}
	if normalizePaymentMethod(appointment.PaymentMethod) == "無收款" {
		return fmt.Errorf("無收款預約不可建立支付訂單")
	}
	// 当前预约主表只支持“未收/已确认收款”两态，绑定支付订单时必须保持全额收款闭环，
	// 避免把外部支付引入成系统尚未支持的半收款状态。
	if appointment.TotalAmount != tradeAmt {
		return fmt.Errorf("支付金額必須與預約應收金額一致")
	}
	if !isOneOf(appointment.Status, "completed", "cancelled") {
		return fmt.Errorf("僅已完成或已取消的預約可綁定支付訂單")
	}
	return nil
}

// findBlockingPaymentOrderForAppointmentTx 查找同预约下仍会阻塞新建单的活跃支付单。
// 在返回前会先把可由本地规则收敛的过期/超时订单就地转成终态，避免把脏状态也算成活跃单。
func (h *Handler) findBlockingPaymentOrderForAppointmentTx(tx *gorm.DB, appointmentID uint) (*models.PaymentOrder, error) {
	var orders []models.PaymentOrder
	if err := tx.
		Where("appointment_id = ? AND status IN ?", appointmentID, []string{"pending", "paying", "expired"}).
		Order("created_at desc").
		Find(&orders).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	for i := range orders {
		if err := h.syncPaymentOrderDerivedState(tx, &orders[i], now); err != nil {
			return nil, err
		}
		if isActivePaymentOrderStatus(orders[i].Status) {
			return &orders[i], nil
		}
	}
	return nil, nil
}

// buildBlockingPaymentOrderMessage 根据阻塞订单状态返回更准确的冲突提示，便于管理员判断是“待发给客户”还是“客户已在支付”。
func buildBlockingPaymentOrderMessage(order models.PaymentOrder) string {
	switch order.Status {
	case "paying":
		return "該預約已有處理中的支付單，請勿重複建立"
	default:
		return "該預約已有待支付訂單，請勿重複建立"
	}
}

// isActivePaymentOrderStatus 表示该状态仍会阻塞同预约继续创建新支付单。
func isActivePaymentOrderStatus(status string) bool {
	return status == "pending" || status == "paying"
}

// findBlockingPayingOrderForAppointmentTx 查找同预约下已进入 paying 的其他支付单。
// 只阻塞 sibling paying，不阻塞 sibling pending，这样历史重复链接里仍只有第一张能进入支付流程。
func (h *Handler) findBlockingPayingOrderForAppointmentTx(tx *gorm.DB, appointmentID uint, excludeOrderID uint) (*models.PaymentOrder, error) {
	var orders []models.PaymentOrder
	if err := tx.
		Where("appointment_id = ? AND id <> ? AND status = ?", appointmentID, excludeOrderID, "paying").
		Order("updated_at desc").
		Find(&orders).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	for i := range orders {
		if err := h.syncPaymentOrderDerivedState(tx, &orders[i], now); err != nil {
			return nil, err
		}
		if orders[i].Status == "paying" {
			return &orders[i], nil
		}
	}
	return nil, nil
}

// tryClaimPaymentOrder 原子地把订单从 pending 标记为 paying，作为支付请求的占位锁。
func (h *Handler) tryClaimPaymentOrder(orderID uint) (bool, *models.PaymentOrder, error) {
	var latest models.PaymentOrder
	claimed := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&latest, "id = ?", orderID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if err := h.syncPaymentOrderDerivedState(tx, &latest, time.Now()); err != nil {
			return err
		}
		if err := h.syncPaymentOrderAppointmentState(tx, &latest); err != nil {
			return err
		}
		// 历史脏数据里可能仍有未绑定预约的支付单，这类订单必须在支付入口即时关闭，
		// 否则旧链接仍能绕过“支付单必须绑定预约”的新规则继续发起第三方扣款。
		if latest.AppointmentID == nil {
			result := tx.Model(&models.PaymentOrder{}).
				Where("id = ? AND status IN ?", latest.ID, []string{"pending", "paying"}).
				Updates(map[string]any{
					"status":       "cancelled",
					"res_code_msg": "支付訂單必須綁定預約，目前連結已失效",
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				latest.Status = "cancelled"
				latest.ResCodeMsg = "支付訂單必須綁定預約，目前連結已失效"
			} else if err := tx.First(&latest, "id = ?", latest.ID).Error; err != nil {
				return err
			}
			return &paymentOrderActionError{
				status:  http.StatusConflict,
				message: "支付訂單必須綁定預約，目前連結已失效",
			}
		}
		if latest.Status != "pending" {
			if latest.Status == "cancelled" && strings.TrimSpace(latest.ResCodeMsg) != "" {
				return &paymentOrderActionError{
					status:  http.StatusConflict,
					message: latest.ResCodeMsg,
				}
			}
			return nil
		}
		// 在同一预约上只允许一张支付单进入 paying。
		// 创建阶段已经阻止新重复单，但这里仍要兜住历史脏数据与并发点击两条旧链接的场景。
		blockingOrder, err := h.findBlockingPayingOrderForAppointmentTx(tx, *latest.AppointmentID, latest.ID)
		if err != nil {
			return err
		}
		if blockingOrder != nil {
			return &paymentOrderActionError{
				status:  http.StatusConflict,
				message: "該預約已有其他支付單處理中，請勿重複付款",
			}
		}

		result := tx.Model(&models.PaymentOrder{}).
			Where("id = ? AND status = ?", latest.ID, "pending").
			Updates(map[string]any{
				"status":       "paying",
				"res_code":     "",
				"res_code_msg": "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			claimed = true
			latest.Status = "paying"
			latest.ResCode = ""
			latest.ResCodeMsg = ""
			return nil
		}
		return tx.First(&latest, "id = ?", latest.ID).Error
	})
	if err != nil {
		return false, nil, err
	}
	if claimed {
		return true, nil, nil
	}
	if latest.ID == 0 {
		return false, nil, nil
	}
	return false, &latest, nil
}

// revertClaimedPaymentOrder 只在第三方请求尚未发出前回滚本地占位，保留用户可立即重试能力。
func (h *Handler) revertClaimedPaymentOrder(orderID uint, message string) error {
	updates := map[string]any{
		"status":       "pending",
		"res_code":     "",
		"res_code_msg": message,
	}
	return h.db.Model(&models.PaymentOrder{}).
		Where("id = ? AND status = ?", orderID, "paying").
		Updates(updates).Error
}

// refreshPaymentOrderStatus 在订单被读取时同步收敛可由本地数据明确判断出的失效状态。
func (h *Handler) refreshPaymentOrderStatus(order *models.PaymentOrder) error {
	if order == nil {
		return nil
	}
	now := time.Now()
	if err := h.syncPaymentOrderDerivedState(h.db, order, now); err != nil {
		return err
	}
	return h.syncPaymentOrderAppointmentState(h.db, order)
}

// syncPaymentOrderDerivedState 根据本地可确定的规则同步订单状态。
// 这里负责 ATM 过期与“确认中超时”两类无需依赖外部回调的状态收敛。
func (h *Handler) syncPaymentOrderDerivedState(db *gorm.DB, order *models.PaymentOrder, now time.Time) error {
	if order == nil {
		return nil
	}
	nextStatus, nextMessage, shouldUpdateMessage := derivePaymentOrderState(*order, now)
	if nextStatus == order.Status && (!shouldUpdateMessage || nextMessage == order.ResCodeMsg) {
		return nil
	}

	updates := map[string]any{}
	if nextStatus != order.Status {
		updates["status"] = nextStatus
	}
	if shouldUpdateMessage && nextMessage != order.ResCodeMsg {
		updates["res_code_msg"] = nextMessage
	}
	if len(updates) == 0 {
		return nil
	}

	result := db.Model(&models.PaymentOrder{}).
		Where("id = ? AND status = ?", order.ID, order.Status).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		order.Status = nextStatus
		if shouldUpdateMessage {
			order.ResCodeMsg = nextMessage
		}
		return nil
	}

	var latest models.PaymentOrder
	if err := db.First(&latest, "id = ?", order.ID).Error; err != nil {
		return err
	}
	*order = latest
	return nil
}

// derivePaymentOrderState 基于本地已知字段推导订单当前应展示的状态和附带提示。
func derivePaymentOrderState(order models.PaymentOrder, now time.Time) (string, string, bool) {
	if order.Status == "paid" {
		return order.Status, "", false
	}
	if order.Status == "paying" && isPaymentOrderAwaitingConfirmation(order) &&
		now.After(order.UpdatedAt.Add(paymentOrderConfirmationTimeout)) {
		return "failed", paymentOrderTimeoutMessage, true
	}
	if !isOneOf(order.Status, "pending", "paying", "expired") {
		return order.Status, "", false
	}
	if order.PayNo == "" || strings.TrimSpace(order.ATMExpireDate) == "" {
		return order.Status, "", false
	}

	expireAt, err := parseATMPaymentExpireTime(order.ATMExpireDate)
	if err != nil {
		return order.Status, "", false
	}
	if now.After(expireAt) {
		return "expired", "", false
	}
	return order.Status, "", false
}

// isPaymentOrderAwaitingConfirmation 识别“请求已发出但结果未知”的暂挂订单。
// 这类订单短时间内必须禁止再次发起支付，超过超时时间后再自动降级为 failed 以恢复可操作性。
func isPaymentOrderAwaitingConfirmation(order models.PaymentOrder) bool {
	if order.Status != "paying" {
		return false
	}
	if strings.TrimSpace(order.PayNo) != "" || order.PaidAt != nil {
		return false
	}
	message := strings.TrimSpace(order.ResCodeMsg)
	return message == paymentOrderCreditPendingMessage || message == paymentOrderATMPendingMessage
}

// syncPaymentOrderAppointmentState 确保支付单与预约主表的“已收款”状态不会背离。
// 一旦预约已被其他路径确认收款，所有未成功的支付单都应即时关闭，阻止重复扣款。
func (h *Handler) syncPaymentOrderAppointmentState(db *gorm.DB, order *models.PaymentOrder) error {
	if order == nil {
		return nil
	}
	if !isActivePaymentOrderStatus(order.Status) {
		return nil
	}
	if order.AppointmentID == nil {
		result := db.Model(&models.PaymentOrder{}).
			Where("id = ? AND status IN ?", order.ID, []string{"pending", "paying"}).
			Updates(map[string]any{
				"status":       "cancelled",
				"res_code_msg": "支付訂單必須綁定預約，目前連結已失效",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			order.Status = "cancelled"
			order.ResCodeMsg = "支付訂單必須綁定預約，目前連結已失效"
			return nil
		}

		var latest models.PaymentOrder
		if err := db.First(&latest, "id = ?", order.ID).Error; err != nil {
			return err
		}
		*order = latest
		return nil
	}

	var appointment models.Appointment
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&appointment, "id = ?", *order.AppointmentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if !appointment.PaymentReceived {
		return nil
	}

	result := db.Model(&models.PaymentOrder{}).
		Where("id = ? AND status IN ?", order.ID, []string{"pending", "paying"}).
		Updates(map[string]any{
			"status":       "cancelled",
			"res_code_msg": paymentOrderAlreadyPaidMessage,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		order.Status = "cancelled"
		order.ResCodeMsg = paymentOrderAlreadyPaidMessage
		return nil
	}

	var latest models.PaymentOrder
	if err := db.First(&latest, "id = ?", order.ID).Error; err != nil {
		return err
	}
	*order = latest
	return nil
}

// parseATMPaymentExpireTime 解析 PAYUNi 返回的 ATM 缴费截止时间。
func parseATMPaymentExpireTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty atm expire date")
	}
	// PAYUNi 返回的是台湾本地时间且不带时区；系统按东八区解释即可。
	return time.ParseInLocation(paymentOrderExpireLayout, trimmed, time.FixedZone("UTC+8", 8*60*60))
}

// updatePaymentOrderState 统一处理支付订单状态更新，并在支付成功时把预约主表一并收敛。
func (h *Handler) updatePaymentOrderState(orderID uint, updates map[string]any, appointmentPaymentMethod string) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		var order models.PaymentOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ?", orderID).Error; err != nil {
			return err
		}

		nextStatus, _ := updates["status"].(string)
		shouldPromoteStatus := nextStatus == "" || shouldPromotePaymentOrderStatus(order.Status, nextStatus)
		if nextStatus != "" && !shouldPromoteStatus {
			return nil
		}
		merged := make(map[string]any, len(updates))
		for key, value := range updates {
			merged[key] = value
		}
		if len(merged) > 0 {
			if err := tx.Model(&order).Updates(merged).Error; err != nil {
				return err
			}
		}

		if nextStatus == "paid" && shouldPromoteStatus && order.AppointmentID != nil {
			paidAt, _ := updates["paid_at"].(time.Time)
			if paidAt.IsZero() {
				paidAt = time.Now().UTC()
			}
			if err := h.markAppointmentPaidByPaymentOrder(tx, *order.AppointmentID, order.TradeAmt, appointmentPaymentMethod, paidAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// shouldPromotePaymentOrderStatus 用最小状态机保护终态，避免旧回调把已付款订单覆盖回失败/过期。
func shouldPromotePaymentOrderStatus(current, next string) bool {
	if next == "" {
		return false
	}
	currentRank := paymentOrderStatusRank(current)
	nextRank := paymentOrderStatusRank(next)
	if nextRank > currentRank {
		return true
	}
	if nextRank == currentRank && current == next {
		return true
	}
	return false
}

// paymentOrderStatusRank 定义支付订单状态的单向推进优先级。
func paymentOrderStatusRank(status string) int {
	switch status {
	case "pending":
		return 0
	case "paying":
		return 1
	case "failed", "cancelled", "expired":
		return 2
	case "paid":
		return 3
	default:
		return 0
	}
}

// markAppointmentPaidByPaymentOrder 把外部支付结果收敛回预约主表。
func (h *Handler) markAppointmentPaidByPaymentOrder(tx *gorm.DB, appointmentID uint, paidAmount int, paymentMethod string, paidAt time.Time) error {
	var appointment models.Appointment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&appointment, "id = ?", appointmentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			logger.Warnf("支付订单关联预约不存在，跳过预约收款回写 (AppointmentID=%d)", appointmentID)
			return nil
		}
		return err
	}

	// 预约主表当前只区分现金 / 转账 / 無收款；外部支付渠道全部落到“轉帳”，
	// 更细的信用卡 / ATM 明细继续留在 PaymentOrder 上追踪。
	appointment.PaymentMethod = normalizePaymentMethod(paymentMethod)
	appointment.PaidAmount = paidAmount
	appointment.PaymentReceived = true
	utcPaidAt := paidAt.UTC()
	appointment.PaymentTime = &utcPaidAt

	return tx.Model(&appointment).Updates(map[string]any{
		"payment_method":   appointment.PaymentMethod,
		"paid_amount":      appointment.PaidAmount,
		"payment_received": appointment.PaymentReceived,
		"payment_time":     appointment.PaymentTime,
	}).Error
}

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
			respondMessage(c, http.StatusNotFound, "支付訂單不存在或連結無效")
		} else {
			respondMessage(c, http.StatusInternalServerError, "查詢支付訂單失敗")
		}
		return nil, err
	}
	if err := h.refreshPaymentOrderStatus(&order); err != nil {
		respondMessage(c, http.StatusInternalServerError, "重新整理支付訂單狀態失敗")
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
