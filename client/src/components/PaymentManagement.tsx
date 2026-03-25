import { useState, useEffect } from 'react';
import { CreditCard, Plus, Copy, Check, ExternalLink, Building2, Clock, CheckCircle2, XCircle, Loader2, AlertTriangle, X } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { cn } from '../lib/utils';
import { createPaymentOrder, listPaymentOrders, CreatePaymentOrderRequest } from '../lib/api';
import { toast } from 'react-hot-toast';
import { format, parseISO } from 'date-fns';

// ============================================================================
// 支付管理页面（管理员专用）
//
// 功能：
//   1. 创建支付订单 → 生成支付链接 → 复制发送给客户
//   2. 查看所有支付订单记录 → 跟踪支付状态
// ============================================================================

// ---------- 订单状态映射 ----------
const STATUS_MAP: Record<string, { label: string; color: string; icon: typeof CheckCircle2 }> = {
  pending:   { label: '待支付', color: 'text-amber-600 bg-amber-50 border-amber-200', icon: Clock },
  paying:    { label: '處理中', color: 'text-blue-600 bg-blue-50 border-blue-200', icon: Loader2 },
  paid:      { label: '已付款', color: 'text-emerald-600 bg-emerald-50 border-emerald-200', icon: CheckCircle2 },
  failed:    { label: '失敗',   color: 'text-red-600 bg-red-50 border-red-200', icon: XCircle },
  expired:   { label: '已過期', color: 'text-slate-500 bg-slate-50 border-slate-200', icon: AlertTriangle },
  cancelled: { label: '已取消', color: 'text-slate-500 bg-slate-50 border-slate-200', icon: XCircle },
};

// ---------- 支付方式标签 ----------
const METHOD_LABEL: Record<string, string> = {
  credit: '信用卡',
  atm: 'ATM 轉帳',
  both: '信用卡 / ATM',
};

