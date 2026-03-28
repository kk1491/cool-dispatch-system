import { useState } from 'react';
import {
  ChevronLeft, Plus, DollarSign, ArrowDownLeft, ArrowUpRight, 
  FileText, Calendar
} from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { toast } from 'react-hot-toast';
import { cn } from '../lib/utils';
import { useTablePagination } from '../lib/tablePagination';
import {
  CASH_LEDGER_ADD_RETURN_LABEL,
  CASH_LEDGER_ADD_RETURN_TITLE,
  CASH_LEDGER_ENTRY_TITLE,
  CASH_LEDGER_RETURN_AMOUNT_LABEL,
  CASH_LEDGER_RETURN_DEFAULT_NOTE,
  CASH_LEDGER_RETURN_NOTE_PLACEHOLDER,
  CASH_LEDGER_RETURN_SUCCESS_MESSAGE,
  CASH_LEDGER_SCOPE_NOTE,
  CASH_LEDGER_TITLE,
  getAppointmentCollectedAmount,
  getAppointmentClosedAt,
  isAppointmentRevenueCounted,
  isCashAppointment,
} from '../lib/appointmentMetrics';
import { Button, Card } from './shared';
import MobileInfiniteCardList from './MobileInfiniteCardList';
import TablePagination from './TablePagination';
import { User, Appointment, CashLedgerCreatePayload, CashLedgerEntry } from '../types';

interface CashLedgerProps {
  technician: User;
  appointments: Appointment[];
  ledgerEntries: CashLedgerEntry[];
  onAddEntry: (entry: CashLedgerCreatePayload) => void;
  onBack: () => void;
}

