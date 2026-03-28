import { useState } from 'react';
import { format, parseISO, startOfMonth, endOfMonth, subMonths, isWithinInterval } from 'date-fns';
import { Download, Search, Filter, CreditCard } from 'lucide-react';
import * as XLSX from 'xlsx';
import { cn } from '../lib/utils';
import {
  getAppointmentCollectedAmount,
  getAppointmentClosedAt,
  getPaymentCollectionBadgeClass,
  getPaymentMethodBadgeClass,
  getPaymentMethodLabel,
  getPaymentMethodFilterOptions,
  getChargeableAmount,
  getPaymentCollectionLabel,
  getOutstandingAmount,
  isAppointmentFinished,
  LEGACY_PAYMENT_METHOD_LABEL,
  PAYMENT_COLLECTION_FILTER_OPTIONS,
} from '../lib/appointmentMetrics';
import { isPaymentLinkCreatableAppointment } from '../lib/paymentOrder';
import { Button, Card } from './shared';
import { Appointment, User } from '../types';
import PaymentOrderCreateDialog from './PaymentOrderCreateDialog';

interface FinancialReportViewProps {
  appointments: Appointment[];
  technicians: User[];
  onRefreshData?: () => Promise<unknown>;
}

type DatePreset = 'all' | 'thisMonth' | 'lastMonth' | 'custom';

function exportFinancialXLSX(data: Appointment[], technicians: User[]) {
  const headers = ['結案日期', '客戶姓名', '台數', '機型明細', '師傅', '付款方式', '應收金額', '實收金額', '未收餘額'];
  const rows = data.map(a => {
    const date = format(parseISO(getAppointmentClosedAt(a)), 'yyyy-MM-dd');
    const paid = getAppointmentCollectedAmount(a);
    const expected = getChargeableAmount(a);
    const diff = getOutstandingAmount(a);
    const collectionLabel = getPaymentCollectionLabel(a);
    const paymentMethodLabel = getPaymentMethodLabel(a);
    const units = a.items.length;
    const unitDetail = a.items.map(i => i.type).join('+');
    const tech = technicians.find(t => t.id === a.technician_id)?.name || a.technician_name || '(未指派)';
    return [date, a.customer_name, units, unitDetail, tech, `${paymentMethodLabel} / ${collectionLabel}`, expected, paid, diff];
  });

  const totalExpected = data.reduce((s, a) => s + getChargeableAmount(a), 0);
  const totalPaid = data.reduce((s, a) => s + getAppointmentCollectedAmount(a), 0);
  const totalDiff = data.reduce((s, a) => s + getOutstandingAmount(a), 0);
  const totalUnits = data.reduce((s, a) => s + a.items.length, 0);
  rows.push([]);
  rows.push(['合計', '', totalUnits, '', '', '', totalExpected, totalPaid, totalDiff]);

  const ws = XLSX.utils.aoa_to_sheet([headers, ...rows]);

  const colWidths = [12, 10, 6, 18, 8, 10, 12, 12, 12];
  ws['!cols'] = colWidths.map(w => ({ wch: w }));

  const wb = XLSX.utils.book_new();
  XLSX.utils.book_append_sheet(wb, ws, '財務報表');
  XLSX.writeFile(wb, `財務報表_${format(new Date(), 'yyyyMMdd')}.xlsx`);
}

