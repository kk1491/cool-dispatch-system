import { useState, useMemo } from 'react';
import { format, parseISO, startOfWeek, addDays, isSameDay } from 'date-fns';
import { zhTW } from 'date-fns/locale';
import { ChevronLeft, ChevronRight, Plus } from 'lucide-react';
import { cn } from '../lib/utils';
import { User, Appointment } from '../types';
import { Button } from './shared';

interface ScheduleGanttProps {
  technicians: User[];
  appointments: Appointment[];
  onSelectAppointment: (appt: Appointment) => void;
  onQuickCreate: (techId: number, dateTime: string) => void;
}

type ViewMode = 'day' | 'week';

const HOURS = Array.from({ length: 11 }, (_, i) => i + 8);

const STATUS_COLORS: Record<Appointment['status'], { bg: string; border: string; text: string }> = {
  pending: { bg: 'bg-amber-100', border: 'border-amber-300', text: 'text-amber-800' },
  assigned: { bg: 'bg-blue-100', border: 'border-blue-300', text: 'text-blue-800' },
  arrived: { bg: 'bg-violet-100', border: 'border-violet-300', text: 'text-violet-800' },
  completed: { bg: 'bg-emerald-100', border: 'border-emerald-300', text: 'text-emerald-800' },
  cancelled: { bg: 'bg-rose-100', border: 'border-rose-300', text: 'text-rose-800' },
};

const STATUS_LABELS: Record<Appointment['status'], string> = {
  pending: '待指派',
  assigned: '已分派',
  arrived: '清洗中',
  completed: '已完成',
  cancelled: '無法清洗',
};

