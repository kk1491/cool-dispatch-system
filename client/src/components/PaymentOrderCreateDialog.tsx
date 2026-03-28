import { useEffect, useMemo, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { format, parseISO } from 'date-fns';
import { CheckCircle2, Copy, CreditCard, Loader2, X } from 'lucide-react';
import { toast } from 'react-hot-toast';

import { createPaymentOrder, CreatePaymentOrderRequest, CreatePaymentOrderResponse } from '../lib/api';
import { getOutstandingAmount } from '../lib/appointmentMetrics';
import {
  buildPaymentOrderAppointmentOptionLabel,
  buildPaymentOrderDescription,
  isPaymentLinkCreatableAppointment,
} from '../lib/paymentOrder';
import { cn } from '../lib/utils';
import { Appointment } from '../types';

interface PaymentOrderCreateDialogProps {
  open: boolean;
  onClose: () => void;
  appointments: Appointment[];
  initialAppointmentId?: number;
  onCreated?: (result: CreatePaymentOrderResponse) => Promise<unknown> | unknown;
}

const APPOINTMENT_STATUS_LABEL: Record<Appointment['status'], string> = {
  pending: '待指派',
  assigned: '已指派',
  arrived: '已到場',
  completed: '已完成',
  cancelled: '已取消',
};

// PaymentOrderCreateDialog 把三個入口共用的建單流程集中到同一個元件，
// 避免預填、提交、成功視窗與錯誤處理在不同頁面再次分叉。
export default function PaymentOrderCreateDialog({
  open,
  onClose,
  appointments,
  initialAppointmentId,
  onCreated,
}: PaymentOrderCreateDialogProps) {
  const [selectedAppointmentId, setSelectedAppointmentId] = useState('');
  const [formAmt, setFormAmt] = useState('');
  const [formDesc, setFormDesc] = useState('');
  const [formName, setFormName] = useState('');
  const [formEmail, setFormEmail] = useState('');
  const [formPhone, setFormPhone] = useState('');
  const [formMethod, setFormMethod] = useState('both');
  const [isCreating, setIsCreating] = useState(false);
  const [createdLink, setCreatedLink] = useState('');
  const [showLinkModal, setShowLinkModal] = useState(false);

  const payableAppointments = useMemo(
    () => appointments
      .filter(isPaymentLinkCreatableAppointment)
      .sort((left, right) => right.scheduled_at.localeCompare(left.scheduled_at)),
    [appointments],
  );

  const selectedAppointment = useMemo(
    () => appointments.find(appointment => String(appointment.id) === selectedAppointmentId),
    [appointments, selectedAppointmentId],
  );

  const selectedAppointmentIsCreatable = selectedAppointment
    ? isPaymentLinkCreatableAppointment(selectedAppointment)
    : false;

  // syncSelectedAppointment 把預約上的核心欄位回填到表單，避免快建入口仍要人工重打。
  const syncSelectedAppointment = (appointmentId: string) => {
    setSelectedAppointmentId(appointmentId);

    const appointment = appointments.find(item => String(item.id) === appointmentId);
    if (!appointment) {
      setFormAmt('');
      setFormDesc('');
      setFormName('');
      setFormPhone('');
      return;
    }

    setFormAmt(String(getOutstandingAmount(appointment)));
    setFormDesc(buildPaymentOrderDescription(appointment));
    setFormName(appointment.customer_name);
    setFormPhone(appointment.phone || '');
  };

  // resetCreateForm 在關閉與建單成功後統一清空狀態，避免上一筆預約殘留到下一次操作。
  const resetCreateForm = () => {
    setSelectedAppointmentId('');
    setFormAmt('');
    setFormDesc('');
    setFormName('');
    setFormEmail('');
    setFormPhone('');
    setFormMethod('both');
  };

  useEffect(() => {
    if (!open) {
      resetCreateForm();
      return;
    }

    if (initialAppointmentId) {
      syncSelectedAppointment(String(initialAppointmentId));
      return;
    }

    resetCreateForm();
  }, [initialAppointmentId, open, appointments]);

  const handleClose = () => {
    resetCreateForm();
    onClose();
  };

  const handleCreate = async () => {
    const tradeAmt = parseInt(formAmt, 10);
    if (!selectedAppointmentId) {
      toast.error('請先綁定要收款的預約');
      return;
    }
    if (!selectedAppointment || !selectedAppointmentIsCreatable) {
      toast.error('目前這筆預約不可建立付款連結');
      return;
    }
    if (!tradeAmt || tradeAmt <= 0) {
      toast.error('請輸入有效金額');
      return;
    }
    if (!formDesc.trim()) {
      toast.error('請輸入商品說明');
      return;
    }
    if (!formName.trim()) {
      toast.error('請輸入客戶名稱');
      return;
    }

    setIsCreating(true);
    try {
      const payload: CreatePaymentOrderRequest = {
        trade_amt: tradeAmt,
        prod_desc: formDesc.trim(),
        customer_name: formName.trim(),
        payment_method: formMethod,
        customer_email: formEmail.trim() || undefined,
        customer_phone: formPhone.trim() || undefined,
        appointment_id: Number(selectedAppointmentId),
      };
      const result = await createPaymentOrder(payload);
      const fullLink = `${window.location.origin}${result.payment_url}`;

      setCreatedLink(fullLink);
      setShowLinkModal(true);
      toast.success('支付連結已建立');
      handleClose();
      try {
        await onCreated?.(result);
      } catch (refreshError) {
        // 建單已成功時，後續列表刷新失敗不能覆蓋成功結果，否則管理員會誤以為連結未建立。
        console.error(refreshError);
      }
    } catch (err: any) {
      toast.error(err.message || '建立支付訂單失敗');
    } finally {
      setIsCreating(false);
    }
  };

  return (
    <>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[80] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4"
            onClick={handleClose}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9, y: 20 }}
              transition={{ type: 'spring', damping: 25, stiffness: 300 }}
              className="bg-white rounded-2xl shadow-2xl w-full max-w-xl overflow-hidden"
              onClick={event => event.stopPropagation()}
            >
              <div className="px-6 py-5 border-b border-slate-100 flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-blue-50 rounded-lg flex items-center justify-center">
                    <CreditCard className="w-5 h-5 text-blue-600" />
                  </div>
                  <div>
                    <h3 className="text-lg font-bold text-slate-900">建立支付連結</h3>
                    <p className="text-xs text-slate-400 mt-0.5">綁定預約後即可直接把付款網址發給客戶</p>
                  </div>
                </div>
                <button
                  onClick={handleClose}
                  className="w-8 h-8 rounded-full hover:bg-slate-100 flex items-center justify-center transition-colors"
                >
                  <X className="w-4 h-4 text-slate-400" />
                </button>
              </div>

              <div className="px-6 py-5 space-y-4">
                <div>
                  <label className="block text-xs font-medium text-slate-500 mb-1.5">關聯預約 *</label>
                  <select
                    value={selectedAppointmentId}
                    onChange={event => syncSelectedAppointment(event.target.value)}
                    disabled={payableAppointments.length === 0 && !selectedAppointment}
                    className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm bg-white disabled:bg-slate-50 disabled:text-slate-400"
                  >
                    <option value="">
                      {payableAppointments.length === 0 ? '目前沒有可建立付款連結的預約' : '請選擇未收款的預約'}
                    </option>
                    {payableAppointments.map(appointment => (
                      <option key={appointment.id} value={appointment.id}>
                        {buildPaymentOrderAppointmentOptionLabel(appointment)}
                      </option>
                    ))}
                  </select>
                  {!selectedAppointment ? (
                    <p className="text-xs text-slate-400 mt-1.5">僅顯示未收款、非無收款且仍有未收餘額的預約</p>
                  ) : !selectedAppointmentIsCreatable ? (
                    <p className="text-xs text-red-500 mt-1.5">這筆預約目前不符合建立付款連結條件，請確認收款狀態與應收金額。</p>
                  ) : (
                    <p className="text-xs text-slate-400 mt-1.5">建立後會保留在當前頁面，方便直接複製並發送連結。</p>
                  )}
                </div>

                {selectedAppointment && (
                  <div className="rounded-xl border border-blue-100 bg-blue-50/70 p-4 space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-semibold text-slate-900">{selectedAppointment.customer_name}</span>
                      <span className="text-xs text-blue-700 bg-white/80 rounded-full px-2 py-0.5">
                        {APPOINTMENT_STATUS_LABEL[selectedAppointment.status]}
                      </span>
                    </div>
                    <div className="grid grid-cols-2 gap-2 text-xs text-slate-500">
                      <span>預約 #{selectedAppointment.id}</span>
                      <span>{format(parseISO(selectedAppointment.scheduled_at), 'yyyy/MM/dd HH:mm')}</span>
                      <span>總額 NT$ {selectedAppointment.total_amount.toLocaleString()}</span>
                      <span>待收 NT$ {getOutstandingAmount(selectedAppointment).toLocaleString()}</span>
                    </div>
                    <p className="text-xs text-slate-500 break-all">{selectedAppointment.address}</p>
                  </div>
                )}

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">客戶名稱 *</label>
                    <input
                      type="text"
                      placeholder="王小明"
                      value={formName}
                      onChange={event => setFormName(event.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">電話（選填）</label>
                    <input
                      type="tel"
                      placeholder="0912345678"
                      value={formPhone}
                      onChange={event => setFormPhone(event.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">金額 (NT$) *</label>
                    <input
                      type="number"
                      placeholder="1500"
                      value={formAmt}
                      onChange={event => setFormAmt(event.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">支付方式</label>
                    <select
                      value={formMethod}
                      onChange={event => setFormMethod(event.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm bg-white"
                    >
                      <option value="both">信用卡 / ATM</option>
                      <option value="credit">僅信用卡</option>
                      <option value="atm">僅 ATM</option>
                    </select>
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-medium text-slate-500 mb-1.5">商品說明 *</label>
                  <input
                    type="text"
                    placeholder="居家清潔服務費"
                    value={formDesc}
                    onChange={event => setFormDesc(event.target.value)}
                    className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                  />
                </div>

                <div>
                  <label className="block text-xs font-medium text-slate-500 mb-1.5">Email（選填）</label>
                  <input
                    type="email"
                    placeholder="customer@example.com"
                    value={formEmail}
                    onChange={event => setFormEmail(event.target.value)}
                    className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                  />
                </div>
              </div>

              <div className="px-6 py-4 bg-slate-50 border-t border-slate-100 flex gap-3">
                <button
                  onClick={handleClose}
                  className="flex-1 py-2.5 rounded-lg text-sm font-medium text-slate-600 border border-slate-200 hover:bg-white transition-colors"
                >
                  取消
                </button>
                <button
                  onClick={handleCreate}
                  disabled={isCreating || !selectedAppointmentId || !formAmt || !formDesc || !formName || !selectedAppointmentIsCreatable}
                  className={cn(
                    'flex-1 py-2.5 rounded-lg text-sm font-bold transition-all flex items-center justify-center gap-2',
                    isCreating || !selectedAppointmentId || !formAmt || !formDesc || !formName || !selectedAppointmentIsCreatable
                      ? 'bg-slate-200 text-slate-400 cursor-not-allowed'
                      : 'bg-blue-600 text-white hover:bg-blue-700 shadow-sm'
                  )}
                >
                  {isCreating ? (
                    <><Loader2 className="w-4 h-4 animate-spin" /> 建立中...</>
                  ) : (
                    <>建立連結</>
                  )}
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {showLinkModal && createdLink && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[85] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4"
            onClick={() => setShowLinkModal(false)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9, y: 20 }}
              transition={{ type: 'spring', damping: 25, stiffness: 300 }}
              className="bg-white rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-5"
              onClick={event => event.stopPropagation()}
            >
              <div className="text-center space-y-2">
                <div className="w-16 h-16 bg-emerald-50 rounded-full flex items-center justify-center mx-auto">
                  <CheckCircle2 className="w-8 h-8 text-emerald-500" />
                </div>
                <h3 className="text-lg font-bold text-slate-900">支付連結已建立</h3>
                <p className="text-sm text-slate-500">直接發送以下連結給客戶即可付款</p>
              </div>

              <div className="bg-slate-50 rounded-lg p-3 flex items-center gap-2">
                <input
                  readOnly
                  value={createdLink}
                  className="flex-1 bg-transparent text-xs text-slate-700 font-mono outline-none"
                />
                <button
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(createdLink);
                      toast.success('連結已複製');
                    } catch {
                      toast.error('複製失敗');
                    }
                  }}
                  className="flex-shrink-0 inline-flex items-center gap-1 px-3 py-1.5 bg-blue-600 text-white rounded-md text-xs font-medium hover:bg-blue-700 transition-colors"
                >
                  <Copy className="w-3 h-3" /> 複製
                </button>
              </div>

              <button
                onClick={() => setShowLinkModal(false)}
                className="w-full py-2.5 text-sm font-medium text-slate-600 border border-slate-200 rounded-lg hover:bg-slate-50 transition-colors"
              >
                關閉
              </button>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}
