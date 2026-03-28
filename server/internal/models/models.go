package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// User 表示系统登录账号，同时承载管理员和技师基础资料。
type User struct {
	// ID 是数据库主键，也是内部鉴权与关联查询使用的稳定用户编号。
	ID uint `json:"id" gorm:"primaryKey;comment:用户主键"`
	// Name 是后台和移动端展示的账号名称。
	Name string `json:"name" gorm:"not null;comment:用户名称"`
	// Role 标识账号角色，当前仅允许 admin 或 technician。
	Role string `json:"role" gorm:"not null;index;comment:用户角色"`
	// Phone 作为登录唯一键和运维 CLI 查找键使用。
	Phone string `json:"phone" gorm:"not null;index;comment:用户手机号"`
	// PasswordHash 仅在服务端参与认证，绝不回传给前端。
	PasswordHash string `json:"-" gorm:"column:password_hash;not null;comment:密码哈希"`
	// Color 是技师在前端排班和工单列表中的展示色。
	Color *string `json:"color,omitempty" gorm:"comment:技师展示色"`
	// Skills 记录技师可服务项目列表，按 JSON 数组存储。
	Skills datatypes.JSON `json:"skills,omitempty" gorm:"type:jsonb;default:'[]';comment:技师技能列表"`
	// ZoneID 关联技师默认负责的服务区域。
	ZoneID *string `json:"zone_id,omitempty" gorm:"comment:默认服务区域ID"`
	// Availability 记录技师可接单时间段，按 JSON 数组存储。
	Availability datatypes.JSON `json:"availability,omitempty" gorm:"type:jsonb;default:'[]';comment:技师可用时段"`
	// CreatedAt 是账号创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是账号最近一次资料更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除账号，便于管理端恢复误删的技师资料。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// Customer 表示客户主档，统一聚合电话、地址与 LINE 绑定信息。
type Customer struct {
	// ID 是客户主键，当前通常使用手机号或外部稳定标识生成。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:客户主键"`
	// Name 是客户在后台和派工页面中的展示名称。
	Name string `json:"name" gorm:"not null;comment:客户名称"`
	// Phone 是客户联系电话，也是预约同步客户档时的主要匹配条件之一。
	Phone string `json:"phone" gorm:"not null;index;comment:客户手机号"`
	// Address 是客户主要服务地址。
	Address string `json:"address" gorm:"not null;comment:客户地址"`
	// LineID 保留旧版或外部系统使用的 LINE 标识。
	LineID *string `json:"line_id,omitempty" gorm:"comment:LINE标识"`
	// LineName 是当前已同步的 LINE 昵称。
	LineName *string `json:"line_name,omitempty" gorm:"comment:LINE昵称"`
	// LinePicture 是当前已同步的 LINE 头像地址。
	LinePicture *string `json:"line_picture,omitempty" gorm:"comment:LINE头像地址"`
	// LineUID 是 LINE 官方用户 UID，用于 webhook 关联客户。
	LineUID *string `json:"line_uid,omitempty" gorm:"index;comment:LINE用户UID"`
	// LineJoinedAt 是客户关注 LINE 官方账号的时间。
	LineJoinedAt *time.Time `json:"line_joined_at,omitempty" gorm:"comment:LINE关注时间"`
	// LineData 存储额外的 LINE 资料快照，供后续扩展使用。
	LineData datatypes.JSON `json:"line_data,omitempty" gorm:"type:jsonb;default:'{}';comment:LINE扩展资料"`
	// CreatedAt 是客户主档创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是客户主档最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除客户主档，便于误删后恢复。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// Appointment 表示一次上门服务预约及其排程、支付和作业状态。
type Appointment struct {
	// ID 是预约主键。
	ID uint `json:"id" gorm:"primaryKey;comment:预约主键"`
	// CustomerName 是预约时快照化保存的客户称呼。
	CustomerName string `json:"customer_name" gorm:"not null;comment:客户名称快照"`
	// Address 是本次预约服务地址。
	Address string `json:"address" gorm:"not null;comment:服务地址"`
	// Phone 是本次预约联系电话。
	Phone string `json:"phone" gorm:"not null;index;comment:联系电话"`
	// Items 存储服务项目列表及其报价明细。
	Items datatypes.JSON `json:"items" gorm:"type:jsonb;default:'[]';comment:服务项目列表"`
	// ExtraItems 存储额外收费项目列表。
	ExtraItems datatypes.JSON `json:"extra_items" gorm:"type:jsonb;default:'[]';comment:额外收费项目列表"`
	// PaymentMethod 表示本单使用的付款方式。
	PaymentMethod string `json:"payment_method" gorm:"not null;comment:付款方式"`
	// TotalAmount 是后端根据服务项和附加项重算后的应收总额。
	TotalAmount int `json:"total_amount" gorm:"not null;comment:应收总金额"`
	// DiscountAmount 是当前预约享受的折扣金额。
	DiscountAmount int `json:"discount_amount,omitempty" gorm:"comment:折扣金额"`
	// PaidAmount 是客户已支付金额。
	PaidAmount int `json:"paid_amount,omitempty" gorm:"comment:已收金额"`
	// ScheduledAt 是预约开始时间。
	ScheduledAt time.Time `json:"scheduled_at" gorm:"index;comment:预约开始时间"`
	// ScheduledEnd 是预约预计结束时间。
	ScheduledEnd *time.Time `json:"scheduled_end,omitempty" gorm:"comment:预约结束时间"`
	// Status 表示预约当前业务状态。
	Status string `json:"status" gorm:"not null;index;comment:预约状态"`
	// CancelReason 记录取消原因。
	CancelReason *string `json:"cancel_reason,omitempty" gorm:"comment:取消原因"`
	// TechnicianID 关联当前被指派的技师账号。
	TechnicianID *uint `json:"technician_id,omitempty" gorm:"index;comment:指派技师ID"`
	// TechnicianName 是下发给前端的技师名称快照。
	TechnicianName *string `json:"technician_name,omitempty" gorm:"comment:技师名称快照"`
	// Lat 是服务地址纬度。
	Lat *float64 `json:"lat,omitempty" gorm:"comment:纬度"`
	// Lng 是服务地址经度。
	Lng *float64 `json:"lng,omitempty" gorm:"comment:经度"`
	// CheckinTime 是技师到场签到时间。
	CheckinTime *time.Time `json:"checkin_time,omitempty" gorm:"comment:签到时间"`
	// CheckoutTime 是技师离场签退时间。
	CheckoutTime *time.Time `json:"checkout_time,omitempty" gorm:"comment:签退时间"`
	// DepartedTime 是技师出发时间。
	DepartedTime *time.Time `json:"departed_time,omitempty" gorm:"comment:出发时间"`
	// CompletedTime 是服务完成时间。
	CompletedTime *time.Time `json:"completed_time,omitempty" gorm:"comment:完成时间"`
	// PaymentTime 是后端确认收款时间。
	PaymentTime *time.Time `json:"payment_time,omitempty" gorm:"comment:确认收款时间"`
	// Photos 存储作业现场照片地址列表。
	Photos datatypes.JSON `json:"photos" gorm:"type:jsonb;default:'[]';comment:现场照片列表"`
	// PaymentReceived 表示后端是否已确认收款完成。
	PaymentReceived bool `json:"payment_received" gorm:"comment:是否已确认收款"`
	// SignatureData 存储客户签名图片或签名数据。
	SignatureData *string `json:"signature_data,omitempty" gorm:"comment:客户签名数据"`
	// LineUID 用于把预约与 LINE 好友或客户记录关联起来。
	LineUID *string `json:"line_uid,omitempty" gorm:"index;comment:关联LINE用户UID"`
	// ZoneID 记录预约匹配到的服务区域。
	ZoneID *string `json:"zone_id,omitempty" gorm:"index;comment:匹配服务区域ID"`
	// ReviewToken 是公开评价页使用的随机令牌，避免在外链中暴露自增预约 ID。
	ReviewToken *string `json:"review_token,omitempty" gorm:"column:review_public_token;uniqueIndex;comment:公开评价令牌"`
	// CreatedAt 是预约创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是预约最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除预约，支持回收站恢复与延迟物理清理。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// ServiceZone 表示一块可派工服务区域及其覆盖行政区。
type ServiceZone struct {
	// ID 是服务区域主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:服务区域主键"`
	// Name 是服务区域展示名称。
	Name string `json:"name" gorm:"not null;comment:服务区域名称"`
	// Districts 保存该区域包含的行政区列表。
	Districts datatypes.JSON `json:"districts" gorm:"type:jsonb;default:'[]';comment:行政区列表"`
	// AssignedTechnicianIDs 保存被分配到该区域的技师 ID 列表。
	AssignedTechnicianIDs datatypes.JSON `json:"assigned_technician_ids" gorm:"type:jsonb;default:'[]';comment:分配技师ID列表"`
	// CreatedAt 是服务区域创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是服务区域最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除服务区域，避免误删后无法恢复。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// ServiceItem 表示标准服务项目及其默认报价。
type ServiceItem struct {
	// ID 是服务项目主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:服务项目主键"`
	// Name 是服务项目名称。
	Name string `json:"name" gorm:"not null;comment:服务项目名称"`
	// DefaultPrice 是该项目默认报价。
	DefaultPrice int `json:"default_price" gorm:"not null;comment:默认报价"`
	// Description 是服务项目补充说明。
	Description *string `json:"description,omitempty" gorm:"comment:服务项目描述"`
	// CreatedAt 是服务项目创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是服务项目最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除服务项目，便于设置页恢复误删项目。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// ExtraItem 表示可附加到预约中的额外收费项目。
type ExtraItem struct {
	// ID 是额外收费项目主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:额外收费项主键"`
	// Name 是额外收费项目名称。
	Name string `json:"name" gorm:"not null;comment:额外收费项名称"`
	// Price 是该额外收费项目金额。
	Price int `json:"price" gorm:"not null;comment:额外收费金额"`
	// CreatedAt 是额外收费项目创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是额外收费项目最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
	// DeletedAt 用于软删除额外收费项目，避免误删后立即丢失。
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index;comment:软删除时间"`
}

// CashLedgerEntry 表示技师现金收支流水，用于现金账页面和风控核对。
type CashLedgerEntry struct {
	// ID 是现金流水主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:现金流水主键"`
	// TechnicianID 关联流水所属技师。
	TechnicianID uint `json:"technician_id" gorm:"not null;index;comment:技师ID"`
	// AppointmentID 可选关联触发该流水的预约。
	AppointmentID *uint `json:"appointment_id,omitempty" gorm:"index;comment:关联预约ID"`
	// Type 表示流水类型，例如 income 或 expense。
	Type string `json:"type" gorm:"not null;comment:流水类型"`
	// Amount 是本次流水金额。
	Amount int `json:"amount" gorm:"not null;comment:流水金额"`
	// Note 是账务说明或备注。
	Note string `json:"note" gorm:"not null;comment:流水备注"`
	// CreatedAt 是流水发生时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是流水记录最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// Review 表示客户对预约服务的评价记录。
type Review struct {
	// ID 是评价主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:评价主键"`
	// AppointmentID 关联被评价的预约。
	AppointmentID uint `json:"appointment_id" gorm:"not null;uniqueIndex;comment:预约ID"`
	// CustomerName 是评价提交时的客户名称快照。
	CustomerName string `json:"customer_name" gorm:"not null;comment:客户名称快照"`
	// TechnicianID 可选关联被评价的技师。
	TechnicianID *uint `json:"technician_id,omitempty" gorm:"index;comment:技师ID"`
	// TechnicianName 是技师名称快照。
	TechnicianName *string `json:"technician_name,omitempty" gorm:"comment:技师名称快照"`
	// Rating 是客户评分。
	Rating int `json:"rating" gorm:"not null;comment:评分"`
	// Misconducts 记录评价中的异常行为标签列表。
	Misconducts datatypes.JSON `json:"misconducts" gorm:"type:jsonb;default:'[]';comment:异常行为标签列表"`
	// Comment 是客户评价内容。
	Comment string `json:"comment" gorm:"not null;comment:评价内容"`
	// SharedLine 表示该评价是否已经转发到 LINE。
	SharedLine bool `json:"shared_line" gorm:"comment:是否已分享到LINE"`
	// CreatedAt 是评价创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是评价最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// NotificationLog 表示一条预约相关通知发送记录。
type NotificationLog struct {
	// ID 是通知日志主键。
	ID string `json:"id" gorm:"primaryKey;size:64;comment:通知日志主键"`
	// AppointmentID 关联被通知的预约。
	AppointmentID uint `json:"appointment_id" gorm:"not null;index;comment:预约ID"`
	// Type 表示通知渠道或通知类型。
	Type string `json:"type" gorm:"not null;comment:通知类型"`
	// Message 是发送给用户的通知内容。
	Message string `json:"message" gorm:"not null;comment:通知内容"`
	// SentAt 是通知实际发送时间。
	SentAt time.Time `json:"sent_at" gorm:"not null;comment:发送时间"`
	// CreatedAt 是日志创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是日志最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// AppSetting 统一存放系统级配置，当前主要承载回访提醒天数等轻量设置。
type AppSetting struct {
	// Key 是配置项唯一键。
	Key string `json:"key" gorm:"primaryKey;size:128;comment:配置键"`
	// Value 是配置项当前值。
	Value string `json:"value" gorm:"not null;comment:配置值"`
	// Description 是配置项用途说明。
	Description *string `json:"description,omitempty" gorm:"comment:配置说明"`
	// CreatedAt 是配置项创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是配置项最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// LineFriend 表示通过 LINE webhook 同步入库的好友资料。
type LineFriend struct {
	// LineUID 是 LINE 官方唯一用户标识。
	LineUID string `json:"line_uid" gorm:"primaryKey;size:128;comment:LINE用户UID"`
	// LineName 是最近一次同步到的 LINE 昵称。
	LineName string `json:"line_name" gorm:"not null;comment:LINE昵称"`
	// LinePicture 是最近一次同步到的头像地址。
	LinePicture string `json:"line_picture" gorm:"not null;comment:LINE头像地址"`
	// JoinedAt 是首次关注官方账号时间。
	JoinedAt time.Time `json:"joined_at" gorm:"not null;index;comment:首次关注时间"`
	// Phone 是 webhook 中可能携带的手机号。
	Phone *string `json:"phone,omitempty" gorm:"comment:手机号"`
	// LinkedCustomerID 关联已绑定的客户主档 ID。
	LinkedCustomerID *string `json:"linked_customer_id,omitempty" gorm:"index;comment:绑定客户ID"`
	// Status 表示当前好友关系状态。
	Status string `json:"status" gorm:"not null;default:'followed';comment:好友状态"`
	// LastPayload 保存最近一次收到的 webhook 事件体。
	LastPayload datatypes.JSON `json:"last_payload,omitempty" gorm:"type:jsonb;default:'{}';comment:最近一次事件负载"`
	// CreatedAt 是好友记录创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是好友记录最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// LineEvent 表示落库保存的原始 LINE webhook 事件。
type LineEvent struct {
	// ID 是 webhook 事件主键。
	ID uint `json:"id" gorm:"primaryKey;comment:事件主键"`
	// EventType 是 webhook 事件类型，例如 follow/unfollow。
	EventType string `json:"event_type" gorm:"not null;index;comment:事件类型"`
	// LineUID 可选关联触发事件的 LINE 用户。
	LineUID *string `json:"line_uid,omitempty" gorm:"index;comment:关联LINE用户UID"`
	// ReceivedAt 是事件发生或被接收的时间。
	ReceivedAt time.Time `json:"received_at" gorm:"not null;index;comment:接收时间"`
	// Payload 保存原始事件 JSON。
	Payload datatypes.JSON `json:"payload" gorm:"type:jsonb;not null;comment:原始事件内容"`
	// CreatedAt 是事件记录创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是事件记录最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// AuthToken 持久化认证令牌，存入数据库以支持服务器重启后 token 仍然有效。
// 同一用户同时只保留一个有效 token（登录时先删除该用户旧 token）。
type AuthToken struct {
	// ID 是认证令牌主键。
	ID uint `json:"id" gorm:"primaryKey;comment:认证令牌主键"`
	// UserID 关联所属用户。
	UserID uint `json:"user_id" gorm:"not null;index;comment:所属用户ID"`
	// Token 是写入 cookie 和数据库的随机令牌正文。
	Token string `json:"token" gorm:"not null;uniqueIndex;size:128;comment:认证令牌"`
	// ExpiresAt 是令牌过期时间。
	ExpiresAt time.Time `json:"expires_at" gorm:"not null;index;comment:过期时间"`
	// CreatedAt 是令牌创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 是令牌最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// PaymentOrder 记录每一笔支付订单及其完整生命周期。
// 管理员创建订单后生成 PaymentToken，客户凭该 Token 无需登录即可查看和支付。
type PaymentOrder struct {
	// ID 是支付订单主键。
	ID uint `json:"id" gorm:"primaryKey;comment:支付订单主键"`
	// PaymentToken 随机令牌（32字节 URL-safe base64），客户端凭此无登录访问。
	PaymentToken string `json:"payment_token" gorm:"uniqueIndex;size:64;not null;comment:支付令牌"`
	// MerTradeNo PAYUNi 商店订单编号（后端自动生成，限25字内，10分钟内不重复）。
	MerTradeNo string `json:"mer_trade_no" gorm:"uniqueIndex;size:25;not null;comment:PAYUNi商店订单编号"`
	// TradeAmt 订单金额（整数，单位：元）。
	TradeAmt int `json:"trade_amt" gorm:"not null;comment:订单金额"`
	// ProdDesc 商品说明（展示给客户）。
	ProdDesc string `json:"prod_desc" gorm:"not null;comment:商品说明"`
	// PaymentMethod 允许的支付方式：credit=信用卡, atm=ATM转账, both=两种都可。
	PaymentMethod string `json:"payment_method" gorm:"not null;default:'both';comment:允许的支付方式"`
	// CustomerName 消费者名称（管理员创建时填写）。
	CustomerName string `json:"customer_name" gorm:"not null;comment:消费者名称"`
	// CustomerEmail 消费者信箱（选填）。
	CustomerEmail string `json:"customer_email,omitempty" gorm:"comment:消费者信箱"`
	// CustomerPhone 消费者电话（选填）。
	CustomerPhone string `json:"customer_phone,omitempty" gorm:"comment:消费者电话"`
	// AppointmentID 可选关联的预约 ID。
	AppointmentID *uint `json:"appointment_id,omitempty" gorm:"index;comment:关联预约ID"`
	// CreatedByID 创建此支付订单的管理员用户 ID。
	CreatedByID uint `json:"created_by_id" gorm:"not null;comment:创建者管理员ID"`
	// Status 订单状态：pending=待支付, paying=支付中, paid=已支付, failed=失败, expired=过期, cancelled=取消。
	Status string `json:"status" gorm:"not null;default:'pending';index;comment:订单状态"`
	// TradeNo PAYUNi 平台分配的交易序号。
	TradeNo string `json:"trade_no,omitempty" gorm:"comment:PAYUNi平台序号"`
	// TradeStatus PAYUNi 返回的交易状态码。
	TradeStatus string `json:"trade_status,omitempty" gorm:"comment:PAYUNi交易状态码"`
	// PayNo ATM 虚拟帐号（仅 ATM 支付时有值）。
	PayNo string `json:"pay_no,omitempty" gorm:"comment:ATM虚拟帐号"`
	// ATMExpireDate ATM 缴费截止日期。
	ATMExpireDate string `json:"atm_expire_date,omitempty" gorm:"comment:ATM缴费截止日"`
	// AuthCode 信用卡授权码（仅信用卡支付成功时有值）。
	AuthCode string `json:"auth_code,omitempty" gorm:"comment:信用卡授权码"`
	// Card6No 卡号前六码（脱敏存储，用于对帐）。
	Card6No string `json:"card_6_no,omitempty" gorm:"comment:卡号前六码"`
	// Card4No 卡号后四码（脱敏存储，用于对帐）。
	Card4No string `json:"card_4_no,omitempty" gorm:"comment:卡号后四码"`
	// ResCode PAYUNi 回应码。
	ResCode string `json:"res_code,omitempty" gorm:"comment:PAYUNi回应码"`
	// ResCodeMsg PAYUNi 回应码说明。
	ResCodeMsg string `json:"res_code_msg,omitempty" gorm:"comment:PAYUNi回应码说明"`
	// RawResponse PAYUNi 解密后的完整返回（JSON 存储，方便对帐排查）。
	RawResponse datatypes.JSON `json:"raw_response,omitempty" gorm:"type:jsonb;comment:PAYUNi完整返回"`
	// PaidAt 支付成功确认时间。
	PaidAt *time.Time `json:"paid_at,omitempty" gorm:"comment:支付成功时间"`
	// CreatedAt 订单创建时间。
	CreatedAt time.Time `json:"created_at" gorm:"comment:创建时间"`
	// UpdatedAt 订单最近更新时间。
	UpdatedAt time.Time `json:"updated_at" gorm:"comment:更新时间"`
}

// AutoMigrateModels 返回需要参与自动迁移的全部模型列表。
func AutoMigrateModels() []any {
	return []any{
		&User{},
		&Customer{},
		&Appointment{},
		&ServiceZone{},
		&ServiceItem{},
		&ExtraItem{},
		&CashLedgerEntry{},
		&Review{},
		&NotificationLog{},
		&AppSetting{},
		&LineFriend{},
		&LineEvent{},
		&AuthToken{},
		&PaymentOrder{},
	}
}
