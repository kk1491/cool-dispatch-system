import { useState, useRef, useEffect, useCallback } from 'react';
import { 
  ChevronLeft, Camera, Phone, MapPin, Clock, Calendar,
  CheckCircle2, Circle, Upload, X, LogOut, User as UserIcon,
  DollarSign, ClipboardList, Wallet, Navigation, AlertTriangle,
  PenTool, RotateCcw
} from 'lucide-react';
import { format, parseISO, addDays, startOfDay } from 'date-fns';
import { zhTW } from 'date-fns/locale';
import { toast } from 'react-hot-toast';
import { motion, AnimatePresence } from 'motion/react';
import { cn } from '../lib/utils';
import {
  BACKFILL_PAYMENT_METHOD_OPTIONS,
  TECHNICIAN_EARNINGS_SCOPE_NOTE,
  TECHNICIAN_CLOSED_RECORDS_EMPTY_TEXT,
  TECHNICIAN_CLOSED_RECORDS_SUBTITLE,
  TECHNICIAN_CLOSED_RECORDS_TITLE,
  getAppointmentCollectedAmount,
  getAppointmentClosedAt,
  getAppointmentClosedMonthKey,
  getPaymentCollectionBadgeClass,
  getPaymentMethodBadgeClass,
  getPaymentMethodLabel,
  getChargeableAmount,
  getPaymentCollectionLabel,
  getOutstandingAmount,
  getWritablePaymentMethod,
  isCashAppointment,
  isChargeExemptAppointment,
  isAppointmentFinished,
  isAppointmentRevenueCounted,
  isAppointmentSettled,
  isTransferPaymentMethod,
  isTransferAppointment,
  normalizeAppointmentForWrite,
  shouldBackfillPaymentMethod,
} from '../lib/appointmentMetrics';
import { Button, Card, Badge } from './shared';
import { User, Appointment } from '../types';
import { uploadImage, deleteImage } from '../lib/api';

function SignaturePad({ onSave, initialData }: { onSave: (data: string) => void; initialData?: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [isDrawing, setIsDrawing] = useState(false);
  const [hasDrawn, setHasDrawn] = useState(!!initialData);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const rect = canvas.parentElement!.getBoundingClientRect();
    canvas.width = rect.width;
    canvas.height = 160;
    ctx.fillStyle = '#fafafa';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.strokeStyle = '#1a1a1a';
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    if (initialData) {
      const img = new Image();
      img.onload = () => ctx.drawImage(img, 0, 0);
      img.src = initialData;
    }
  }, []);

  const getPos = useCallback((e: React.TouchEvent | React.MouseEvent) => {
    const canvas = canvasRef.current!;
    const rect = canvas.getBoundingClientRect();
    if ('touches' in e) {
      return { x: e.touches[0].clientX - rect.left, y: e.touches[0].clientY - rect.top };
    }
    return { x: (e as React.MouseEvent).clientX - rect.left, y: (e as React.MouseEvent).clientY - rect.top };
  }, []);

  const startDraw = useCallback((e: React.TouchEvent | React.MouseEvent) => {
    e.preventDefault();
    const ctx = canvasRef.current?.getContext('2d');
    if (!ctx) return;
    setIsDrawing(true);
    setHasDrawn(true);
    const pos = getPos(e);
    ctx.beginPath();
    ctx.moveTo(pos.x, pos.y);
  }, [getPos]);

  const draw = useCallback((e: React.TouchEvent | React.MouseEvent) => {
    e.preventDefault();
    if (!isDrawing) return;
    const ctx = canvasRef.current?.getContext('2d');
    if (!ctx) return;
    const pos = getPos(e);
    ctx.lineTo(pos.x, pos.y);
    ctx.stroke();
  }, [isDrawing, getPos]);

  const endDraw = useCallback(() => {
    setIsDrawing(false);
    if (hasDrawn && canvasRef.current) {
      onSave(canvasRef.current.toDataURL('image/png'));
    }
  }, [hasDrawn, onSave]);

  const clearCanvas = () => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.fillStyle = '#fafafa';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.strokeStyle = '#1a1a1a';
    ctx.lineWidth = 2.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    setHasDrawn(false);
  };

  return (
    <Card className="p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider flex items-center gap-1.5">
          <PenTool className="w-3.5 h-3.5" /> 客戶簽名（選填）
        </h4>
        <button
          onClick={clearCanvas}
          data-testid="button-clear-signature"
          className="text-xs text-slate-400 hover:text-slate-600 flex items-center gap-1 transition-colors"
        >
          <RotateCcw className="w-3 h-3" /> 清除
        </button>
      </div>
      <div className="border-2 border-dashed border-slate-200 rounded-md overflow-hidden touch-none" data-testid="signature-pad">
        <canvas
          ref={canvasRef}
          className="w-full cursor-crosshair"
          onMouseDown={startDraw}
          onMouseMove={draw}
          onMouseUp={endDraw}
          onMouseLeave={endDraw}
          onTouchStart={startDraw}
          onTouchMove={draw}
          onTouchEnd={endDraw}
        />
      </div>
      {!hasDrawn && (
        <p className="text-[10px] text-slate-400 text-center">在上方區域簽名</p>
      )}
    </Card>
  );
}

type TabType = 'today' | 'future' | 'earnings' | 'profile';