export default function ScheduleGantt({ technicians, appointments, onSelectAppointment, onQuickCreate }: ScheduleGanttProps) {
  const [viewMode, setViewMode] = useState<ViewMode>('day');
  const [currentDate, setCurrentDate] = useState(new Date());

  const weekStart = useMemo(() => startOfWeek(currentDate, { weekStartsOn: 1 }), [currentDate]);
  const weekDays = useMemo(() => Array.from({ length: 7 }, (_, i) => addDays(weekStart, i)), [weekStart]);

  const navigateDate = (direction: number) => {
    if (viewMode === 'day') {
      setCurrentDate(prev => addDays(prev, direction));
    } else {
      setCurrentDate(prev => addDays(prev, direction * 7));
    }
  };

  const isSlotAvailable = (tech: User, day: number, hour: string): boolean => {
    if (!tech.availability || tech.availability.length === 0) return true;
    const avail = tech.availability.find(a => a.day === day);
    if (!avail) return false;
    return avail.slots.includes(hour);
  };

  const getAppointmentsForTechAndDate = (techId: number, date: Date): Appointment[] => {
    return appointments.filter(appt => {
      if (appt.technician_id !== techId) return false;
      try {
        const apptDate = parseISO(appt.scheduled_at);
        return isSameDay(apptDate, date);
      } catch {
        return false;
      }
    });
  };

  const getAppointmentAtHour = (techId: number, date: Date, hour: number): Appointment | undefined => {
    return appointments.find(appt => {
      if (appt.technician_id !== techId) return false;
      try {
        const apptDate = parseISO(appt.scheduled_at);
        return isSameDay(apptDate, date) && apptDate.getHours() === hour;
      } catch {
        return false;
      }
    });
  };

  const handleEmptySlotClick = (techId: number, date: Date, hour: number) => {
    const dateTime = new Date(date);
    dateTime.setHours(hour, 0, 0, 0);
    onQuickCreate(techId, dateTime.toISOString());
  };

  const renderDayView = () => (
    <div className="overflow-x-auto">
      <div className="min-w-[800px]">
        <div className="grid" style={{ gridTemplateColumns: `120px repeat(${HOURS.length}, 1fr)` }}>
          <div className="sticky left-0 z-10 bg-slate-50 border-b border-r border-slate-200 p-2 text-xs font-bold text-slate-500 uppercase">
            師傅
          </div>
          {HOURS.map(hour => (
            <div key={hour} className="border-b border-r border-slate-200 p-2 text-xs font-medium text-slate-500 text-center bg-slate-50">
              {String(hour).padStart(2, '0')}:00
            </div>
          ))}

          {technicians.map(tech => (
            <div key={tech.id} className="contents">
              <div className="sticky left-0 z-10 bg-white border-b border-r border-slate-200 p-3 flex items-center gap-2">
                <div
                  className="w-3 h-3 rounded-full flex-shrink-0"
                  style={{ backgroundColor: tech.color || '#6b7280' }}
                />
                <span className="text-sm font-medium text-slate-900 truncate" data-testid={`gantt-tech-name-${tech.id}`}>{tech.name}</span>
              </div>
              {HOURS.map(hour => {
                const dayOfWeek = currentDate.getDay();
                const hourStr = `${String(hour).padStart(2, '0')}:00`;
                const available = isSlotAvailable(tech, dayOfWeek, hourStr);
                const appt = getAppointmentAtHour(tech.id, currentDate, hour);
                const colors = appt ? STATUS_COLORS[appt.status] : null;

                return (
                  <div
                    key={hour}
                    className={cn(
                      "border-b border-r border-slate-200 p-1 min-h-[60px] relative transition-colors",
                      !available && !appt ? "bg-slate-100" : "bg-white",
                      !appt && available && "hover:bg-blue-50 cursor-pointer group"
                    )}
                    onClick={() => {
                      if (appt) return;
                      if (available) handleEmptySlotClick(tech.id, currentDate, hour);
                    }}
                    data-testid={`gantt-slot-${tech.id}-${hour}`}
                  >
                    {appt ? (
                      <div
                        onClick={(e) => { e.stopPropagation(); onSelectAppointment(appt); }}
                        className={cn(
                          "rounded-lg p-1.5 h-full cursor-pointer border transition-all hover:shadow-md",
                          colors?.bg, colors?.border, colors?.text
                        )}
                        data-testid={`gantt-appt-${appt.id}`}
                      >
                        <p className="text-[10px] font-bold truncate">{appt.customer_name}</p>
                        <p className="text-[9px] truncate opacity-70">{STATUS_LABELS[appt.status]}</p>
                      </div>
                    ) : (
                      available && (
                        <div className="invisible group-hover:visible absolute inset-0 flex items-center justify-center">
                          <Plus className="w-4 h-4 text-blue-400" />
                        </div>
                      )
                    )}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );

  const renderWeekView = () => (
    <div className="overflow-x-auto">
      <div className="min-w-[900px]">
        <div className="grid" style={{ gridTemplateColumns: `120px repeat(7, 1fr)` }}>
          <div className="sticky left-0 z-10 bg-slate-50 border-b border-r border-slate-200 p-2 text-xs font-bold text-slate-500 uppercase">
            師傅
          </div>
          {weekDays.map(day => (
            <div
              key={day.toISOString()}
              className={cn(
                "border-b border-r border-slate-200 p-2 text-center bg-slate-50",
                isSameDay(day, new Date()) && "bg-blue-50"
              )}
            >
              <p className="text-xs font-bold text-slate-700">
                {format(day, 'EEE', { locale: zhTW })}
              </p>
              <p className={cn(
                "text-sm font-medium",
                isSameDay(day, new Date()) ? "text-blue-600" : "text-slate-500"
              )}>
                {format(day, 'MM/dd')}
              </p>
            </div>
          ))}

          {technicians.map(tech => (
            <div key={tech.id} className="contents">
              <div className="sticky left-0 z-10 bg-white border-b border-r border-slate-200 p-3 flex items-center gap-2">
                <div
                  className="w-3 h-3 rounded-full flex-shrink-0"
                  style={{ backgroundColor: tech.color || '#6b7280' }}
                />
                <span className="text-sm font-medium text-slate-900 truncate">{tech.name}</span>
              </div>
              {weekDays.map(day => {
                const dayAppts = getAppointmentsForTechAndDate(tech.id, day);
                const dayOfWeek = day.getDay();
                const hasAvailability = tech.availability?.some(a => a.day === dayOfWeek);

                return (
                  <div
                    key={day.toISOString()}
                    className={cn(
                      "border-b border-r border-slate-200 p-1 min-h-[80px]",
                      !hasAvailability && dayAppts.length === 0 ? "bg-slate-100" : "bg-white",
                      isSameDay(day, new Date()) && "bg-blue-50/30"
                    )}
                  >
                    {dayAppts.length > 0 ? (
                      <div className="space-y-1">
                        {dayAppts.map(appt => {
                          const colors = STATUS_COLORS[appt.status];
                          const apptTime = format(parseISO(appt.scheduled_at), 'HH:mm');
                          return (
                            <div
                              key={appt.id}
                              onClick={() => onSelectAppointment(appt)}
                              className={cn(
                                "rounded-md p-1.5 cursor-pointer border transition-all hover:shadow-md",
                                colors.bg, colors.border, colors.text
                              )}
                              data-testid={`gantt-week-appt-${appt.id}`}
                            >
                              <p className="text-[10px] font-bold truncate">{apptTime} {appt.customer_name}</p>
                              <p className="text-[9px] truncate opacity-70">{STATUS_LABELS[appt.status]}</p>
                            </div>
                          );
                        })}
                      </div>
                    ) : (
                      hasAvailability && (
                        <div
                          className="h-full w-full flex items-center justify-center cursor-pointer hover:bg-blue-50 rounded-md transition-colors group"
                          onClick={() => {
                            const dateTime = new Date(day);
                            dateTime.setHours(9, 0, 0, 0);
                            onQuickCreate(tech.id, dateTime.toISOString());
                          }}
                          data-testid={`gantt-week-empty-${tech.id}-${format(day, 'yyyy-MM-dd')}`}
                        >
                          <Plus className="w-4 h-4 text-slate-300 invisible group-hover:visible" />
                        </div>
                      )
                    )}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            className="rounded-full w-10 h-10 p-0"
            onClick={() => navigateDate(-1)}
            data-testid="button-gantt-prev"
          >
            <ChevronLeft className="w-5 h-5" />
          </Button>
          <h3 className="text-lg font-bold text-slate-900 min-w-[180px] text-center" data-testid="text-gantt-date">
            {viewMode === 'day'
              ? format(currentDate, 'yyyy/MM/dd (EEE)', { locale: zhTW })
              : `${format(weekStart, 'MM/dd')} - ${format(addDays(weekStart, 6), 'MM/dd')}`
            }
          </h3>
          <Button
            variant="outline"
            className="rounded-full w-10 h-10 p-0"
            onClick={() => navigateDate(1)}
            data-testid="button-gantt-next"
          >
            <ChevronRight className="w-5 h-5" />
          </Button>
          <Button
            variant="secondary"
            className="text-xs px-3 py-1.5"
            onClick={() => setCurrentDate(new Date())}
            data-testid="button-gantt-today"
          >
            今天
          </Button>
        </div>
        <div className="flex gap-1 bg-slate-100 rounded-md p-1">
          <button
            onClick={() => setViewMode('day')}
            data-testid="button-gantt-day-view"
            className={cn(
              "px-4 py-2 rounded-lg text-sm font-medium transition-all",
              viewMode === 'day' ? "bg-white text-slate-900 shadow-sm" : "text-slate-500 hover:text-slate-700"
            )}
          >
            日視圖
          </button>
          <button
            onClick={() => setViewMode('week')}
            data-testid="button-gantt-week-view"
            className={cn(
              "px-4 py-2 rounded-lg text-sm font-medium transition-all",
              viewMode === 'week' ? "bg-white text-slate-900 shadow-sm" : "text-slate-500 hover:text-slate-700"
            )}
          >
            週視圖
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3 text-xs">
        <span className="text-slate-500 font-medium">圖例:</span>
        {Object.entries(STATUS_LABELS).map(([status, label]) => {
          const colors = STATUS_COLORS[status as Appointment['status']];
          return (
            <div key={status} className="flex items-center gap-1.5">
              <div className={cn("w-3 h-3 rounded-sm border", colors.bg, colors.border)} />
              <span className="text-slate-600">{label}</span>
            </div>
          );
        })}
        <div className="flex items-center gap-1.5 ml-2">
          <div className="w-3 h-3 rounded-sm bg-slate-100 border border-slate-300" />
          <span className="text-slate-600">不可用</span>
        </div>
        <div className="flex items-center gap-1.5">
          <div className="w-3 h-3 rounded-sm bg-white border border-slate-300" />
          <span className="text-slate-600">可用</span>
        </div>
      </div>

      <div className="bg-white border border-slate-200/60 rounded-lg overflow-hidden">
        {viewMode === 'day' ? renderDayView() : renderWeekView()}
      </div>
    </div>
  );
}
