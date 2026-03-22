import { Appointment, AppointmentReadablePaymentMethod, AppointmentWritablePaymentMethod, StandardPaymentMethod } from '../types';

export type PaymentCollectionLabel = '無收款' | '未收款' | '已收款';
export type PaymentMethodLabel = '現金' | '轉帳' | '無收款' | '待補收款方式（舊資料）';
export type BackfillPaymentMethodOption = Exclude<StandardPaymentMethod, '無收款'>;
export type AppointmentPaymentWriteModel = {
  payment_method: AppointmentWritablePaymentMethod;
  payment_received: boolean;
  paid_amount: number;
};

export const TECHNICIAN_CLOSED_RECORDS_TITLE = '結案紀錄';
export const TECHNICIAN_CLOSED_RECORDS_SUBTITLE = '收款與結案口徑';
export const TECHNICIAN_CLOSED_RECORDS_EMPTY_TEXT = '本月尚無結案紀錄';
export const CASH_LEDGER_TITLE = '現金帳務管理';
export const CASH_LEDGER_ENTRY_TITLE = '現金帳明細';
export const CASH_LEDGER_OPEN_BUTTON_LABEL = '現金帳';
export const CASH_LEDGER_ADD_RETURN_LABEL = '新增回繳';
export const CASH_LEDGER_ADD_RETURN_TITLE = '新增回繳紀錄';
export const CASH_LEDGER_RETURN_AMOUNT_LABEL = '回繳金額';
export const CASH_LEDGER_RETURN_NOTE_PLACEHOLDER = '例：本週回繳';
export const CASH_LEDGER_RETURN_DEFAULT_NOTE = '回繳現金';
export const CASH_LEDGER_RETURN_SUCCESS_MESSAGE = '回繳紀錄已新增';
export const CASH_LEDGER_RETURN_FAILURE_MESSAGE = '新增回繳紀錄失敗';

// PAYMENT_REVENUE_SCOPE_NOTE 统一定义技师页与现金账共用的统计口径说明，
// 避免两个页面各自维护文案后再次出现“金额一致但提示不一致”的体验偏差。
export const PAYMENT_REVENUE_SCOPE_NOTE = '總實收只統計已結案且已確認收款的工單；現金帳只追蹤其中的現金實收與回繳。';
export const TECHNICIAN_EARNINGS_SCOPE_NOTE = `${PAYMENT_REVENUE_SCOPE_NOTE} 未收款、轉帳與無收款請在本頁核對。`;
export const CASH_LEDGER_SCOPE_NOTE = `${PAYMENT_REVENUE_SCOPE_NOTE} 轉帳、未收款與無收款不會進入現金帳餘額，請回技師頁「結案紀錄」核對。`;
export const LEGACY_UNCOLLECTED_PAYMENT_METHOD = '未收款';
export const LEGACY_PAYMENT_METHOD_LABEL: PaymentMethodLabel = '待補收款方式（舊資料）';
export const CASH_PAYMENT_METHOD = '現金';
export const TRANSFER_PAYMENT_METHOD = '轉帳';
export const NO_CHARGE_PAYMENT_METHOD = '無收款';
export const PAYMENT_COLLECTION_FILTER_OPTIONS: readonly PaymentCollectionLabel[] = ['未收款', '已收款', '無收款'];
export const BACKFILL_PAYMENT_METHOD_OPTIONS: readonly BackfillPaymentMethodOption[] = ['現金', '轉帳'];

// STANDARD_PAYMENT_METHODS 统一维护当前仍允许人工选择的真实付款方式，
// 让编辑页、技师补录入口和其它写路径共用同一组来源，避免 legacy 值继续扩散。
export const STANDARD_PAYMENT_METHODS: readonly StandardPaymentMethod[] = ['現金', '轉帳', '無收款'];

// isClosedAppointment 统一定义「已结案」口径，避免不同页面对 completed/cancelled 的统计含义继续漂移。
export function isClosedAppointment(appt: Appointment): boolean {
  return appt.status === 'completed' || appt.status === 'cancelled';
}

