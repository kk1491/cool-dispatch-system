import { useState, Fragment } from 'react';
import { Search, ChevronDown, ChevronUp, Phone, MapPin, MessageSquare, Star, Calendar, DollarSign, Hash } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { Card, Badge } from './shared';
import { Customer, Appointment, Review } from '../types';

interface CustomerManagementProps {
  customers: Customer[];
  onUpdate: (c: Customer[]) => void;
  appointments: Appointment[];
  reviews: Review[];
}

export default function CustomerManagement({ customers, onUpdate, appointments, reviews }: CustomerManagementProps) {
  const [search, setSearch] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  
  const filtered = customers.filter(c => 
    c.name.includes(search) || c.phone.includes(search) || c.address.includes(search)
  );

  const getCustomerAppointments = (c: Customer) => {
    return appointments
      .filter(a => a.phone === c.phone || a.customer_name === c.name)
      .sort((a, b) => new Date(b.scheduled_at).getTime() - new Date(a.scheduled_at).getTime());
  };

  const getCustomerReviews = (c: Customer) => {
    const custApptIds = getCustomerAppointments(c).map(a => a.id);
    return reviews.filter(r => custApptIds.includes(r.appointment_id));
  };

  const getCustomerStats = (c: Customer) => {
    const custAppts = getCustomerAppointments(c);
    const completedAppts = custAppts.filter(a => a.status === 'completed');
    const custReviews = getCustomerReviews(c);
    const totalSpent = completedAppts.reduce((sum, a) => sum + a.total_amount, 0);
    const lastService = completedAppts.length > 0 
      ? completedAppts[0].checkout_time || completedAppts[0].scheduled_at 
      : null;
    const avgRating = custReviews.length > 0 
      ? custReviews.reduce((sum, r) => sum + r.rating, 0) / custReviews.length 
      : null;

    return {
      totalServices: completedAppts.length,
      totalSpent,
      lastServiceDate: lastService,
      avgRating,
    };
  };

  const toggleExpand = (id: string) => {
    setExpandedId(prev => prev === id ? null : id);
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center flex-wrap gap-2">
        <h2 className="text-2xl font-bold">顧客管理</h2>
        <div className="relative w-64">
          <input 
            data-testid="input-search-customer"
            type="text" 
            placeholder="搜尋顧客..." 
            className="w-full pl-10 pr-4 py-2 bg-white border border-slate-200 rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
        </div>
      </div>

      <Card className="overflow-hidden">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="bg-slate-50 text-xs font-bold text-slate-400 uppercase tracking-wider">
              <th className="px-6 py-4">姓名</th>
              <th className="px-6 py-4">手機 (ID)</th>
              <th className="px-6 py-4">地址</th>
              <th className="px-6 py-4">LINE ID</th>
              <th className="px-6 py-4">建立時間</th>
              <th className="px-6 py-4 w-10"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {filtered.map(c => {
              const isExpanded = expandedId === c.id;
              const custAppts = getCustomerAppointments(c);
              const custReviews = getCustomerReviews(c);
              const stats = getCustomerStats(c);

              return (
                <Fragment key={c.id}>
                  <tr
                    className="hover:bg-slate-50 transition-colors cursor-pointer"
                    data-testid={`row-customer-${c.id}`}
                    onClick={() => toggleExpand(c.id)}
                  >
                    <td className="px-6 py-4 font-bold">{c.name}</td>
                    <td className="px-6 py-4 text-sm text-slate-500">{c.phone}</td>
                    <td className="px-6 py-4 text-sm text-slate-500">{c.address}</td>
                    <td className="px-6 py-4 text-sm text-slate-500">{c.line_uid || c.line_id || '-'}</td>
                    <td className="px-6 py-4 text-sm text-slate-500">{format(parseISO(c.created_at), 'yyyy/MM/dd')}</td>
                    <td className="px-6 py-4 text-sm text-slate-400">
                      {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                    </td>
                  </tr>
                  {isExpanded && (
                    <tr data-testid={`detail-customer-${c.id}`}>
                      <td colSpan={6} className="px-0 py-0">
                        <div className="bg-slate-50 border-t border-slate-100 p-6 space-y-6">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                            <div className="bg-white rounded-lg border border-slate-200/60 p-5 space-y-3">
                              <h3 className="text-sm font-bold text-slate-700">基本資訊</h3>
                              <div className="space-y-2 text-sm">
                                <div className="flex items-center gap-2 text-slate-600">
                                  <span className="font-medium text-slate-800">{c.name}</span>
                                </div>
                                <div className="flex items-center gap-2 text-slate-500">
                                  <Phone className="w-3.5 h-3.5" />
                                  <span>{c.phone || '-'}</span>
                                </div>
                                <div className="flex items-center gap-2 text-slate-500">
                                  <MapPin className="w-3.5 h-3.5" />
                                  <span>{c.address || '-'}</span>
                                </div>
                                <div className="flex items-center gap-2 text-slate-500">
                                  <MessageSquare className="w-3.5 h-3.5" />
                                  <span>LINE: {c.line_uid || c.line_id ? (
                                    <span className="text-green-600 font-medium">已連結</span>
                                  ) : (
                                    <span className="text-slate-400">未連結</span>
                                  )}</span>
                                </div>
                              </div>
                            </div>

                            <div className="bg-white rounded-lg border border-slate-200/60 p-5 space-y-3">
                              <h3 className="text-sm font-bold text-slate-700">統計摘要</h3>
                              <div className="grid grid-cols-2 gap-3">
                                <div className="flex items-center gap-2" data-testid={`stat-total-services-${c.id}`}>
                                  <Hash className="w-3.5 h-3.5 text-blue-500" />
                                  <div>
                                    <p className="text-xs text-slate-400">總服務次數</p>
                                    <p className="text-sm font-bold text-slate-800">{stats.totalServices} 次</p>
                                  </div>
                                </div>
                                <div className="flex items-center gap-2" data-testid={`stat-total-spent-${c.id}`}>
                                  <DollarSign className="w-3.5 h-3.5 text-emerald-500" />
                                  <div>
                                    <p className="text-xs text-slate-400">總消費金額</p>
                                    <p className="text-sm font-bold text-slate-800">${stats.totalSpent.toLocaleString()}</p>
                                  </div>
                                </div>
                                <div className="flex items-center gap-2" data-testid={`stat-last-service-${c.id}`}>
                                  <Calendar className="w-3.5 h-3.5 text-amber-500" />
                                  <div>
                                    <p className="text-xs text-slate-400">最後服務日</p>
                                    <p className="text-sm font-bold text-slate-800">
                                      {stats.lastServiceDate ? format(new Date(stats.lastServiceDate), 'yyyy/MM/dd') : '-'}
                                    </p>
                                  </div>
                                </div>
                                <div className="flex items-center gap-2" data-testid={`stat-avg-rating-${c.id}`}>
                                  <Star className="w-3.5 h-3.5 text-yellow-500" />
                                  <div>
                                    <p className="text-xs text-slate-400">平均評分</p>
                                    <p className="text-sm font-bold text-slate-800">
                                      {stats.avgRating !== null ? stats.avgRating.toFixed(1) : '-'}
                                    </p>
                                  </div>
                                </div>
                              </div>
                            </div>
                          </div>

                          <div className="bg-white rounded-lg border border-slate-200/60 p-5 space-y-3">
                            <h3 className="text-sm font-bold text-slate-700">服務歷史</h3>
                            {custAppts.length === 0 ? (
                              <p className="text-sm text-slate-400">尚無預約紀錄</p>
                            ) : (
                              <div className="overflow-x-auto">
                                <table className="w-full text-left text-sm" data-testid={`table-history-${c.id}`}>
                                  <thead>
                                    <tr className="text-xs text-slate-400 uppercase tracking-wider border-b border-slate-100">
                                      <th className="pb-2 pr-4">日期</th>
                                      <th className="pb-2 pr-4">清洗內容</th>
                                      <th className="pb-2 pr-4">師傅</th>
                                      <th className="pb-2 pr-4">金額</th>
                                      <th className="pb-2">狀態</th>
                                    </tr>
                                  </thead>
                                  <tbody className="divide-y divide-slate-50">
                                    {custAppts.map(a => (
                                      <tr key={a.id} data-testid={`history-row-${a.id}`}>
                                        <td className="py-2 pr-4 text-slate-600">
                                          {format(parseISO(a.scheduled_at), 'yyyy/MM/dd')}
                                        </td>
                                        <td className="py-2 pr-4 text-slate-600">
                                          {a.items.map(item => `${item.type}`).join('、')}
                                          {a.items.length > 0 && ` (${a.items.length}台)`}
                                        </td>
                                        <td className="py-2 pr-4 text-slate-600">
                                          {a.technician_name || '-'}
                                        </td>
                                        <td className="py-2 pr-4 text-slate-600 font-medium">
                                          ${a.total_amount.toLocaleString()}
                                        </td>
                                        <td className="py-2">
                                          <Badge status={a.status} />
                                        </td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              </div>
                            )}
                          </div>

                          {custReviews.length > 0 && (
                            <div className="bg-white rounded-lg border border-slate-200/60 p-5 space-y-3">
                              <h3 className="text-sm font-bold text-slate-700">客戶評價</h3>
                              <div className="space-y-3">
                                {custReviews.map(r => (
                                  <div key={r.id} className="border border-slate-100 rounded-md p-3 space-y-1" data-testid={`review-${r.id}`}>
                                    <div className="flex items-center gap-2">
                                      <div className="flex items-center gap-0.5">
                                        {Array.from({ length: 5 }).map((_, i) => (
                                          <Star
                                            key={i}
                                            className={`w-3.5 h-3.5 ${i < r.rating ? 'text-yellow-400 fill-yellow-400' : 'text-slate-200'}`}
                                          />
                                        ))}
                                      </div>
                                      <span className="text-xs text-slate-400">
                                        {format(parseISO(r.created_at), 'yyyy/MM/dd')}
                                      </span>
                                    </div>
                                    <p className="text-sm text-slate-600">{r.comment}</p>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </Card>
    </div>
  );
}
