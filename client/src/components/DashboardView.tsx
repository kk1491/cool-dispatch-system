import { format } from 'date-fns';
import { zhTW } from 'date-fns/locale';
import { UserIcon, Calendar, TrendingUp, Clock, CheckCircle2, AlertCircle, Wrench } from 'lucide-react';
import { getAppointmentCollectedAmount, isAppointmentFinished, isAppointmentRevenueCounted } from '../lib/appointmentMetrics';
import { Card, Badge } from './shared';
import { Appointment, User, Customer, Review } from '../types';

interface DashboardViewProps {
  appointments: Appointment[];
  technicians: User[];
  customers: Customer[];
  reviews: Review[];
}

function getGreeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return '早安';
  if (hour < 18) return '午安';
  return '晚安';
}

export default function DashboardView({ appointments, technicians, customers, reviews }: DashboardViewProps) {
  const today = new Date().toISOString().split('T')[0];
  const todayAppts = appointments.filter(a => a.scheduled_at.split('T')[0] === today);

  const todayPending = todayAppts.filter(a => a.status === 'pending' || a.status === 'assigned').length;
  const todayInProgress = todayAppts.filter(a => a.status === 'arrived').length;
  const todayCompleted = todayAppts.filter(isAppointmentFinished).length;
  const todayRevenue = todayAppts
    .filter(isAppointmentRevenueCounted)
    .reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);

  const recentAppts = [...appointments]
    .sort((a, b) => new Date(b.scheduled_at).getTime() - new Date(a.scheduled_at).getTime())
    .slice(0, 5);

  const techStats = technicians.map(tech => {
    const techTodayAppts = todayAppts.filter(a => a.technician_id === tech.id);
    const taskCount = techTodayAppts.length;
    const hasArrived = techTodayAppts.some(a => a.status === 'arrived');
    const allCompleted = taskCount > 0 && techTodayAppts.every(isAppointmentFinished);
    const hasAssigned = techTodayAppts.some(a => a.status === 'assigned');

    let statusLabel = '休息中';
    let statusColor = 'bg-slate-100 text-slate-500';
    if (hasArrived) {
      statusLabel = '工作中';
      statusColor = 'bg-emerald-50 text-emerald-700';
    } else if (allCompleted) {
      statusLabel = '已完工';
      statusColor = 'bg-blue-50 text-blue-700';
    } else if (hasAssigned) {
      statusLabel = '待出發';
      statusColor = 'bg-amber-50 text-amber-700';
    }

    return { tech, taskCount, statusLabel, statusColor };
  });

  return (
    <div className="space-y-8">
      <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-2">
        <div>
          <h1 className="text-2xl font-bold text-slate-900" data-testid="text-dashboard-greeting">
            {getGreeting()}，管理員
          </h1>
          <p className="text-sm text-slate-500 mt-1" data-testid="text-dashboard-date">
            {format(new Date(), 'yyyy 年 M 月 d 日 EEEE', { locale: zhTW })}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card className="p-5" data-testid="card-stat-pending">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-9 h-9 rounded-md bg-amber-50 flex items-center justify-center">
              <AlertCircle className="w-5 h-5 text-amber-500" />
            </div>
          </div>
          <p className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">今日待指派</p>
          <p className="text-3xl font-bold text-slate-900" data-testid="text-stat-pending">{todayPending}</p>
        </Card>
        <Card className="p-5" data-testid="card-stat-inprogress">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-9 h-9 rounded-md bg-violet-50 flex items-center justify-center">
              <Wrench className="w-5 h-5 text-violet-500" />
            </div>
          </div>
          <p className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">今日進行中</p>
          <p className="text-3xl font-bold text-slate-900" data-testid="text-stat-inprogress">{todayInProgress}</p>
        </Card>
        <Card className="p-5" data-testid="card-stat-completed">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-9 h-9 rounded-md bg-emerald-50 flex items-center justify-center">
              <CheckCircle2 className="w-5 h-5 text-emerald-500" />
            </div>
          </div>
          <p className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">今日已完成</p>
          <p className="text-3xl font-bold text-slate-900" data-testid="text-stat-completed">{todayCompleted}</p>
        </Card>
        <Card className="p-5" data-testid="card-stat-revenue">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-9 h-9 rounded-md bg-blue-50 flex items-center justify-center">
              <TrendingUp className="w-5 h-5 text-blue-500" />
            </div>
          </div>
          <p className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">今日營收</p>
          <p className="text-3xl font-bold text-slate-900" data-testid="text-stat-revenue">${todayRevenue.toLocaleString()}</p>
        </Card>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card className="p-0 overflow-hidden" data-testid="card-tech-attendance">
          <div className="px-5 py-4 border-b border-slate-100">
            <h3 className="text-sm font-semibold text-slate-800">師傅出勤狀態</h3>
          </div>
          <div className="divide-y divide-slate-50">
            {techStats.map(({ tech, taskCount, statusLabel, statusColor }) => (
              <div key={tech.id} className="flex items-center gap-3 px-5 py-3" data-testid={`row-tech-${tech.id}`}>
                <div
                  className="w-9 h-9 rounded-full flex items-center justify-center text-white text-sm font-bold shrink-0"
                  style={{ backgroundColor: tech.color || '#6366f1' }}
                >
                  {tech.name.charAt(0)}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-slate-800 truncate" data-testid={`text-tech-name-${tech.id}`}>{tech.name}</p>
                  <p className="text-xs text-slate-400" data-testid={`text-tech-tasks-${tech.id}`}>今日 {taskCount} 筆任務</p>
                </div>
                <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${statusColor}`} data-testid={`badge-tech-status-${tech.id}`}>
                  {statusLabel}
                </span>
              </div>
            ))}
            {techStats.length === 0 && (
              <div className="px-5 py-8 text-center text-sm text-slate-400">尚無師傅資料</div>
            )}
          </div>
        </Card>

        <Card className="p-0 overflow-hidden" data-testid="card-recent-appts">
          <div className="px-5 py-4 border-b border-slate-100">
            <h3 className="text-sm font-semibold text-slate-800">近期預約快覽</h3>
          </div>
          <div className="divide-y divide-slate-50">
            {recentAppts.map(appt => (
              <div key={appt.id} className="flex items-center gap-3 px-5 py-3" data-testid={`row-appt-${appt.id}`}>
                <div className="w-9 h-9 rounded-full bg-slate-100 flex items-center justify-center shrink-0">
                  <Calendar className="w-4 h-4 text-slate-500" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-slate-800 truncate" data-testid={`text-appt-customer-${appt.id}`}>{appt.customer_name}</p>
                  <p className="text-xs text-slate-400" data-testid={`text-appt-time-${appt.id}`}>
                    {format(new Date(appt.scheduled_at), 'M/d HH:mm')}
                  </p>
                </div>
                <Badge status={appt.status} />
              </div>
            ))}
            {recentAppts.length === 0 && (
              <div className="px-5 py-8 text-center text-sm text-slate-400">尚無預約資料</div>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}
