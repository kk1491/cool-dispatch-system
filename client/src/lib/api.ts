import { Appointment, AppointmentCreatePayload, AppointmentUpdatePayload, CashLedgerCreatePayload, CashLedgerEntry, Customer, ExtraItem, LineFriend, NotificationLog, NotificationLogDraft, Review, ReviewDraft, ServiceItem, ServiceZone, User } from '../types';
import { getAppointmentPaymentWriteModel, shouldPreserveLegacyPaymentFieldsOnUpdate } from './appointmentMetrics';

export interface BootstrapPayload {
  users: User[];
  customers: Customer[];
  appointments: Appointment[];
  line_friends: LineFriend[];
  extra_fee_products: ExtraItem[];
  cash_ledger_entries: CashLedgerEntry[];
  zones: ServiceZone[];
  reviews: Review[];
  notification_logs: NotificationLog[];
  service_items: ServiceItem[];
  settings: SettingsPayload;
}

export interface SettingsPayload {
  reminder_days: number;
  webhook?: WebhookSettingsPayload;
}

export interface WebhookSettingsPayload {
  enabled: boolean;
  effective_enabled: boolean;
  url: string;
  url_source: string;
  url_is_public: boolean;
  has_line_channel_secret: boolean;
  status_message: string;
  dependency_summary: string;
}

export type AppDataSnapshot = BootstrapPayload;

export interface ReviewContextPayload {
  appointment: Appointment | null;
  review?: Review | null;
}

export interface DashboardPageData {
  appointments: Appointment[];
  technicians: User[];
  customers: Customer[];
  reviews: Review[];
}

export interface CustomerPageData {
  customers: Customer[];
  appointments: Appointment[];
  reviews: Review[];
}

export interface TechnicianPageData {
  technicians: User[];
  appointments: Appointment[];
  reviews: Review[];
  zones: ServiceZone[];
}

export interface ReminderPageData {
  customers: Customer[];
  appointments: Appointment[];
  settings: SettingsPayload;
}

export interface LinePageData {
  line_friends: LineFriend[];
  customers: Customer[];
}

export interface ZonePageData {
  zones: ServiceZone[];
  technicians: User[];
}

export interface SettingsPageData {
  extra_fee_products: ExtraItem[];
  service_items: ServiceItem[];
  settings: SettingsPayload;
}

export interface FinancialReportPageData {
  appointments: Appointment[];
  technicians: User[];
}

export interface ReviewDashboardPageData {
  reviews: Review[];
  technicians: User[];
  appointments: Appointment[];
}

export interface CashLedgerPageData {
  technicians: User[];
  appointments: Appointment[];
  cash_ledger_entries: CashLedgerEntry[];
}

export const AUTH_REQUIRED_EVENT = 'app:auth-required';

export class AuthRequiredError extends Error {
  status: number;

  constructor(message = '需要先登入', status = 401) {
    super(message);
    this.name = 'AuthRequiredError';
    this.status = status;
  }
}

// API_BASE_URL 默认留空，开发态优先走 Vite `/api` 代理，兼容本地与 Replit 远程预览。
// 只有在显式配置 VITE_API_BASE_URL 时，前端才会改为直连指定后端地址。
const API_BASE_URL = import.meta.env.DEV
  ? (import.meta.env.VITE_API_BASE_URL?.trim() || '')
  : '';

// buildApiUrl 统一拼接开发态绝对地址和生产态相对地址，确保跨域请求与同域部署都能正常工作。
export function buildApiUrl(path: string): string {
  return `${API_BASE_URL}${path}`;
}

function isAuthRequiredMessage(message: string): boolean {
  const trimmed = message.trim();
  return trimmed === '需要先登入' ||
    trimmed === '尚未登入' ||
    trimmed === '登入憑證無效或已過期';
}

function notifyAuthRequired(message: string): void {
  if (typeof window === 'undefined') {
    return;
  }
  window.dispatchEvent(new CustomEvent(AUTH_REQUIRED_EVENT, {
    detail: { message },
  }));
}

// requestJSON 统一处理 JSON 请求、错误消息和跨域凭据配置，避免每个业务请求重复样板代码。
export async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(buildApiUrl(path), {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    ...init,
  });

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;
    try {
      const body = await response.json();
      if (body?.message) {
        message = body.message;
      }
    } catch {
      // 保持默认错误消息即可，不再额外抛出解析异常。
    }
    if (response.status === 401 && isAuthRequiredMessage(message)) {
      notifyAuthRequired(message);
      throw new AuthRequiredError(message, response.status);
    }
    throw new Error(message);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