// getAppointmentClosedAt 优先返回真正的结案时间，避免技师页继续按预约时间归月，
// 造成跨月完工工单在统计与现金账核对时落到不同月份。
export function getAppointmentClosedAt(appt: Appointment): string {
  return appt.payment_time || appt.checkout_time || appt.completed_time || appt.scheduled_at;
}

// getAppointmentClosedMonthKey 把结案口径统一映射成 `yyyy-MM` 字符串，供月报与技师页复用。
export function getAppointmentClosedMonthKey(appt: Appointment): string {
  return getAppointmentClosedAt(appt).slice(0, 7);
}

// getPaymentMethodFilterOptions 统一生成财务页等筛选器可用的付款方式列表，
// 避免页面继续手写「現金 / 轉帳 / 無收款 / 舊資料」数组后与展示口径脱节。
export function getPaymentMethodFilterOptions(hasLegacyPaymentMethod: boolean): readonly PaymentMethodLabel[] {
  return hasLegacyPaymentMethod
    ? [...STANDARD_PAYMENT_METHODS, LEGACY_PAYMENT_METHOD_LABEL]
    : STANDARD_PAYMENT_METHODS;
}

// normalizeReadablePaymentMethod 让前端读路径也兼容后端已经支持的旧别名，
// 避免现金账、财务页、技师页继续各自拿原始 payment_method 字符串做判断。
export function normalizeReadablePaymentMethod(method: AppointmentReadablePaymentMethod | string): AppointmentReadablePaymentMethod | string {
  const trimmed = String(method).trim();
  switch (trimmed.toLowerCase()) {
    case LEGACY_UNCOLLECTED_PAYMENT_METHOD:
      return LEGACY_UNCOLLECTED_PAYMENT_METHOD;
    case CASH_PAYMENT_METHOD:
    case '现金':
    case 'cash':
      return CASH_PAYMENT_METHOD;
    case TRANSFER_PAYMENT_METHOD:
    case '转账':
    case 'transfer':
    case 'bank_transfer':
    case 'bank transfer':
      return TRANSFER_PAYMENT_METHOD;
    case NO_CHARGE_PAYMENT_METHOD:
    case '无需收款':
    case 'no_charge':
    case 'no charge':
      return NO_CHARGE_PAYMENT_METHOD;
    default:
      return trimmed;
  }
}

// isLegacyUncollectedAppointment 兼容历史上把“未收款”写进 payment_method 的旧数据。
// 这类工单本质上仍属于“可收款但未收款”，不能被误当成「無收款」免收费单。
export function isLegacyUncollectedAppointment(appt: Appointment): boolean {
  return normalizeReadablePaymentMethod(appt.payment_method) === LEGACY_UNCOLLECTED_PAYMENT_METHOD;
}

// isStandardPaymentMethod 统一识别是否为当前允许写入的真实付款方式，
// 方便表单把历史占位值作为只读兼容处理，而不是继续当作正常选项透传。
export function isStandardPaymentMethod(method: AppointmentReadablePaymentMethod): method is StandardPaymentMethod {
  return STANDARD_PAYMENT_METHODS.includes(method as StandardPaymentMethod);
}

// getWritablePaymentMethod 把读模型中的 legacy 占位值收敛成真实写值。
// 默认回退到 `現金`，用于普通编辑/同步场景先阻止 `未收款` 继续扩散；
// 技师补录或显式选择时可以传入更准确的 fallback 覆盖默认值。
export function getWritablePaymentMethod(
  method: AppointmentReadablePaymentMethod,
  fallback: AppointmentWritablePaymentMethod = CASH_PAYMENT_METHOD,
): AppointmentWritablePaymentMethod {
  const normalized = normalizeReadablePaymentMethod(method);
  return isStandardPaymentMethod(normalized as AppointmentReadablePaymentMethod)
    ? (normalized as StandardPaymentMethod)
    : fallback;
}

