package httpapi

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// respondMessage 统一输出对外 JSON message，并在出口处完成繁体中文转译。
func respondMessage(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"message": localizedAPIMessage(message)})
}

// abortWithMessage 统一输出会中断链路的 JSON message，并在出口处完成繁体中文转译。
func abortWithMessage(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"message": localizedAPIMessage(message)})
}

// localizedAPIMessage 把当前后端对外返回的英文 message 统一转换成繁体中文，
// 底层校验函数仍可保留既有英文错误，避免影响内部逻辑与低层单元测试。
func localizedAPIMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "發生未知錯誤"
	}

	switch trimmed {
	case "request body too large":
		return "請求體過大"
	case "Not Found":
		return "找不到請求的資源"
	case "invalid credentials":
		return "帳號或密碼錯誤"
	case "failed to create auth token":
		return "建立登入憑證失敗"
	case "not authenticated":
		return "尚未登入"
	case "invalid or expired token":
		return "登入憑證無效或已過期"
	case "logged out":
		return "已登出"
	case "authentication required":
		return "需要先登入"
	case "forbidden":
		return "無權限執行此操作"
	case "invalid login payload":
		return "登入請求格式不正確"
	case "invalid technicians payload":
		return "技師資料格式不正確"
	case "invalid zones payload":
		return "區域資料格式不正確"
	case "invalid service items payload":
		return "服務項目資料格式不正確"
	case "invalid extra items payload":
		return "額外項目資料格式不正確"
	case "invalid review payload":
		return "評價資料格式不正確"
	case "invalid review share payload":
		return "評價分享資料格式不正確"
	case "invalid notification payload":
		return "通知資料格式不正確"
	case "invalid reminder settings payload":
		return "回訪設定資料格式不正確"
	case "invalid webhook settings payload":
		return "Webhook 設定資料格式不正確"
	case "invalid customers payload":
		return "客戶資料格式不正確"
	case "invalid line friend payload":
		return "LINE 好友資料格式不正確"
	case "invalid webhook payload":
		return "Webhook 請求格式不正確"
	case "line webhook is disabled by admin setting":
		return "LINE Webhook 已被管理員停用"
	case "line webhook secret is not configured":
		return "尚未設定 LINE Webhook 密鑰"
	case "missing line webhook signature":
		return "缺少 LINE Webhook 簽章"
	case "invalid line webhook signature":
		return "LINE Webhook 簽章無效"
	case "invalid review token":
		return "評價令牌無效"
	case "appointment not found":
		return "找不到預約資料"
	case "review not found":
		return "找不到評價資料"
	case "customer not found":
		return "找不到客戶資料"
	case "technician not found":
		return "找不到技師資料"
	case "line friend not found":
		return "找不到 LINE 好友資料"
	case "failed to generate review token":
		return "產生評價令牌失敗"
	case "failed to create appointment":
		return "建立預約失敗"
	case "failed to update appointment":
		return "更新預約失敗"
	case "failed to delete appointment":
		return "刪除預約失敗"
	case "failed to create review":
		return "建立評價失敗"
	case "failed to update review share status":
		return "更新評價分享狀態失敗"
	case "failed to create notification log":
		return "建立通知紀錄失敗"
	case "failed to create cash ledger entry":
		return "建立現金帳流水失敗"
	case "failed to replace technicians":
		return "覆寫技師資料失敗"
	case "failed to replace zones":
		return "覆寫區域資料失敗"
	case "failed to replace service items":
		return "覆寫服務項目資料失敗"
	case "failed to replace extra items":
		return "覆寫額外項目資料失敗"
	case "failed to replace customers":
		return "覆寫客戶資料失敗"
	case "failed to delete customer":
		return "刪除客戶失敗"
	case "failed to link line friend":
		return "綁定 LINE 好友失敗"
	case "failed to reload line friend":
		return "重新載入 LINE 好友資料失敗"
	case "failed to persist webhook":
		return "儲存 Webhook 資料失敗"
	case "failed to update line friend":
		return "更新 LINE 好友資料失敗"
	case "failed to update reminder_days":
		return "更新回訪天數失敗"
	case "failed to update webhook setting":
		return "更新 Webhook 設定失敗"
	case "failed to reload webhook setting":
		return "重新載入 Webhook 設定失敗"
	case "appointment must be completed before review":
		return "預約完成後才能提交評價"
	case "scheduled_end must be later than scheduled_at":
		return "預約結束時間必須晚於預約時間"
	case "discount_amount cannot be greater than subtotal":
		return "折扣金額不可大於小計"
	case "technician_id is required for the current status":
		return "目前狀態必須指定技師"
	case "lat and lng must either both be set or both be empty":
		return "緯度與經度必須同時填寫或同時留空"
	case "cancel_reason is required when status is cancelled":
		return "狀態為已取消時，取消原因為必填欄位"
	case "departed_time requires technician_id":
		return "填寫出發時間時必須指定技師"
	case "checkin_time requires technician_id":
		return "填寫到場時間時必須指定技師"
	case "completed_time requires completed status":
		return "填寫完工時間時，狀態必須為已完成"
	case "checkout_time requires completed or cancelled status":
		return "填寫離場時間時，狀態必須為已完成或已取消"
	case "payment_received requires completed or cancelled status":
		return "只有已完成或已取消的預約才能標記為已收款"
	case "paid_amount is required when payment_received is true":
		return "收款狀態為已收款時，實收金額為必填欄位"
	case "payment_received must be true when paid_amount is greater than 0":
		return "實收金額大於 0 時，收款狀態必須為已收款"
	case "payment_time requires payment_received to be true":
		return "填寫收款時間前必須先標記為已收款"
	case "appointment_id is required for collect":
		return "收款流水必須帶預約 ID"
	case "appointment_id must be empty for return":
		return "回繳流水不可帶預約 ID"
	case "appointment does not belong to technician":
		return "預約不屬於該技師"
	case "collect entry already exists for appointment":
		return "該預約已存在收款流水"
	case "collect entry requires a cash appointment":
		return "收款流水只允許綁定現金收款預約"
	case "collect entry requires a finished appointment":
		return "收款流水只允許綁定已完成或已取消的預約"
	case "collect entry requires a payment_received appointment":
		return "收款流水只允許綁定已確認收款的預約"
	case "amount must equal appointment paid amount":
		return "金額必須等於預約實收金額"
	case "technicians payload must not be empty":
		return "技師資料不得為空"
	case "technician name is required":
		return "技師名稱為必填欄位"
	case "technician phone is required":
		return "技師手機號碼為必填欄位"
	case "zones payload must not be empty":
		return "區域資料不得為空"
	case "zone id and name are required":
		return "區域 ID 與名稱為必填欄位"
	case "service items payload must not be empty":
		return "服務項目資料不得為空"
	case "service item id and name are required":
		return "服務項目 ID 與名稱為必填欄位"
	case "extra items payload must not be empty":
		return "額外項目資料不得為空"
	case "extra item id and name are required":
		return "額外項目 ID 與名稱為必填欄位"
	case "customers payload must not be empty":
		return "客戶資料不得為空"
	case "customer id and name are required":
		return "客戶 ID 與名稱為必填欄位"
	case "customer phone and address are required":
		return "客戶電話與地址為必填欄位"
	case "reminder_days must be greater than 0":
		return "回訪天數必須大於 0"
	case "replace payload must not be empty":
		return "資料不得為空"
	case "invalid cash_ledger type":
		return "現金帳類型無效"
	case "appointment is required":
		return "預約資料為必填"
	case "invalid misconducts":
		return "不當行為資料格式不正確"
	case "invalid status":
		return "狀態無效"
	case "invalid payment_method":
		return "付款方式無效"
	case "items must be a valid array":
		return "服務項目必須是有效陣列"
	case "extra_items must be a valid array":
		return "額外項目必須是有效陣列"
	case "photos must be a valid array":
		return "照片必須是有效陣列"
	case "rating must be between 1 and 5":
		return "評分必須介於 1 到 5 之間"
	case "return amount exceeds current cash balance":
		return "回繳金額不可超過目前現金餘額"
	case "empty request body":
		return "請求體不得為空"
	case "invalid json payload":
		return "JSON 請求格式不正確"
	case "technician password updated":
		return "技師密碼已更新"
	case "technician password is required":
		return "技師密碼為必填欄位"
	case "invalid technician password payload":
		return "技師密碼資料格式不正確"
	case "invalid technician id":
		return "技師 ID 無效"
	case "failed to hash technician password":
		return "密碼加密失敗"
	case "failed to update technician password":
		return "更新技師密碼失敗"
	}

	if strings.HasPrefix(trimmed, "failed to load ") {
		return "載入" + translateAPIEntity(strings.TrimPrefix(trimmed, "failed to load ")) + "失敗"
	}
	if strings.HasPrefix(trimmed, "failed to create ") {
		return "建立" + translateAPIEntity(strings.TrimPrefix(trimmed, "failed to create ")) + "失敗"
	}
	if strings.HasPrefix(trimmed, "failed to update ") {
		return "更新" + translateAPIEntity(strings.TrimPrefix(trimmed, "failed to update ")) + "失敗"
	}
	if strings.HasPrefix(trimmed, "failed to delete ") {
		return "刪除" + translateAPIEntity(strings.TrimPrefix(trimmed, "failed to delete ")) + "失敗"
	}
	if strings.HasPrefix(trimmed, "failed to reload ") {
		return "重新載入" + translateAPIEntity(strings.TrimPrefix(trimmed, "failed to reload ")) + "失敗"
	}
	if strings.HasSuffix(trimmed, " not found") {
		return "找不到" + translateAPIEntity(strings.TrimSuffix(trimmed, " not found"))
	}
	if strings.HasPrefix(trimmed, "invalid ") {
		return translateAPIField(strings.TrimPrefix(trimmed, "invalid ")) + "無效"
	}
	if strings.HasSuffix(trimmed, " is required") {
		return translateAPIField(strings.TrimSuffix(trimmed, " is required")) + "為必填欄位"
	}
	if strings.HasSuffix(trimmed, " must not be empty") {
		return translateAPIField(strings.TrimSuffix(trimmed, " must not be empty")) + "不得為空"
	}
	if strings.HasSuffix(trimmed, " must be greater than 0") {
		return translateAPIField(strings.TrimSuffix(trimmed, " must be greater than 0")) + "必須大於 0"
	}
	if strings.HasSuffix(trimmed, " must be greater than or equal to 0") {
		return translateAPIField(strings.TrimSuffix(trimmed, " must be greater than or equal to 0")) + "必須大於或等於 0"
	}
	if strings.Contains(trimmed, " cannot be greater than ") {
		parts := strings.SplitN(trimmed, " cannot be greater than ", 2)
		return translateAPIField(parts[0]) + "不可大於" + translateAPIField(parts[1])
	}
	if strings.Contains(trimmed, " must be later than ") {
		parts := strings.SplitN(trimmed, " must be later than ", 2)
		return translateAPIField(parts[0]) + "必須晚於" + translateAPIField(parts[1])
	}
	if strings.Contains(trimmed, " cannot be earlier than ") {
		parts := strings.SplitN(trimmed, " cannot be earlier than ", 2)
		return translateAPIField(parts[0]) + "不可早於" + translateAPIField(parts[1])
	}
	if strings.Contains(trimmed, " requires ") {
		parts := strings.SplitN(trimmed, " requires ", 2)
		return translateAPIField(parts[0]) + "需要" + translateAPIRequirement(parts[1])
	}
	if strings.Contains(trimmed, " must reference a technician user") {
		return translateAPIField(strings.TrimSuffix(trimmed, " must reference a technician user")) + "必須指向技師使用者"
	}
	if strings.Contains(trimmed, " must reference a technician") {
		return translateAPIField(strings.TrimSuffix(trimmed, " must reference a technician")) + "必須指向技師"
	}
	if strings.Contains(trimmed, " already exists for appointment") {
		return translateAPIField(strings.TrimSuffix(trimmed, " already exists for appointment")) + "已存在於該預約"
	}
	if strings.Contains(trimmed, " does not belong to technician") {
		return translateAPIField(strings.TrimSuffix(trimmed, " does not belong to technician")) + "不屬於該技師"
	}
	if strings.HasPrefix(trimmed, "photo value at index ") && strings.HasSuffix(trimmed, " must not be empty") {
		index := strings.TrimSuffix(strings.TrimPrefix(trimmed, "photo value at index "), " must not be empty")
		return "照片索引 " + index + " 的值不得為空"
	}
	if strings.HasPrefix(trimmed, "json: unknown field ") {
		field := strings.Trim(trimmed[len("json: unknown field "):], `"`)
		return "未知欄位：" + field
	}
	if strings.Contains(trimmed, "cannot unmarshal") || trimmed == "unexpected EOF" {
		return "JSON 請求格式不正確"
	}

	return trimmed
}

