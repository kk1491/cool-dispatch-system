package payuni

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// PAYUNi 支付 API 客户端
// 封装信用卡幕后支付 + ATM 虚拟帐号转账两种方式，共用同一套加解密和 HTTP 发送逻辑。
// ============================================================================

// apiVersion 是 PAYUNi API 固定版本号。
const apiVersion = "1.3"

// Client 封装 PAYUNi API 调用（信用卡 + ATM 共用）。
type Client struct {
	// BaseURL 是 PAYUNi API 基础 URL（不含 API 路径），例如 https://sandbox-api.payuni.com.tw
	BaseURL string
	// MerID 是 PAYUNi 平台分配的商店代号
	MerID string
	// HashKey 是 AES-GCM 加密密钥（32 字节）
	HashKey string
	// HashIV 是 AES-GCM 加密向量（16 字节）
	HashIV string
	// NotifyURL 是交易结果异步通知回调网址（选填）
	NotifyURL string
	// HTTP 是自定义 HTTP 客户端（为空时使用默认客户端，超时 30 秒）
	HTTP *http.Client
}

// httpClient 返回可用的 HTTP 客户端，未设置时回退到默认超时客户端。
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// ==================== 信用卡支付 ====================

// CreditPayRequest 信用卡幕后支付请求参数。
type CreditPayRequest struct {
	// MerTradeNo 商店订单编号（限 25 字内，格式 [A-Za-z0-9_-]，10 分钟内不可重复）
	MerTradeNo string
	// TradeAmt 订单金额
	TradeAmt int
	// CardNo 信用卡号码（支援 Visa/MasterCard/JCB/银联）
	CardNo string
	// CardExpired 信用卡有效日期（格式: MMYY）
	CardExpired string
	// CardCVC 信用卡安全码（银联 Debit 卡无安全码可免填）
	CardCVC string
	// CardInst 信用卡分期数：1=一次付清（默认），3/6/9/12/18/24/30=分期数
	CardInst string
	// ProdDesc 商品说明（长度限制 550，可用半角分号 ; 带入多个叙述）
	ProdDesc string
	// UsrMail 消费者信箱
	UsrMail string
	// CreditToken 信用卡 Token（绑卡用，长度限制 150，格式 [A-Za-z0-9@.#$%_-]）
	CreditToken string
	// CreditTokenType 信用卡 Token 记录类型：1=会员（默认），2=商店
	CreditTokenType int
	// CreditTokenExpired 信用卡 Token 有效期间（格式: MMYY，默认以信用卡到期日为主）
	CreditTokenExpired string
	// CreditHash 信用卡 Hash（有值时 CardNo/CardExpired/CardCVC 为非必填）
	CreditHash string
	// API3D 幕后强制 3D 验证：1=强制 3D
	API3D int
	// ReturnURL 3D 验证完成后返回网址（仅 API3D=1 时使用）
	ReturnURL string
	// UserIP 消费者 IP（若有带入则列入全平台风险管控，支持 IPv4 和 IPv6）
	UserIP string
	// Cardholder 持卡人英文名称（启用 3D 交易时需输入，供发卡行验证）
	Cardholder int
}

// CreditPayResponse 信用卡支付返回结果（外层）。
type CreditPayResponse struct {
	// Status 外层状态代码：SUCCESS / UNKNOWN / UNAPPROVED / 错误代码
	Status string
	// MerID 商店代号
	MerID string
	// Version 版本
	Version string
	// EncryptInfo 加密字串（需解密取得交易明细）
	EncryptInfo string
	// HashInfo 加密 Hash（用于验签）
	HashInfo string
}

// CreditPayDetail 信用卡支付解密后的交易明细。
type CreditPayDetail struct {
	// Status 状态代码
	Status string
	// Message 状态说明
	Message string
	// MerID 商店代号
	MerID string
	// MerTradeNo 商店订单编号
	MerTradeNo string
	// Gateway 交易标记（1=幕后）
	Gateway string
	// TradeNo UNi 序号
	TradeNo string
	// TradeAmt 订单金额
	TradeAmt string
	// TradeStatus 订单状态：1=已付款 2=付款失败 3=付款取消 8=订单待确认
	TradeStatus string
	// PaymentType 支付工具：1=信用卡
	PaymentType string
	// CardBank 发卡银行代码
	CardBank string
	// Card6No 卡号前六码
	Card6No string
	// Card4No 卡号后四码
	Card4No string
	// CardInst 分期数
	CardInst string
	// FirstAmt 首期金额
	FirstAmt string
	// EachAmt 每期金额
	EachAmt string
	// ResCode 回应码
	ResCode string
	// ResCodeMsg 回应码叙述
	ResCodeMsg string
	// AuthCode 授权码
	AuthCode string
	// AuthBank 授权银行代码
	AuthBank string
	// AuthBankName 授权银行名称
	AuthBankName string
	// AuthType 授权类型：1=一次 2=分期 7=银联
	AuthType string
	// AuthDay 授权日期（格式: YYYYMMDD）
	AuthDay string
	// AuthTime 授权时间（格式: HHIISS）
	AuthTime string
	// CreditHash 信用卡 Token Hash（有 CreditToken 且授权成功才会压码）
	CreditHash string
	// CreditLife 信用卡 Token 有效日期（格式: MMYY）
	CreditLife string
	// CoBrandCode 联名卡代号
	CoBrandCode string
}