export function fetchBootstrap(): Promise<BootstrapPayload> {
  return requestJSON<BootstrapPayload>('/api/bootstrap');
}

export function fetchAppointments(): Promise<Appointment[]> {
  return requestJSON<Appointment[]>('/api/appointments');
}

export function fetchTechnicians(): Promise<User[]> {
  return requestJSON<User[]>('/api/technicians');
}

export function fetchCustomers(): Promise<Customer[]> {
  return requestJSON<Customer[]>('/api/customers');
}

export function fetchZones(): Promise<ServiceZone[]> {
  return requestJSON<ServiceZone[]>('/api/zones');
}

export function fetchServiceItems(): Promise<ServiceItem[]> {
  return requestJSON<ServiceItem[]>('/api/service-items');
}

export function fetchExtraItems(): Promise<ExtraItem[]> {
  return requestJSON<ExtraItem[]>('/api/extra-items');
}

export function fetchCashLedgerEntries(): Promise<CashLedgerEntry[]> {
  return requestJSON<CashLedgerEntry[]>('/api/cash-ledger');
}

export function fetchReviews(): Promise<Review[]> {
  return requestJSON<Review[]>('/api/reviews');
}

export function fetchNotificationLogs(): Promise<NotificationLog[]> {
  return requestJSON<NotificationLog[]>('/api/notifications');
}

export function fetchSettings(): Promise<SettingsPayload> {
  return requestJSON<SettingsPayload>('/api/settings');
}

export async function fetchAppSnapshot(): Promise<AppDataSnapshot> {
  const [
    users,
    customers,
    appointments,
    lineFriends,
    extraFeeProducts,
    cashLedgerEntries,
    zones,
    reviews,
    notificationLogs,
    serviceItems,
    settings,
  ] = await Promise.all([
    fetchTechnicians(),
    fetchCustomers(),
    fetchAppointments(),
    fetchLineData(),
    fetchExtraItems(),
    fetchCashLedgerEntries(),
    fetchZones(),
    fetchReviews(),
    fetchNotificationLogs(),
    fetchServiceItems(),
    fetchSettings(),
  ]);

  return {
    users,
    customers,
    appointments,
    line_friends: lineFriends,
    extra_fee_products: extraFeeProducts,
    cash_ledger_entries: cashLedgerEntries,
    zones,
    reviews,
    notification_logs: notificationLogs,
    service_items: serviceItems,
    settings,
  };
}

// fetchReviewContext 为公开评价页提供最小读取集合，统一按随机评价令牌读取，避免外链暴露自增预约 ID。
export function fetchReviewContext(reviewToken: string): Promise<ReviewContextPayload> {
  return requestJSON<ReviewContextPayload>(`/api/reviews/token/${encodeURIComponent(reviewToken)}/context`);
}

// fetchLineData 供 LINE 管理頁直接讀取專用資料來源，避免該頁只依賴 bootstrap 附帶欄位。
export function fetchLineData(): Promise<LineFriend[]> {
  return requestJSON<LineFriend[]>('/api/line-data');
}

// fetchDashboardPageData 首批改走后端页面级读模型，避免首页每次进入都在前端并行拼装四份资源。
export function fetchDashboardPageData(): Promise<DashboardPageData> {
  return requestJSON<DashboardPageData>('/api/dashboard-page-data');
}

// fetchCustomerPageData 直接命中顧客頁聚合接口，減少客戶/預約/評價三路請求編排。
export function fetchCustomerPageData(): Promise<CustomerPageData> {
  return requestJSON<CustomerPageData>('/api/customer-page-data');
}

// fetchTechnicianPageData 改走後端聚合接口，避免技師頁刷新時四路資料快照不同步，
// 造成結案統計、收款提示與區域/評價資料短暫對不上。
export function fetchTechnicianPageData(): Promise<TechnicianPageData> {
  return requestJSON<TechnicianPageData>('/api/technician-page-data');
}

// fetchReminderPageData 改走後端聚合接口，讓客戶、工單與提醒設定使用同一份後端快照，
// 避免回訪頁刷新時三路請求的時序漂移導致統計不同步。
export function fetchReminderPageData(): Promise<ReminderPageData> {
  return requestJSON<ReminderPageData>('/api/reminder-page-data');
}

// fetchLinePageData 由后端一次返回好友和客户映射，避免页面继续混用多个读取源。
export function fetchLinePageData(): Promise<LinePageData> {
  return requestJSON<LinePageData>('/api/line-page-data');
}