func translateAPIEntity(value string) string {
	switch strings.TrimSpace(value) {
	case "bootstrap data":
		return "首頁初始化資料"
	case "dashboard page data":
		return "首頁總覽資料"
	case "customer page data":
		return "客戶頁資料"
	case "settings page data":
		return "設定頁資料"
	case "line page data":
		return "LINE 頁資料"
	case "technician page data":
		return "技師頁資料"
	case "reminder page data":
		return "回訪頁資料"
	case "zone page data":
		return "區域頁資料"
	case "financial report page data":
		return "財務報表頁資料"
	case "review dashboard page data":
		return "評價看板頁資料"
	case "cash ledger page data":
		return "現金帳頁資料"
	case "appointments":
		return "預約資料"
	case "appointment":
		return "預約資料"
	case "technicians":
		return "技師資料"
	case "technician":
		return "技師資料"
	case "customers":
		return "客戶資料"
	case "customer":
		return "客戶資料"
	case "zones":
		return "區域資料"
	case "service items":
		return "服務項目資料"
	case "extra items":
		return "額外項目資料"
	case "cash ledger entries":
		return "現金帳流水資料"
	case "cash ledger entry":
		return "現金帳流水"
	case "reviews":
		return "評價資料"
	case "review":
		return "評價資料"
	case "review context":
		return "評價內容"
	case "review share status":
		return "評價分享狀態"
	case "notification logs":
		return "通知紀錄"
	case "notification log":
		return "通知紀錄"
	case "settings":
		return "設定資料"
	case "webhook settings":
		return "Webhook 設定"
	case "webhook setting":
		return "Webhook 設定"
	case "line data":
		return "LINE 資料"
	case "line friend":
		return "LINE 好友資料"
	default:
		return strings.TrimSpace(value)
	}
}

