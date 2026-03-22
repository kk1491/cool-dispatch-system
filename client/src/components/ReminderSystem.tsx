import { useState, useMemo } from 'react';
import { Clock, Phone, MapPin, Plus, Search, CalendarDays, AlertCircle } from 'lucide-react';
import { format, differenceInDays, parseISO } from 'date-fns';
import { cn } from '../lib/utils';
import { Customer, Appointment } from '../types';
import { Button, Card } from './shared';

interface ReminderSystemProps {
  customers: Customer[];
  appointments: Appointment[];
  reminderDays: number;
  onCreateAppointment: (customer: Customer) => void;
}

interface ReminderItem {
  customer: Customer;
  lastServiceDate: string;
  daysSinceService: number;
  isOverdue: boolean;
}

export default function ReminderSystem({
  customers,
  appointments,
  reminderDays,
  onCreateAppointment,
}: ReminderSystemProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<'days' | 'name'>('days');

  const reminderItems = useMemo(() => {
    const items: ReminderItem[] = [];
    const today = new Date();

    for (const customer of customers) {
      const completedAppts = appointments.filter(
        a => a.status === 'completed' && (a.phone === customer.phone || a.customer_name === customer.name)
      );

      if (completedAppts.length === 0) continue;

      const sorted = completedAppts.sort((a, b) => {
        const dateA = a.checkout_time || a.scheduled_at;
        const dateB = b.checkout_time || b.scheduled_at;
        return new Date(dateB).getTime() - new Date(dateA).getTime();
      });

      const lastDate = sorted[0].checkout_time || sorted[0].scheduled_at;
      const daysSince = differenceInDays(today, parseISO(lastDate));
      const isOverdue = daysSince >= reminderDays;

      items.push({
        customer,
        lastServiceDate: lastDate,
        daysSinceService: daysSince,
        isOverdue,
      });
    }

    return items;
  }, [customers, appointments, reminderDays]);

  const filteredItems = useMemo(() => {
    let result = reminderItems;

    if (searchQuery) {
      result = result.filter(
        item =>
          item.customer.name.includes(searchQuery) ||
          item.customer.phone.includes(searchQuery) ||
          item.customer.address.includes(searchQuery)
      );
    }

    result.sort((a, b) => {
      if (sortBy === 'days') return b.daysSinceService - a.daysSinceService;
      return a.customer.name.localeCompare(b.customer.name);
    });

    return result;
  }, [reminderItems, searchQuery, sortBy]);

  const overdueCount = reminderItems.filter(item => item.isOverdue).length;
  const upcomingCount = reminderItems.filter(item => !item.isOverdue && item.daysSinceService >= reminderDays * 0.8).length;

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="p-4 bg-rose-50 border-rose-100/50">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-rose-100 rounded-md flex items-center justify-center">
              <AlertCircle className="w-5 h-5 text-rose-600" />
            </div>
            <div>
              <p className="text-[10px] font-bold text-rose-400 uppercase tracking-wider" data-testid="text-overdue-label">已到期需回訪</p>
              <p className="text-2xl font-bold text-rose-900" data-testid="text-overdue-count">{overdueCount}</p>
            </div>
          </div>
        </Card>
        <Card className="p-4 bg-amber-50 border-amber-100/50">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-amber-100 rounded-md flex items-center justify-center">
              <Clock className="w-5 h-5 text-amber-600" />
            </div>
            <div>
              <p className="text-[10px] font-bold text-amber-400 uppercase tracking-wider" data-testid="text-upcoming-label">即將到期</p>
              <p className="text-2xl font-bold text-amber-900" data-testid="text-upcoming-count">{upcomingCount}</p>
            </div>
          </div>
        </Card>
        <Card className="p-4 bg-slate-50 border-slate-100/50">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-slate-100 rounded-md flex items-center justify-center">
              <CalendarDays className="w-5 h-5 text-slate-600" />
            </div>
            <div>
              <p className="text-[10px] font-bold text-slate-400 uppercase tracking-wider" data-testid="text-reminder-setting-label">回訪週期</p>
              <p className="text-2xl font-bold text-slate-900" data-testid="text-reminder-days">{reminderDays} 天</p>
            </div>
          </div>
        </Card>
      </div>

      <div className="flex flex-col md:flex-row gap-3">
        <div className="flex-1 relative">
          <input
            data-testid="input-reminder-search"
            type="text"
            placeholder="搜尋客戶姓名、電話或地址..."
            className="w-full pl-10 pr-4 py-2.5 bg-white border border-slate-100 rounded-md text-sm focus:outline-none focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all"
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
          />
          <Search className="w-4 h-4 text-slate-300 absolute left-3.5 top-1/2 -translate-y-1/2" />
        </div>
        <div className="flex gap-2">
          <button
            data-testid="button-sort-days"
            onClick={() => setSortBy('days')}
            className={cn(
              'px-4 py-2 rounded-md text-sm font-medium transition-all whitespace-nowrap',
              sortBy === 'days' ? 'bg-blue-600 text-white' : 'bg-white text-slate-500 border border-slate-100'
            )}
          >
            依天數排序
          </button>
          <button
            data-testid="button-sort-name"
            onClick={() => setSortBy('name')}
            className={cn(
              'px-4 py-2 rounded-md text-sm font-medium transition-all whitespace-nowrap',
              sortBy === 'name' ? 'bg-blue-600 text-white' : 'bg-white text-slate-500 border border-slate-100'
            )}
          >
            依姓名排序
          </button>
        </div>
      </div>

      {filteredItems.length === 0 ? (
        <Card className="p-12">
          <div className="text-center">
            <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
              <Clock className="text-slate-300 w-8 h-8" />
            </div>
            <p className="text-slate-500" data-testid="text-no-reminders">目前沒有符合條件的回訪提醒</p>
          </div>
        </Card>
      ) : (
        <div className="space-y-3">
          {filteredItems.map(item => {
            const progressPercent = Math.min((item.daysSinceService / reminderDays) * 100, 100);
            const progressColor = item.isOverdue
              ? 'bg-rose-500'
              : progressPercent >= 80
              ? 'bg-amber-500'
              : 'bg-emerald-500';

            return (
              <Card
                key={item.customer.id}
                className={cn(
                  'p-5',
                  item.isOverdue && 'border-rose-200/60 bg-rose-50/30'
                )}
                data-testid={`card-reminder-${item.customer.id}`}
              >
                <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
                  <div className="flex-1 space-y-2">
                    <div className="flex items-center gap-3 flex-wrap">
                      <h3 className="text-base font-bold text-slate-900" data-testid={`text-reminder-name-${item.customer.id}`}>
                        {item.customer.name}
                      </h3>
                      {item.isOverdue && (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-rose-100 text-rose-700 border border-rose-200/50">
                          需回訪
                        </span>
                      )}
                      {!item.isOverdue && progressPercent >= 80 && (
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-amber-100 text-amber-700 border border-amber-200/50">
                          即將到期
                        </span>
                      )}
                    </div>

                    <div className="flex flex-wrap items-center gap-4 text-sm text-slate-500">
                      <span className="flex items-center gap-1.5">
                        <Phone className="w-3.5 h-3.5" />
                        {item.customer.phone}
                      </span>
                      <span className="flex items-center gap-1.5">
                        <MapPin className="w-3.5 h-3.5" />
                        {item.customer.address}
                      </span>
                    </div>

                    <div className="flex items-center gap-3 text-xs text-slate-400">
                      <span data-testid={`text-last-service-${item.customer.id}`}>
                        上次服務: {format(parseISO(item.lastServiceDate), 'yyyy/MM/dd')}
                      </span>
                      <span className="text-slate-300">|</span>
                      <span
                        className={cn(
                          'font-bold',
                          item.isOverdue ? 'text-rose-600' : progressPercent >= 80 ? 'text-amber-600' : 'text-slate-600'
                        )}
                        data-testid={`text-days-since-${item.customer.id}`}
                      >
                        距今 {item.daysSinceService} 天
                      </span>
                    </div>

                    <div className="w-full bg-slate-100 rounded-full h-1.5 mt-1">
                      <div
                        className={cn('h-1.5 rounded-full transition-all', progressColor)}
                        style={{ width: `${progressPercent}%` }}
                      />
                    </div>
                  </div>

                  <div className="flex gap-2 shrink-0">
                    <a
                      href={`tel:${item.customer.phone}`}
                      className="flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-slate-100 text-slate-700 hover:bg-slate-200 transition-all"
                      data-testid={`button-call-${item.customer.id}`}
                    >
                      <Phone className="w-4 h-4" />
                      撥打電話
                    </a>
                    <Button
                      variant="primary"
                      onClick={() => onCreateAppointment(item.customer)}
                      data-testid={`button-create-appt-${item.customer.id}`}
                    >
                      <Plus className="w-4 h-4" />
                      建立預約
                    </Button>
                  </div>
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