interface TechnicianDashboardProps {
  user: User;
  appointments: Appointment[];
  onStatusUpdate: (appt: Appointment, status: Appointment['status'], patch?: Partial<Appointment>) => void;
  onUpdateAppointment: (appt: Appointment) => void;
  onLogout: () => void;
}

const WORKFLOW_STEPS = [
  { key: 'depart', label: '出發到達' },
  { key: 'clean', label: '清洗作業' },
  { key: 'payment', label: '收款確認' },
  { key: 'done', label: '任務完成' },
];

function getStepIndex(appt: Appointment): number {
  if (isAppointmentSettled(appt) && isAppointmentFinished(appt)) return 3;
  if (isAppointmentFinished(appt)) return 2;
  if (appt.status === 'arrived') return 1;
  if (appt.status === 'assigned') return 0;
  return 0;
}

function WorkflowStepper({ currentStep }: { currentStep: number }) {
  const allDone = currentStep >= WORKFLOW_STEPS.length;
  return (
    <div className="flex items-center justify-between px-2 py-4" data-testid="workflow-stepper">
      {WORKFLOW_STEPS.map((step, idx) => {
        const isDone = allDone || idx < currentStep;
        const isCurrent = !allDone && idx === currentStep;
        return (
          <div key={step.key} className="flex items-center flex-1">
            <div className="flex flex-col items-center flex-shrink-0">
              <div className={cn(
                "w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold transition-all",
                isDone ? "bg-emerald-500 text-white" :
                isCurrent ? "bg-blue-600 text-white ring-4 ring-blue-100" :
                "bg-slate-100 text-slate-400"
              )}>
                {isDone ? <CheckCircle2 className="w-4 h-4" /> : idx + 1}
              </div>
              <span className={cn(
                "text-[10px] mt-1 font-medium whitespace-nowrap",
                isDone ? "text-emerald-600" :
                isCurrent ? "text-blue-600" :
                "text-slate-400"
              )}>{step.label}</span>
            </div>
            {idx < WORKFLOW_STEPS.length - 1 && (
              <div className={cn(
                "flex-1 h-0.5 mx-1 mt-[-14px]",
                isDone ? "bg-emerald-400" : "bg-slate-200"
              )} />
            )}
          </div>
        );
      })}
    </div>
  );
}

function AppointmentCard({ appt, onClick, readonly }: { appt: Appointment; onClick: () => void; readonly?: boolean }) {
  const time = format(parseISO(appt.scheduled_at), 'HH:mm');
  const isActive = appt.status === 'arrived';
  const isAssigned = appt.status === 'assigned';
  const chargeableAmount = getChargeableAmount(appt);
  const paymentMethodLabel = getPaymentMethodLabel(appt);

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      onClick={onClick}
      data-testid={`card-appointment-${appt.id}`}
      className={cn(
        "bg-white rounded-lg p-4 shadow-sm border cursor-pointer active:scale-[0.98] transition-all",
        isActive ? "border-violet-200 ring-2 ring-violet-100" :
        isAssigned ? "border-blue-200" :
        "border-slate-100"
      )}
    >
      <div className="flex items-start justify-between mb-2">
        <div className="flex items-center gap-2">
          <div className={cn(
            "w-10 h-10 rounded-md flex items-center justify-center text-sm font-bold",
            isActive ? "bg-violet-100 text-violet-700" :
            isAssigned ? "bg-blue-100 text-blue-700" :
            "bg-slate-100 text-slate-500"
          )}>
            {time}
          </div>
          <div>
            <p className="font-bold text-slate-900 text-base" data-testid={`text-customer-${appt.id}`}>{appt.customer_name}</p>
            <p className="text-xs text-slate-400 truncate max-w-[180px]">{appt.address}</p>
          </div>
        </div>
        <Badge status={appt.status} />
      </div>
      <div className="flex items-center gap-3 mt-2 text-xs text-slate-500">
        <span className="flex items-center gap-1">
          <ClipboardList className="w-3 h-3" />
          {appt.items.length} 台 {appt.items.map(i => i.type).join('/')}
        </span>
        <span className="flex items-center gap-1">
          <DollarSign className="w-3 h-3" />
          ${chargeableAmount}
        </span>
        <span className="flex items-center gap-1">
          <Wallet className="w-3 h-3" />
          {paymentMethodLabel}
        </span>
      </div>
      {!readonly && isActive && (
        <div className="mt-3 pt-2 border-t border-violet-100">
          <p className="text-xs font-medium text-violet-600">清洗進行中...</p>
        </div>
      )}
    </motion.div>
  );
}