// getAppointmentPaymentWriteModel 统一把预约读模型收敛成支付写模型，
// 避免编辑页、技师页、照片上传等不同更新入口各自拼 payment_* 字段后继续漂移。
export function getAppointmentPaymentWriteModel(appt: Appointment): AppointmentPaymentWriteModel {
  // 所有写路径都必须落到真实付款方式，不能再把 legacy `未收款` 原样写回后端。
  // 若调用方尚未显式补录旧资料，这里统一按 fallback 收敛成 `現金`，保证写模型规范化；
  // 技师确认收款场景会先把 payment_method 改成用户所选真实值，再由这里透传。
  const payment_method = getWritablePaymentMethod(appt.payment_method);
  const payment_received = payment_method === '無收款' ? false : Boolean(appt.payment_received);
  const paid_amount = payment_method === '無收款' || !payment_received
    ? 0
    : (appt.paid_amount ?? getChargeableAmount(appt));

  return {
    payment_method,
    payment_received,
    paid_amount,
  };
}

// shouldPreserveLegacyPaymentFieldsOnUpdate 用来标记“旧资料尚未补录付款方式”的普通编辑场景。
// 这类更新只应提交非支付字段，避免前端把读模型中的 legacy 值强行收敛成 `現金`
// 后造成「只是改地址/照片，却顺手改掉付款方式」的隐性副作用。
export function shouldPreserveLegacyPaymentFieldsOnUpdate(appt: Appointment): boolean {
  return isLegacyUncollectedAppointment(appt) && !Boolean(appt.payment_received);
}

// normalizeAppointmentForWrite 在进入任意更新接口前，把预约读模型中的支付字段显式收敛成写模型。
// 这样编辑页、技师页、照片上传等入口即使继续传 Appointment 对象，也不会把 legacy `未收款` 带入写接口边界。
export function normalizeAppointmentForWrite(appt: Appointment): Appointment {
  const paymentWriteModel = getAppointmentPaymentWriteModel(appt);
  return {
    ...appt,
    payment_method: paymentWriteModel.payment_method,
    payment_received: paymentWriteModel.payment_received,
    paid_amount: paymentWriteModel.paid_amount,
  };
}

// shouldBackfillPaymentMethod 明确标记哪些工单仍需要人工补录真实付款方式，
// 避免技师页、编辑页各自手写 `payment_method === 未收款` 判断后继续漂移。
export function shouldBackfillPaymentMethod(appt: Appointment): boolean {
  return isLegacyUncollectedAppointment(appt) && isCollectibleAppointment(appt) && !Boolean(appt.payment_received);
}

// isChargeExemptAppointment 统一识别「無收款」工单；这类工单允许直接结案，但不产生真实收款或未收余额。
export function isChargeExemptAppointment(appt: Appointment): boolean {
  return normalizeReadablePaymentMethod(appt.payment_method) === NO_CHARGE_PAYMENT_METHOD;
}

// isPaymentSettled 统一识别支付是否已确认；無收款工单按业务约束直接视为已确认。
// 注意：这里保留“已确认即可结案”的技师流程语义，不把部分收款的未收余额混进流程判断。
export function isPaymentSettled(appt: Appointment): boolean {
  return isChargeExemptAppointment(appt) || Boolean(appt.payment_received);
}

// isCollectibleAppointment 统一识别「可收款」工单；無收款不计入应收、实收、未收余额。
export function isCollectibleAppointment(appt: Appointment): boolean {
  return !isChargeExemptAppointment(appt);
}

// getPaymentCollectionLabel 统一把收款口径映射成页面可读标签。
// 这里优先表达“無收款 / 未收款 / 已收款”业务状态，避免不同页面再各自猜测展示文案。
export function getPaymentCollectionLabel(appt: Appointment): PaymentCollectionLabel {
  if (isChargeExemptAppointment(appt)) {
    return '無收款';
  }
  if (isCollectedAppointment(appt) && getOutstandingAmount(appt) === 0) {
    return '已收款';
  }
  return '未收款';
}

// getPaymentCollectionBadgeClass 统一返回收款状态标签样式，避免财务页、技师页各自复制颜色映射后继续漂移。
export function getPaymentCollectionBadgeClass(label: PaymentCollectionLabel): string {
  switch (label) {
    case '已收款':
      return 'bg-emerald-50 text-emerald-700';
    case '未收款':
      return 'bg-rose-50 text-rose-600';
    default:
      return 'bg-slate-100 text-slate-500';
  }
}