// fetchZonePageData 改走後端聚合接口，避免區域與技師列表多路請求產生不同步快照。
export function fetchZonePageData(): Promise<ZonePageData> {
  return requestJSON<ZonePageData>('/api/zone-page-data');
}

// fetchSettingsPageData 首批改走设置页专用 GET，后端统一返回服务项、额外费用和提醒配置。
export function fetchSettingsPageData(): Promise<SettingsPageData> {
  return requestJSON<SettingsPageData>('/api/settings-page-data');
}

// fetchFinancialReportPageData 直接讀取後端財務頁聚合結果，避免財務頁刷新時工單與技師列表時序漂移。
export function fetchFinancialReportPageData(): Promise<FinancialReportPageData> {
  return requestJSON<FinancialReportPageData>('/api/financial-report-page-data');
}

// fetchReviewDashboardPageData 改走後端聚合接口，讓評價、技師與工單使用同一份後端快照，
// 避免評價看板刷新時三路請求的時序漂移導致統計與歸屬對不上。
export function fetchReviewDashboardPageData(): Promise<ReviewDashboardPageData> {
  return requestJSON<ReviewDashboardPageData>('/api/review-dashboard-page-data');
}

// fetchCashLedgerPageData 改走後端聚合接口，讓技師、工單與現金流水使用同一份後端快照，
// 降低現金帳與技師頁切換時出現統計/提示不同步的窗口。
export function fetchCashLedgerPageData(): Promise<CashLedgerPageData> {
  return requestJSON<CashLedgerPageData>('/api/cash-ledger-page-data');
}

export function login(phone: string, password: string): Promise<{ user: User }> {
  return requestJSON<{ user: User }>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ phone, password }),
  });
}

// fetchAuthMe 通过 cookie 中的 token 恢复登录态，页面刷新/服务重启后自动恢复用户身份。
// token 剩余有效期不足 29 天时，后端自动续期并刷新 cookie。
export async function fetchAuthMe(): Promise<User | null> {
  try {
    const data = await requestJSON<{ user: User }>('/api/auth/me');
    return data.user;
  } catch (error) {
    if (error instanceof AuthRequiredError) {
      return null;
    }
    // token 不存在、已过期或无效时静默返回 null，让前端显示登录页。
    return null;
  }
}

// logoutRequest 调用后端注销接口，删除数据库中的 token 并清除 cookie。
export function logoutRequest(): Promise<{ message: string }> {
  return requestJSON<{ message: string }>('/api/auth/logout', {
    method: 'POST',
  });
}

