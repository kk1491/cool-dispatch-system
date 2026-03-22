import { useState, useMemo } from 'react';
import { Star, Filter, BarChart3, AlertTriangle } from 'lucide-react';
import { cn } from '../lib/utils';
import { Review, User, MisconductType } from '../types';
import { Card, Button } from './shared';

const MISCONDUCT_LABELS: Record<MisconductType, string> = {
  private_contact: '要求加私人聯繫方式',
  not_clean: '現場未清理乾淨',
  bad_attitude: '服務態度不佳',
  late_arrival: '遲到未通知',
  damage_property: '損壞物品',
  overcharge: '額外加價',
  other: '其他問題',
};

interface ReviewDashboardProps {
  reviews: Review[];
  technicians: User[];
  appointments: { id: number; technician_id?: number; technician_name?: string }[];
}

export default function ReviewDashboard({ reviews, technicians, appointments }: ReviewDashboardProps) {
  const [techFilter, setTechFilter] = useState<number | 'all'>('all');
  const [ratingFilter, setRatingFilter] = useState<number | 'all'>('all');

  const enrichedReviews = useMemo(() => {
    return reviews.map(review => {
      const appt = appointments.find(a => a.id === review.appointment_id);
      return {
        ...review,
        technician_id: review.technician_id ?? appt?.technician_id,
        technician_name: review.technician_name ?? appt?.technician_name,
      };
    });
  }, [reviews, appointments]);

  const filteredReviews = useMemo(() => {
    return enrichedReviews.filter(review => {
      const matchesTech = techFilter === 'all' || review.technician_id === techFilter;
      const matchesRating = ratingFilter === 'all' || review.rating === ratingFilter;
      return matchesTech && matchesRating;
    });
  }, [enrichedReviews, techFilter, ratingFilter]);

  const avgRating = filteredReviews.length > 0
    ? (filteredReviews.reduce((sum, r) => sum + r.rating, 0) / filteredReviews.length).toFixed(1)
    : '0.0';

  const ratingDistribution = useMemo(() => {
    const dist = [0, 0, 0, 0, 0];
    filteredReviews.forEach(r => { dist[r.rating - 1]++; });
    return dist;
  }, [filteredReviews]);

  const misconductStats = useMemo(() => {
    const stats: Partial<Record<MisconductType, number>> = {};
    filteredReviews.forEach(r => {
      (r.misconducts || []).forEach(m => {
        stats[m] = (stats[m] || 0) + 1;
      });
    });
    return Object.entries(stats)
      .sort(([, a], [, b]) => b - a) as [MisconductType, number][];
  }, [filteredReviews]);

  const maxCount = Math.max(...ratingDistribution, 1);

  return (
    <div className="space-y-8">
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="p-6 bg-amber-50 border-amber-100/50">
          <div className="flex items-center justify-between mb-2">
            <p className="text-[10px] font-bold text-amber-400 uppercase tracking-wider">平均評分</p>
            <BarChart3 className="w-4 h-4 text-amber-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <p className="text-3xl font-bold text-amber-900" data-testid="text-avg-rating">{avgRating}</p>
            <div className="flex gap-0.5">
              {[1, 2, 3, 4, 5].map(s => (
                <Star key={s} className={cn("w-4 h-4", parseFloat(avgRating) >= s ? "text-amber-400 fill-amber-400" : "text-amber-200")} />
              ))}
            </div>
          </div>
          <p className="text-xs text-amber-600 mt-1">{filteredReviews.length} 則評價</p>
        </Card>

        <Card className="p-6">
          <p className="text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-3">評分分布</p>
          <div className="space-y-1.5">
            {[5, 4, 3, 2, 1].map(star => (
              <div key={star} className="flex items-center gap-2">
                <span className="text-xs text-slate-500 w-4 text-right">{star}</span>
                <Star className="w-3 h-3 text-amber-400 fill-amber-400" />
                <div className="flex-1 bg-slate-100 rounded-full h-2 overflow-hidden">
                  <div
                    className="bg-amber-400 h-full rounded-full transition-all"
                    style={{ width: `${(ratingDistribution[star - 1] / maxCount) * 100}%` }}
                  />
                </div>
                <span className="text-xs text-slate-400 w-6 text-right" data-testid={`text-rating-count-${star}`}>{ratingDistribution[star - 1]}</span>
              </div>
            ))}
          </div>
        </Card>

        <Card className="p-6">
          <p className="text-[10px] font-bold text-slate-400 uppercase tracking-wider mb-3">快速篩選</p>
          <div className="space-y-3">
            <div>
              <label className="text-xs text-slate-500 mb-1 block">師傅</label>
              <select
                value={techFilter}
                onChange={e => setTechFilter(e.target.value === 'all' ? 'all' : parseInt(e.target.value))}
                data-testid="select-review-tech-filter"
                className="w-full px-3 py-2 bg-slate-50 border border-slate-100 rounded-md text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="all">所有師傅</option>
                {technicians.map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-500 mb-1 block">星級</label>
              <select
                value={ratingFilter}
                onChange={e => setRatingFilter(e.target.value === 'all' ? 'all' : parseInt(e.target.value))}
                data-testid="select-review-rating-filter"
                className="w-full px-3 py-2 bg-slate-50 border border-slate-100 rounded-md text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="all">所有星級</option>
                {[5, 4, 3, 2, 1].map(s => (
                  <option key={s} value={s}>{s} 星</option>
                ))}
              </select>
            </div>
            <Button
              variant="outline"
              className="w-full text-xs"
              data-testid="button-reset-review-filters"
              onClick={() => { setTechFilter('all'); setRatingFilter('all'); }}
            >
              <Filter className="w-3 h-3" />
              重設篩選
            </Button>
          </div>
        </Card>
      </div>

      {misconductStats.length > 0 && (
        <Card className="p-6 border-rose-100" data-testid="section-misconduct-stats">
          <div className="flex items-center gap-2 mb-4">
            <AlertTriangle className="w-4 h-4 text-rose-500" />
            <p className="text-sm font-bold text-rose-700">不良行為統計</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {misconductStats.map(([key, count]) => (
              <span
                key={key}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-rose-50 border border-rose-100 rounded-lg text-xs text-rose-700 font-medium"
                data-testid={`stat-misconduct-${key}`}
              >
                {MISCONDUCT_LABELS[key]}
                <span className="bg-rose-500 text-white px-1.5 py-0.5 rounded-full text-[10px] font-bold">{count}</span>
              </span>
            ))}
          </div>
        </Card>
      )}

      <div className="space-y-3">
        <h3 className="text-xs font-bold text-slate-400 uppercase tracking-wider">評價列表 ({filteredReviews.length})</h3>
        {filteredReviews.length === 0 ? (
          <Card className="p-10 text-center">
            <div className="w-14 h-14 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
              <Star className="w-7 h-7 text-slate-300" />
            </div>
            <p className="text-slate-500 text-sm" data-testid="text-no-reviews">目前沒有符合條件的評價</p>
          </Card>
        ) : (
          <Card className="overflow-hidden" data-testid="table-reviews">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-100 bg-slate-50/60">
                    <th className="text-left px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">客戶</th>
                    <th className="text-left px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">評分</th>
                    <th className="text-left px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">師傅</th>
                    <th className="text-left px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">留言</th>
                    <th className="text-left px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">違規事項</th>
                    <th className="text-right px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">訂單</th>
                    <th className="text-right px-4 py-3 text-[10px] font-bold text-slate-400 uppercase tracking-wider">日期</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredReviews.map((review, idx) => (
                    <tr
                      key={review.id}
                      className={cn("border-b border-slate-50 hover:bg-slate-50/50 transition-colors", idx % 2 === 1 && "bg-slate-25")}
                      data-testid={`row-review-${review.id}`}
                    >
                      <td className="px-4 py-3">
                        <span className="font-medium text-slate-900" data-testid={`text-review-name-${review.id}`}>{review.customer_name}</span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-0.5">
                          {[1, 2, 3, 4, 5].map(s => (
                            <Star key={s} className={cn("w-3.5 h-3.5", review.rating >= s ? "text-amber-400 fill-amber-400" : "text-slate-200")} />
                          ))}
                        </div>
                      </td>
                      <td className="px-4 py-3" data-testid={`cell-review-tech-${review.id}`}>
                        {review.technician_name ? (
                          <span className="px-2 py-0.5 bg-slate-100 rounded-full text-[10px] font-medium text-slate-500">{review.technician_name}</span>
                        ) : (
                          <span className="text-slate-300 text-xs">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 max-w-[200px]">
                        {review.comment ? (
                          <p className="text-slate-600 text-xs truncate" data-testid={`text-review-comment-${review.id}`} title={review.comment}>{review.comment}</p>
                        ) : (
                          <span className="text-slate-300 text-xs">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3" data-testid={`cell-review-misconducts-${review.id}`}>
                        {review.misconducts && review.misconducts.length > 0 ? (
                          <div className="flex flex-wrap gap-1">
                            {review.misconducts.map(m => (
                              <span key={m} className="inline-block px-1.5 py-0.5 bg-rose-50 border border-rose-100 rounded text-[10px] font-medium text-rose-600 whitespace-nowrap">
                                {MISCONDUCT_LABELS[m]}
                              </span>
                            ))}
                          </div>
                        ) : (
                          <span className="px-1.5 py-0.5 bg-green-50 border border-green-100 rounded text-[10px] font-medium text-green-600">無</span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right" data-testid={`cell-review-order-${review.id}`}>
                        <span className="text-xs text-slate-400">#{review.appointment_id}</span>
                      </td>
                      <td className="px-4 py-3 text-right" data-testid={`cell-review-date-${review.id}`}>
                        <span className="text-xs text-slate-400">{new Date(review.created_at).toLocaleDateString('zh-TW')}</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