// getPaymentMethodLabel 统一把付款方式映射成页面展示文案。
// 历史脏数据会把「未收款」误写进 payment_method，这里明确标成待补录旧资料，
// 避免页面把它当成真实付款方式，同时保留人工修正入口。
export function getPaymentMethodLabel(appt: Appointment): PaymentMethodLabel {
  if (isLegacyUncollectedAppointment(appt)) {
    return LEGACY_PAYMENT_METHOD_LABEL;
  }
  const normalized = normalizeReadablePaymentMethod(appt.payment_method);
  if (isStandardPaymentMethod(normalized as AppointmentReadablePaymentMethod)) {
    return normalized as PaymentMethodLabel;
  }
  return LEGACY_PAYMENT_METHOD_LABEL;
}

// getPaymentMethodBadgeClass 统一返回付款方式标签样式，避免财务页、技师页继续各写一套颜色规则。
export function getPaymentMethodBadgeClass(label: PaymentMethodLabel): string {
  switch (label) {
    case '現金':
      return 'bg-amber-100 text-amber-700';
    case '轉帳':
      return 'bg-blue-100 text-blue-700';
    case LEGACY_PAYMENT_METHOD_LABEL:
      return 'bg-amber-50 text-amber-700';
    default:
      return 'bg-slate-100 text-slate-500';
  }
}

// isCashAppointment / isTransferAppointment 把付款方式判断集中到同一处，
// 避免技师页、现金账页再次直接比较原始字符串，遗漏 legacy 占位值的兼容说明。
export function isCashPaymentMethod(method: AppointmentReadablePaymentMethod | string): boolean {
  return normalizeReadablePaymentMethod(method) === CASH_PAYMENT_METHOD;
}

export function isTransferPaymentMethod(method: AppointmentReadablePaymentMethod | string): boolean {
  return normalizeReadablePaymentMethod(method) === TRANSFER_PAYMENT_METHOD;
}

export function isCashAppointment(appt: Appointment): boolean {
  return isCashPaymentMethod(appt.payment_method);
}

export function isTransferAppointment(appt: Appointment): boolean {
  return isTransferPaymentMethod(appt.payment_method);
}

// isCollectedAppointment 统一识别「已收款」工单，只统计已结案且需要真实收款的预约。
export function isCollectedAppointment(appt: Appointment): boolean {
  return isClosedAppointment(appt) && isCollectibleAppointment(appt) && Boolean(appt.payment_received);
}

// isOutstandingAppointment 统一识别「仍有未收余额」的工单。
// 这里既覆盖完全未收款，也覆盖已确认收款但仍有尾款/折让差额的场景，确保财务页与技师页口径一致。
export function isOutstandingAppointment(appt: Appointment): boolean {
  if (!isClosedAppointment(appt) || !isCollectibleAppointment(appt)) {
    return false;
  }
  return getChargeableAmount(appt) > getCollectedAmount(appt);
}

// getChargeableAmount 统一计算应收金额；無收款工单应收为 0，避免财务页把免收费任务算成待收。
export function getChargeableAmount(appt: Appointment): number {
  if (!isCollectibleAppointment(appt)) {
    return 0;
  }
  return Math.max(appt.total_amount, 0);
}

// getCollectedAmount 统一计算真实实收金额，避免把「已完工但未收款」误算进财务与绩效统计。
export function getCollectedAmount(appt: Appointment): number {
  if (!isCollectedAppointment(appt)) {
    return 0;
  }
  return appt.paid_amount ?? getChargeableAmount(appt);
}

// getOutstandingAmount 统一计算未收余额；无收款工单与已全额收款工单都应返回 0。
// 只要应收大于实收，就把差额视为未收余额，避免部分收款在统计页被误算成已结清。
export function getOutstandingAmount(appt: Appointment): number {
  if (!isOutstandingAppointment(appt)) {
    return 0;
  }
  return Math.max(getChargeableAmount(appt) - getCollectedAmount(appt), 0);
}

// 下面这组旧命名仍被多个页面引用；保留兼容导出，避免大范围改名期间出现构建中断。
export const isAppointmentFinished = isClosedAppointment;
export const isAppointmentSettled = isPaymentSettled;
export const isAppointmentRevenueCounted = isCollectedAppointment;
export const getAppointmentCollectedAmount = getCollectedAmount;