function TaskDetail({ appt, onBack, onStatusUpdate, onUpdateAppointment, readonly }: {
  appt: Appointment;
  onBack: () => void;
  onStatusUpdate: (appt: Appointment, status: Appointment['status'], patch?: Partial<Appointment>) => void;
  onUpdateAppointment: (appt: Appointment) => void;
  readonly?: boolean;
}) {
  const currentStep = getStepIndex(appt);
  const paymentSettled = isAppointmentSettled(appt);
  const paymentSkipped = isChargeExemptAppointment(appt);
  const needsPaymentMethodBackfill = shouldBackfillPaymentMethod(appt);
  const chargeableAmount = getChargeableAmount(appt);
  const collectedAmount = getAppointmentCollectedAmount(appt);
  const outstandingAmount = getOutstandingAmount(appt);
  const collectionLabel = getPaymentCollectionLabel(appt);
  // legacyPaymentMethod 只在历史 `未收款` 脏数据的收款确认阶段启用，
  // 让技师能把占位值补录为真实付款方式，避免后续月报继续落在未知分类。
  const [legacyPaymentMethod, setLegacyPaymentMethod] = useState<Appointment['payment_method']>(
    getWritablePaymentMethod(appt.payment_method, '現金'),
  );
  const effectivePaymentMethod = needsPaymentMethodBackfill ? legacyPaymentMethod : appt.payment_method;
  const paymentMethodLabel = getPaymentMethodLabel({ ...appt, payment_method: effectivePaymentMethod });
  const [paidAmount, setPaidAmount] = useState(
    paymentSkipped ? 0 : (appt.paid_amount ?? chargeableAmount),
  );
  const [signatureData, setSignatureData] = useState<string | undefined>(appt.signature_data);

  return (
    <motion.div
      initial={{ x: '100%' }}
      animate={{ x: 0 }}
      exit={{ x: '100%' }}
      transition={{ type: 'spring', damping: 25, stiffness: 200 }}
      className="fixed inset-0 bg-slate-50 z-50 overflow-y-auto"
    >
      <div className="sticky top-0 bg-white/90 backdrop-blur-lg z-10 border-b border-slate-100">
        <div className="flex items-center gap-3 px-4 py-3">
          <button
            onClick={onBack}
            data-testid="button-back"
            className="w-10 h-10 rounded-full bg-slate-100 flex items-center justify-center active:bg-slate-200 transition-colors"
          >
            <ChevronLeft className="w-5 h-5 text-slate-600" />
          </button>
          <div className="flex-1 min-w-0">
            <h2 className="text-lg font-bold text-slate-900 truncate" data-testid="text-task-customer">{appt.customer_name}</h2>
            <p className="text-xs text-slate-400">{format(parseISO(appt.scheduled_at), 'MM/dd HH:mm')}</p>
          </div>
          <Badge status={appt.status} />
        </div>
        <WorkflowStepper currentStep={currentStep} />
      </div>

      <div className="p-4 pb-8 space-y-4">
        {readonly && (
          <div className="bg-blue-50 border border-blue-200 rounded-md p-3 text-center">
            <p className="text-sm text-blue-700 font-medium">預覽模式 — 預約尚未到達可操作時間</p>
          </div>
        )}

        {currentStep < 3 && (
          <Card className="p-4 space-y-3">
            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">客戶資訊</h4>
            <a
              href={`tel:${appt.phone}`}
              data-testid="link-call-customer"
              className="flex items-center gap-3 p-3 bg-blue-50 rounded-md active:bg-blue-100 transition-colors"
            >
              <div className="w-10 h-10 bg-blue-500 rounded-full flex items-center justify-center">
                <Phone className="w-5 h-5 text-white" />
              </div>
              <div>
                <p className="text-sm font-bold text-blue-900">{appt.phone}</p>
                <p className="text-[10px] text-blue-500">點擊撥打電話</p>
              </div>
            </a>
            <a
              href={`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(appt.address)}`}
              target="_blank"
              rel="noreferrer"
              data-testid="link-open-map"
              className="flex items-center gap-3 p-3 bg-emerald-50 rounded-md active:bg-emerald-100 transition-colors"
            >
              <div className="w-10 h-10 bg-emerald-500 rounded-full flex items-center justify-center">
                <MapPin className="w-5 h-5 text-white" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-bold text-emerald-900 truncate">{appt.address}</p>
                <p className="text-[10px] text-emerald-500">點擊開啟導航</p>
              </div>
              <Navigation className="w-4 h-4 text-emerald-400" />
            </a>
          </Card>
        )}

        {currentStep <= 1 && (
          <Card className="p-4 space-y-3">
            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">清洗內容 ({appt.items.length} 台)</h4>
            <div className="space-y-2">
              {appt.items.map((item, idx) => (
                <div key={item.id} className="flex items-center justify-between bg-slate-50 p-3 rounded-md">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 bg-white rounded-lg flex items-center justify-center shadow-sm text-xs font-bold text-slate-500">{idx + 1}</div>
                    <div>
                      <p className="text-sm font-bold text-slate-900">{item.type}</p>
                      {item.note && <p className="text-[10px] text-slate-500">{item.note}</p>}
                    </div>
                  </div>
                  <span className="text-sm font-bold text-slate-700">${item.price}</span>
                </div>
              ))}
              {appt.extra_items && appt.extra_items.length > 0 && (
                <>
                  <div className="border-t border-slate-100 my-2" />
                  {appt.extra_items.map(item => (
                    <div key={item.id} className="flex items-center justify-between bg-amber-50 p-3 rounded-md">
                      <span className="text-sm text-amber-800">{item.name}</span>
                      <span className="text-sm font-bold text-amber-700">${item.price}</span>
                    </div>
                  ))}
                </>
              )}
            </div>
            <div className="flex justify-between items-center pt-2 border-t border-slate-100">
              <span className="text-sm text-slate-500">應收總計</span>
              <span className="text-lg font-bold text-slate-900">${chargeableAmount}</span>
            </div>
          </Card>
        )}

        {!readonly && currentStep === 0 && appt.status === 'assigned' && (
          <div className="pt-4">
            <Button
              data-testid="button-depart"
              className="w-full py-[14px] rounded-lg text-lg font-bold shadow-lg shadow-blue-200"
              onClick={async () => {
                try {
                  const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
                    navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 5000 });
                  });
                  // 出发时间由后端服务器自动填充，不依赖客户端本地时钟
                  onStatusUpdate(appt, 'arrived', {
                    lat: pos.coords.latitude,
                    lng: pos.coords.longitude,
                  });
                } catch {
                  // 定位失败时仅做状态变更，出发时间仍由后端填充
                  onStatusUpdate(appt, 'arrived', {});
                }
                toast.success('已出發，GPS 定位已記錄');
              }}
            >
              <Navigation className="w-5 h-5" />
              我已出發
            </Button>
          </div>
        )}

        {!readonly && currentStep === 1 && appt.status === 'arrived' && (
          <div className="space-y-4 pt-2">
            <Card className="p-4 space-y-3">
              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">上傳完工照片</h4>
              <div className="flex gap-3 overflow-x-auto pb-2">
                {appt.photos?.map((p, i) => (
                  <div key={i} className="relative w-24 h-24 flex-shrink-0">
                    <img src={p} className="w-full h-full object-cover rounded-lg" referrerPolicy="no-referrer" />
                    <button
                      onClick={async () => {
                        // 删除图片时同步从 Cloudflare 图床移除
                        try {
                          await deleteImage(p);
                        } catch (err) {
                          console.warn('图床删除失败，已跳过:', err);
                        }
                        onUpdateAppointment({ ...appt, photos: appt.photos?.filter((_, idx) => idx !== i) });
                      }}
                      className="absolute -top-2 -right-2 bg-red-500 text-white rounded-full p-1 shadow-md"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                ))}
                <label
                  data-testid="button-upload-photo"
                  className="w-24 h-24 bg-slate-100 border-2 border-dashed border-slate-300 rounded-lg flex flex-col items-center justify-center cursor-pointer active:bg-slate-200 transition-colors flex-shrink-0"
                >
                  <Camera className="w-6 h-6 text-slate-400" />
                  <span className="text-[10px] text-slate-500 mt-1">拍照上傳</span>
                  <input type="file" className="hidden" accept="image/*" capture="environment" onChange={async (e) => {
                    const file = e.target.files?.[0];
                    if (!file) return;
                    // 重置 input 值，允许重复选择同一文件
                    e.target.value = '';
                    try {
                      toast.loading('正在上傳照片...', { id: 'photo-upload' });
                      const result = await uploadImage(file);
                      toast.success('照片上傳成功', { id: 'photo-upload' });
                      onUpdateAppointment({ ...appt, photos: [...(appt.photos || []), result.url] });
                    } catch (err) {
                      console.error('图片上传失败:', err);
                      toast.error('照片上傳失敗，請重試', { id: 'photo-upload' });
                    }
                  }} />
                </label>
              </div>
              {(!appt.photos || appt.photos.length === 0) && (
                <p className="text-xs text-amber-600">請至少上傳一張完工照片</p>
              )}
            </Card>

            <Button
              data-testid="button-complete"
              className="w-full py-[14px] rounded-lg text-lg font-bold bg-emerald-600 hover:bg-emerald-700 shadow-lg shadow-emerald-200"
              disabled={!appt.photos || appt.photos.length === 0}
              onClick={() => {
                // completed_time 由后端服务器自动填充，不依赖客户端本地时钟
                onStatusUpdate(appt, 'completed', {});
                toast.success('清洗完成！');
              }}
            >
              <CheckCircle2 className="w-5 h-5" />
              完成清洗
            </Button>

            <Button
              data-testid="button-cannot-clean"
              variant="danger"
              className="w-full py-[14px] rounded-lg text-lg font-bold"
              onClick={() => {
                const reason = prompt('請輸入無法清洗原因：');
                if (reason) {
                  onUpdateAppointment({
                    ...appt,
                    status: 'cancelled',
                    cancel_reason: reason,
                    total_amount: 500,
                    items: [],
                    extra_items: [{ id: 'transport', name: '車馬費', price: 500 }]
                  });
                  toast.success('已回報無法清洗，車馬費 $500');
                }
              }}
            >
              <AlertTriangle className="w-5 h-5" />
              無法清洗
            </Button>
          </div>
        )}

        {!readonly && currentStep === 2 && (appt.status === 'completed' || appt.status === 'cancelled') && !paymentSettled && (
          <div className="space-y-4 pt-2">
            <div className="bg-blue-600 text-white p-6 rounded-lg text-center">
              <p className="text-xs text-blue-200 font-medium mb-1">
                {appt.status === 'cancelled' ? '車馬費' : '應收款項'}
              </p>
              <p className="text-5xl font-bold tracking-tighter" data-testid="text-amount-due">${chargeableAmount}</p>
              <p className="text-xs text-blue-300 mt-2">付款方式：{paymentMethodLabel}</p>
              {appt.status === 'cancelled' && appt.cancel_reason && (
                <p className="text-xs mt-2 text-rose-200">原因：{appt.cancel_reason}</p>
              )}
            </div>

            <Card className="p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-700 mb-2">實收金額</label>
                <input
                  data-testid="input-paid-amount"
                  type="number"
                  value={paidAmount}
                  onChange={(e) => setPaidAmount(parseInt(e.target.value) || 0)}
                  className="w-full px-4 py-4 rounded-md border border-slate-200 text-2xl font-bold text-center focus:outline-none focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>

              {needsPaymentMethodBackfill && (
                <div>
                  <label className="block text-sm font-medium text-slate-700 mb-2">補錄付款方式</label>
                  <select
                    data-testid="select-legacy-payment-method"
                    value={legacyPaymentMethod}
                    onChange={(e) => setLegacyPaymentMethod(e.target.value as Appointment['payment_method'])}
                    className="w-full px-4 py-3 rounded-md border border-amber-200 bg-amber-50 text-sm font-medium text-amber-900 focus:outline-none focus:ring-1 focus:ring-amber-400"
                  >
                    {BACKFILL_PAYMENT_METHOD_OPTIONS.map(method => (
                      <option key={method} value={method}>{method}</option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-amber-700">
                    舊資料的付款方式未補齊，確認收款時會一併回寫成你選擇的方式。
                  </p>
                </div>
              )}

              {isTransferPaymentMethod(effectivePaymentMethod) && (
                <button
                  data-testid="button-show-account"
                  className="w-full p-4 bg-blue-50 rounded-md text-center active:bg-blue-100 transition-colors"
                  onClick={() => {
                    alert('收款帳戶：\n銀行代碼：822 (中國信託)\n帳號：123456789012');
                  }}
                >
                  <p className="text-sm font-bold text-blue-700">顯示收款帳號</p>
                  <p className="text-[10px] text-blue-500 mt-0.5">822 中國信託 ****9012</p>
                </button>
              )}
            </Card>

            <SignaturePad 
              onSave={(data) => setSignatureData(data)} 
              initialData={appt.signature_data}
            />

            <Button
              data-testid="button-confirm-payment"
              className="w-full py-[14px] rounded-lg text-lg font-bold shadow-lg shadow-blue-200"
              onClick={() => {
                // payment_time 属于后端派生字段；技师端这里只提交收款状态、金额与签名，
                // 避免继续在前端构造只读时间字段造成代码语义与 API 契约漂移。
                onUpdateAppointment(normalizeAppointmentForWrite({
                  ...appt,
                  payment_method: effectivePaymentMethod,
                  payment_received: true,
                  paid_amount: paidAmount,
                  signature_data: signatureData,
                }));
                toast.success('收款確認成功！');
              }}
            >
              <DollarSign className="w-5 h-5" />
              確認收款（{paymentMethodLabel}）
            </Button>
          </div>
        )}

        {currentStep === 3 && paymentSettled && (
          <div className="space-y-4 pt-2">
            <div className="bg-emerald-50 border border-emerald-200 p-6 rounded-lg text-center">
              <CheckCircle2 className="w-12 h-12 text-emerald-500 mx-auto mb-2" />
              <p className="text-lg font-bold text-emerald-800">任務完成</p>
              <p className="text-xs text-emerald-600 mt-1">
                {paymentSkipped ? '本單為無收款工單，已直接結案' : `收款方式：${paymentMethodLabel}`}
              </p>
            </div>

            <Card className="p-4 space-y-3">
              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">收款摘要</h4>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <span className="text-sm text-slate-500">應收金額</span>
                  <span className="text-sm font-bold">${chargeableAmount}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-sm text-slate-500">{paymentSkipped ? '收款狀態' : '實收金額'}</span>
                  {paymentSkipped ? (
                    <span className={cn("text-xs px-2 py-0.5 rounded-full font-bold", getPaymentCollectionBadgeClass(collectionLabel))}>
                      {collectionLabel}
                    </span>
                  ) : (
                    <span className="text-sm font-bold text-emerald-700">${collectedAmount}</span>
                  )}
                </div>
                {!paymentSkipped && outstandingAmount !== 0 && (
                  <div className="flex justify-between items-center pt-1 border-t border-slate-100">
                    <span className="text-sm text-rose-500">差額</span>
                    <span className="text-sm font-bold text-rose-600">
                      ${outstandingAmount}
                    </span>
                  </div>
                )}
              </div>
            </Card>

            <Card className="p-4 space-y-2">
              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">時間紀錄</h4>
              {appt.departed_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">出發時間</span>
                  <span className="font-medium">{format(parseISO(appt.departed_time), 'HH:mm')}</span>
                </div>
              )}
              {appt.checkin_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">到達簽到</span>
                  <span className="font-medium">{format(parseISO(appt.checkin_time), 'HH:mm')}</span>
                </div>
              )}
              {appt.completed_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">清洗完成</span>
                  <span className="font-medium">{format(parseISO(appt.completed_time), 'HH:mm')}</span>
                </div>
              )}
              {appt.checkout_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">結案時間</span>
                  <span className="font-medium">{format(parseISO(appt.checkout_time), 'HH:mm')}</span>
                </div>
              )}
              {appt.payment_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">收款確認</span>
                  <span className="font-medium">{format(parseISO(appt.payment_time), 'HH:mm')}</span>
                </div>
              )}
              {paymentSkipped && !appt.payment_time && (
                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">收款確認</span>
                  <span className="font-medium text-slate-400">無收款，免確認</span>
                </div>
              )}
            </Card>

            {appt.signature_data && (
              <Card className="p-4 space-y-2">
                <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider flex items-center gap-1.5">
                  <PenTool className="w-3.5 h-3.5" /> 客戶簽名
                </h4>
                <div className="border border-slate-100 rounded-md overflow-hidden bg-slate-50">
                  <img src={appt.signature_data} alt="客戶簽名" className="w-full h-auto" data-testid="img-signature" />
                </div>
              </Card>
            )}

            {appt.photos && appt.photos.length > 0 && (
              <Card className="p-4 space-y-2">
                <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">完工照片</h4>
                <div className="grid grid-cols-3 gap-2">
                  {appt.photos.map((p, i) => (
                    <img key={i} src={p} className="w-full aspect-square object-cover rounded-md" referrerPolicy="no-referrer" />
                  ))}
                </div>
              </Card>
            )}
          </div>
        )}
      </div>
    </motion.div>
  );
}