// Credit3DResponse 信用卡 3D 验证返回结果（API3D=1 时返回此结构）。
type Credit3DResponse struct {
	// Status 状态代码：SUCCESS=建立幕后 3D 成功
	Status string
	// Message 状态说明
	Message string
	// URL 强制 3D 导页网址（前端需导向此 URL 进行 3D 验证）
	URL string
}

// CreditPay 发起信用卡幕后支付。
//
// 信用卡交易流程：
//   - 非 3D：付款人 → 商店（本系统） → PAYUNi → 收单银行 → PAYUNi → 商店 → 付款人
//   - 3D：  付款人 → 商店 → PAYUNi → 返回 3D URL → 付款人 3D 验证 → 银行 → PAYUNi → 商店
//
// 返回说明：
//   - 非 3D 成功时，返回的 EncryptInfo 需解密获取 CreditPayDetail
//   - API3D=1 时，返回的 EncryptInfo 需解密获取 Credit3DResponse（含 3D 导页 URL）
//   - UNKNOWN 时需等待 NotifyURL 或 15 分钟后查询
func (c *Client) CreditPay(ctx context.Context, req CreditPayRequest) (*CreditPayResponse, error) {
	// 组装 EncryptInfo 内的请求参数
	params := url.Values{}
	params.Set("MerID", c.MerID)
	params.Set("MerTradeNo", req.MerTradeNo)
	params.Set("TradeAmt", strconv.Itoa(req.TradeAmt))
	params.Set("Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	params.Set("ProdDesc", req.ProdDesc)

	// 信用卡核心参数
	if req.CardNo != "" {
		params.Set("CardNo", req.CardNo)
	}
	if req.CardExpired != "" {
		params.Set("CardExpired", req.CardExpired)
	}
	if req.CardCVC != "" {
		params.Set("CardCVC", req.CardCVC)
	}
	if req.CardInst != "" {
		params.Set("CardInst", req.CardInst)
	}
	if req.UsrMail != "" {
		params.Set("UsrMail", req.UsrMail)
	}

	// 信用卡 Token 相关（选填）
	if req.CreditToken != "" {
		params.Set("CreditToken", req.CreditToken)
	}
	if req.CreditTokenType > 0 {
		params.Set("CreditTokenType", strconv.Itoa(req.CreditTokenType))
	}
	if req.CreditTokenExpired != "" {
		params.Set("CreditTokenExpired", req.CreditTokenExpired)
	}
	if req.CreditHash != "" {
		params.Set("CreditHash", req.CreditHash)
	}

	// 3D 验证相关（选填）
	if req.API3D == 1 {
		params.Set("API3D", "1")
	}
	if req.ReturnURL != "" {
		params.Set("ReturnURL", req.ReturnURL)
	}
	if req.Cardholder == 1 {
		params.Set("Cardholder", "1")
	}

	// 风控参数（选填）
	if req.UserIP != "" {
		params.Set("UserIP", req.UserIP)
	}

	// 通知网址（选填）
	if c.NotifyURL != "" {
		params.Set("NotifyURL", c.NotifyURL)
	}

	// 发送请求到 PAYUNi
	respBody, err := c.doPost(ctx, "/api/credit", params.Encode())
	if err != nil {
		return nil, fmt.Errorf("payuni: 信用卡支付请求失败: %w", err)
	}

	// 解析外层返回参数
	respValues, err := url.ParseQuery(respBody)
	if err != nil {
		return nil, fmt.Errorf("payuni: 解析信用卡返回参数失败: %w", err)
	}

	return &CreditPayResponse{
		Status:      respValues.Get("Status"),
		MerID:       respValues.Get("MerID"),
		Version:     respValues.Get("Version"),
		EncryptInfo: respValues.Get("EncryptInfo"),
		HashInfo:    respValues.Get("HashInfo"),
	}, nil
}

// ==================== ATM 转账支付 ====================

// ATMPayRequest ATM 虚拟帐号转账请求参数。
type ATMPayRequest struct {
	// MerTradeNo 商店订单编号（限 25 字内，格式 [A-Za-z0-9_-]，10 分钟内不可重复）
	MerTradeNo string
	// TradeAmt 订单金额
	TradeAmt int
	// BankType 银行代码（数字）
	BankType string
	// ProdDesc 商品说明（长度限制 550）
	ProdDesc string
	// UsrMail 消费者信箱
	UsrMail string
	// PaySet 缴费帐号类型：1=单缴帐号/一次性（默认）
	PaySet int
	// ExpireDate 缴费截止日期（格式: YYYY-MM-DD，默认当日+7天，最大+180天）
	ExpireDate string
}

// ATMPayResponse ATM 转账支付返回结果（外层）。
type ATMPayResponse struct {
	// Status 外层状态代码：SUCCESS / 错误代码
	Status string
	// MerID 商店代号
	MerID string
	// Version 版本
	Version string
	// EncryptInfo 加密字串（需解密取得交易明细）
	EncryptInfo string
	// HashInfo 加密 Hash（用于验签）
	HashInfo string
}

// ATMPayDetail ATM 转账支付解密后的交易明细。
type ATMPayDetail struct {
	// Status 状态代码
	Status string
	// Message 状态说明
	Message string
	// MerID 商店代号
	MerID string
	// MerTradeNo 商店订单编号
	MerTradeNo string
	// TradeNo UNi 序号
	TradeNo string
	// TradeAmt 订单金额
	TradeAmt string
	// TradeStatus 订单状态：0=取号成功
	TradeStatus string
	// PaymentType 支付工具：2=ATM 转账
	PaymentType string
	// BankType 银行代码
	BankType string
	// PayNo 缴费虚拟帐号（核心返回值，需展示给消费者）
	PayNo string
	// PaySet 缴费帐号类型：1=单缴帐号（一次性）
	PaySet string
	// ExpireDate 缴费截止日期（格式: YYYY-MM-DD HH:II:SS）
	ExpireDate string
}

// ATMPay 发起 ATM 虚拟帐号转账取号。
//
// ATM 交易流程：
//   1. 付款人确认付款 → 商店（本系统）发送取号请求
//   2. PAYUNi 返回虚拟帐号 → 商店展示帐号给付款人
//   3. 付款人至 ATM 使用虚拟帐号转账缴款
//   4. 银行传送交易完成资讯给 PAYUNi
//   5. PAYUNi 通过 NotifyURL 通知商店交易完成
//   6. 商店更新订单状态并通知付款人
func (c *Client) ATMPay(ctx context.Context, req ATMPayRequest) (*ATMPayResponse, error) {
	// 组装 EncryptInfo 内的请求参数
	params := url.Values{}
	params.Set("MerID", c.MerID)
	params.Set("MerTradeNo", req.MerTradeNo)
	params.Set("TradeAmt", strconv.Itoa(req.TradeAmt))
	params.Set("Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	params.Set("BankType", req.BankType)
	params.Set("ProdDesc", req.ProdDesc)

	if req.UsrMail != "" {
		params.Set("UsrMail", req.UsrMail)
	}
	if req.PaySet > 0 {
		params.Set("PaySet", strconv.Itoa(req.PaySet))
	}
	if req.ExpireDate != "" {
		params.Set("ExpireDate", req.ExpireDate)
	}

	// 通知网址（选填）
	if c.NotifyURL != "" {
		params.Set("NotifyURL", c.NotifyURL)
	}

	// 发送请求到 PAYUNi
	respBody, err := c.doPost(ctx, "/api/atm", params.Encode())
	if err != nil {
		return nil, fmt.Errorf("payuni: ATM 取号请求失败: %w", err)
	}

	// 解析外层返回参数
	respValues, err := url.ParseQuery(respBody)
	if err != nil {
		return nil, fmt.Errorf("payuni: 解析 ATM 返回参数失败: %w", err)
	}

	return &ATMPayResponse{
		Status:      respValues.Get("Status"),
		MerID:       respValues.Get("MerID"),
		Version:     respValues.Get("Version"),
		EncryptInfo: respValues.Get("EncryptInfo"),
		HashInfo:    respValues.Get("HashInfo"),
	}, nil
}

// ==================== 通用解密方法 ====================

// DecryptResponse 解密 PAYUNi 返回的 EncryptInfo 并验证 HashInfo。
// 返回解密后的 URL-encoded query string，调用方可用 url.ParseQuery 解析具体字段。
func (c *Client) DecryptResponse(encryptInfo, hashInfo string) (url.Values, error) {
	// 验证 HashInfo 签名
	expectedHash := GetHash(encryptInfo, c.HashKey, c.HashIV)
	if expectedHash != hashInfo {
		return nil, fmt.Errorf("payuni: HashInfo 验签失败（期望: %s，实际: %s）", expectedHash, hashInfo)
	}

	// 解密 EncryptInfo
	decrypted, err := Decrypt(encryptInfo, c.HashKey, c.HashIV)
	if err != nil {
		return nil, fmt.Errorf("payuni: 解密 EncryptInfo 失败: %w", err)
	}

	// 解析为 url.Values
	values, err := url.ParseQuery(decrypted)
	if err != nil {
		return nil, fmt.Errorf("payuni: 解析解密数据失败: %w", err)
	}

	return values, nil
}

// ParseCreditPayDetail 将解密后的 url.Values 映射为 CreditPayDetail 结构体。
func ParseCreditPayDetail(v url.Values) *CreditPayDetail {
	return &CreditPayDetail{
		Status:       v.Get("Status"),
		Message:      v.Get("Message"),
		MerID:        v.Get("MerID"),
		MerTradeNo:   v.Get("MerTradeNo"),
		Gateway:      v.Get("Gateway"),
		TradeNo:      v.Get("TradeNo"),
		TradeAmt:     v.Get("TradeAmt"),
		TradeStatus:  v.Get("TradeStatus"),
		PaymentType:  v.Get("PaymentType"),
		CardBank:     v.Get("CardBank"),
		Card6No:      v.Get("Card6No"),
		Card4No:      v.Get("Card4No"),
		CardInst:     v.Get("CardInst"),
		FirstAmt:     v.Get("FirstAmt"),
		EachAmt:      v.Get("EachAmt"),
		ResCode:      v.Get("ResCode"),
		ResCodeMsg:   v.Get("ResCodeMsg"),
		AuthCode:     v.Get("AuthCode"),
		AuthBank:     v.Get("AuthBank"),
		AuthBankName: v.Get("AuthBankName"),
		AuthType:     v.Get("AuthType"),
		AuthDay:      v.Get("AuthDay"),
		AuthTime:     v.Get("AuthTime"),
		CreditHash:   v.Get("CreditHash"),
		CreditLife:   v.Get("CreditLife"),
		CoBrandCode:  v.Get("CoBrandCode"),
	}
}

// ParseCredit3DResponse 将解密后的 url.Values 映射为 Credit3DResponse 结构体。
// 当 API3D=1 时，PAYUNi 返回 3D 导页网址而非交易结果。
func ParseCredit3DResponse(v url.Values) *Credit3DResponse {
	return &Credit3DResponse{
		Status:  v.Get("Status"),
		Message: v.Get("Message"),
		URL:     v.Get("URL"),
	}
}

// ParseATMPayDetail 将解密后的 url.Values 映射为 ATMPayDetail 结构体。
func ParseATMPayDetail(v url.Values) *ATMPayDetail {
	return &ATMPayDetail{
		Status:      v.Get("Status"),
		Message:     v.Get("Message"),
		MerID:       v.Get("MerID"),
		MerTradeNo:  v.Get("MerTradeNo"),
		TradeNo:     v.Get("TradeNo"),
		TradeAmt:    v.Get("TradeAmt"),
		TradeStatus: v.Get("TradeStatus"),
		PaymentType: v.Get("PaymentType"),
		BankType:    v.Get("BankType"),
		PayNo:       v.Get("PayNo"),
		PaySet:      v.Get("PaySet"),
		ExpireDate:  v.Get("ExpireDate"),
	}
}

// ==================== 内部 HTTP 发送 ====================

// doPost 执行 PAYUNi API 的 HTTP POST 请求。
// 参数 plainParams 是未加密的 URL-encoded query string。
// 返回 PAYUNi 原始响应体。
func (c *Client) doPost(ctx context.Context, apiPath, plainParams string) (string, error) {
	// AES-GCM 加密
	encryptInfo, err := Encrypt(plainParams, c.HashKey, c.HashIV)
	if err != nil {
		return "", fmt.Errorf("payuni: 加密请求参数失败: %w", err)
	}

	// SHA256 签名
	hashInfo := GetHash(encryptInfo, c.HashKey, c.HashIV)

	// 组装外层请求参数
	postData := url.Values{}
	postData.Set("MerID", c.MerID)
	postData.Set("Version", apiVersion)
	postData.Set("EncryptInfo", encryptInfo)
	postData.Set("HashInfo", hashInfo)

	// 创建 HTTP 请求
	reqURL := strings.TrimRight(c.BaseURL, "/") + apiPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(postData.Encode()))
	if err != nil {
		return "", fmt.Errorf("payuni: 创建 HTTP 请求失败: %w", err)
	}

	// PAYUNi 要求 header 加入 user-agent，建议内容为 "payuni"
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("User-Agent", "payuni")

	// 发送请求
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("payuni: HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("payuni: 读取响应体失败: %w", err)
	}

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("payuni: HTTP 状态码异常: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