export default function PaymentManagement() {
  // ---------- 状态 ----------
  const [orders, setOrders] = useState<any[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  // 创建表单
  const [formAmt, setFormAmt] = useState('');
  const [formDesc, setFormDesc] = useState('');
  const [formName, setFormName] = useState('');
  const [formEmail, setFormEmail] = useState('');
  const [formPhone, setFormPhone] = useState('');
  const [formMethod, setFormMethod] = useState('both');
  // 复制链接状态
  const [copiedToken, setCopiedToken] = useState('');
  // 新创建的支付链接（弹窗）
  const [createdLink, setCreatedLink] = useState('');
  const [showLinkModal, setShowLinkModal] = useState(false);

  // ---------- 加载订单列表 ----------
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
    loadOrders();
  }, []);

  // ---------- 创建支付订单 ----------
  const handleCreate = async () => {
    const tradeAmt = parseInt(formAmt);
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
      };
      const result = await createPaymentOrder(payload);

      // 拼完整支付链接
      const baseUrl = window.location.origin;
      const fullLink = `${baseUrl}${result.payment_url}`;
      setCreatedLink(fullLink);
      setShowLinkModal(true);

      // 重置表单
      setFormAmt('');
      setFormDesc('');
      setFormName('');
      setFormEmail('');
      setFormPhone('');
      setFormMethod('both');
      setShowCreate(false);

      // 刷新列表
      await loadOrders();
      toast.success('支付訂單已建立');
    } catch (err: any) {
      toast.error(err.message || '建立支付訂單失敗');
    } finally {
      setIsCreating(false);
    }
  };

  // ---------- 复制支付链接 ----------
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

  // ---------- 获取状态配置 ----------
  const getStatusConfig = (status: string) => STATUS_MAP[status] || STATUS_MAP.pending;

  return (
    <div className="space-y-6">
      {/* 标题栏 + 创建按钮 */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-bold text-slate-900">支付訂單</h3>
          <p className="text-sm text-slate-500 mt-0.5">管理支付連結與查看交易記錄</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-2 px-4 py-2.5 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 transition-colors shadow-sm"
        >
          <Plus className="w-4 h-4" /> 建立支付連結
        </button>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {[
          { label: '全部', count: orders.length, color: 'text-slate-700' },
          { label: '待支付', count: orders.filter(o => o.status === 'pending' || o.status === 'paying').length, color: 'text-amber-600' },
          { label: '已付款', count: orders.filter(o => o.status === 'paid').length, color: 'text-emerald-600' },
          { label: '已失敗', count: orders.filter(o => o.status === 'failed' || o.status === 'cancelled').length, color: 'text-red-500' },
        ].map(stat => (
          <div key={stat.label} className="bg-white rounded-lg border border-slate-200/60 p-4 text-center">
            <p className={cn('text-2xl font-bold', stat.color)}>{stat.count}</p>
            <p className="text-xs text-slate-400 mt-1">{stat.label}</p>
          </div>
        ))}
      </div>

      {/* 订单列表 */}
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
            const statusConfig = getStatusConfig(order.status);
            const StatusIcon = statusConfig.icon;
            return (
              <div key={order.id} className="bg-white rounded-lg border border-slate-200/60 p-4 hover:shadow-sm transition-shadow">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex-1 min-w-0 space-y-2">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm font-bold text-slate-900">{order.customer_name}</span>
                      <span className={cn('inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium border', statusConfig.color)}>
                        <StatusIcon className={cn('w-3 h-3', order.status === 'paying' && 'animate-spin')} />
                        {statusConfig.label}
                      </span>
                      <span className="text-xs text-slate-400">{METHOD_LABEL[order.payment_method] || order.payment_method}</span>
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
                        <span className="text-emerald-500">✓ {format(parseISO(order.paid_at), 'MM/dd HH:mm')}</span>
                      )}
                    </div>
                  </div>

                  {/* 操作按钮 */}
                  <div className="flex items-center gap-1.5 flex-shrink-0">
                    {(order.status === 'pending' || order.status === 'paying') && (
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
                    {(order.status === 'pending' || order.status === 'paying') && (
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

      {/* ==================== 创建支付订单弹窗 ==================== */}
      <AnimatePresence>
        {showCreate && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[80] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4"
            onClick={() => setShowCreate(false)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9, y: 20 }}
              transition={{ type: 'spring', damping: 25, stiffness: 300 }}
              className="bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="px-6 py-5 border-b border-slate-100 flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-blue-50 rounded-lg flex items-center justify-center">
                    <CreditCard className="w-5 h-5 text-blue-600" />
                  </div>
                  <h3 className="text-lg font-bold text-slate-900">建立支付連結</h3>
                </div>
                <button
                  onClick={() => setShowCreate(false)}
                  className="w-8 h-8 rounded-full hover:bg-slate-100 flex items-center justify-center transition-colors"
                >
                  <X className="w-4 h-4 text-slate-400" />
                </button>
              </div>

              <div className="px-6 py-5 space-y-4">
                <div>
                  <label className="block text-xs font-medium text-slate-500 mb-1.5">客戶名稱 *</label>
                  <input
                    type="text"
                    placeholder="王小明"
                    value={formName}
                    onChange={(e) => setFormName(e.target.value)}
                    className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">金額 (NT$) *</label>
                    <input
                      type="number"
                      placeholder="1500"
                      value={formAmt}
                      onChange={(e) => setFormAmt(e.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">支付方式</label>
                    <select
                      value={formMethod}
                      onChange={(e) => setFormMethod(e.target.value)}
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
                    onChange={(e) => setFormDesc(e.target.value)}
                    className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">Email（選填）</label>
                    <input
                      type="email"
                      placeholder="customer@example.com"
                      value={formEmail}
                      onChange={(e) => setFormEmail(e.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-500 mb-1.5">電話（選填）</label>
                    <input
                      type="tel"
                      placeholder="0912345678"
                      value={formPhone}
                      onChange={(e) => setFormPhone(e.target.value)}
                      className="w-full px-3.5 py-2.5 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm"
                    />
                  </div>
                </div>
              </div>

              <div className="px-6 py-4 bg-slate-50 border-t border-slate-100 flex gap-3">
                <button
                  onClick={() => setShowCreate(false)}
                  className="flex-1 py-2.5 rounded-lg text-sm font-medium text-slate-600 border border-slate-200 hover:bg-white transition-colors"
                >
                  取消
                </button>
                <button
                  onClick={handleCreate}
                  disabled={isCreating || !formAmt || !formDesc || !formName}
                  className={cn(
                    'flex-1 py-2.5 rounded-lg text-sm font-bold transition-all flex items-center justify-center gap-2',
                    isCreating || !formAmt || !formDesc || !formName
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

      {/* ==================== 支付链接生成成功弹窗 ==================== */}
      <AnimatePresence>
        {showLinkModal && createdLink && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[80] flex items-center justify-center bg-black/40 backdrop-blur-sm p-4"
            onClick={() => setShowLinkModal(false)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9, y: 20 }}
              transition={{ type: 'spring', damping: 25, stiffness: 300 }}
              className="bg-white rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-5"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="text-center space-y-2">
                <div className="w-16 h-16 bg-emerald-50 rounded-full flex items-center justify-center mx-auto">
                  <CheckCircle2 className="w-8 h-8 text-emerald-500" />
                </div>
                <h3 className="text-lg font-bold text-slate-900">支付連結已建立！</h3>
                <p className="text-sm text-slate-500">複製以下連結發送給客戶</p>
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
    </div>
  );
}
