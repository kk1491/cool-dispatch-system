import { useState, useEffect } from 'react';
import { useParams } from 'wouter';
import { Star, Send, AlertTriangle, CheckCircle2, MessageCircle, Heart } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { cn } from '../lib/utils';
import { Review, ReviewDraft, MisconductType, Appointment } from '../types';
import { fetchReviewContext, updateReviewShareLine } from '../lib/api';
import { toast } from 'react-hot-toast';

const MISCONDUCT_OPTIONS: { key: MisconductType; label: string; icon: string }[] = [
  { key: 'private_contact', label: '師傅要求加私人聯繫方式', icon: '📱' },
  { key: 'not_clean', label: '現場沒有清理乾淨', icon: '🧹' },
  { key: 'bad_attitude', label: '服務態度不佳', icon: '😤' },
  { key: 'late_arrival', label: '遲到未事先通知', icon: '⏰' },
  { key: 'damage_property', label: '損壞家中物品', icon: '🔨' },
  { key: 'overcharge', label: '現場額外加價', icon: '💰' },
  { key: 'other', label: '其他問題', icon: '⚠️' },
];

interface ReviewPageProps {
  onSubmit: (reviewToken: string, review: ReviewDraft) => Promise<Review>;
}

export default function ReviewPage({ onSubmit }: ReviewPageProps) {
  const params = useParams<{ reviewToken: string }>();
  const reviewToken = params.reviewToken || '';
  const [remoteAppointment, setRemoteAppointment] = useState<Appointment | null>(null);
  const [remoteReview, setRemoteReview] = useState<Review | null>(null);
  const [isLoadingContext, setIsLoadingContext] = useState(true);
  const [contextError, setContextError] = useState('');
  // 评价页已统一依赖后端 context 接口，不再回退读取父组件注入的本地列表数据。
  const appointment = remoteAppointment;
  const existingReview = remoteReview;

  const [rating, setRating] = useState<number>(0);
  const [hoverRating, setHoverRating] = useState<number>(0);
  const [misconducts, setMisconducts] = useState<MisconductType[]>([]);
  const [comment, setComment] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const [submittedRating, setSubmittedRating] = useState(0);
  const [sharedLine, setSharedLine] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isSharingLine, setIsSharingLine] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const loadContext = async () => {
      if (!reviewToken.trim()) {
        setIsLoadingContext(false);
        setContextError('無效的評價連結');
        return;
      }

      try {
        const data = await fetchReviewContext(reviewToken);
        if (cancelled) {
          return;
        }
        setRemoteAppointment(data.appointment || null);
        setRemoteReview(data.review || null);
        setContextError('');
      } catch (error) {
        if (cancelled) {
          return;
        }
        console.error(error);
        setContextError('讀取評價頁資料失敗，請稍後再試。');
      } finally {
        if (!cancelled) {
          setIsLoadingContext(false);
        }
      }
    };

    loadContext();
    return () => {
      cancelled = true;
    };
  }, [reviewToken]);

  useEffect(() => {
    if (existingReview) {
      setRating(existingReview.rating);
      setMisconducts(existingReview.misconducts || []);
      setComment(existingReview.comment || '');
      setSubmitted(true);
      setSubmittedRating(existingReview.rating);
      setSharedLine(Boolean(existingReview.shared_line));
    }
  }, [existingReview]);

  const toggleMisconduct = (key: MisconductType) => {
    setMisconducts(prev =>
      prev.includes(key) ? prev.filter(k => k !== key) : [...prev, key]
    );
  };

  // handleSubmit 只在后端成功保存评价后再切换感谢页，避免接口失败却误提示已完成提交。
  const handleSubmit = async () => {
    if (rating === 0 || !appointment || isSubmitting) return;

    const review: ReviewDraft = {
      rating: rating as 1 | 2 | 3 | 4 | 5,
      misconducts,
      comment,
      shared_line: sharedLine,
    };

    setIsSubmitting(true);
    try {
      const saved = await onSubmit(reviewToken, review);
      setRemoteReview(saved);
      setSubmitted(true);
      setSubmittedRating(saved.rating);
    } catch (error) {
      console.error(error);
    } finally {
      setIsSubmitting(false);
    }
  };

  // handleLineShare 先在点击上下文中打开分享页，再回写后端状态，避免浏览器拦截弹窗。
  const handleLineShare = async () => {
    if (sharedLine || isSharingLine) {
      return;
    }

    const shareText = '我剛請了CoolDispatch來洗冷氣，服務很讚！推薦給你 👉';
    const shareUrl = 'https://cooldispatch.com';
    const lineShareUrl = `https://social-plugins.line.me/lineit/share?url=${encodeURIComponent(shareUrl)}&text=${encodeURIComponent(shareText)}`;
    window.open(lineShareUrl, '_blank');

    setIsSharingLine(true);
    try {
      const saved = await updateReviewShareLine(reviewToken, true);
      setRemoteReview(saved);
      setSharedLine(Boolean(saved.shared_line));
    } catch (error) {
      console.error(error);
      toast.error('更新分享狀態失敗，請稍後再試');
    } finally {
      setIsSharingLine(false);
    }
  };

  if (isLoadingContext) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <div className="w-10 h-10 rounded-full border-4 border-slate-200 border-t-blue-600 animate-spin mx-auto mb-4" />
          <p className="text-sm text-slate-500">正在載入評價資料...</p>
        </div>
      </div>
    );
  }

  if (contextError && !appointment) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <AlertTriangle className="w-8 h-8 text-slate-400" />
          </div>
          <h2 className="text-xl font-bold text-slate-900 mb-2">載入失敗</h2>
          <p className="text-sm text-slate-500">{contextError}</p>
        </div>
      </div>
    );
  }

  if (!appointment) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <AlertTriangle className="w-8 h-8 text-slate-400" />
          </div>
          <h2 className="text-xl font-bold text-slate-900 mb-2" data-testid="text-review-not-found">找不到此預約</h2>
          <p className="text-sm text-slate-500">此評價連結不存在、已失效或已被移除。</p>
        </div>
      </div>
    );
  }

  if (appointment.status !== 'completed') {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-10 text-center max-w-md w-full">
          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <AlertTriangle className="w-8 h-8 text-amber-400" />
          </div>
          <h2 className="text-xl font-bold text-slate-900 mb-2" data-testid="text-review-not-completed">此訂單尚未完成</h2>
          <p className="text-sm text-slate-500">訂單完成後才能填寫評價</p>
        </div>
      </div>
    );
  }

  const displayRating = hoverRating || rating;
  const ratingLabels: Record<number, string> = { 1: '非常不滿意', 2: '不太滿意', 3: '普通', 4: '滿意', 5: '非常滿意' };

  return (
    <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
      <div className="max-w-lg w-full">
        <AnimatePresence mode="wait">
          {!submitted ? (
            <motion.div
              key="form"
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -20 }}
              className="space-y-5"
            >
              <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-8 space-y-8">
                <div className="text-center space-y-2">
                  <div className="w-14 h-14 bg-blue-50 rounded-lg flex items-center justify-center mx-auto mb-4">
                    <Star className="w-7 h-7 text-blue-600" />
                  </div>
                  <h1 className="text-2xl font-bold text-slate-900" data-testid="text-review-title">服務評價</h1>
                  <p className="text-sm text-slate-500">
                    {appointment.customer_name} 您好，感謝您使用 CoolDispatch 冷氣清洗服務
                  </p>
                </div>

                <div className="bg-slate-50 rounded-lg p-4 space-y-2">
                  <div className="flex justify-between items-center">
                    <span className="text-xs font-bold text-slate-400 uppercase tracking-wider">預約資訊</span>
                    <span className="text-xs text-slate-400">#{appointment.id}</span>
                  </div>
                  <p className="text-sm font-medium text-slate-900" data-testid="text-review-customer">{appointment.customer_name}</p>
                  <p className="text-xs text-slate-500">{appointment.address}</p>
                  <div className="flex gap-4 mt-1">
                    {appointment.technician_name && (
                      <p className="text-xs text-slate-500" data-testid="text-review-tech">服務師傅：{appointment.technician_name}</p>
                    )}
                    <p className="text-xs text-slate-500" data-testid="text-review-items">{appointment.items.map(i => i.type).join('、')} × {appointment.items.length} 台</p>
                  </div>
                </div>

                <div className="space-y-3">
                  <h3 className="text-xs font-bold text-slate-400 uppercase tracking-wider text-center">請為本次服務評分</h3>
                  <div className="flex justify-center gap-2" data-testid="rating-stars">
                    {[1, 2, 3, 4, 5].map((star) => (
                      <button
                        key={star}
                        type="button"
                        onClick={() => setRating(star)}
                        onMouseEnter={() => setHoverRating(star)}
                        onMouseLeave={() => setHoverRating(0)}
                        data-testid={`button-star-${star}`}
                        className="p-1 transition-transform active:scale-90"
                      >
                        <Star
                          className={cn(
                            "w-10 h-10 transition-colors",
                            displayRating >= star
                              ? "text-amber-400 fill-amber-400"
                              : "text-slate-200"
                          )}
                        />
                      </button>
                    ))}
                  </div>
                  {displayRating > 0 && (
                    <p className="text-center text-sm text-slate-500" data-testid="text-rating-label">
                      {ratingLabels[displayRating]}
                    </p>
                  )}
                </div>
              </div>

              <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-6 space-y-4" data-testid="section-misconduct">
                <div>
                  <h3 className="text-sm font-bold text-slate-700 mb-1">師傅是否有以下情況？</h3>
                  <p className="text-xs text-slate-400">若無問題可直接跳過此區塊</p>
                </div>
                <div className="space-y-2">
                  {MISCONDUCT_OPTIONS.map(opt => (
                    <label
                      key={opt.key}
                      data-testid={`checkbox-misconduct-${opt.key}`}
                      className={cn(
                        "flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-all border",
                        misconducts.includes(opt.key)
                          ? "bg-rose-50 border-rose-200"
                          : "bg-white border-slate-100 hover:bg-slate-50"
                      )}
                    >
                      <input
                        type="checkbox"
                        checked={misconducts.includes(opt.key)}
                        onChange={() => toggleMisconduct(opt.key)}
                        className="sr-only"
                      />
                      <div className={cn(
                        "w-5 h-5 rounded-md border-2 flex items-center justify-center flex-shrink-0 transition-colors",
                        misconducts.includes(opt.key)
                          ? "bg-rose-500 border-rose-500 text-white"
                          : "border-slate-300"
                      )}>
                        {misconducts.includes(opt.key) && (
                          <svg className="w-3 h-3" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M2 6l3 3 5-5" />
                          </svg>
                        )}
                      </div>
                      <span className="text-lg">{opt.icon}</span>
                      <span className="text-sm text-slate-700">{opt.label}</span>
                    </label>
                  ))}
                </div>
              </div>

              <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-6 space-y-3" data-testid="section-comment">
                <div className="flex items-center gap-2">
                  <MessageCircle className="w-4 h-4 text-slate-400" />
                  <h3 className="text-sm font-bold text-slate-700">想對我們說的話</h3>
                  <span className="text-xs text-slate-400">（選填）</span>
                </div>
                <textarea
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder="請留下您的寶貴意見，幫助我們改善服務..."
                  data-testid="input-review-comment"
                  className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500 text-sm resize-none"
                  rows={4}
                />
              </div>

              <button
                onClick={handleSubmit}
                disabled={rating === 0 || isSubmitting}
                data-testid="button-submit-review"
                className={cn(
                  "w-full py-4 rounded-lg font-bold transition-all active:scale-[0.98] flex items-center justify-center gap-2",
                  rating > 0 && !isSubmitting
                    ? "bg-blue-600 text-white hover:bg-blue-700 shadow-sm shadow-blue-100"
                    : "bg-slate-100 text-slate-400 cursor-not-allowed"
                )}
              >
                <Send className="w-4 h-4" />
                {isSubmitting ? '送出中...' : '送出評價'}
              </button>

              <p className="text-center text-xs text-slate-400">
                此為一次性評價連結，送出後無法修改
              </p>
            </motion.div>
          ) : (
            <motion.div
              key="thanks"
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              className="space-y-5"
            >
              <div className="bg-white rounded-lg shadow-sm border border-slate-200/60 p-8 text-center space-y-4">
                <div className="w-16 h-16 bg-emerald-50 rounded-full flex items-center justify-center mx-auto">
                  <CheckCircle2 className="w-8 h-8 text-emerald-500" />
                </div>
                <h2 className="text-2xl font-bold text-slate-900" data-testid="text-review-thanks">感謝您的評價！</h2>
                <p className="text-sm text-slate-500">您的回饋對我們非常重要，將幫助我們持續改善服務品質</p>
                <div className="flex justify-center gap-1">
                  {[1, 2, 3, 4, 5].map((star) => (
                    <Star
                      key={star}
                      className={cn(
                        "w-6 h-6",
                        submittedRating >= star ? "text-amber-400 fill-amber-400" : "text-slate-200"
                      )}
                    />
                  ))}
                </div>
              </div>

              {submittedRating >= 4 && (
                <div className="bg-gradient-to-br from-green-500 to-emerald-600 rounded-lg shadow-lg p-6 text-center text-white" data-testid="review-share-cta">
                  <Heart className="w-10 h-10 mx-auto mb-3 opacity-90" />
                  <h3 className="text-lg font-bold mb-2">喜歡我們的服務嗎？</h3>
                  <p className="text-sm opacity-90 mb-5">分享給朋友，讓他們也能享受專業的冷氣清洗服務！</p>
                  <button
                    data-testid="button-share-line"
                    onClick={handleLineShare}
                    disabled={sharedLine || isSharingLine}
                    className={cn(
                      "inline-flex items-center gap-2 font-bold px-6 py-3 rounded-xl transition-colors shadow-md",
                      sharedLine || isSharingLine
                        ? "bg-green-50 text-green-600 cursor-not-allowed"
                        : "bg-white text-green-600 hover:bg-green-50"
                    )}
                  >
                    <svg viewBox="0 0 24 24" className="w-5 h-5 fill-[#06C755]">
                      <path d="M19.365 9.863c.349 0 .63.285.63.631 0 .345-.281.63-.63.63H17.61v1.125h1.755c.349 0 .63.283.63.63 0 .344-.281.629-.63.629h-2.386c-.345 0-.627-.285-.627-.629V8.108c0-.345.282-.63.63-.63h2.386c.346 0 .627.285.627.63 0 .349-.281.63-.63.63H17.61v1.125h1.755zm-3.855 3.016c0 .27-.174.51-.432.596-.064.021-.133.031-.199.031-.211 0-.391-.09-.51-.25l-2.443-3.317v2.94c0 .344-.279.629-.631.629-.346 0-.626-.285-.626-.629V8.108c0-.27.173-.51.43-.595.06-.023.136-.033.194-.033.195 0 .375.104.495.254l2.462 3.33V8.108c0-.345.282-.63.63-.63.345 0 .63.285.63.63v4.771zm-5.741 0c0 .344-.282.629-.631.629-.345 0-.627-.285-.627-.629V8.108c0-.345.282-.63.63-.63.346 0 .628.285.628.63v4.771zm-2.466.629H4.917c-.345 0-.63-.285-.63-.629V8.108c0-.345.285-.63.63-.63.348 0 .63.285.63.63v4.141h1.756c.348 0 .629.283.629.63 0 .344-.282.629-.629.629M24 10.314C24 4.943 18.615.572 12 .572S0 4.943 0 10.314c0 4.811 4.27 8.842 10.035 9.608.391.082.923.258 1.058.59.12.301.079.766.038 1.08l-.164 1.02c-.045.301-.24 1.186 1.049.645 1.291-.539 6.916-4.078 9.436-6.975C23.176 14.393 24 12.458 24 10.314" />
                    </svg>
                    {sharedLine ? '已完成分享' : isSharingLine ? '分享中...' : '一鍵分享給朋友'}
                  </button>
                  {sharedLine && (
                    <p className="text-xs mt-3 opacity-75" data-testid="text-shared-thanks">感謝您的推薦！</p>
                  )}
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
}