func translateAPIField(value string) string {
	switch strings.TrimSpace(value) {
	case "review token":
		return "評價令牌"
	case "appointment id":
		return "預約 ID"
	case "customer id":
		return "客戶 ID"
	case "line uid":
		return "LINE UID"
	case "scheduled_at":
		return "預約時間"
	case "scheduled_end":
		return "預約結束時間"
	case "created_at":
		return "建立時間"
	case "sent_at":
		return "發送時間"
	case "line_joined_at":
		return "LINE 加入時間"
	case "payment_method":
		return "付款方式"
	case "discount_amount":
		return "折扣金額"
	case "paid_amount":
		return "實收金額"
	case "payment_received":
		return "收款狀態"
	case "payment_time":
		return "收款時間"
	case "customer_name":
		return "客戶姓名"
	case "address":
		return "地址"
	case "phone":
		return "電話"
	case "line_uid":
		return "LINE UID"
	case "technician_id":
		return "技師 ID"
	case "cancel_reason":
		return "取消原因"
	case "departed_time":
		return "出發時間"
	case "checkin_time":
		return "到場時間"
	case "completed_time":
		return "完工時間"
	case "checkout_time":
		return "離場時間"
	case "lat":
		return "緯度"
	case "lng":
		return "經度"
	case "status":
		return "狀態"
	case "items":
		return "服務項目"
	case "extra_items":
		return "額外項目"
	case "photos":
		return "照片"
	case "note":
		return "備註"
	case "message":
		return "訊息內容"
	case "rating":
		return "評分"
	case "appointment":
		return "預約資料"
	case "appointment_id":
		return "預約 ID"
	case "notification type":
		return "通知類型"
	case "cash_ledger type":
		return "現金帳類型"
	case "amount":
		return "金額"
	case "reminder_days":
		return "回訪天數"
	case "technicians payload":
		return "技師資料"
	case "zones payload":
		return "區域資料"
	case "service items payload":
		return "服務項目資料"
	case "extra items payload":
		return "額外項目資料"
	case "customers payload":
		return "客戶資料"
	case "zone id and name":
		return "區域 ID 與名稱"
	case "service item id and name":
		return "服務項目 ID 與名稱"
	case "extra item id and name":
		return "額外項目 ID 與名稱"
	case "customer id and name":
		return "客戶 ID 與名稱"
	case "customer phone and address":
		return "客戶電話與地址"
	case "technician name":
		return "技師名稱"
	case "technician phone":
		return "技師手機號碼"
	case "phone or line_uid":
		return "電話或 LINE UID"
	case "item id":
		return "服務項目 ID"
	case "item type":
		return "服務項目類型"
	case "item price":
		return "項目金額"
	case "extra_item id":
		return "額外項目 ID"
	case "extra_item name":
		return "額外項目名稱"
	case "collect entry":
		return "收款流水"
	case "return amount":
		return "回繳金額"
	default:
		return strings.TrimSpace(value)
	}
}

func translateAPIRequirement(value string) string {
	switch strings.TrimSpace(value) {
	case "technician_id":
		return "技師 ID"
	case "technician_id must reference a technician user":
		return "有效的技師使用者"
	case "technician_id must reference a technician":
		return "有效的技師"
	case "completed status":
		return "已完成狀態"
	case "completed or cancelled status":
		return "已完成或已取消狀態"
	case "payment_received to be true":
		return "收款狀態為已收款"
	case "a technician user":
		return "有效的技師使用者"
	case "a technician":
		return "有效的技師"
	case "a cash appointment":
		return "現金收款的預約"
	case "a finished appointment":
		return "已完成或已取消的預約"
	case "a payment_received appointment":
		return "已確認收款的預約"
	default:
		return translateAPIField(value)
	}
}