export default function CashLedger({ technician, appointments, ledgerEntries, onAddEntry, onBack }: CashLedgerProps) {
  const [showAddForm, setShowAddForm] = useState(false);
  const [returnAmount, setReturnAmount] = useState('');
  const [returnNote, setReturnNote] = useState('');

  const techEntries = ledgerEntries.filter(e => e.technician_id === technician.id);

  // 现金账直接复用统一“已收款”口径，避免这里继续旁路 payment_received 导致绩效页与财务页不一致。
  const cashAppointments = appointments.filter(
    a => a.technician_id === technician.id && isCashAppointment(a) && isAppointmentRevenueCounted(a)
  );

  const autoCollectEntries: CashLedgerEntry[] = cashAppointments.map(a => ({
    id: `auto-${a.id}`,
    technician_id: technician.id,
    appointment_id: a.id,
    type: 'collect' as const,
    amount: getAppointmentCollectedAmount(a),
    note: `${a.customer_name} - 現金收款`,
    // 现金账入账时间统一复用结案口径，避免与技师页、财务页按不同时间字段排序。
    created_at: getAppointmentClosedAt(a),
  }));

  const manualCollectEntries = techEntries.filter(entry => {
    if (entry.type !== 'collect' || entry.id.startsWith('auto-')) {
      return false;
    }
    // 历史资料里曾把预约收款同时写进 cash_ledger_entries。
    // 现在界面会优先按预约自动生成 collect 行，因此这里要排除“仍能在 appointments 中找到”的旧 collect，
    // 避免同一张工单在列表与余额里重复出现；若关联预约已不存在，则保留原流水供人工核账。
    if (entry.appointment_id && appointments.some(appt => appt.id === entry.appointment_id)) {
      return false;
    }
    return true;
  });
  const returnEntries = techEntries.filter(e => e.type === 'return');

  const allEntries = [...autoCollectEntries, ...manualCollectEntries, ...returnEntries]
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
  const {
    page,
    pageSize,
    totalItems,
    totalPages,
    paginatedItems,
    setPage,
    setPageSize,
  } = useTablePagination(allEntries, [technician.id, ledgerEntries.length, appointments.length]);

  const totalCollected = autoCollectEntries.reduce((sum, e) => sum + e.amount, 0) +
    manualCollectEntries.reduce((sum, e) => sum + e.amount, 0);
  const totalReturned = returnEntries.reduce((sum, e) => sum + e.amount, 0);
  const balance = totalCollected - totalReturned;

  // handleAddReturn 只提交现金账写接口需要的字段，避免把前端临时主键带回后端。
  const handleAddReturn = () => {
    const amount = parseInt(returnAmount);
    if (!amount || amount <= 0) {
      toast.error('請輸入有效金額');
      return;
    }

    const newEntry: CashLedgerCreatePayload = {
      technician_id: technician.id,
      type: 'return',
      amount,
      note: returnNote || CASH_LEDGER_RETURN_DEFAULT_NOTE,
      // created_at 不传，由后端服务器自动用 UTC 当前时间填充
    };

    onAddEntry(newEntry);
    setReturnAmount('');
    setReturnNote('');
    setShowAddForm(false);
    toast.success(CASH_LEDGER_RETURN_SUCCESS_MESSAGE);
  };

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-4">
        <Button variant="outline" onClick={onBack} className="rounded-full w-10 h-10 p-0" data-testid="button-ledger-back">
          <ChevronLeft className="w-5 h-5" />
        </Button>
        <div className="flex items-center gap-3">
          <div 
            className="w-12 h-12 rounded-lg flex items-center justify-center text-lg font-bold text-white"
            style={{ backgroundColor: technician.color }}
          >
            {technician.name[0]}
          </div>
          <div>
            <h2 className="text-xl font-bold text-slate-900" data-testid="text-ledger-tech-name">{technician.name}</h2>
            <p className="text-sm text-slate-500">{CASH_LEDGER_TITLE}</p>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="p-4 bg-emerald-50 border-emerald-100/50">
          <div className="flex items-center gap-2 mb-1">
            <ArrowDownLeft className="w-4 h-4 text-emerald-600" />
            <p className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider">已收現金</p>
          </div>
          <p className="text-2xl font-bold text-emerald-900" data-testid="text-total-collected">${totalCollected.toLocaleString()}</p>
        </Card>
        <Card className="p-4 bg-blue-50 border-blue-100/50">
          <div className="flex items-center gap-2 mb-1">
            <ArrowUpRight className="w-4 h-4 text-blue-600" />
            <p className="text-[10px] font-bold text-blue-400 uppercase tracking-wider">已回繳</p>
          </div>
          <p className="text-2xl font-bold text-blue-900" data-testid="text-total-returned">${totalReturned.toLocaleString()}</p>
        </Card>
        <Card className={cn("p-4", balance > 0 ? "bg-amber-50 border-amber-100/50" : "bg-slate-50 border-slate-100/50")}>
          <div className="flex items-center gap-2 mb-1">
            <DollarSign className="w-4 h-4 text-amber-600" />
            <p className="text-[10px] font-bold text-amber-400 uppercase tracking-wider">未回繳餘額</p>
          </div>
          <p className={cn("text-2xl font-bold", balance > 0 ? "text-amber-900" : "text-slate-900")} data-testid="text-balance">${balance.toLocaleString()}</p>
        </Card>
      </div>

      <Card className="p-4 bg-slate-50 border-slate-100">
        <p className="text-sm font-medium text-slate-700">
          統計口徑：{CASH_LEDGER_SCOPE_NOTE}
        </p>
      </Card>

      <div className="flex justify-between items-center">
        <h3 className="text-lg font-bold flex items-center gap-2">
          <FileText className="w-5 h-5 text-slate-400" /> {CASH_LEDGER_ENTRY_TITLE}
        </h3>
        <Button onClick={() => setShowAddForm(!showAddForm)} data-testid="button-add-return">
          <Plus className="w-4 h-4" /> {CASH_LEDGER_ADD_RETURN_LABEL}
        </Button>
      </div>

      {showAddForm && (
        <Card className="p-6 space-y-4">
          <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">{CASH_LEDGER_ADD_RETURN_TITLE}</h4>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">{CASH_LEDGER_RETURN_AMOUNT_LABEL}</label>
              <input
                data-testid="input-return-amount"
                type="number"
                value={returnAmount}
                onChange={e => setReturnAmount(e.target.value)}
                placeholder="輸入金額"
                className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">備註</label>
              <input
                data-testid="input-return-note"
                type="text"
                value={returnNote}
                onChange={e => setReturnNote(e.target.value)}
                placeholder={CASH_LEDGER_RETURN_NOTE_PLACEHOLDER}
                className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              />
            </div>
          </div>
          <div className="flex gap-2 justify-end">
            <Button variant="outline" onClick={() => setShowAddForm(false)} data-testid="button-cancel-return">取消</Button>
            <Button onClick={handleAddReturn} data-testid="button-confirm-return">確認新增</Button>
          </div>
        </Card>
      )}

      <Card className="overflow-hidden">
        {allEntries.length === 0 ? (
          <div className="text-center py-16">
            <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
              <DollarSign className="text-slate-300 w-8 h-8" />
            </div>
            <p className="text-slate-500">尚無帳務紀錄</p>
          </div>
        ) : (
          <>
          <div className="space-y-3 p-3 md:hidden">
            <MobileInfiniteCardList
              items={allEntries}
              resetDeps={[technician.id, ledgerEntries.length, appointments.length]}
              getKey={item => item.id}
              renderItem={entry => {
                const relatedAppt = entry.appointment_id
                  ? appointments.find(a => a.id === entry.appointment_id)
                  : null;

                return (
                  <Card className="p-4 shadow-none">
                    <div className="space-y-3 text-sm text-slate-600">
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex items-center gap-2 text-slate-500">
                          <Calendar className="h-3.5 w-3.5" />
                          {format(parseISO(entry.created_at), 'yyyy/MM/dd HH:mm')}
                        </div>
                        <span className={cn(
                          'rounded-full px-3 py-1 text-xs font-bold border',
                          entry.type === 'collect'
                            ? 'bg-emerald-50 text-emerald-700 border-emerald-200/50'
                            : 'bg-blue-50 text-blue-700 border-blue-200/50',
                        )}>
                          {entry.type === 'collect' ? '收款' : '回繳'}
                        </span>
                      </div>
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-slate-400">金額</span>
                        <span className={cn('font-bold', entry.type === 'collect' ? 'text-emerald-700' : 'text-blue-700')}>
                          {entry.type === 'collect' ? '+' : '-'}${entry.amount.toLocaleString()}
                        </span>
                      </div>
                      <div className="flex items-start justify-between gap-3">
                        <span className="text-slate-400">關聯訂單</span>
                        <span className="max-w-[65%] text-right">{relatedAppt ? `#${relatedAppt.id} ${relatedAppt.customer_name}` : '-'}</span>
                      </div>
                      <div className="flex items-start justify-between gap-3">
                        <span className="text-slate-400">備註</span>
                        <span className="max-w-[65%] text-right">{entry.note}</span>
                      </div>
                    </div>
                  </Card>
                );
              }}
            />
          </div>
          <div className="hidden overflow-x-auto md:block">
            <table className="w-full text-sm text-left text-slate-600">
              <thead className="text-xs text-slate-700 uppercase bg-slate-50">
                <tr>
                  <th className="px-4 py-3">日期</th>
                  <th className="px-4 py-3">類型</th>
                  <th className="px-4 py-3">金額</th>
                  <th className="px-4 py-3">關聯訂單</th>
                  <th className="px-4 py-3">備註</th>
                </tr>
              </thead>
              <tbody>
                {paginatedItems.map(entry => {
                  const relatedAppt = entry.appointment_id 
                    ? appointments.find(a => a.id === entry.appointment_id) 
                    : null;
                  return (
                    <tr key={entry.id} className="bg-white border-b" data-testid={`row-ledger-${entry.id}`}>
                      <td className="px-4 py-3 text-slate-500">
                        <div className="flex items-center gap-2">
                          <Calendar className="w-3.5 h-3.5" />
                          {format(parseISO(entry.created_at), 'yyyy/MM/dd HH:mm')}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={cn(
                          "px-3 py-1 rounded-full text-xs font-bold border",
                          entry.type === 'collect' 
                            ? "bg-emerald-50 text-emerald-700 border-emerald-200/50" 
                            : "bg-blue-50 text-blue-700 border-blue-200/50"
                        )} data-testid={`badge-ledger-type-${entry.id}`}>
                          {entry.type === 'collect' ? '收款' : '回繳'}
                        </span>
                      </td>
                      <td className={cn(
                        "px-4 py-3 font-bold",
                        entry.type === 'collect' ? "text-emerald-700" : "text-blue-700"
                      )}>
                        {entry.type === 'collect' ? '+' : '-'}${entry.amount.toLocaleString()}
                      </td>
                      <td className="px-4 py-3 text-slate-500">
                        {relatedAppt ? `#${relatedAppt.id} ${relatedAppt.customer_name}` : '-'}
                      </td>
                      <td className="px-4 py-3 text-slate-500">{entry.note}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          </>
        )}
        <TablePagination
          className="hidden md:flex"
          page={page}
          pageSize={pageSize}
          totalItems={totalItems}
          totalPages={totalPages}
          onPageChange={setPage}
          onPageSizeChange={setPageSize}
          itemLabel="筆"
        />
      </Card>
    </div>
  );
}
