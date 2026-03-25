import { useState, useEffect } from 'react';
import { useParams } from 'wouter';
import { CreditCard, Building2, AlertTriangle, CheckCircle2, Loader2, Copy, Check } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { cn } from '../lib/utils';
import { getPaymentOrderByToken, tokenCreditPay, tokenATMPay } from '../lib/api';
import { toast } from 'react-hot-toast';

// ============================================================================
// 客户支付页面（公开，无需登录，凭 PaymentToken 访问）
//
// 流程：
//   1. 管理员创建支付订单 → 生成 Token 链接
//   2. 客户打开链接 → 本页面加载订单信息
//   3. 客户选择信用卡或 ATM → 填写支付信息 → 完成支付
// ============================================================================

// PaymentOrderInfo 从后端 GET /api/payment/token/:payToken 返回的订单信息
interface PaymentOrderInfo {
  trade_amt: number;
  prod_desc: string;
  payment_method: string;
  customer_name: string;
  status: string;
  mer_trade_no: string;
  pay_no: string;
  atm_expire_date: string;
}

export default function PaymentPage() {
  const params = useParams<{ payToken: string }>();
  const payToken = params.payToken || '';

  // ---------- 状态管理 ----------
  const [orderInfo, setOrderInfo] = useState<PaymentOrderInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  // 当前选择的支付方式标签页: credit / atm
  const [activeTab, setActiveTab] = useState<'credit' | 'atm'>('credit');
  // 信用卡表单
  const [cardNo, setCardNo] = useState('');
  const [cardExpired, setCardExpired] = useState('');
  const [cardCvc, setCardCvc] = useState('');
  const [cardInst, setCardInst] = useState('1');
  // ATM 银行选择
  const [bankType, setBankType] = useState('');
  // 支付中状态
  const [isSubmitting, setIsSubmitting] = useState(false);
  // 支付结果
  const [payResult, setPayResult] = useState<any>(null);
  const [paySuccess, setPaySuccess] = useState(false);
  const [payError, setPayError] = useState('');
  // ATM 取号结果
  const [atmResult, setAtmResult] = useState<any>(null);
  // 复制帐号状态
  const [copied, setCopied] = useState(false);

  // ---------- 加载订单信息 ----------
  useEffect(() => {
    let cancelled = false;

    const loadOrder = async () => {
      if (!payToken.trim()) {
        setIsLoading(false);
        setLoadError('無效的支付連結');
        return;
      }

      try {
        const data = await getPaymentOrderByToken(payToken);
        if (cancelled) return;
        setOrderInfo(data);

        // 如果已经有 ATM 虚拟帐号，直接显示
        if (data.pay_no) {
          setAtmResult({
            pay_no: data.pay_no,
            trade_amt: String(data.trade_amt),
            atm_expire_date: data.atm_expire_date,
          });
        }

        // 根据允许的支付方式设置默认标签
        if (data.payment_method === 'atm') {
          setActiveTab('atm');
        }

        setLoadError('');
      } catch (error) {
        if (cancelled) return;
        console.error(error);
        setLoadError('支付連結不存在或已失效');
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    loadOrder();
    return () => { cancelled = true; };
  }, [payToken]);

  // ---------- 信用卡支付 ----------
  const handleCreditPay = async () => {
    if (!cardNo || !cardExpired || !cardCvc || isSubmitting) return;

    setIsSubmitting(true);
    setPayError('');

    try {
      const result = await tokenCreditPay(payToken, {
        card_no: cardNo.replace(/\s/g, ''),
        card_expired: cardExpired.replace(/\//g, ''),
        card_cvc: cardCvc,
        card_inst: cardInst,
      });

      setPayResult(result);
      if (result.trade_status === '1') {
        setPaySuccess(true);
      } else {
        setPayError(result.res_code_msg || result.message || '付款失敗');
      }
    } catch (err: any) {
      setPayError(err.message || '付款請求失敗');
    } finally {
      setIsSubmitting(false);
    }
  };

  // ---------- ATM 取号 ----------
  const handleATMPay = async () => {
    if (!bankType || isSubmitting) return;

    setIsSubmitting(true);
    setPayError('');

    try {
      const result = await tokenATMPay(payToken, bankType);
      if (result.pay_no) {
        setAtmResult(result);
      } else {
        setPayError(result.message || 'ATM 取號失敗');
      }
    } catch (err: any) {
      setPayError(err.message || 'ATM 取號請求失敗');
    } finally {
      setIsSubmitting(false);
    }
  };

  // ---------- 复制帐号 ----------
  const copyPayNo = async () => {
    if (!atmResult?.pay_no) return;
    try {
      await navigator.clipboard.writeText(atmResult.pay_no);
      setCopied(true);
      toast.success('帳號已複製');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('複製失敗');
    }
  };

  // ---------- 加载中 ----------
  if (isLoading) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <Loader2 className="w-10 h-10 text-blue-500 animate-spin mx-auto mb-4" />
          <p className="text-sm text-slate-500">正在載入支付資訊...</p>
        </div>
      </div>
    );
  }

  // ---------- 加载失败 ----------
  if (loadError || !orderInfo) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <AlertTriangle className="w-8 h-8 text-slate-400" />
          </div>
          <h2 className="text-xl font-bold text-slate-900 mb-2">載入失敗</h2>
          <p className="text-sm text-slate-500">{loadError || '無法取得支付資訊'}</p>
        </div>
      </div>
    );
  }

  // ---------- 已支付 ----------
  if (orderInfo.status === 'paid' || paySuccess) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <motion.div
          initial={{ opacity: 0, scale: 0.95 }}
          animate={{ opacity: 1, scale: 1 }}
          className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full"
        >
          <div className="w-20 h-20 bg-emerald-50 rounded-full flex items-center justify-center mx-auto mb-4">
            <CheckCircle2 className="w-10 h-10 text-emerald-500" />
          </div>
          <h2 className="text-2xl font-bold text-slate-900 mb-2">付款成功！</h2>
          <p className="text-sm text-slate-500 mb-4">感謝您的付款，訂單已確認</p>
          <div className="bg-slate-50 rounded-lg p-4 space-y-2 text-left">
            <div className="flex justify-between">
              <span className="text-xs text-slate-400">金額</span>
              <span className="text-sm font-bold text-slate-900">NT$ {orderInfo.trade_amt.toLocaleString()}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-xs text-slate-400">商品</span>
              <span className="text-sm text-slate-700">{orderInfo.prod_desc}</span>
            </div>
            {payResult?.auth_code && (
              <div className="flex justify-between">
                <span className="text-xs text-slate-400">授權碼</span>
                <span className="text-sm text-slate-700">{payResult.auth_code}</span>
              </div>
            )}
            {payResult?.card_6_no && payResult?.card_4_no && (
              <div className="flex justify-between">
                <span className="text-xs text-slate-400">卡號</span>
                <span className="text-sm text-slate-700">{payResult.card_6_no}******{payResult.card_4_no}</span>
              </div>
            )}
          </div>
        </motion.div>
      </div>
    );
  }

  // ---------- ATM 已取号（等待缴费）----------
  if (atmResult?.pay_no) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="max-w-md w-full space-y-4"
        >
          <div className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-8 text-center space-y-4">
            <div className="w-16 h-16 bg-blue-50 rounded-full flex items-center justify-center mx-auto">
              <Building2 className="w-8 h-8 text-blue-600" />
            </div>
            <h2 className="text-xl font-bold text-slate-900">ATM 轉帳資訊</h2>
            <p className="text-sm text-slate-500">請至 ATM 或網路銀行使用以下帳號完成轉帳</p>
          </div>

          <div className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-6 space-y-4">
            <div className="space-y-3">
              <div className="flex justify-between items-center">
                <span className="text-xs font-bold text-slate-400 uppercase">繳費帳號</span>
                <button
                  onClick={copyPayNo}
                  className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700 transition-colors"
                >
                  {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                  {copied ? '已複製' : '複製'}
                </button>
              </div>
              <p className="text-2xl font-mono font-bold text-slate-900 tracking-wider text-center bg-slate-50 rounded-lg py-3">
                {atmResult.pay_no}
              </p>
            </div>

            <div className="border-t border-slate-100 pt-4 space-y-3">
              <div className="flex justify-between">
                <span className="text-sm text-slate-400">轉帳金額</span>
                <span className="text-lg font-bold text-blue-600">NT$ {Number(atmResult.trade_amt).toLocaleString()}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm text-slate-400">繳費期限</span>
                <span className="text-sm font-medium text-red-500">{atmResult.atm_expire_date}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm text-slate-400">商品說明</span>
                <span className="text-sm text-slate-700">{orderInfo.prod_desc}</span>
              </div>
            </div>
          </div>

          <div className="bg-amber-50 border border-amber-200 rounded-lg p-4 text-center">
            <p className="text-xs text-amber-700">
              ⚠️ 請務必在繳費期限前完成轉帳，逾期帳號將失效
            </p>
          </div>
        </motion.div>
      </div>
    );
  }

  // ---------- 主支付表单 ----------
  // 判断可用的支付方式标签
  const showCredit = orderInfo.payment_method === 'credit' || orderInfo.payment_method === 'both';
  const showATM = orderInfo.payment_method === 'atm' || orderInfo.payment_method === 'both';

  // 台湾常用 ATM 银行代码
  const BANK_OPTIONS = [
    { code: '004', name: '台灣銀行' },
    { code: '005', name: '土地銀行' },
    { code: '006', name: '合庫銀行' },
    { code: '007', name: '第一銀行' },
    { code: '008', name: '華南銀行' },
    { code: '009', name: '彰化銀行' },
    { code: '011', name: '上海銀行' },
    { code: '012', name: '台北富邦' },
    { code: '013', name: '國泰世華' },
    { code: '017', name: '兆豐銀行' },
    { code: '021', name: '花旗銀行' },
    { code: '048', name: '王道銀行' },
    { code: '050', name: '台灣企銀' },
    { code: '052', name: '渣打銀行' },
    { code: '053', name: '台中銀行' },
    { code: '081', name: '匯豐銀行' },
    { code: '101', name: '瑞興銀行' },
    { code: '102', name: '華泰銀行' },
    { code: '108', name: '陽信銀行' },
    { code: '118', name: '板信銀行' },
    { code: '147', name: '三信銀行' },
    { code: '700', name: '中華郵政' },
    { code: '803', name: '聯邦銀行' },
    { code: '805', name: '遠東銀行' },
    { code: '806', name: '元大銀行' },
    { code: '807', name: '永豐銀行' },
    { code: '808', name: '玉山銀行' },
    { code: '809', name: '凱基銀行' },
    { code: '810', name: '星展銀行' },
    { code: '812', name: '台新銀行' },
    { code: '815', name: '日盛銀行' },
    { code: '816', name: '安泰銀行' },
    { code: '822', name: '中國信託' },
  ];

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center p-4 sm:p-6">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="max-w-lg w-full space-y-4"
      >
        {/* 订单信息卡片 */}
        <div className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-6">
          <div className="text-center space-y-3 mb-5">
            <div className="w-14 h-14 bg-blue-50 rounded-full flex items-center justify-center mx-auto">
              <CreditCard className="w-7 h-7 text-blue-600" />
            </div>
            <h1 className="text-xl font-bold text-slate-900">線上付款</h1>
          </div>

          <div className="bg-slate-50 rounded-lg p-4 space-y-2">
            <div className="flex justify-between items-center">
              <span className="text-xs font-bold text-slate-400 uppercase tracking-wider">付款資訊</span>
              <span className="text-xs text-slate-400">{orderInfo.mer_trade_no}</span>
            </div>
            <div className="flex justify-between items-baseline">
              <span className="text-sm text-slate-600">{orderInfo.prod_desc}</span>
              <span className="text-xl font-bold text-blue-600">NT$ {orderInfo.trade_amt.toLocaleString()}</span>
            </div>
            <p className="text-xs text-slate-400">{orderInfo.customer_name}</p>
          </div>
        </div>

        {/* 支付方式选择标签 */}
        {showCredit && showATM && (
          <div className="flex bg-white rounded-xl shadow-sm border border-slate-200/60 p-1.5">
            <button
              onClick={() => { setActiveTab('credit'); setPayError(''); }}
              className={cn(
                'flex-1 py-2.5 rounded-lg text-sm font-medium transition-all flex items-center justify-center gap-2',
                activeTab === 'credit'
                  ? 'bg-blue-600 text-white shadow-sm'
                  : 'text-slate-500 hover:text-slate-700'
              )}
            >
              <CreditCard className="w-4 h-4" /> 信用卡
            </button>
            <button
              onClick={() => { setActiveTab('atm'); setPayError(''); }}
              className={cn(
                'flex-1 py-2.5 rounded-lg text-sm font-medium transition-all flex items-center justify-center gap-2',
                activeTab === 'atm'
                  ? 'bg-blue-600 text-white shadow-sm'
                  : 'text-slate-500 hover:text-slate-700'
              )}
            >
              <Building2 className="w-4 h-4" /> ATM 轉帳
            </button>
          </div>
        )}

        {/* 信用卡表单 */}
        <AnimatePresence mode="wait">
          {activeTab === 'credit' && showCredit && (
            <motion.div
              key="credit"
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 10 }}
              className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-6 space-y-4"
            >
              <h3 className="text-sm font-bold text-slate-700">信用卡資訊</h3>

              <div className="space-y-3">
                <div>
                  <label className="block text-xs text-slate-500 mb-1">卡號</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    maxLength={19}
                    placeholder="4147 6310 0000 0001"
                    value={cardNo}
                    onChange={(e) => setCardNo(e.target.value)}
                    className="w-full px-4 py-3 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm font-mono"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs text-slate-500 mb-1">有效期限</label>
                    <input
                      type="text"
                      inputMode="numeric"
                      maxLength={5}
                      placeholder="MM/YY"
                      value={cardExpired}
                      onChange={(e) => setCardExpired(e.target.value)}
                      className="w-full px-4 py-3 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm font-mono"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-slate-500 mb-1">安全碼</label>
                    <input
                      type="text"
                      inputMode="numeric"
                      maxLength={4}
                      placeholder="CVV"
                      value={cardCvc}
                      onChange={(e) => setCardCvc(e.target.value)}
                      className="w-full px-4 py-3 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm font-mono"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-xs text-slate-500 mb-1">分期方式</label>
                  <select
                    value={cardInst}
                    onChange={(e) => setCardInst(e.target.value)}
                    className="w-full px-4 py-3 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm bg-white"
                  >
                    <option value="1">一次付清</option>
                    <option value="3">3 期</option>
                    <option value="6">6 期</option>
                    <option value="12">12 期</option>
                    <option value="18">18 期</option>
                    <option value="24">24 期</option>
                    <option value="30">30 期</option>
                  </select>
                </div>
              </div>

              {payError && (
                <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-center">
                  <p className="text-xs text-red-600">{payError}</p>
                </div>
              )}

              <button
                onClick={handleCreditPay}
                disabled={!cardNo || !cardExpired || !cardCvc || isSubmitting}
                className={cn(
                  'w-full py-3.5 rounded-lg font-bold text-sm transition-all flex items-center justify-center gap-2',
                  !cardNo || !cardExpired || !cardCvc || isSubmitting
                    ? 'bg-slate-100 text-slate-400 cursor-not-allowed'
                    : 'bg-blue-600 text-white hover:bg-blue-700 active:scale-[0.98] shadow-sm'
                )}
              >
                {isSubmitting ? (
                  <><Loader2 className="w-4 h-4 animate-spin" /> 處理中...</>
                ) : (
                  <>確認付款 NT$ {orderInfo.trade_amt.toLocaleString()}</>
                )}
              </button>

              <p className="text-center text-xs text-slate-400">
                支援 Visa / MasterCard / JCB / 銀聯卡
              </p>
            </motion.div>
          )}

          {/* ATM 表单 */}
          {activeTab === 'atm' && showATM && (
            <motion.div
              key="atm"
              initial={{ opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -10 }}
              className="bg-white rounded-xl shadow-sm border border-slate-200/60 p-6 space-y-4"
            >
              <h3 className="text-sm font-bold text-slate-700">ATM 轉帳</h3>

              <div>
                <label className="block text-xs text-slate-500 mb-1">選擇轉帳銀行</label>
                <select
                  value={bankType}
                  onChange={(e) => setBankType(e.target.value)}
                  className="w-full px-4 py-3 rounded-lg border border-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 text-sm bg-white"
                >
                  <option value="">請選擇銀行</option>
                  {BANK_OPTIONS.map((bank) => (
                    <option key={bank.code} value={bank.code}>
                      {bank.code} {bank.name}
                    </option>
                  ))}
                </select>
              </div>

              {payError && (
                <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-center">
                  <p className="text-xs text-red-600">{payError}</p>
                </div>
              )}

              <button
                onClick={handleATMPay}
                disabled={!bankType || isSubmitting}
                className={cn(
                  'w-full py-3.5 rounded-lg font-bold text-sm transition-all flex items-center justify-center gap-2',
                  !bankType || isSubmitting
                    ? 'bg-slate-100 text-slate-400 cursor-not-allowed'
                    : 'bg-blue-600 text-white hover:bg-blue-700 active:scale-[0.98] shadow-sm'
                )}
              >
                {isSubmitting ? (
                  <><Loader2 className="w-4 h-4 animate-spin" /> 取號中...</>
                ) : (
                  <>取得轉帳帳號</>
                )}
              </button>

              <p className="text-center text-xs text-slate-400">
                取號後請在期限內完成轉帳
              </p>
            </motion.div>
          )}
        </AnimatePresence>

        {/* 安全保障提示 */}
        <p className="text-center text-xs text-slate-400 pb-4">
          🔒 本交易由 PAYUNi 統一金流安全處理，卡號資料不經本站伺服器儲存
        </p>
      </motion.div>
    </div>
  );
}