export default function FinancialReportView({ appointments, technicians, onRefreshData }: FinancialReportViewProps) {
  const [datePreset, setDatePreset] = useState<DatePreset>('all');
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');
  const [techFilter, setTechFilter] = useState<number | 'all'>('all');
  const [paymentMethodFilter, setPaymentMethodFilter] = useState<string>('all');
  const [collectionFilter, setCollectionFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [paymentDialogAppointmentId, setPaymentDialogAppointmentId] = useState<number | undefined>(undefined);

  const now = new Date();
  const thisMonthStart = startOfMonth(now);
  const thisMonthEnd = endOfMonth(now);
  const lastMonthStart = startOfMonth(subMonths(now, 1));
  const lastMonthEnd = endOfMonth(subMonths(now, 1));

  // 财务报表统一纳入全部已结案工单，再用统一金额函数区分無收款 / 未收款 / 已收款。
  const completed = appointments.filter(isAppointmentFinished);
  // hasLegacyPaymentMethod 让筛选器在存在历史脏数据时给出明确入口，避免旧工单只能在列表里偶然看见却无法单独核查。
  const hasLegacyPaymentMethod = completed.some(a => getPaymentMethodLabel(a) === LEGACY_PAYMENT_METHOD_LABEL);

  const filtered = completed.filter(a => {
    const apptDate = parseISO(getAppointmentClosedAt(a));
    const collectionLabel = getPaymentCollectionLabel(a);
    const paymentMethodLabel = getPaymentMethodLabel(a);

    if (datePreset === 'thisMonth') {
      if (!isWithinInterval(apptDate, { start: thisMonthStart, end: thisMonthEnd })) return false;
    } else if (datePreset === 'lastMonth') {
      if (!isWithinInterval(apptDate, { start: lastMonthStart, end: lastMonthEnd })) return false;
    } else if (datePreset === 'custom') {
      if (customStart && apptDate < parseISO(customStart)) return false;
      if (customEnd && apptDate > parseISO(customEnd + 'T23:59:59')) return false;
    }

    if (techFilter !== 'all' && a.technician_id !== techFilter) return false;

    // 付款方式筛选统一比对展示口径，确保历史 `未收款` 脏数据也能被明确筛出。
    if (paymentMethodFilter !== 'all' && paymentMethodLabel !== paymentMethodFilter) return false;

    // 收款状态必须按统一 collectionLabel 过滤，不能再把「未收款」误当成付款方式值比较。
    if (collectionFilter !== 'all' && collectionLabel !== collectionFilter) return false;

    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      if (!a.customer_name.toLowerCase().includes(q) && !a.phone.includes(q)) return false;
    }

    return true;
  });

  const totalExpected = filtered.reduce((sum, a) => sum + getChargeableAmount(a), 0);
  const totalPaid = filtered.reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);
  const totalDiff = filtered.reduce((sum, a) => sum + getOutstandingAmount(a), 0);
  const totalUnits = filtered.reduce((sum, a) => sum + a.items.length, 0);

  const dateLabel = datePreset === 'thisMonth' ? format(thisMonthStart, 'yyyy年M月')
    : datePreset === 'lastMonth' ? format(lastMonthStart, 'yyyy年M月')
    : datePreset === 'custom' ? '自訂區間'
    : '全部時間';

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">財務報表</h2>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            className="text-xs py-2 px-3"
            data-testid="button-export-financial-csv"
            onClick={() => exportFinancialXLSX(filtered, technicians)}
          >
            <Download className="w-3.5 h-3.5 mr-1" /> 匯出報表 (.xlsx)
          </Button>
        </div>
      </div>

      <Card className="p-5 space-y-4 border-slate-100">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex gap-1 bg-slate-100 p-1 rounded-lg">
            {([
              { key: 'all', label: '全部' },
              { key: 'thisMonth', label: '本月' },
              { key: 'lastMonth', label: '上月' },
              { key: 'custom', label: '自訂' },
            ] as { key: DatePreset; label: string }[]).map(item => (
              <button
                key={item.key}
                data-testid={`button-date-${item.key}`}
                onClick={() => setDatePreset(item.key)}
                className={cn(
                  "px-4 py-2 rounded-md text-xs font-bold transition-all",
                  datePreset === item.key
                    ? "bg-white text-slate-900 shadow-sm"
                    : "text-slate-500 hover:text-slate-700"
                )}
              >
                {item.label}
              </button>
            ))}
          </div>

          {datePreset === 'custom' && (
            <div className="flex items-center gap-2 bg-white border border-slate-200 rounded-lg overflow-hidden">
              <input
                type="date"
                data-testid="input-date-start"
                className="px-3 py-2 text-xs focus:outline-none bg-transparent"
                value={customStart}
                onChange={e => setCustomStart(e.target.value)}
              />
              <span className="text-xs text-slate-300">~</span>
              <input
                type="date"
                data-testid="input-date-end"
                className="px-3 py-2 text-xs focus:outline-none bg-transparent"
                value={customEnd}
                onChange={e => setCustomEnd(e.target.value)}
              />
            </div>
          )}

          <select
            data-testid="select-tech-filter"
            value={techFilter === 'all' ? 'all' : String(techFilter)}
            onChange={e => setTechFilter(e.target.value === 'all' ? 'all' : Number(e.target.value))}
            className="px-3 py-2 rounded-lg border border-slate-200 text-xs bg-white focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="all">全部師傅</option>
            {technicians.map(t => (
              <option key={t.id} value={t.id}>{t.name}</option>
            ))}
          </select>

          <select
            data-testid="select-payment-method-filter"
            value={paymentMethodFilter}
            onChange={e => setPaymentMethodFilter(e.target.value)}
            className="px-3 py-2 rounded-lg border border-slate-200 text-xs bg-white focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="all">全部付款方式</option>
            {getPaymentMethodFilterOptions(hasLegacyPaymentMethod).map(method => (
              <option key={method} value={method}>{method}</option>
            ))}
          </select>

          <select
            data-testid="select-collection-filter"
            value={collectionFilter}
            onChange={e => setCollectionFilter(e.target.value)}
            className="px-3 py-2 rounded-lg border border-slate-200 text-xs bg-white focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="all">全部收款狀態</option>
            {PAYMENT_COLLECTION_FILTER_OPTIONS.map(label => (
              <option key={label} value={label}>{label}</option>
            ))}
          </select>

          <div className="relative flex-1 min-w-[160px]">
            <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-slate-300" />
            <input
              data-testid="input-search-financial"
              type="text"
              placeholder="搜尋客戶姓名或電話"
              className="w-full pl-9 pr-3 py-2 rounded-lg border border-slate-200 text-xs bg-white focus:outline-none focus:ring-1 focus:ring-blue-500"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
            />
          </div>
        </div>

        <div className="flex items-center gap-4 text-xs text-slate-500">
          <Filter className="w-3.5 h-3.5 text-slate-300" />
          <span data-testid="text-filter-summary">{dateLabel} / 共 <b className="text-slate-800">{filtered.length}</b> 筆 / <b className="text-slate-800">{totalUnits}</b> 台</span>
          {(datePreset !== 'all' || techFilter !== 'all' || paymentMethodFilter !== 'all' || collectionFilter !== 'all' || searchQuery) && (
            <button
              data-testid="button-clear-financial-filters"
              onClick={() => {
                setDatePreset('all');
                setTechFilter('all');
                setPaymentMethodFilter('all');
                setCollectionFilter('all');
                setSearchQuery('');
                setCustomStart('');
                setCustomEnd('');
              }}
              className="text-blue-500 hover:text-blue-700 font-medium"
            >
              清除篩選
            </button>
          )}
        </div>
      </Card>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-5">
        <Card className="p-5 bg-blue-600 text-white border-none shadow-lg">
          <p className="text-[10px] font-bold opacity-50 uppercase tracking-wider mb-1">應收總額</p>
          <p className="text-3xl font-black" data-testid="text-total-expected">${totalExpected.toLocaleString()}</p>
        </Card>
        <Card className="p-5 bg-emerald-50 border-emerald-100">
          <p className="text-[10px] font-bold text-emerald-500 uppercase tracking-wider mb-1">實收總額</p>
          <p className="text-3xl font-black text-emerald-900" data-testid="text-total-paid">${totalPaid.toLocaleString()}</p>
        </Card>
        <Card className="p-5 bg-rose-50 border-rose-100">
          <p className="text-[10px] font-bold text-rose-500 uppercase tracking-wider mb-1">未收餘額 (價差)</p>
          <p className="text-3xl font-black text-rose-900" data-testid="text-total-diff">${totalDiff.toLocaleString()}</p>
        </Card>
        <Card className="p-5 bg-violet-50 border-violet-100">
          <p className="text-[10px] font-bold text-violet-500 uppercase tracking-wider mb-1">清洗台數</p>
          <p className="text-3xl font-black text-violet-900" data-testid="text-total-units">{totalUnits} <span className="text-lg">台</span></p>
        </Card>
      </div>

      <Card className="overflow-hidden border-slate-100">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="bg-slate-50 text-[10px] font-bold text-slate-400 uppercase tracking-widest border-b border-slate-100">
                <th className="px-5 py-4">結案日期</th>
                <th className="px-5 py-4">客戶姓名</th>
                <th className="px-5 py-4 text-center">台數</th>
                <th className="px-5 py-4">師傅</th>
                <th className="px-5 py-4">付款方式</th>
                <th className="px-5 py-4 text-right">應收金額</th>
                <th className="px-5 py-4 text-right">實收金額</th>
                <th className="px-5 py-4 text-right">未收餘額</th>
                <th className="px-5 py-4 text-center">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-6 py-12 text-center text-slate-400">目前尚無符合條件的訂單資料</td>
                </tr>
              ) : (
                filtered.map(a => {
                  const expected = getChargeableAmount(a);
                  const paid = getAppointmentCollectedAmount(a);
                  const diff = getOutstandingAmount(a);
                  const collectionLabel = getPaymentCollectionLabel(a);
                  const paymentMethodLabel = getPaymentMethodLabel(a);
                  const canCreatePaymentLink = isPaymentLinkCreatableAppointment(a);
                  const unitCount = a.items.length;
                  const unitDetail = a.items.map(i => i.type).join('+');
                  const tech = technicians.find(t => t.id === a.technician_id);
                  return (
                    <tr key={a.id} className="hover:bg-slate-50/50 transition-colors" data-testid={`row-financial-${a.id}`}>
                      <td className="px-5 py-4 text-sm text-slate-500">{format(parseISO(getAppointmentClosedAt(a)), 'yyyy/MM/dd')}</td>
                      <td className="px-5 py-4 text-sm font-bold text-slate-900">{a.customer_name}</td>
                      <td className="px-5 py-4 text-center" data-testid={`text-units-${a.id}`}>
                        <span className="text-sm font-bold text-slate-700">{unitCount}</span>
                        <span className="text-[10px] text-slate-400 ml-1">台</span>
                        {unitDetail && (
                          <div className="text-[10px] text-slate-400 mt-0.5">{unitDetail}</div>
                        )}
                      </td>
                      <td className="px-5 py-4" data-testid={`text-tech-${a.id}`}>
                        {tech ? (
                          <div className="flex items-center gap-2">
                            <div
                              className="w-6 h-6 rounded-md flex items-center justify-center text-[10px] font-bold text-white flex-shrink-0"
                              style={{ backgroundColor: tech.color }}
                            >
                              {tech.name[0]}
                            </div>
                            <span className="text-sm text-slate-700 font-medium">{tech.name}</span>
                          </div>
                        ) : (
                          <span className="text-sm text-slate-300">{a.technician_name || '—'}</span>
                        )}
                      </td>
                      <td className="px-5 py-4 text-sm text-slate-600">
                        <div className="flex items-center gap-2 whitespace-nowrap">
                          <span
                            className={cn(
                              "px-2 py-1 rounded text-[10px] font-bold whitespace-nowrap",
                              getPaymentMethodBadgeClass(paymentMethodLabel)
                            )}
                            data-testid={`text-payment-${a.id}`}
                          >
                            {paymentMethodLabel}
                          </span>
                          <span
                            className={cn(
                              "px-2 py-1 rounded text-[10px] font-bold whitespace-nowrap",
                              getPaymentCollectionBadgeClass(collectionLabel)
                            )}
                            data-testid={`text-collection-status-${a.id}`}
                          >
                            {collectionLabel}
                          </span>
                        </div>
                      </td>
                      <td className="px-5 py-4 text-sm text-right font-medium text-slate-500">${expected.toLocaleString()}</td>
                      <td className="px-5 py-4 text-sm text-right font-bold text-emerald-600">${paid.toLocaleString()}</td>
                      <td className="px-5 py-4 text-sm text-right font-bold">
                        {diff > 0 ? (
                          <span className="text-rose-500">-${diff.toLocaleString()}</span>
                        ) : diff < 0 ? (
                          <span className="text-blue-600">+${Math.abs(diff).toLocaleString()}</span>
                        ) : (
                          <span className="text-slate-300">-</span>
                        )}
                      </td>
                      <td className="px-5 py-4 text-center">
                        {canCreatePaymentLink ? (
                          <button
                            type="button"
                            onClick={() => setPaymentDialogAppointmentId(a.id)}
                            className="inline-flex items-center gap-1 whitespace-nowrap rounded-lg bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-100"
                          >
                            <CreditCard className="h-3.5 w-3.5" />
                            建立付款連結
                          </button>
                        ) : (
                          <span className="whitespace-nowrap text-xs text-slate-300">不可建立</span>
                        )}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      <PaymentOrderCreateDialog
        open={Boolean(paymentDialogAppointmentId)}
        onClose={() => setPaymentDialogAppointmentId(undefined)}
        appointments={appointments}
        initialAppointmentId={paymentDialogAppointmentId}
        onCreated={onRefreshData}
      />
    </div>
  );
}
