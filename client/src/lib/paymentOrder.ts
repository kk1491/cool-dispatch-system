import { format, parseISO } from 'date-fns';

import { getOutstandingAmount, isCollectibleAppointment } from './appointmentMetrics';
import { Appointment } from '../types';

// isPaymentLinkCreatableAppointment 統一建立付款連結的前端口徑。
// 本輪規則只要求：可收款、尚未確認收款、且仍有未收餘額。
export function isPaymentLinkCreatableAppointment(appointment: Appointment): boolean {
  if (!isCollectibleAppointment(appointment)) {
    return false;
  }
  if (Boolean(appointment.payment_received)) {
    return false;
  }
  return getOutstandingAmount(appointment) > 0;
}

// buildPaymentOrderDescription 用預約內容預填商品說明，避免三個入口各自維護文案拼接規則。
export function buildPaymentOrderDescription(appointment: Appointment): string {
  const itemSummary = appointment.items
    .map(item => item.type?.trim())
    .filter(Boolean)
    .slice(0, 3)
    .join(' / ');

  return itemSummary
    ? `${appointment.customer_name} ${itemSummary} 服務費`
    : `${appointment.customer_name} 預約服務費`;
}

// buildPaymentOrderAppointmentOptionLabel 統一下拉選單的展示格式，讓金額與時間一眼可辨識。
export function buildPaymentOrderAppointmentOptionLabel(appointment: Appointment): string {
  const scheduledAt = format(parseISO(appointment.scheduled_at), 'MM/dd HH:mm');
  return `#${appointment.id} ${appointment.customer_name}｜${scheduledAt}｜待收 NT$ ${getOutstandingAmount(appointment).toLocaleString()}`;
}