// createAppointment 仅接收创建态字段，防止前端把临时主键或创建时间误传给后端。
export function createAppointment(payload: AppointmentCreatePayload): Promise<Appointment> {
  return requestJSON<Appointment>('/api/appointments', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

// toAppointmentUpdatePayload 统一把读模型裁剪成写模型，避免严格 JSON 接口因多余字段返回 400。
// payment_time 属于服务端派生字段，确认收款时由后端沿用旧值或自动补齐，客户端不再透传。
// 支付相关字段统一委托 getAppointmentPaymentWriteModel 归一，避免不同更新入口各自拼装后继续漂移。
export function toAppointmentUpdatePayload(payload: Appointment): AppointmentUpdatePayload {
  const basePayload: AppointmentUpdatePayload = {
    customer_name: payload.customer_name,
    address: payload.address,
    phone: payload.phone,
    items: payload.items,
    extra_items: (payload.extra_items ?? []).map(({ id, name, price }) => ({ id, name, price })),
    discount_amount: payload.discount_amount ?? 0,
    scheduled_at: payload.scheduled_at,
    scheduled_end: payload.scheduled_end,
    status: payload.status,
    cancel_reason: payload.cancel_reason,
    technician_id: payload.technician_id,
    lat: payload.lat,
    lng: payload.lng,
    checkin_time: payload.checkin_time,
    checkout_time: payload.checkout_time,
    departed_time: payload.departed_time,
    completed_time: payload.completed_time,
    photos: payload.photos ?? [],
    signature_data: payload.signature_data,
    line_uid: payload.line_uid,
  };

  // 旧资料若仍停留在 legacy `未收款` 占位值，普通编辑先不透传 payment_* 字段，
  // 让后端沿用原记录，直到技师确认收款或管理员显式补录真实付款方式。
  if (shouldPreserveLegacyPaymentFieldsOnUpdate(payload)) {
    return basePayload;
  }

  const paymentWriteModel = getAppointmentPaymentWriteModel(payload);

  return {
    ...basePayload,
    payment_method: paymentWriteModel.payment_method,
    paid_amount: paymentWriteModel.paid_amount,
    payment_received: paymentWriteModel.payment_received,
  };
}

// updateAppointment 只接收预约主键与写 DTO，避免调用方继续把读模型直接传给 API 层。
export function updateAppointment(id: number, payload: AppointmentUpdatePayload): Promise<Appointment> {
  return requestJSON<Appointment>(`/api/appointments/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export function deleteAppointment(id: number): Promise<{ deleted: boolean }> {
  return requestJSON<{ deleted: boolean }>(`/api/appointments/${id}`, {
    method: 'DELETE',
  });
}

// TechnicianWithPassword 扩展 User 类型，携带可选密码字段用于新增或修改技师密码。
export interface TechnicianWithPassword extends User {
  password?: string;
}

export function replaceTechnicians(payload: (User | TechnicianWithPassword)[]): Promise<User[]> {
  return requestJSON<User[]>('/api/technicians', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

// updateTechnicianPassword 独立修改指定技师的登录密码，同时吊销其所有旧令牌。
export function updateTechnicianPassword(techId: number, password: string): Promise<{ message: string }> {
  return requestJSON<{ message: string }>(`/api/technicians/${techId}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  });
}

export function replaceZones(payload: ServiceZone[]): Promise<ServiceZone[]> {
  return requestJSON<ServiceZone[]>('/api/zones', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function replaceServiceItems(payload: ServiceItem[]): Promise<ServiceItem[]> {
  return requestJSON<ServiceItem[]>('/api/service-items', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function replaceExtraItems(payload: ExtraItem[]): Promise<ExtraItem[]> {
  return requestJSON<ExtraItem[]>('/api/extra-items', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function createCashLedgerEntry(payload: CashLedgerCreatePayload): Promise<CashLedgerEntry> {
  return requestJSON<CashLedgerEntry>('/api/cash-ledger', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function createReview(reviewToken: string, payload: ReviewDraft): Promise<Review> {
  return requestJSON<Review>(`/api/reviews/token/${encodeURIComponent(reviewToken)}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

// updateReviewShareLine 把客户在评价完成页的分享动作真实回写到后端。
export function updateReviewShareLine(reviewToken: string, sharedLine: boolean): Promise<Review> {
  return requestJSON<Review>(`/api/reviews/token/${encodeURIComponent(reviewToken)}/share-line`, {
    method: 'PATCH',
    body: JSON.stringify({ shared_line: sharedLine }),
  });
}

export function createNotificationLog(payload: NotificationLogDraft): Promise<NotificationLog> {
  return requestJSON<NotificationLog>('/api/notifications', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateReminderDays(reminderDays: number): Promise<{ reminder_days: number }> {
  return requestJSON<{ reminder_days: number }>('/api/settings/reminder-days', {
    method: 'PUT',
    body: JSON.stringify({ reminder_days: reminderDays }),
  });
}

// updateWebhookEnabled 更新管理员持久化的 webhook 开关，不会直接改动服务端环境变量。
export function updateWebhookEnabled(enabled: boolean): Promise<WebhookSettingsPayload> {
  return requestJSON<WebhookSettingsPayload>('/api/settings/webhook-enabled', {
    method: 'PUT',
    body: JSON.stringify({ enabled }),
  });
}

// replaceCustomers 批量更新客户资料到后端，与师傅/区域等 replace 接口保持一致。
export function replaceCustomers(payload: Customer[]): Promise<Customer[]> {
  return requestJSON<Customer[]>('/api/customers', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

// deleteCustomer 删除指定客户。
export function deleteCustomer(id: string): Promise<{ deleted: boolean }> {
  return requestJSON<{ deleted: boolean }>(`/api/customers/${id}`, {
    method: 'DELETE',
  });
}

// linkLineFriendCustomer 维护 LINE 好友与客户的绑定关系，传 null 表示解绑。
export function linkLineFriendCustomer(lineUid: string, customerId: string | null): Promise<LineFriend> {
  return requestJSON<LineFriend>(`/api/line-friends/${encodeURIComponent(lineUid)}/customer`, {
    method: 'PUT',
    body: JSON.stringify({ customer_id: customerId }),
  });
}

// ---------- Cloudflare Images 图床相关 API ----------

// ImageUploadResult 是图片上传成功后的响应结构。
export interface ImageUploadResult {
  // id 是 Cloudflare Images 分配的唯一图片标识。
  id: string;
  // url 是图片的公开访问地址。
  url: string;
}

// uploadImage 将图片文件上传到 Cloudflare Images 图床。
// 使用 FormData/multipart 格式发送，不走 requestJSON 的 JSON Content-Type。
export async function uploadImage(file: File): Promise<ImageUploadResult> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch(buildApiUrl('/api/upload/image'), {
    method: 'POST',
    credentials: 'include',
    body: formData,
    // 注意：不设置 Content-Type，让浏览器自动设置 multipart/form-data 并附带 boundary。
  });

  if (!response.ok) {
    let message = `Upload failed with status ${response.status}`;
    try {
      const body = await response.json();
      if (body?.message) {
        message = body.message;
      }
    } catch {
      // 保持默认错误消息即可。
    }
    if (response.status === 401) {
      notifyAuthRequired(message);
      throw new AuthRequiredError(message, response.status);
    }
    throw new Error(message);
  }

  return response.json() as Promise<ImageUploadResult>;
}

// deleteImage 从 Cloudflare Images 图床删除指定图片。
// 传入图片的公开访问 URL，后端会自动提取图片 ID 进行删除。
export function deleteImage(imageUrl: string): Promise<{ deleted: boolean }> {
  return requestJSON<{ deleted: boolean }>('/api/upload/image', {
    method: 'DELETE',
    body: JSON.stringify({ url: imageUrl }),
  });
}

// ============================================================================
// PAYUNi 支付相关 API
// ============================================================================

// ---------- 管理员：创建支付订单请求体 ----------
export interface CreatePaymentOrderRequest {
  trade_amt: number;
  prod_desc: string;
  customer_name: string;
  payment_method?: string;   // credit / atm / both，默认 both
  customer_email?: string;
  customer_phone?: string;
  appointment_id: number;
}

// ---------- 管理员：创建支付订单返回体 ----------
export interface CreatePaymentOrderResponse {
  order: any;
  payment_token: string;
  payment_url: string;
}

// createPaymentOrder 管理员创建支付订单，返回支付链接（需登录）。
export function createPaymentOrder(
  payload: CreatePaymentOrderRequest
): Promise<CreatePaymentOrderResponse> {
  return requestJSON<CreatePaymentOrderResponse>('/api/payment/orders', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

// PaymentOrderRecord 管理员支付管理页使用的支付订单读模型。
// 保持字段显式声明，避免页面继续依赖 any 访问状态和关联预约。
export interface PaymentOrderRecord {
  id: number;
  payment_token: string;
  mer_trade_no: string;
  trade_amt: number;
  prod_desc: string;
  payment_method: string;
  customer_name: string;
  customer_email?: string;
  customer_phone?: string;
  appointment_id?: number;
  status: string;
  trade_no?: string;
  trade_status?: string;
  pay_no?: string;
  atm_expire_date?: string;
  auth_code?: string;
  card_6_no?: string;
  card_4_no?: string;
  res_code?: string;
  res_code_msg?: string;
  paid_at?: string;
  created_at: string;
  updated_at?: string;
}

// listPaymentOrders 管理员查看所有支付订单记录（需登录）。
export function listPaymentOrders(): Promise<PaymentOrderRecord[]> {
  return requestJSON<PaymentOrderRecord[]>('/api/payment/orders');
}

// ---------- 客户公开：订单信息（无需登录，凭 Token） ----------
export interface PaymentOrderInfo {
  trade_amt: number;
  prod_desc: string;
  payment_method: string;
  customer_name: string;
  status: string;
  mer_trade_no: string;
  res_code_msg?: string;
  pay_no?: string;
  atm_expire_date?: string;
  appointment_id?: number;
}

// getPaymentOrderByToken 客户凭支付令牌查看订单信息。
export function getPaymentOrderByToken(payToken: string): Promise<PaymentOrderInfo> {
  return requestJSON<PaymentOrderInfo>(`/api/payment/token/${payToken}`);
}

// ---------- 客户公开：信用卡支付请求体 ----------
export interface TokenCreditPayRequest {
  card_no: string;
  card_expired: string;   // MMYY
  card_cvc: string;
  card_inst?: string;     // 1=一次付清
}

// tokenCreditPay 客户凭支付令牌发起信用卡支付。
export function tokenCreditPay(
  payToken: string,
  payload: TokenCreditPayRequest
): Promise<any> {
  return requestJSON(`/api/payment/token/${payToken}/credit`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

// tokenATMPay 客户凭支付令牌发起 ATM 虚拟帐号取号。
export function tokenATMPay(
  payToken: string,
  bankType: string
): Promise<any> {
  return requestJSON(`/api/payment/token/${payToken}/atm`, {
    method: 'POST',
    body: JSON.stringify({ bank_type: bankType }),
  });
}
