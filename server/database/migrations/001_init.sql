-- ============================================================================
-- 001_init.sql
-- 初始化核心业务表、索引与数据库 comment。
-- 用于新库首次建库，覆盖用户、客户、预约、评价、通知、LINE、认证等主链路数据结构。
-- ============================================================================

-- ---------- 核心业务表 ----------
CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  role TEXT NOT NULL,
  phone TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  color TEXT,
  skills JSONB NOT NULL DEFAULT '[]'::jsonb,
  zone_id TEXT,
  availability JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS line_friends (
  line_uid TEXT PRIMARY KEY,
  line_name TEXT NOT NULL,
  line_picture TEXT NOT NULL,
  joined_at TIMESTAMPTZ NOT NULL,
  phone TEXT,
  linked_customer_id TEXT,
  status TEXT NOT NULL DEFAULT 'followed',
  last_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS line_events (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  line_uid TEXT,
  received_at TIMESTAMPTZ NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  phone TEXT NOT NULL,
  address TEXT NOT NULL,
  line_id TEXT,
  line_name TEXT,
  line_picture TEXT,
  line_uid TEXT UNIQUE,
  line_joined_at TIMESTAMPTZ,
  line_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS appointments (
  id BIGSERIAL PRIMARY KEY,
  customer_name TEXT NOT NULL,
  address TEXT NOT NULL,
  phone TEXT NOT NULL,
  items JSONB NOT NULL DEFAULT '[]'::jsonb,
  extra_items JSONB NOT NULL DEFAULT '[]'::jsonb,
  payment_method TEXT NOT NULL,
  total_amount INTEGER NOT NULL,
  discount_amount INTEGER NOT NULL DEFAULT 0,
  paid_amount INTEGER NOT NULL DEFAULT 0,
  scheduled_at TIMESTAMPTZ NOT NULL,
  scheduled_end TIMESTAMPTZ,
  status TEXT NOT NULL,
  cancel_reason TEXT,
  technician_id BIGINT,
  technician_name TEXT,
  lat DOUBLE PRECISION,
  lng DOUBLE PRECISION,
  checkin_time TIMESTAMPTZ,
  checkout_time TIMESTAMPTZ,
  departed_time TIMESTAMPTZ,
  completed_time TIMESTAMPTZ,
  payment_time TIMESTAMPTZ,
  photos JSONB NOT NULL DEFAULT '[]'::jsonb,
  payment_received BOOLEAN NOT NULL DEFAULT FALSE,
  signature_data TEXT,
  line_uid TEXT,
  zone_id TEXT,
  review_public_token TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS service_zones (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  districts JSONB NOT NULL DEFAULT '[]'::jsonb,
  assigned_technician_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS service_items (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  default_price INTEGER NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS extra_items (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  price INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cash_ledger_entries (
  id TEXT PRIMARY KEY,
  technician_id BIGINT NOT NULL,
  appointment_id BIGINT,
  type TEXT NOT NULL,
  amount INTEGER NOT NULL,
  note TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS reviews (
  id TEXT PRIMARY KEY,
  appointment_id BIGINT NOT NULL,
  customer_name TEXT NOT NULL,
  technician_id BIGINT,
  technician_name TEXT,
  rating INTEGER NOT NULL,
  misconducts JSONB NOT NULL DEFAULT '[]'::jsonb,
  comment TEXT NOT NULL DEFAULT '',
  shared_line BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notification_logs (
  id TEXT PRIMARY KEY,
  appointment_id BIGINT NOT NULL,
  type TEXT NOT NULL,
  message TEXT NOT NULL,
  sent_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS auth_tokens (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  token TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ---------- 查询与关联索引 ----------
CREATE INDEX IF NOT EXISTS idx_customers_phone ON customers(phone);
CREATE INDEX IF NOT EXISTS idx_appointments_phone ON appointments(phone);
CREATE INDEX IF NOT EXISTS idx_appointments_scheduled_at ON appointments(scheduled_at DESC);
CREATE INDEX IF NOT EXISTS idx_appointments_status ON appointments(status);
CREATE INDEX IF NOT EXISTS idx_appointments_technician_id ON appointments(technician_id);
CREATE INDEX IF NOT EXISTS idx_appointments_line_uid ON appointments(line_uid);
CREATE INDEX IF NOT EXISTS idx_appointments_zone_id ON appointments(zone_id);
CREATE INDEX IF NOT EXISTS idx_cash_ledger_entries_technician_id ON cash_ledger_entries(technician_id);
CREATE INDEX IF NOT EXISTS idx_cash_ledger_entries_appointment_id ON cash_ledger_entries(appointment_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_reviews_appointment_id ON reviews(appointment_id);
CREATE INDEX IF NOT EXISTS idx_reviews_technician_id ON reviews(technician_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_appointment_id ON notification_logs(appointment_id);
CREATE INDEX IF NOT EXISTS idx_line_friends_linked_customer_id ON line_friends(linked_customer_id);
CREATE INDEX IF NOT EXISTS idx_line_events_received_at ON line_events(received_at DESC);
CREATE INDEX IF NOT EXISTS idx_line_events_line_uid ON line_events(line_uid);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_expires_at ON auth_tokens(expires_at);

-- ---------- 表与字段注释 ----------
COMMENT ON TABLE users IS '系统用户表';
COMMENT ON COLUMN users.id IS '用户主键';
COMMENT ON COLUMN users.name IS '用户名称';
COMMENT ON COLUMN users.role IS '用户角色';
COMMENT ON COLUMN users.phone IS '用户手机号';
COMMENT ON COLUMN users.password_hash IS '密码哈希';
COMMENT ON COLUMN users.color IS '技师展示色';
COMMENT ON COLUMN users.skills IS '技师技能列表';
COMMENT ON COLUMN users.zone_id IS '默认服务区域ID';
COMMENT ON COLUMN users.availability IS '技师可用时段';
COMMENT ON COLUMN users.created_at IS '创建时间';
COMMENT ON COLUMN users.updated_at IS '更新时间';

COMMENT ON TABLE line_friends IS 'LINE好友表';
COMMENT ON COLUMN line_friends.line_uid IS 'LINE用户UID';
COMMENT ON COLUMN line_friends.line_name IS 'LINE昵称';
COMMENT ON COLUMN line_friends.line_picture IS 'LINE头像地址';
COMMENT ON COLUMN line_friends.joined_at IS '首次关注时间';
COMMENT ON COLUMN line_friends.phone IS '手机号';
COMMENT ON COLUMN line_friends.linked_customer_id IS '绑定客户ID';
COMMENT ON COLUMN line_friends.status IS '好友状态';
COMMENT ON COLUMN line_friends.last_payload IS '最近一次事件负载';
COMMENT ON COLUMN line_friends.created_at IS '创建时间';
COMMENT ON COLUMN line_friends.updated_at IS '更新时间';

COMMENT ON TABLE line_events IS 'LINE事件表';
COMMENT ON COLUMN line_events.id IS '事件主键';
COMMENT ON COLUMN line_events.event_type IS '事件类型';
COMMENT ON COLUMN line_events.line_uid IS '关联LINE用户UID';
COMMENT ON COLUMN line_events.received_at IS '接收时间';
COMMENT ON COLUMN line_events.payload IS '原始事件内容';
COMMENT ON COLUMN line_events.created_at IS '创建时间';
COMMENT ON COLUMN line_events.updated_at IS '更新时间';

COMMENT ON TABLE customers IS '客户主档表';
COMMENT ON COLUMN customers.id IS '客户主键';
COMMENT ON COLUMN customers.name IS '客户名称';
COMMENT ON COLUMN customers.phone IS '客户手机号';
COMMENT ON COLUMN customers.address IS '客户地址';
COMMENT ON COLUMN customers.line_id IS 'LINE标识';
COMMENT ON COLUMN customers.line_name IS 'LINE昵称';
COMMENT ON COLUMN customers.line_picture IS 'LINE头像地址';
COMMENT ON COLUMN customers.line_uid IS 'LINE用户UID';
COMMENT ON COLUMN customers.line_joined_at IS 'LINE关注时间';
COMMENT ON COLUMN customers.line_data IS 'LINE扩展资料';
COMMENT ON COLUMN customers.created_at IS '创建时间';
COMMENT ON COLUMN customers.updated_at IS '更新时间';

COMMENT ON TABLE appointments IS '预约工单表';
COMMENT ON COLUMN appointments.id IS '预约主键';
COMMENT ON COLUMN appointments.customer_name IS '客户名称快照';
COMMENT ON COLUMN appointments.address IS '服务地址';
COMMENT ON COLUMN appointments.phone IS '联系电话';
COMMENT ON COLUMN appointments.items IS '服务项目列表';
COMMENT ON COLUMN appointments.extra_items IS '额外收费项目列表';
COMMENT ON COLUMN appointments.payment_method IS '付款方式';
COMMENT ON COLUMN appointments.total_amount IS '应收总金额';
COMMENT ON COLUMN appointments.discount_amount IS '折扣金额';
COMMENT ON COLUMN appointments.paid_amount IS '已收金额';
COMMENT ON COLUMN appointments.scheduled_at IS '预约开始时间';
COMMENT ON COLUMN appointments.scheduled_end IS '预约结束时间';
COMMENT ON COLUMN appointments.status IS '预约状态';
COMMENT ON COLUMN appointments.cancel_reason IS '取消原因';
COMMENT ON COLUMN appointments.technician_id IS '指派技师ID';
COMMENT ON COLUMN appointments.technician_name IS '技师名称快照';
COMMENT ON COLUMN appointments.lat IS '纬度';
COMMENT ON COLUMN appointments.lng IS '经度';
COMMENT ON COLUMN appointments.checkin_time IS '签到时间';
COMMENT ON COLUMN appointments.checkout_time IS '签退时间';
COMMENT ON COLUMN appointments.departed_time IS '出发时间';
COMMENT ON COLUMN appointments.completed_time IS '完成时间';
COMMENT ON COLUMN appointments.payment_time IS '确认收款时间';
COMMENT ON COLUMN appointments.photos IS '现场照片列表';
COMMENT ON COLUMN appointments.payment_received IS '是否已确认收款';
COMMENT ON COLUMN appointments.signature_data IS '客户签名数据';
COMMENT ON COLUMN appointments.line_uid IS '关联LINE用户UID';
COMMENT ON COLUMN appointments.zone_id IS '匹配服务区域ID';
COMMENT ON COLUMN appointments.review_public_token IS '公开评价令牌';
COMMENT ON COLUMN appointments.created_at IS '创建时间';
COMMENT ON COLUMN appointments.updated_at IS '更新时间';

COMMENT ON TABLE service_zones IS '服务区域表';
COMMENT ON COLUMN service_zones.id IS '服务区域主键';
COMMENT ON COLUMN service_zones.name IS '服务区域名称';
COMMENT ON COLUMN service_zones.districts IS '行政区列表';
COMMENT ON COLUMN service_zones.assigned_technician_ids IS '分配技师ID列表';
COMMENT ON COLUMN service_zones.created_at IS '创建时间';
COMMENT ON COLUMN service_zones.updated_at IS '更新时间';

COMMENT ON TABLE service_items IS '服务项目表';
COMMENT ON COLUMN service_items.id IS '服务项目主键';
COMMENT ON COLUMN service_items.name IS '服务项目名称';
COMMENT ON COLUMN service_items.default_price IS '默认报价';
COMMENT ON COLUMN service_items.description IS '服务项目描述';
COMMENT ON COLUMN service_items.created_at IS '创建时间';
COMMENT ON COLUMN service_items.updated_at IS '更新时间';

COMMENT ON TABLE extra_items IS '额外收费项表';
COMMENT ON COLUMN extra_items.id IS '额外收费项主键';
COMMENT ON COLUMN extra_items.name IS '额外收费项名称';
COMMENT ON COLUMN extra_items.price IS '额外收费金额';
COMMENT ON COLUMN extra_items.created_at IS '创建时间';
COMMENT ON COLUMN extra_items.updated_at IS '更新时间';

COMMENT ON TABLE cash_ledger_entries IS '现金流水表';
COMMENT ON COLUMN cash_ledger_entries.id IS '现金流水主键';
COMMENT ON COLUMN cash_ledger_entries.technician_id IS '技师ID';
COMMENT ON COLUMN cash_ledger_entries.appointment_id IS '关联预约ID';
COMMENT ON COLUMN cash_ledger_entries.type IS '流水类型';
COMMENT ON COLUMN cash_ledger_entries.amount IS '流水金额';
COMMENT ON COLUMN cash_ledger_entries.note IS '流水备注';
COMMENT ON COLUMN cash_ledger_entries.created_at IS '创建时间';
COMMENT ON COLUMN cash_ledger_entries.updated_at IS '更新时间';

COMMENT ON TABLE reviews IS '评价表';
COMMENT ON COLUMN reviews.id IS '评价主键';
COMMENT ON COLUMN reviews.appointment_id IS '预约ID';
COMMENT ON COLUMN reviews.customer_name IS '客户名称快照';
COMMENT ON COLUMN reviews.technician_id IS '技师ID';
COMMENT ON COLUMN reviews.technician_name IS '技师名称快照';
COMMENT ON COLUMN reviews.rating IS '评分';
COMMENT ON COLUMN reviews.misconducts IS '异常行为标签列表';
COMMENT ON COLUMN reviews.comment IS '评价内容';
COMMENT ON COLUMN reviews.shared_line IS '是否已分享到LINE';
COMMENT ON COLUMN reviews.created_at IS '创建时间';
COMMENT ON COLUMN reviews.updated_at IS '更新时间';

COMMENT ON TABLE notification_logs IS '通知日志表';
COMMENT ON COLUMN notification_logs.id IS '通知日志主键';
COMMENT ON COLUMN notification_logs.appointment_id IS '预约ID';
COMMENT ON COLUMN notification_logs.type IS '通知类型';
COMMENT ON COLUMN notification_logs.message IS '通知内容';
COMMENT ON COLUMN notification_logs.sent_at IS '发送时间';
COMMENT ON COLUMN notification_logs.created_at IS '创建时间';
COMMENT ON COLUMN notification_logs.updated_at IS '更新时间';

COMMENT ON TABLE app_settings IS '系统设置表';
COMMENT ON COLUMN app_settings.key IS '配置键';
COMMENT ON COLUMN app_settings.value IS '配置值';
COMMENT ON COLUMN app_settings.description IS '配置说明';
COMMENT ON COLUMN app_settings.created_at IS '创建时间';
COMMENT ON COLUMN app_settings.updated_at IS '更新时间';

COMMENT ON TABLE auth_tokens IS '认证令牌表';
COMMENT ON COLUMN auth_tokens.id IS '认证令牌主键';
COMMENT ON COLUMN auth_tokens.user_id IS '所属用户ID';
COMMENT ON COLUMN auth_tokens.token IS '认证令牌';
COMMENT ON COLUMN auth_tokens.expires_at IS '过期时间';
COMMENT ON COLUMN auth_tokens.created_at IS '创建时间';
COMMENT ON COLUMN auth_tokens.updated_at IS '更新时间';