export default function TechnicianDashboard({ user, appointments, onStatusUpdate, onUpdateAppointment, onLogout }: TechnicianDashboardProps) {
  const [tab, setTab] = useState<TabType>('today');
  const [selectedAppt, setSelectedAppt] = useState<Appointment | null>(null);
  const [detailReadonly, setDetailReadonly] = useState(false);

  const myAppts = appointments.filter(a => a.technician_id === user.id);
  const today = new Date().toISOString().split('T')[0];
  const todaysAppts = myAppts
    .filter(a => a.scheduled_at.startsWith(today))
    .sort((a, b) => {
      const order: Record<string, number> = { arrived: 0, assigned: 1, completed: 2, cancelled: 3, pending: 4 };
      const oa = order[a.status] ?? 5;
      const ob = order[b.status] ?? 5;
      if (oa !== ob) return oa - ob;
      return new Date(a.scheduled_at).getTime() - new Date(b.scheduled_at).getTime();
    });

  const todayStats = {
    pending: todaysAppts.filter(a => a.status === 'assigned').length,
    active: todaysAppts.filter(a => a.status === 'arrived').length,
    done: todaysAppts.filter(a => a.status === 'completed' || a.status === 'cancelled').length,
  };

  const currentMonth = format(new Date(), 'yyyy-MM');
  // 下面三组月度集合分别代表：已结案、真实已收款、無收款。
  // 技师页既要展示绩效完成数，也要展示真实收款和未收余额，避免把“已完成”偷换成“已收款”。
  const monthFinishedAppointments = myAppts.filter(
    a => isAppointmentFinished(a) && getAppointmentClosedMonthKey(a) === currentMonth
  );
  const monthCollectedAppointments = monthFinishedAppointments.filter(isAppointmentRevenueCounted);
  const monthChargeExemptAppointments = monthFinishedAppointments.filter(isChargeExemptAppointment);

  const monthTotalEarnings = monthCollectedAppointments.reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);
  const monthOutstandingAmount = monthFinishedAppointments.reduce((sum, a) => sum + getOutstandingAmount(a), 0);
  const monthCashEarnings = monthCollectedAppointments
    .filter(isCashAppointment)
    .reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);
  const monthTransferEarnings = monthCollectedAppointments
    .filter(isTransferAppointment)
    .reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);

  const liveAppt = selectedAppt
    ? appointments.find(a => a.id === selectedAppt.id) || selectedAppt
    : null;

  const tabs: { key: TabType; icon: typeof ClipboardList; label: string }[] = [
    { key: 'today', icon: ClipboardList, label: '今日任務' },
    { key: 'future', icon: Calendar, label: '未來預約' },
    // 入口标签统一走公共常量，避免技师页、现金账页各自维护标题后再漂移。
    { key: 'earnings', icon: Wallet, label: TECHNICIAN_CLOSED_RECORDS_TITLE },
    { key: 'profile', icon: UserIcon, label: '個人' },
  ];

  return (
    <div className="min-h-screen bg-slate-50 pb-20">
      <AnimatePresence>
        {liveAppt && (
        <TaskDetail
          key={liveAppt.id}
          appt={liveAppt}
          readonly={detailReadonly}
          onBack={() => { setSelectedAppt(null); setDetailReadonly(false); }}
          onStatusUpdate={(appt, status, patch) => {
            onStatusUpdate(appt, status, patch);
            const updated = appointments.find(a => a.id === appt.id);
            if (updated) setSelectedAppt({ ...updated, status });
          }}
          onUpdateAppointment={(updated) => {
            onUpdateAppointment(updated);
            setSelectedAppt(updated);
            }}
          />
        )}
      </AnimatePresence>

      {tab === 'today' && (
        <div>
          <header className="bg-white px-5 pt-6 pb-4 border-b border-slate-100">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h1 className="text-xl font-bold text-slate-900" data-testid="text-greeting">你好，{user.name}</h1>
                <p className="text-sm text-slate-400">{format(new Date(), 'yyyy年M月d日 EEEE', { locale: zhTW })}</p>
              </div>
              <div className="w-10 h-10 bg-blue-100 rounded-full flex items-center justify-center">
                <UserIcon className="w-5 h-5 text-blue-600" />
              </div>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="bg-blue-50 rounded-md p-3 text-center">
                <p className="text-2xl font-bold text-blue-700" data-testid="text-stat-pending">{todayStats.pending}</p>
                <p className="text-[10px] font-medium text-blue-500">待出發</p>
              </div>
              <div className="bg-violet-50 rounded-md p-3 text-center">
                <p className="text-2xl font-bold text-violet-700" data-testid="text-stat-active">{todayStats.active}</p>
                <p className="text-[10px] font-medium text-violet-500">進行中</p>
              </div>
              <div className="bg-emerald-50 rounded-md p-3 text-center">
                <p className="text-2xl font-bold text-emerald-700" data-testid="text-stat-done">{todayStats.done}</p>
                <p className="text-[10px] font-medium text-emerald-500">已完成</p>
              </div>
            </div>
          </header>

          <div className="p-4 space-y-3">
            <h2 className="text-sm font-bold text-slate-500 uppercase tracking-wider px-1" data-testid="text-today-count">
              今日任務 ({todaysAppts.length})
            </h2>
            {todaysAppts.length === 0 ? (
              <div className="text-center py-16 bg-white rounded-lg border border-slate-100">
                <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-3">
                  <ClipboardList className="w-8 h-8 text-slate-300" />
                </div>
                <p className="text-slate-400 text-sm">今日沒有預約任務</p>
              </div>
            ) : (
              todaysAppts.map(a => (
                <AppointmentCard key={a.id} appt={a} onClick={() => setSelectedAppt(a)} />
              ))
            )}
          </div>
        </div>
      )}

      {tab === 'future' && (
        <div>
          <header className="bg-white px-5 pt-6 pb-4 border-b border-slate-100">
            <h1 className="text-xl font-bold text-slate-900">未來預約</h1>
            <p className="text-sm text-slate-400">未來 7 天的排程</p>
          </header>
          <div className="p-4 space-y-4">
            {Array.from({ length: 7 }, (_, i) => {
              const day = addDays(startOfDay(new Date()), i + 1);
              const dayStr = format(day, 'yyyy-MM-dd');
              const dayLabel = format(day, 'M/d（EEEEE）', { locale: zhTW });
              const dayAppts = myAppts
                .filter(a => a.scheduled_at.startsWith(dayStr))
                .sort((a, b) => new Date(a.scheduled_at).getTime() - new Date(b.scheduled_at).getTime());
              return (
                <div key={dayStr}>
                  <h3 className="text-sm font-bold text-slate-700 mb-2 px-1 flex items-center gap-2">
                    <Calendar className="w-4 h-4 text-slate-400" />
                    {dayLabel}
                    {dayAppts.length > 0 && (
                      <span className="text-xs bg-blue-100 text-blue-600 px-2 py-0.5 rounded-full">{dayAppts.length} 筆</span>
                    )}
                  </h3>
                  {dayAppts.length === 0 ? (
                    <div className="bg-slate-100/50 rounded-md p-4 text-center">
                      <p className="text-sm text-slate-400">無預約</p>
                    </div>
                  ) : (
                    <div className="space-y-2">
                      {dayAppts.map(a => (
                        <AppointmentCard key={a.id} appt={a} onClick={() => { setSelectedAppt(a); setDetailReadonly(true); }} readonly />
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {tab === 'earnings' && (
        <div>
          <header className="bg-white px-5 pt-6 pb-4 border-b border-slate-100">
            <h1 className="text-xl font-bold text-slate-900">{TECHNICIAN_CLOSED_RECORDS_TITLE}</h1>
            <p className="text-sm text-slate-400">{format(new Date(), 'yyyy年M月')} {TECHNICIAN_CLOSED_RECORDS_SUBTITLE}</p>
          </header>
          <div className="p-4 space-y-4">
            <Card className="p-4 bg-slate-50 border-slate-100">
              <p className="text-sm font-medium text-slate-700">
                統計口徑：{TECHNICIAN_EARNINGS_SCOPE_NOTE}
              </p>
            </Card>
            <div className="grid grid-cols-3 gap-3">
              <Card className="p-3 text-center">
                <p className="text-[10px] font-bold text-slate-400 uppercase mb-1">總實收</p>
                <p className="text-xl font-bold text-slate-900" data-testid="text-month-total">${monthTotalEarnings.toLocaleString()}</p>
              </Card>
              <Card className="p-3 text-center">
                <p className="text-[10px] font-bold text-amber-500 uppercase mb-1">現金</p>
                <p className="text-xl font-bold text-amber-700" data-testid="text-month-cash">${monthCashEarnings.toLocaleString()}</p>
              </Card>
              <Card className="p-3 text-center">
                <p className="text-[10px] font-bold text-blue-500 uppercase mb-1">轉帳</p>
                <p className="text-xl font-bold text-blue-700" data-testid="text-month-transfer">${monthTransferEarnings.toLocaleString()}</p>
              </Card>
            </div>

            <Card className="p-4 space-y-3 border-slate-100">
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-500">本月已結案</span>
                <span className="font-bold text-slate-900" data-testid="text-month-finished-count">{monthFinishedAppointments.length} 筆</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-500">本月無收款</span>
                <span className="font-bold text-slate-500" data-testid="text-month-charge-exempt-count">{monthChargeExemptAppointments.length} 筆</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-500">本月未收餘額</span>
                <span className="font-bold text-rose-600" data-testid="text-month-outstanding">
                  ${monthOutstandingAmount.toLocaleString()}
                </span>
              </div>
            </Card>

            <h3 className="text-sm font-bold text-slate-500 uppercase tracking-wider px-1">{TECHNICIAN_CLOSED_RECORDS_TITLE}</h3>
            {monthFinishedAppointments.length === 0 ? (
              <div className="text-center py-12 bg-white rounded-lg border border-slate-100">
                <p className="text-slate-400 text-sm">{TECHNICIAN_CLOSED_RECORDS_EMPTY_TEXT}</p>
              </div>
            ) : (
              <div className="space-y-2">
                {monthFinishedAppointments
                  .sort((a, b) => new Date(getAppointmentClosedAt(b)).getTime() - new Date(getAppointmentClosedAt(a)).getTime())
                  .map(a => {
                    const expected = getChargeableAmount(a);
                    const paid = getAppointmentCollectedAmount(a);
                    const diff = getOutstandingAmount(a);
                    const collectionLabel = getPaymentCollectionLabel(a);
                    const paymentMethodLabel = getPaymentMethodLabel(a);
                    return (
                      <Card key={a.id} className="p-4" data-testid={`earning-record-${a.id}`}>
                        <div className="flex items-start justify-between mb-2">
                          <div>
                            <p className="font-bold text-slate-900">{a.customer_name}</p>
                            <p className="text-xs text-slate-400">{format(parseISO(getAppointmentClosedAt(a)), 'MM/dd HH:mm')}</p>
                          </div>
                          <span className={cn(
                            "text-xs px-2 py-0.5 rounded-full font-medium",
                            getPaymentMethodBadgeClass(paymentMethodLabel)
                          )}>
                            {paymentMethodLabel}
                          </span>
                        </div>
                        <div className="flex items-center gap-2 mb-3">
                          <span className={cn("text-xs px-2 py-0.5 rounded-full font-bold", getPaymentCollectionBadgeClass(collectionLabel))}>
                            {collectionLabel}
                          </span>
                          {diff > 0 && (
                            <span className="text-xs font-bold text-rose-500 bg-rose-50 px-2 py-0.5 rounded-full">
                              待收 ${diff}
                            </span>
                          )}
                        </div>
                        <div className="flex items-center gap-4 text-sm">
                          <div>
                            <span className="text-slate-400">應收 </span>
                            <span className="font-medium">${expected}</span>
                          </div>
                          <div>
                            <span className="text-slate-400">實收 </span>
                            <span className="font-bold text-emerald-700">${paid}</span>
                          </div>
                        </div>
                      </Card>
                    );
                  })}
              </div>
            )}
          </div>
        </div>
      )}

      {tab === 'profile' && (
        <div>
          <header className="bg-white px-5 pt-6 pb-4 border-b border-slate-100">
            <div className="flex flex-col items-center py-4">
              <div className="w-20 h-20 bg-blue-100 rounded-full flex items-center justify-center mb-3">
                <UserIcon className="w-10 h-10 text-blue-600" />
              </div>
              <h1 className="text-xl font-bold text-slate-900" data-testid="text-profile-name">{user.name}</h1>
              <p className="text-sm text-slate-400">{user.phone}</p>
            </div>
          </header>
          <div className="p-4 space-y-4">
            <div className="grid grid-cols-3 gap-3">
              <Card className="p-4 text-center">
                <p className="text-2xl font-bold text-slate-900" data-testid="text-today-done">
                  {todaysAppts.filter(a => a.status === 'completed' || a.status === 'cancelled').length}
                </p>
                <p className="text-[10px] font-medium text-slate-400 mt-1">今日完成</p>
              </Card>
              <Card className="p-4 text-center">
                <p className="text-2xl font-bold text-slate-900" data-testid="text-month-done">{monthFinishedAppointments.length}</p>
                <p className="text-[10px] font-medium text-slate-400 mt-1">本月完成</p>
              </Card>
              <Card className="p-4 text-center">
                <p className="text-lg font-bold text-emerald-700" data-testid="text-month-earnings">${monthTotalEarnings.toLocaleString()}</p>
                <p className="text-[10px] font-medium text-slate-400 mt-1">本月實收</p>
              </Card>
            </div>

            <Card className="p-4">
              <div className="flex items-center justify-between text-sm">
                <span className="text-slate-500">未收餘額</span>
                <span className="font-bold text-rose-600" data-testid="text-profile-month-outstanding">
                  ${monthOutstandingAmount.toLocaleString()}
                </span>
              </div>
            </Card>

            {user.skills && user.skills.length > 0 && (
              <Card className="p-4">
                <h3 className="text-xs font-bold text-slate-400 uppercase tracking-wider mb-2">專長技能</h3>
                <div className="flex gap-2">
                  {user.skills.map(s => (
                    <span key={s} className="text-xs bg-blue-50 text-blue-600 px-3 py-1 rounded-full font-medium">{s}</span>
                  ))}
                </div>
              </Card>
            )}

            <Button
              variant="outline"
              data-testid="button-logout"
              className="w-full py-4 rounded-lg text-base border-red-200 text-red-500 hover:bg-red-50"
              onClick={onLogout}
            >
              <LogOut className="w-5 h-5" />
              登出
            </Button>
          </div>
        </div>
      )}

      <nav className="fixed bottom-0 left-0 right-0 bg-white border-t border-slate-100 z-40 safe-area-bottom" data-testid="tech-bottom-nav">
        <div className="flex justify-around items-center h-16 px-2">
          {tabs.map(t => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              data-testid={`tab-${t.key}`}
              className={cn(
                "flex flex-col items-center gap-0.5 px-3 py-1 rounded-md transition-all min-w-[60px]",
                tab === t.key ? "text-blue-600" : "text-slate-400"
              )}
            >
              <t.icon className="w-5 h-5" />
              <span className="text-[10px] font-medium">{t.label}</span>
            </button>
          ))}
        </div>
      </nav>
    </div>
  );
}
