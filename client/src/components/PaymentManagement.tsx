import { useEffect, useMemo, useState } from 'react';
import {
  AlertTriangle,
  Check,
  CheckCircle2,
  Copy,
  CreditCard,
  ExternalLink,
  Loader2,
  Plus,
  XCircle,
} from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { toast } from 'react-hot-toast';

import { listPaymentOrders, PaymentOrderRecord } from '../lib/api';
import { isPaymentLinkCreatableAppointment } from '../lib/paymentOrder';
import { cn } from '../lib/utils';
import { Appointment } from '../types';
import PaymentOrderCreateDialog from './PaymentOrderCreateDialog';

interface PaymentManagementProps {
  appointments: Appointment[];
  onRefreshData?: () => Promise<unknown>;
}

// ============================================================================
// 支付管理頁（管理員專用）
//
// 功能：
//   1. 查看所有支付訂單記錄與可分享連結
//   2. 透過共用建單對話框建立付款連結
// ============================================================================

const STATUS_MAP: Record<string, { label: string; color: string; icon: typeof CheckCircle2 }> = {
  pending: { label: '待支付', color: 'text-amber-600 bg-amber-50 border-amber-200', icon: Loader2 },
  paying: { label: '處理中', color: 'text-blue-600 bg-blue-50 border-blue-200', icon: Loader2 },
  paid: { label: '已付款', color: 'text-emerald-600 bg-emerald-50 border-emerald-200', icon: CheckCircle2 },
  failed: { label: '失敗', color: 'text-red-600 bg-red-50 border-red-200', icon: XCircle },
  expired: { label: '已過期', color: 'text-slate-500 bg-slate-50 border-slate-200', icon: AlertTriangle },
  cancelled: { label: '已取消', color: 'text-slate-500 bg-slate-50 border-slate-200', icon: XCircle },
};

const METHOD_LABEL: Record<string, string> = {
  credit: '信用卡',
  atm: 'ATM 轉帳',
  both: '信用卡 / ATM',
};

