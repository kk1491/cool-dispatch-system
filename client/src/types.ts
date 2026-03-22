export interface User {
  id: number;
  name: string;
  role: 'admin' | 'technician';
  phone: string;
  color?: string;
  skills?: ACType[];
  zone_id?: string;
  availability?: {
    day: number;
    slots: string[];
  }[];
}

export type ACType = string;

export interface ServiceItem {
  id: string;
  name: string;
  default_price: number;
  description?: string;
}

export interface ACUnit {
  id: string;
  type: string;
  note: string;
  price: number;
}

export interface ExtraItem {
  id: string;
  name: string;
  price: number;
}

// AppointmentWritablePaymentMethod 代表当前仍允许写回后端的真实付款方式集合；
// 新建、编辑、技师补录等写路径都必须收敛到这三个值，不能再把 legacy 占位值继续扩散。
export type AppointmentWritablePaymentMethod = '現金' | '轉帳' | '無收款';

// AppointmentReadablePaymentMethod 同时兼容历史脏数据中的 `未收款`，
// 让前端在读取旧资料、统计筛选、补录修复时仍能识别该异常值。
export type AppointmentReadablePaymentMethod = AppointmentWritablePaymentMethod | '未收款';

// 下面两个别名继续保留，避免现有页面批量改名时中断构建；
// 语义上分别对应“可写付款方式”和“可读付款方式”。
export type StandardPaymentMethod = AppointmentWritablePaymentMethod;
export type PaymentMethod = AppointmentReadablePaymentMethod;

export interface Customer {
  id: string;
  name: string;
  phone: string;
  address: string;
  line_id?: string;
  line_name?: string;
  line_picture?: string;
  line_uid?: string;
  line_joined_at?: string;
  line_data?: Record<string, unknown>;
  created_at: string;
}

export interface Appointment {
  id: number;
  customer_name: string;
  address: string;
  phone: string;
  items: ACUnit[];
  extra_items: ExtraItem[];
  payment_method: AppointmentReadablePaymentMethod;
  total_amount: number;
  discount_amount?: number;
  paid_amount?: number;
  scheduled_at: string;
  scheduled_end?: string;
  status: 'pending' | 'assigned' | 'arrived' | 'completed' | 'cancelled';
  cancel_reason?: string;
  technician_id?: number;
  technician_name?: string;
  lat?: number;
  lng?: number;
  checkin_time?: string;
  checkout_time?: string;
  departed_time?: string;
  completed_time?: string;
  payment_time?: string;
  photos: string[];
  payment_received?: boolean;
  signature_data?: string;
  created_at?: string;
  line_uid?: string;
  zone_id?: string;
  review_token?: string;
}

// AppointmentCreatePayload 显式声明创建预约可写字段，避免把主键、创建时间、作业时间戳等读模型字段混入创建请求。
export interface AppointmentCreatePayload {
  customer_name: string;
  address: string;
  phone: string;
  items: ACUnit[];
  extra_items: ExtraItem[];
  payment_method: AppointmentWritablePaymentMethod;
  discount_amount?: number;
  scheduled_at: string;
  scheduled_end?: string;
  technician_id?: number;
  line_uid?: string;
}

// AppointmentUpdatePayload 显式约束预约更新接口允许写入的字段。
// 前端写模型必须始终收敛到真实付款方式，不能再把 legacy `未收款` 当成可写值继续扩散；
// 历史旧资料的兼容与兜底交给后端按 existing 记录处理。
export interface AppointmentUpdatePayload {
  customer_name: string;
  address: string;
  phone: string;
  items: ACUnit[];
  extra_items: ExtraItem[];
  // payment_method / paid_amount / payment_received 允许在更新时按场景省略。
  // 旧资料若尚未补录真实付款方式，普通编辑只提交非支付字段，后端沿用既有支付状态；
  // 一旦前端显式确认收款或改成真实付款方式，再回写归一化后的写模型。
  payment_method?: AppointmentWritablePaymentMethod;
  discount_amount?: number;
  paid_amount?: number;
  scheduled_at: string;
  scheduled_end?: string;
  status: 'pending' | 'assigned' | 'arrived' | 'completed' | 'cancelled';
  cancel_reason?: string;
  technician_id?: number;
  lat?: number;
  lng?: number;
  checkin_time?: string;
  checkout_time?: string;
  departed_time?: string;
  completed_time?: string;
  photos: string[];
  payment_received?: boolean;
  signature_data?: string;
  line_uid?: string;
}

export interface ServiceZone {
  id: string;
  name: string;
  districts: string[];
  assigned_technician_ids: number[];
}

export interface CashLedgerEntry {
  id: string;
  technician_id: number;
  appointment_id?: number;
  type: 'collect' | 'return';
  amount: number;
  note: string;
  created_at: string;
}

// CashLedgerCreatePayload 仅保留现金账新增接口允许提交的字段，主键与时间戳统一由后端生成或兜底。
export interface CashLedgerCreatePayload {
  technician_id: number;
  appointment_id?: number;
  type: 'collect' | 'return';
  amount: number;
  note: string;
  created_at?: string;
}

export interface CustomerReminder {
  id: string;
  customer_id: string;
  last_service_date: string;
  remind_after_days: number;
  reminded: boolean;
  created_at: string;
}

export type MisconductType =
  | 'private_contact'
  | 'not_clean'
  | 'bad_attitude'
  | 'late_arrival'
  | 'damage_property'
  | 'overcharge'
  | 'other';

export interface Review {
  id: string;
  appointment_id: number;
  customer_name: string;
  technician_id?: number;
  technician_name?: string;
  rating: 1 | 2 | 3 | 4 | 5;
  misconducts: MisconductType[];
  comment: string;
  created_at: string;
  shared_line?: boolean;
}

// ReviewDraft 用于评价提交请求；由后端负责补齐真实主键与创建时间。
export interface ReviewDraft {
  rating: 1 | 2 | 3 | 4 | 5;
  misconducts: MisconductType[];
  comment: string;
  shared_line?: boolean;
}

export interface LineFriend {
  line_uid: string;
  line_name: string;
  line_picture: string;
  joined_at: string;
  line_joined_at?: string;
  phone?: string;
  linked_customer_id?: string;
  status?: string;
  last_payload?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
}

export interface NotificationLog {
  id: string;
  appointment_id: number;
  type: 'line' | 'sms';
  message: string;
  sent_at: string;
}

// NotificationLogDraft 用于通知发送请求；由后端生成真实 id / sent_at。
export interface NotificationLogDraft {
  appointment_id: number;
  type: 'line' | 'sms';
  message: string;
}