function parsePaymentDeadline(raw?: string): Date | null {
  if (!raw) {
    return null;
  }

  const normalized = raw.trim().replace(' ', 'T');
  const parsed = new Date(normalized);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

// getOrderRuntimeStatus 在前端補一層過期判斷，避免 ATM 超時後仍顯示可用。
function getOrderRuntimeStatus(order: PaymentOrderRecord): string {
  if (order.status === 'expired') {
    return 'expired';
  }

  const deadline = parsePaymentDeadline(order.atm_expire_date);
  if (!deadline) {
    return order.status;
  }

  if (order.pay_no && order.status !== 'paid' && deadline.getTime() < Date.now()) {
    return 'expired';
  }

  return order.status;
}

export default function PaymentManagement({ appointments, onRefreshData }: PaymentManagementProps) {
  const [orders, setOrders] = useState<PaymentOrderRecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [copiedToken, setCopiedToken] = useState('');

  const payableAppointments = useMemo(
    () => appointments
      .filter(isPaymentLinkCreatableAppointment)
      .sort((left, right) => right.scheduled_at.localeCompare(left.scheduled_at)),
    [appointments],
  );

  const loadOrders = async () => {
    try {
      const data = await listPaymentOrders();
      setOrders(data || []);
    } catch (err) {
      console.error(err);
      toast.error('載入支付記錄失敗');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadOrders();
  }, []);

  const copyLink = async (token: string) => {
    const baseUrl = window.location.origin;
    const link = `${baseUrl}/pay/${token}`;
    try {
      await navigator.clipboard.writeText(link);
      setCopiedToken(token);
      toast.success('支付連結已複製');
      setTimeout(() => setCopiedToken(''), 2000);
    } catch {
      toast.error('複製失敗');
    }
  };

  const getStatusConfig = (status: string) => STATUS_MAP[status] || STATUS_MAP.pending;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-bold text-slate-900">支付訂單</h3>
          <p className="text-sm text-slate-500 mt-0.5">綁定未收款預約建立支付連結，並集中管理分享中的付款單</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 transition-colors shadow-sm"
        >
          <Plus className="w-4 h-4" /> 建立支付連結
        </button>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[
          { label: '全部', count: orders.length, color: 'text-slate-700' },
          {
            label: '待支付',
            count: orders.filter(order => {
              const status = getOrderRuntimeStatus(order);
              return status === 'pending' || status === 'paying';
            }).length,
            color: 'text-amber-600',
          },
          {
            label: '已付款',
            count: orders.filter(order => getOrderRuntimeStatus(order) === 'paid').length,
            color: 'text-emerald-600',
          },
          {
            label: '已失效',
            count: orders.filter(order => {
              const status = getOrderRuntimeStatus(order);
              return status === 'failed' || status === 'cancelled' || status === 'expired';
            }).length,
            color: 'text-red-500',
          },
        ].map(stat => (
          <div key={stat.label} className="bg-white rounded-lg border border-slate-200/60 p-4 text-center">
            <p className={cn('text-2xl font-bold', stat.color)}>{stat.count}</p>
            <p className="text-xs text-slate-400 mt-1">{stat.label}</p>
          </div>
        ))}
      </div>

      {isLoading ? (
        <div className="text-center py-12">
          <Loader2 className="w-8 h-8 text-blue-500 animate-spin mx-auto mb-3" />
          <p className="text-sm text-slate-500">載入中...</p>
        </div>
      ) : orders.length === 0 ? (
        <div className="text-center py-16 bg-white rounded-lg border border-slate-200/60">
          <CreditCard className="w-12 h-12 text-slate-300 mx-auto mb-3" />
          <p className="text-slate-500 text-sm">尚無支付訂單</p>
          <p className="text-slate-400 text-xs mt-1">點擊「建立支付連結」開始</p>
        </div>
      ) : (
        <div className="space-y-3">
          {orders.map(order => {
            const effectiveStatus = getOrderRuntimeStatus(order);
            const statusConfig = getStatusConfig(effectiveStatus);
            const StatusIcon = statusConfig.icon;
            const canShareLink = effectiveStatus === 'pending' || effectiveStatus === 'paying';

            return (
              <div key={order.id} className="bg-white rounded-lg border border-slate-200/60 p-4 hover:shadow-sm transition-shadow">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex-1 min-w-0 space-y-2">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm font-bold text-slate-900">{order.customer_name}</span>
                      <span className={cn('inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium border', statusConfig.color)}>
                        <StatusIcon className={cn('w-3 h-3', effectiveStatus === 'paying' && 'animate-spin')} />
                        {statusConfig.label}
                      </span>
                      <span className="text-xs text-slate-400">{METHOD_LABEL[order.payment_method] || order.payment_method}</span>
                      {order.appointment_id && (
                        <span className="text-xs text-slate-500 bg-slate-100 rounded-full px-2 py-0.5">
                          預約 #{order.appointment_id}
                        </span>
                      )}
                    </div>

                    <div className="flex items-baseline gap-3">
                      <span className="text-lg font-bold text-blue-600">NT$ {Number(order.trade_amt).toLocaleString()}</span>
                      <span className="text-xs text-slate-400">{order.prod_desc}</span>
                    </div>

                    <div className="flex items-center gap-3 text-xs text-slate-400 flex-wrap">
                      <span>{order.mer_trade_no}</span>
                      {order.trade_no && <span>PAYUNi: {order.trade_no}</span>}
                      {order.auth_code && <span>授權碼: {order.auth_code}</span>}
                      {order.card_6_no && order.card_4_no && <span>卡號: {order.card_6_no}****{order.card_4_no}</span>}
                      {order.pay_no && (
                        <span className="font-mono bg-slate-100 px-1.5 py-0.5 rounded">帳號: {order.pay_no}</span>
                      )}
                      <span>{format(parseISO(order.created_at), 'MM/dd HH:mm')}</span>
                      {order.paid_at && (
                        <span className="text-emerald-500">收款於 {format(parseISO(order.paid_at), 'MM/dd HH:mm')}</span>
                      )}
                      {effectiveStatus === 'expired' && order.atm_expire_date && (
                        <span className="text-red-500">逾期於 {order.atm_expire_date}</span>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-1.5 flex-shrink-0">
                    {canShareLink && (
                      <button
                        onClick={() => copyLink(order.payment_token)}
                        className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-lg transition-colors"
                      >
                        {copiedToken === order.payment_token ? (
                          <><Check className="w-3 h-3" /> 已複製</>
                        ) : (
                          <><Copy className="w-3 h-3" /> 複製連結</>
                        )}
                      </button>
                    )}
                    {canShareLink && (
                      <a
                        href={`/pay/${order.payment_token}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-slate-500 bg-slate-50 hover:bg-slate-100 rounded-lg transition-colors"
                      >
                        <ExternalLink className="w-3 h-3" /> 預覽
                      </a>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <div className="rounded-xl border border-slate-200 bg-white/70 px-4 py-3 text-xs text-slate-500">
        目前可建立付款連結的預約共 <span className="font-semibold text-slate-700">{payableAppointments.length}</span> 筆，
        條件為未收款、非無收款且仍有未收餘額。
      </div>

      <PaymentOrderCreateDialog
        open={showCreate}
        onClose={() => setShowCreate(false)}
        appointments={appointments}
        onCreated={async () => {
          await loadOrders();
          await onRefreshData?.();
        }}
      />
    </div>
  );
}
