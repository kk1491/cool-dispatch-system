import { useState } from 'react';
import { 
  Phone, Calendar, Clock, Plus, ChevronRight, ChevronLeft,
  LayoutDashboard, Star, DollarSign, CheckCircle2, Briefcase, MapPin,
  Palette, RotateCcw, TrendingUp, Wrench, ArrowLeft, Pencil, Trash2,
  BookOpen, CircleDot, Zap, Key, Eye, EyeOff, Lock
} from 'lucide-react';
import { 
  format, parseISO, isSameDay, isSameMonth, addDays, startOfWeek, endOfWeek, eachDayOfInterval 
} from 'date-fns';
import { zhTW } from 'date-fns/locale';
import { toast } from 'react-hot-toast';
import { motion, AnimatePresence } from 'motion/react';
import { CASH_LEDGER_OPEN_BUTTON_LABEL, getAppointmentCollectedAmount, isAppointmentFinished, isAppointmentRevenueCounted } from '../lib/appointmentMetrics';
import { useTablePagination } from '../lib/tablePagination';
import { cn } from '../lib/utils';
import { Button, Card, Badge } from './shared';
import MobileInfiniteCardList from './MobileInfiniteCardList';
import TablePagination from './TablePagination';
import { User, Appointment, ACType, ServiceZone, Review } from '../types';
import { updateTechnicianPassword } from '../lib/api';

interface TechnicianManagementProps {
  technicians: User[];
  appointments: Appointment[];
  onUpdate: (techs: User[]) => void;
  onViewLedger?: (techId: number) => void;
  reviews?: Review[];
  zones?: ServiceZone[];
}

const PRESET_COLORS = [
  '#1677ff', '#059669', '#d97706', '#dc2626', '#7c3aed', '#0891b2', '#be185d', '#65a30d'
];

const SKILL_COLORS: Record<ACType, string> = {
  '分離式': 'bg-blue-100 text-blue-700 border-blue-200',
  '吊隱式': 'bg-violet-100 text-violet-700 border-violet-200',
  '窗型': 'bg-amber-100 text-amber-700 border-amber-200',
};

const TechEditor = ({ tech, onSave, zones, isNew }: { tech: User, onSave: (updated: User, password?: string) => void, zones?: ServiceZone[], isNew?: boolean }) => {
  const [edited, setEdited] = useState<User>(JSON.parse(JSON.stringify(tech)));
  // 密码字段：新增师傅时必填，编辑时可选（为空则保留原密码）。
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [rangeStart, setRangeStart] = useState("09:00");
  const [rangeEnd, setRangeEnd] = useState("18:00");

  const timeSlots = ["09:00", "10:00", "11:00", "12:00", "13:00", "14:00", "15:00", "16:00", "17:00", "18:00"];
  const days = [1, 2, 3, 4, 5, 6, 0];
  const weekdayDays = [1, 2, 3, 4, 5];
  const weekendDays = [6, 0];
  const {
    page: timeSlotPage,
    pageSize: timeSlotPageSize,
    totalItems: timeSlotTotalItems,
    totalPages: timeSlotTotalPages,
    paginatedItems: paginatedTimeSlots,
    setPage: setTimeSlotPage,
    setPageSize: setTimeSlotPageSize,
  } = useTablePagination(timeSlots, []);

  const toggleSlot = (day: number, slot: string) => {
    const availability = edited.availability || [];
    const dayAvail = availability.find(a => a.day === day);
    
    let newAvail;
    if (dayAvail) {
      const isSelected = dayAvail.slots.includes(slot);
      const newSlots = isSelected 
        ? dayAvail.slots.filter(s => s !== slot)
        : [...dayAvail.slots, slot].sort();
      newAvail = availability.map(a => a.day === day ? { ...a, slots: newSlots } : a);
    } else {
      newAvail = [...availability, { day, slots: [slot] }];
    }
    
    setEdited({ ...edited, availability: newAvail });
  };

  const setDaysSlots = (targetDays: number[], slots: string[]) => {
    const availability = edited.availability || [];
    let newAvail = availability.filter(a => !targetDays.includes(a.day));
    if (slots.length > 0) {
      targetDays.forEach(day => {
        newAvail.push({ day, slots: [...slots].sort() });
      });
    }
    setEdited({ ...edited, availability: newAvail });
  };

  const handleSelectWeekdays = () => {
    setDaysSlots(weekdayDays, [...timeSlots]);
  };

  const handleSelectWeekends = () => {
    setDaysSlots(weekendDays, [...timeSlots]);
  };

  const handleClearAll = () => {
    setEdited({ ...edited, availability: [] });
  };

  const handleFillRange = () => {
    const startIdx = timeSlots.indexOf(rangeStart);
    const endIdx = timeSlots.indexOf(rangeEnd);
    if (startIdx === -1 || endIdx === -1 || startIdx > endIdx) return;
    const rangeSlots = timeSlots.slice(startIdx, endIdx + 1);
    setDaysSlots(weekdayDays, rangeSlots);
  };

  const toggleRow = (slot: string) => {
    const availability = edited.availability || [];
    const allSelected = days.every(day => {
      const dayAvail = availability.find(a => a.day === day);
      return dayAvail?.slots.includes(slot);
    });

    let newAvail = [...availability];
    if (allSelected) {
      newAvail = newAvail.map(a => ({
        ...a,
        slots: a.slots.filter(s => s !== slot)
      })).filter(a => a.slots.length > 0);
    } else {
      days.forEach(day => {
        const dayAvail = newAvail.find(a => a.day === day);
        if (dayAvail) {
          if (!dayAvail.slots.includes(slot)) {
            dayAvail.slots = [...dayAvail.slots, slot].sort();
          }
        } else {
          newAvail.push({ day, slots: [slot] });
        }
      });
    }
    setEdited({ ...edited, availability: newAvail });
  };

  const toggleColumn = (day: number) => {
    const availability = edited.availability || [];
    const dayAvail = availability.find(a => a.day === day);
    const allSelected = dayAvail && timeSlots.every(s => dayAvail.slots.includes(s));

    let newAvail;
    if (allSelected) {
      newAvail = availability.filter(a => a.day !== day);
    } else {
      newAvail = availability.filter(a => a.day !== day);
      newAvail.push({ day, slots: [...timeSlots] });
    }
    setEdited({ ...edited, availability: newAvail });
  };

  const toggleSkill = (skill: ACType) => {
    const skills = edited.skills || [];
    const newSkills = skills.includes(skill) ? skills.filter(s => s !== skill) : [...skills, skill];
    setEdited({ ...edited, skills: newSkills });
  };

  const allSkills: ACType[] = ['分離式', '吊隱式', '窗型'];

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-xs font-bold text-slate-400 mb-1">師傅姓名</label>
          <input 
            data-testid="input-tech-name"
            className="w-full px-4 py-3 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            value={edited.name}
            onChange={e => setEdited({ ...edited, name: e.target.value })}
          />
        </div>
        <div>
          <label className="block text-xs font-bold text-slate-400 mb-1">聯繫電話</label>
          <input 
            data-testid="input-tech-phone"
            className="w-full px-4 py-3 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            value={edited.phone}
            onChange={e => setEdited({ ...edited, phone: e.target.value })}
          />
        </div>
      </div>

      {/* 密码输入框：新增师傅时必填，编辑时可选修改 */}
      <div>
        <label className="block text-xs font-bold text-slate-400 mb-1">
          <Key className="w-3.5 h-3.5 inline mr-1" />
          {isNew ? '登入密碼（必填）' : '登入密碼（留空保留原密碼）'}
        </label>
        <div className="relative">
          <input 
            data-testid="input-tech-password"
            type={showPassword ? 'text' : 'password'}
            className="w-full px-4 py-3 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 pr-10"
            value={password}
            onChange={e => setPassword(e.target.value)}
            placeholder={isNew ? '至少 8 位' : '不修改可留空'}
          />
          <button
            type="button"
            onClick={() => setShowPassword(!showPassword)}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
          >
            {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
          </button>
        </div>
      </div>

      <div>
        <label className="block text-xs font-bold text-slate-400 mb-2">
          <Wrench className="w-3.5 h-3.5 inline mr-1" />專業技能
        </label>
        <div className="flex gap-2">
          {allSkills.map(skill => (
            <button
              key={skill}
              onClick={() => toggleSkill(skill)}
              data-testid={`toggle-skill-${skill}`}
              className={cn(
                "px-4 py-2.5 rounded-md text-sm font-medium border transition-all",
                (edited.skills || []).includes(skill)
                  ? SKILL_COLORS[skill]
                  : "bg-slate-50 text-slate-400 border-slate-100 hover:border-slate-300"
              )}
            >
              {skill}
            </button>
          ))}
        </div>
      </div>

      {zones && zones.length > 0 && (
        <div>
          <label className="block text-xs font-bold text-slate-400 mb-2">
            <MapPin className="w-3.5 h-3.5 inline mr-1" />服務區域
          </label>
          <select
            data-testid="select-tech-zone"
            className="w-full px-4 py-3 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            value={edited.zone_id || ''}
            onChange={e => setEdited({ ...edited, zone_id: e.target.value || undefined })}
          >
            <option value="">未指定</option>
            {zones.map(z => (
              <option key={z.id} value={z.id}>{z.name}</option>
            ))}
          </select>
        </div>
      )}

      <div>
        <label className="block text-xs font-bold text-slate-400 mb-2">
          <Palette className="w-3.5 h-3.5 inline mr-1" />代表色彩
        </label>
        <div className="flex gap-2">
          {PRESET_COLORS.map(c => (
            <button
              key={c}
              onClick={() => setEdited({ ...edited, color: c })}
              data-testid={`color-${c}`}
              className={cn(
                "w-9 h-9 rounded-md transition-all",
                edited.color === c ? "ring-2 ring-offset-2 ring-slate-900" : "hover:ring-1 hover:ring-slate-300"
              )}
              style={{ backgroundColor: c }}
            />
          ))}
        </div>
      </div>

      <div className="space-y-4">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">可預約時段設定</h4>

        <div className="flex flex-wrap gap-2">
          <button
            data-testid="button-select-weekdays"
            onClick={handleSelectWeekdays}
            className="px-3 py-2 rounded-md text-xs font-medium bg-blue-50 text-blue-600 border border-blue-200 hover:bg-blue-100 transition-colors"
          >
            全選平日
          </button>
          <button
            data-testid="button-select-weekends"
            onClick={handleSelectWeekends}
            className="px-3 py-2 rounded-md text-xs font-medium bg-amber-50 text-amber-600 border border-amber-200 hover:bg-amber-100 transition-colors"
          >
            全選週末
          </button>
          <button
            data-testid="button-clear-all-slots"
            onClick={handleClearAll}
            className="px-3 py-2 rounded-md text-xs font-medium bg-slate-50 text-slate-500 border border-slate-200 hover:bg-slate-100 transition-colors"
          >
            清除全部
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2 bg-slate-50 rounded-md p-3">
          <span className="text-xs font-medium text-slate-500">上班時間範圍：</span>
          <select
            data-testid="select-range-start"
            value={rangeStart}
            onChange={e => setRangeStart(e.target.value)}
            className="px-2 py-1.5 rounded-md text-xs bg-white border border-slate-200 focus:ring-1 focus:ring-blue-500"
          >
            {timeSlots.map(s => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <span className="text-xs text-slate-400">~</span>
          <select
            data-testid="select-range-end"
            value={rangeEnd}
            onChange={e => setRangeEnd(e.target.value)}
            className="px-2 py-1.5 rounded-md text-xs bg-white border border-slate-200 focus:ring-1 focus:ring-blue-500"
          >
            {timeSlots.map(s => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <button
            data-testid="button-fill-range"
            onClick={handleFillRange}
            className="px-3 py-1.5 rounded-md text-xs font-medium bg-blue-600 text-white hover:bg-blue-700 transition-colors"
          >
            填入平日
          </button>
        </div>

        <div className="space-y-3 md:hidden">
          <MobileInfiniteCardList
            items={timeSlots}
            getKey={slot => slot}
            renderItem={slot => (
              <Card className="p-4 shadow-none">
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <button
                      data-testid={`toggle-slot-${slot}`}
                      onClick={() => toggleRow(slot)}
                      className="rounded-md bg-slate-100 px-3 py-1.5 text-sm font-medium text-slate-700"
                    >
                      {slot}
                    </button>
                    <span className="text-xs text-slate-400">點選下方日期切換</span>
                  </div>
                  <div className="grid grid-cols-4 gap-2">
                    {days.map(d => {
                      const isAvailable = edited.availability?.find(a => a.day === d)?.slots.includes(slot);
                      return (
                        <button
                          key={d}
                          onClick={() => toggleSlot(d, slot)}
                          className={cn(
                            'rounded-lg border px-2 py-2 text-xs font-bold transition-all',
                            isAvailable
                              ? 'border-blue-200 bg-blue-600 text-white'
                              : 'border-slate-200 bg-white text-slate-500'
                          )}
                        >
                          {['日', '一', '二', '三', '四', '五', '六'][d]}
                        </button>
                      );
                    })}
                  </div>
                </div>
              </Card>
            )}
          />
        </div>
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr>
                <th className="p-2 text-[10px] font-bold text-slate-400">時段</th>
                {days.map(d => {
                  const dayAvail = edited.availability?.find(a => a.day === d);
                  const allDaySelected = dayAvail && timeSlots.every(s => dayAvail.slots.includes(s));
                  return (
                    <th key={d} className="p-2 text-center">
                      <button
                        data-testid={`toggle-day-${d}`}
                        onClick={() => toggleColumn(d)}
                        className={cn(
                          "text-[10px] font-bold px-2 py-1 rounded-md transition-colors cursor-pointer",
                          allDaySelected
                            ? "bg-blue-100 text-blue-700"
                            : "text-slate-400 hover:bg-slate-100 hover:text-slate-600"
                        )}
                      >
                        {['日', '一', '二', '三', '四', '五', '六'][d]}
                      </button>
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody>
              {paginatedTimeSlots.map(slot => {
                const allRowSelected = days.every(day => {
                  const dayAvail = edited.availability?.find(a => a.day === day);
                  return dayAvail?.slots.includes(slot);
                });
                return (
                  <tr key={slot} className="border-t border-slate-50">
                    <td className="p-2">
                      <button
                        data-testid={`toggle-slot-${slot}`}
                        onClick={() => toggleRow(slot)}
                        className={cn(
                          "text-xs font-medium px-2 py-1 rounded-md transition-colors cursor-pointer",
                          allRowSelected
                            ? "bg-blue-100 text-blue-700"
                            : "text-slate-500 hover:bg-slate-100 hover:text-slate-700"
                        )}
                      >
                        {slot}
                      </button>
                    </td>
                    {days.map(d => {
                      const isAvailable = edited.availability?.find(a => a.day === d)?.slots.includes(slot);
                      return (
                        <td key={d} className="p-1 text-center">
                          <button
                            onClick={() => toggleSlot(d, slot)}
                            className={cn(
                              "w-full h-8 rounded-lg transition-all",
                              isAvailable ? "bg-blue-600" : "bg-slate-100 hover:bg-slate-200"
                            )}
                          />
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <TablePagination
          page={timeSlotPage}
          pageSize={timeSlotPageSize}
          totalItems={timeSlotTotalItems}
          totalPages={timeSlotTotalPages}
          onPageChange={setTimeSlotPage}
          onPageSizeChange={setTimeSlotPageSize}
          itemLabel="列"
          className="hidden rounded-lg border border-slate-100 md:flex"
        />
      </div>

      <Button data-testid="button-save-tech" className="w-full py-4" onClick={() => {
        // 新增师傅时密码必填校验。
        if (isNew && password.trim().length < 8) {
          toast.error('師傅密碼至少需要 8 位');
          return;
        }
        // 编辑时密码不为空但小于 8 位时提示。
        if (!isNew && password.trim().length > 0 && password.trim().length < 8) {
          toast.error('師傅密碼至少需要 8 位');
          return;
        }
        onSave(edited, password.trim() || undefined);
      }}>
        儲存師傅資料
      </Button>
    </div>
  );
};

export default function TechnicianManagement({ technicians, appointments, onUpdate, onViewLedger, reviews = [], zones = [] }: TechnicianManagementProps) {
  const [selectedTech, setSelectedTech] = useState<User | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [currentWeek, setCurrentWeek] = useState(new Date());
  const [activeTab, setActiveTab] = useState<'individual' | 'master'>('individual');
  // 密码修改相关状态
  const [showPasswordSection, setShowPasswordSection] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);

  const weekDays = eachDayOfInterval({
    start: startOfWeek(currentWeek, { weekStartsOn: 1 }),
    end: endOfWeek(currentWeek, { weekStartsOn: 1 })
  });

  const today = new Date();
  const isCurrentWeek = weekDays.some(d => isSameDay(d, today));

  const getTechStats = (tech: User) => {
    const techAppts = appointments.filter(a => a.technician_id === tech.id);
    const todayAppts = techAppts.filter(a => isSameDay(parseISO(a.scheduled_at), today));
    const monthAppts = techAppts.filter(a => isSameMonth(parseISO(a.scheduled_at), today));
    // 完成数看「已结案」，收款额看「真实已收款」，避免把無收款/未收款混进营收。
    const monthFinished = monthAppts.filter(isAppointmentFinished);
    const monthRevenue = monthAppts
      .filter(isAppointmentRevenueCounted)
      .reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0);
    
    const techApptIds = techAppts.map(a => a.id);
    const techReviews = reviews.filter(r => techApptIds.includes(r.appointment_id));
    const avgRating = techReviews.length > 0
      ? techReviews.reduce((sum, r) => sum + r.rating, 0) / techReviews.length
      : 0;

    return {
      todayTotal: todayAppts.length,
      todayDone: todayAppts.filter(isAppointmentFinished).length,
      todayPending: todayAppts.filter(a => a.status === 'assigned' || a.status === 'arrived').length,
      monthCompleted: monthFinished.length,
      monthRevenue,
      avgRating,
      reviewCount: techReviews.length,
    };
  };

  const getZoneName = (zoneId?: string) => {
    if (!zoneId) return null;
    return zones.find(z => z.id === zoneId)?.name || null;
  };

  const getTodayStatus = (tech: User) => {
    const techAppts = appointments.filter(a => a.technician_id === tech.id && isSameDay(parseISO(a.scheduled_at), today));
    const hasArrived = techAppts.some(a => a.status === 'arrived');
    const allDone = techAppts.length > 0 && techAppts.every(isAppointmentFinished);
    const hasPending = techAppts.some(a => a.status === 'assigned');

    if (hasArrived) return { label: '清洗中', color: 'bg-violet-100 text-violet-700' };
    if (allDone) return { label: '已收工', color: 'bg-emerald-100 text-emerald-700' };
    if (hasPending) return { label: '待出發', color: 'bg-blue-100 text-blue-700' };
    if (techAppts.length === 0) return { label: '休息中', color: 'bg-slate-100 text-slate-500' };
    return { label: '待指派', color: 'bg-amber-100 text-amber-700' };
  };

  const handleAddTech = () => {
    const newTech: User = {
      id: Date.now(),
      name: '新師傅',
      phone: '',
      role: 'technician',
      color: PRESET_COLORS[technicians.length % PRESET_COLORS.length],
      skills: [],
      availability: [
        { day: 1, slots: ["09:00", "10:00", "11:00", "13:00", "14:00", "15:00", "16:00"] },
        { day: 2, slots: ["09:00", "10:00", "11:00", "13:00", "14:00", "15:00", "16:00"] },
        { day: 3, slots: ["09:00", "10:00", "11:00", "13:00", "14:00", "15:00", "16:00"] },
        { day: 4, slots: ["09:00", "10:00", "11:00", "13:00", "14:00", "15:00", "16:00"] },
        { day: 5, slots: ["09:00", "10:00", "11:00", "13:00", "14:00", "15:00", "16:00"] },
      ]
    };
    // 新增师傅时不立即同步后端，先进入编辑模式让用户填写完整资料后再保存。
    setSelectedTech(newTech);
    setIsEditing(true);
    setActiveTab('individual');
  };

  const handleSave = (updated: User, password?: string) => {
    // 判断是否为新增师傅（本地临时 ID 尚未存在于 technicians 列表中）。
    const isNew = !technicians.some(t => t.id === updated.id);
    // 构建带密码的 payload，同时传给后端。
    const techWithPw = password ? { ...updated, password } : updated;
    if (isNew) {
      // 新增场景：将编辑完的师傅追加到列表一并同步后端（带密码）。
      onUpdate([...technicians.map(t => ({ ...t })), techWithPw]);
    } else {
      // 编辑场景：替换既有师傅数据并同步后端（可带密码）。
      onUpdate(technicians.map(t => t.id === updated.id ? techWithPw : { ...t }));
    }
    setSelectedTech(updated);
    setIsEditing(false);
    toast.success('師傅資料已更新');
  };

  const handleDelete = (id: number) => {
    if (confirm('確定要刪除此師傅嗎？')) {
      onUpdate(technicians.filter(t => t.id !== id));
      setSelectedTech(null);
      toast.success('師傅已刪除');
    }
  };

  const renderWeekCalendar = (techId: number, techColor?: string) => (
    <div className="grid grid-cols-7 gap-2">
      {weekDays.map(day => {
        const dayAppts = appointments.filter(a => 
          a.technician_id === techId && 
          isSameDay(parseISO(a.scheduled_at), day)
        );
        const isToday = isSameDay(day, today);
        
        return (
          <div key={day.toString()} className="space-y-2">
            <div className={cn(
              "text-center py-2 px-1 rounded-md transition-colors",
              isToday ? "bg-blue-600 text-white" : "bg-slate-50"
            )}>
              <div className="text-[10px] uppercase font-bold opacity-60">
                {format(day, 'EEE', { locale: zhTW })}
              </div>
              <div className="text-base font-black">{format(day, 'd')}</div>
            </div>
            <div className={cn(
              "min-h-[180px] rounded-md p-1 space-y-1.5",
              isToday ? "bg-blue-50/50" : "bg-slate-50/30"
            )}>
              {dayAppts.map(appt => (
                <div 
                  key={appt.id} 
                  data-testid={`calendar-appt-${appt.id}`}
                  className="p-2 bg-white border border-slate-100 rounded-md shadow-sm text-[10px] leading-tight hover:shadow-md transition-shadow cursor-default"
                  style={{ borderLeft: `3px solid ${techColor || '#6b7280'}` }}
                >
                  <div className="font-bold text-slate-900">{format(parseISO(appt.scheduled_at), 'HH:mm')}</div>
                  <div className="text-slate-500 truncate">{appt.customer_name}</div>
                  <div className="mt-1"><Badge status={appt.status} /></div>
                </div>
              ))}
              {dayAppts.length === 0 && (
                <div className="h-full flex items-center justify-center py-10">
                  <div className="w-8 h-8 rounded-lg border-2 border-dashed border-slate-200" />
                </div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );

  const renderMasterCalendar = () => {
    const dailyTotals = weekDays.map(day => 
      appointments.filter(a => 
        technicians.some(t => t.id === a.technician_id) &&
        isSameDay(parseISO(a.scheduled_at), day)
      ).length
    );

    return (
      <Card className="p-6 overflow-x-auto">
        <div className="min-w-[1000px]">
          <div className="flex items-center justify-between mb-8">
            <h3 className="text-lg font-bold flex items-center gap-2">
              <LayoutDashboard className="w-5 h-5 text-slate-400" /> 全體師傅行程表
            </h3>
            <div className="flex items-center gap-2">
              {!isCurrentWeek && (
                <button
                  onClick={() => setCurrentWeek(new Date())}
                  className="px-3 py-1.5 text-xs font-medium text-slate-500 hover:text-slate-700 hover:bg-slate-100 rounded-lg transition-colors flex items-center gap-1"
                  data-testid="button-master-this-week"
                >
                  <RotateCcw className="w-3.5 h-3.5" /> 本週
                </button>
              )}
              <button onClick={() => setCurrentWeek(addDays(currentWeek, -7))} className="p-2 hover:bg-slate-100 rounded-lg transition-colors" data-testid="button-master-prev-week">
                <ChevronLeft className="w-5 h-5" />
              </button>
              <span className="text-sm font-bold min-w-[120px] text-center">
                {format(weekDays[0], 'MM/dd')} - {format(weekDays[6], 'MM/dd')}
              </span>
              <button onClick={() => setCurrentWeek(addDays(currentWeek, 7))} className="p-2 hover:bg-slate-100 rounded-lg transition-colors" data-testid="button-master-next-week">
                <ChevronRight className="w-5 h-5" />
              </button>
            </div>
          </div>

          <div className="grid grid-cols-8 border-b border-slate-100 pb-4 mb-4">
            <div className="font-bold text-xs text-slate-400 uppercase tracking-wider">師傅</div>
            {weekDays.map((day, i) => {
              const isToday = isSameDay(day, today);
              return (
                <div key={day.toString()} className={cn(
                  "text-center rounded-lg py-1",
                  isToday && "bg-blue-600 text-white"
                )}>
                  <div className={cn("text-[10px] uppercase font-bold", isToday ? "text-slate-300" : "text-slate-400")}>
                    {format(day, 'EEE', { locale: zhTW })}
                  </div>
                  <div className="text-sm font-bold">{format(day, 'MM/dd')}</div>
                </div>
              );
            })}
          </div>

          <div className="space-y-4">
            {technicians.map(tech => (
              <div key={tech.id} className="grid grid-cols-8 gap-3 items-start">
                <div className="flex items-center gap-2.5">
                  <div 
                    className="w-3 h-3 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tech.color }}
                  />
                  <div 
                    className="w-8 h-8 rounded-lg flex items-center justify-center font-bold text-white text-xs flex-shrink-0"
                    style={{ backgroundColor: tech.color }}
                  >
                    {tech.name[0]}
                  </div>
                  <div className="text-sm font-bold truncate">{tech.name}</div>
                </div>
                {weekDays.map(day => {
                  const dayAppts = appointments.filter(a => 
                    a.technician_id === tech.id && 
                    isSameDay(parseISO(a.scheduled_at), day)
                  );
                  const isToday = isSameDay(day, today);
                  return (
                    <div key={day.toString()} className={cn(
                      "space-y-1 rounded-lg p-1 min-h-[40px]",
                      isToday && "bg-blue-50/60"
                    )}>
                      {dayAppts.map(appt => (
                        <div 
                          key={appt.id}
                          data-testid={`master-appt-${appt.id}`}
                          className="p-1.5 rounded-lg text-[10px] shadow-sm border hover:shadow-md transition-shadow"
                          style={{ 
                            backgroundColor: `${tech.color}12`,
                            borderColor: `${tech.color}30`,
                            color: tech.color,
                          }}
                        >
                          <div className="font-bold">{format(parseISO(appt.scheduled_at), 'HH:mm')}</div>
                          <div className="truncate opacity-80">{appt.customer_name}</div>
                        </div>
                      ))}
                      {dayAppts.length === 0 && (
                        <div className="h-10 rounded-lg border border-dashed border-slate-100" />
                      )}
                    </div>
                  );
                })}
              </div>
            ))}
          </div>

          <div className="grid grid-cols-8 gap-3 mt-4 pt-4 border-t border-slate-100">
            <div className="text-xs font-bold text-slate-400 flex items-center">
              <TrendingUp className="w-3.5 h-3.5 mr-1" /> 合計
            </div>
            {dailyTotals.map((total, i) => {
              const isToday = isSameDay(weekDays[i], today);
              return (
                <div key={i} className={cn(
                  "text-center text-sm font-black rounded-lg py-1",
                  isToday ? "bg-blue-100 text-blue-700" : "text-slate-400"
                )}>
                  {total}
                </div>
              );
            })}
          </div>
        </div>
      </Card>
    );
  };

  const renderTechCard = (tech: User) => {
    const stats = getTechStats(tech);
    const zoneName = getZoneName(tech.zone_id);
    const status = getTodayStatus(tech);
    const todayAppts = appointments
      .filter(a => a.technician_id === tech.id && isSameDay(parseISO(a.scheduled_at), today))
      .sort((a, b) => a.scheduled_at.localeCompare(b.scheduled_at));

    return (
      <motion.div
        key={tech.id}
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        className="group"
      >
        <Card 
          className="p-0 overflow-hidden hover:shadow-lg transition-all duration-300 cursor-pointer border-slate-100 hover:border-slate-200"
          onClick={() => { setSelectedTech(tech); setIsEditing(false); }}
          data-testid={`button-tech-${tech.id}`}
        >
          <div className="p-5">
            <div className="flex items-start gap-4">
              <div 
                className="w-14 h-14 rounded-xl flex items-center justify-center text-xl font-bold text-white shadow-md flex-shrink-0"
                style={{ backgroundColor: tech.color }}
                data-testid={`avatar-tech-${tech.id}`}
              >
                {tech.name[0]}
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2.5 mb-1">
                  <span className="text-base font-bold text-slate-900" data-testid={`text-tech-name-${tech.id}`}>{tech.name}</span>
                  <span className={cn("text-[10px] font-bold px-2 py-0.5 rounded-full", status.color)} data-testid={`status-tech-${tech.id}`}>
                    {status.label}
                  </span>
                </div>
                <div className="flex items-center gap-3 text-xs text-slate-400">
                  <span className="flex items-center gap-1" data-testid={`text-tech-phone-${tech.id}`}>
                    <Phone className="w-3 h-3" />{tech.phone}
                  </span>
                  {zoneName && (
                    <span className="flex items-center gap-1">
                      <MapPin className="w-3 h-3" />{zoneName}
                    </span>
                  )}
                </div>
                {tech.skills && tech.skills.length > 0 && (
                  <div className="flex gap-1.5 mt-2">
                    {tech.skills.map(skill => (
                      <span key={skill} className={cn("text-[10px] font-medium px-2 py-0.5 rounded-md border", SKILL_COLORS[skill])}>
                        {skill}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="grid grid-cols-4 border-t border-slate-50">
            <div className="py-3 px-2 text-center border-r border-slate-50">
              <div className="text-lg font-black text-blue-600">{stats.todayDone}/{stats.todayTotal}</div>
              <div className="text-[10px] text-slate-400 font-medium">今日任務</div>
            </div>
            <div className="py-3 px-2 text-center border-r border-slate-50">
              <div className="text-lg font-black text-emerald-600">{stats.monthCompleted}</div>
              <div className="text-[10px] text-slate-400 font-medium">本月完成</div>
            </div>
            <div className="py-3 px-2 text-center border-r border-slate-50">
              <div className="text-lg font-black text-amber-600">${(stats.monthRevenue / 1000).toFixed(0)}k</div>
              <div className="text-[10px] text-slate-400 font-medium">本月收款</div>
            </div>
            <div className="py-3 px-2 text-center">
              <div className="text-lg font-black text-violet-600 flex items-center justify-center gap-0.5">
                {stats.avgRating > 0 ? (
                  <>{stats.avgRating.toFixed(1)} <Star className="w-3 h-3 fill-violet-400 text-violet-400" /></>
                ) : '—'}
              </div>
              <div className="text-[10px] text-slate-400 font-medium">
                {stats.reviewCount > 0 ? `${stats.reviewCount}則` : '評價'}
              </div>
            </div>
          </div>

          {todayAppts.length > 0 && (
            <div className="px-5 py-3 bg-slate-50/80 border-t border-slate-100">
              <div className="flex items-center gap-1.5 mb-2">
                <Clock className="w-3 h-3 text-slate-400" />
                <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider">今日行程</span>
              </div>
              <div className="flex gap-2 overflow-x-auto pb-1">
                {todayAppts.map(appt => (
                  <div key={appt.id} className="flex items-center gap-2 bg-white rounded-md px-2.5 py-1.5 border border-slate-100 flex-shrink-0" data-testid={`today-appt-${appt.id}`}>
                    <span className="text-xs font-bold text-slate-700">{format(parseISO(appt.scheduled_at), 'HH:mm')}</span>
                    <span className="text-xs text-slate-500">{appt.customer_name}</span>
                    <Badge status={appt.status} />
                  </div>
                ))}
              </div>
            </div>
          )}
        </Card>
      </motion.div>
    );
  };

  const renderTechDetail = (tech: User) => {
    const stats = getTechStats(tech);
    const zoneName = getZoneName(tech.zone_id);
    const status = getTodayStatus(tech);

    return (
      <motion.div
        initial={{ opacity: 0, x: 20 }}
        animate={{ opacity: 1, x: 0 }}
        className="space-y-6"
      >
        <div className="flex items-center gap-3">
          <button
            onClick={() => { setSelectedTech(null); setIsEditing(false); }}
            className="p-2 hover:bg-slate-100 rounded-lg transition-colors"
            data-testid="button-back-to-list"
          >
            <ArrowLeft className="w-5 h-5 text-slate-500" />
          </button>
          <span className="text-sm text-slate-400 font-medium">返回師傅列表</span>
        </div>

        <Card className="p-0 overflow-hidden">
          {/* 蓝色区域仅保留为顶部装饰条，高度压低，避免继续成为姓名与操作区的视觉背景。 */}
          <div 
            className="relative h-12 sm:h-14"
            style={{ background: `linear-gradient(135deg, ${tech.color}, ${tech.color}cc)` }}
          >
            <div className="absolute inset-0 opacity-10" style={{ backgroundImage: 'url("data:image/svg+xml,%3Csvg width=\'20\' height=\'20\' viewBox=\'0 0 20 20\' xmlns=\'http://www.w3.org/2000/svg\'%3E%3Cg fill=\'%23fff\' fill-opacity=\'1\' fill-rule=\'evenodd\'%3E%3Ccircle cx=\'3\' cy=\'3\' r=\'1.5\'/%3E%3C/g%3E%3C/svg%3E")' }} />
          </div>
          <div className="px-6 pb-6 pt-4">
            {/* 仅头像略微压住装饰条，姓名、电话、区域与按钮全部放回白底正文流，彻底切断与蓝色背景的重叠。 */}
            <div className="relative z-10 mb-5">
              <div className="flex flex-col gap-4 xl:flex-row xl:items-end">
                <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-end sm:gap-5 xl:flex-1">
                  <div 
                    className="-mt-8 sm:-mt-10 w-20 h-20 rounded-2xl flex items-center justify-center text-3xl font-bold text-white shadow-xl border-4 border-white shrink-0"
                    style={{ backgroundColor: tech.color }}
                  >
                    {tech.name[0]}
                  </div>
                  <div className="min-w-0 pt-1 xl:pb-1">
                    <div className="flex flex-wrap items-center gap-2.5">
                      <h2 className="text-2xl font-bold text-slate-900 break-words">{tech.name}</h2>
                      <span className={cn("text-xs font-bold px-2.5 py-0.5 rounded-full", status.color)}>
                        {status.label}
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-slate-500">
                      <a href={`tel:${tech.phone}`} className="flex items-center gap-1 hover:text-blue-600 transition-colors" data-testid="link-tech-phone">
                        <Phone className="w-3.5 h-3.5" /> {tech.phone}
                      </a>
                      {zoneName && (
                        <span className="flex items-center gap-1">
                          <MapPin className="w-3.5 h-3.5" /> {zoneName}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
                {/* 操作按钮改为自动换行，避免窄屏时把标题区继续顶回蓝色头部。 */}
                <div className="flex flex-wrap gap-2 xl:justify-end xl:pb-1">
                  {onViewLedger && (
                    <Button variant="outline" className="text-xs py-2 px-3" onClick={() => onViewLedger(tech.id)} data-testid="button-view-ledger">
                      <BookOpen className="w-3.5 h-3.5" /> {CASH_LEDGER_OPEN_BUTTON_LABEL}
                    </Button>
                  )}
                  <Button variant="outline" className="text-xs py-2 px-3" onClick={() => setIsEditing(!isEditing)} data-testid="button-edit-tech">
                    <Pencil className="w-3.5 h-3.5" /> {isEditing ? '取消' : '編輯'}
                  </Button>
                  <Button variant="danger" className="text-xs py-2 px-3" onClick={() => handleDelete(tech.id)} data-testid="button-delete-tech">
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              </div>
            </div>

            {tech.skills && tech.skills.length > 0 && (
              <div className="flex flex-wrap gap-2 mb-4">
                {tech.skills.map(skill => (
                  <span key={skill} className={cn("text-xs font-medium px-3 py-1 rounded-lg border", SKILL_COLORS[skill])}>
                    {skill}
                  </span>
                ))}
              </div>
            )}

            <div className="grid grid-cols-4 gap-4">
              <div className="bg-blue-50 rounded-xl p-4 text-center" data-testid="stat-today">
                <Briefcase className="w-5 h-5 text-blue-500 mx-auto mb-1.5" />
                <div className="text-2xl font-black text-blue-700">{stats.todayDone}/{stats.todayTotal}</div>
                <div className="text-[11px] text-blue-500 font-medium mt-0.5">今日任務</div>
              </div>
              <div className="bg-emerald-50 rounded-xl p-4 text-center" data-testid="stat-month-done">
                <CheckCircle2 className="w-5 h-5 text-emerald-500 mx-auto mb-1.5" />
                <div className="text-2xl font-black text-emerald-700">{stats.monthCompleted}</div>
                <div className="text-[11px] text-emerald-500 font-medium mt-0.5">本月完成</div>
              </div>
              <div className="bg-amber-50 rounded-xl p-4 text-center" data-testid="stat-month-revenue">
                <DollarSign className="w-5 h-5 text-amber-500 mx-auto mb-1.5" />
                <div className="text-2xl font-black text-amber-700">${stats.monthRevenue.toLocaleString()}</div>
                <div className="text-[11px] text-amber-500 font-medium mt-0.5">本月收款</div>
              </div>
              <div className="bg-violet-50 rounded-xl p-4 text-center" data-testid="stat-rating">
                <Star className="w-5 h-5 text-violet-500 mx-auto mb-1.5" />
                <div className="text-2xl font-black text-violet-700">
                  {stats.avgRating > 0 ? stats.avgRating.toFixed(1) : '—'}
                </div>
                <div className="text-[11px] text-violet-500 font-medium mt-0.5">
                  {stats.reviewCount > 0 ? `${stats.reviewCount} 則評價` : '尚無評價'}
                </div>
              </div>
            </div>
          </div>
        </Card>

        {/* 密码管理区块 */}
        <Card className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-bold flex items-center gap-2">
              <Lock className="w-5 h-5 text-slate-400" />密碼管理
            </h3>
            <Button 
              variant="outline" 
              className="text-xs py-2 px-3"
              onClick={() => { setShowPasswordSection(!showPasswordSection); setNewPassword(''); setShowNewPassword(false); }}
              data-testid="button-toggle-password"
            >
              <Key className="w-3.5 h-3.5" />
              {showPasswordSection ? '取消' : '修改密碼'}
            </Button>
          </div>
          {showPasswordSection ? (
            <div className="space-y-3">
              <div className="relative">
                <input
                  data-testid="input-change-password"
                  type={showNewPassword ? 'text' : 'password'}
                  className="w-full px-4 py-3 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 pr-10"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  placeholder="輸入新密碼（至少 8 位）"
                />
                <button
                  type="button"
                  onClick={() => setShowNewPassword(!showNewPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                >
                  {showNewPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                </button>
              </div>
              <Button
                className="w-full py-3"
                data-testid="button-save-password"
                disabled={passwordSaving || newPassword.trim().length < 8}
                onClick={async () => {
                  if (newPassword.trim().length < 8) {
                    toast.error('密碼至少需要 8 位');
                    return;
                  }
                  setPasswordSaving(true);
                  try {
                    await updateTechnicianPassword(tech.id, newPassword.trim());
                    toast.success('師傅密碼已更新');
                    setShowPasswordSection(false);
                    setNewPassword('');
                    setShowNewPassword(false);
                  } catch (err) {
                    console.error(err);
                    toast.error('更新密碼失敗');
                  } finally {
                    setPasswordSaving(false);
                  }
                }}
              >
                {passwordSaving ? '保存中...' : '確認修改密碼'}
              </Button>
            </div>
          ) : (
            <p className="text-sm text-slate-400">點擊「修改密碼」可重置師傅的登入密碼。</p>
          )}
        </Card>

        <Card className="p-6">
          {isEditing ? (
            <TechEditor tech={tech} onSave={handleSave} zones={zones} isNew={false} />
          ) : (
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <h3 className="font-bold flex items-center gap-2">
                  <Calendar className="w-5 h-5 text-slate-400" /> 師傅行程表
                </h3>
                <div className="flex items-center gap-2">
                  {!isCurrentWeek && (
                    <button
                      onClick={() => setCurrentWeek(new Date())}
                      className="px-3 py-1.5 text-xs font-medium text-slate-500 hover:text-slate-700 hover:bg-slate-100 rounded-lg transition-colors flex items-center gap-1"
                      data-testid="button-this-week"
                    >
                      <RotateCcw className="w-3.5 h-3.5" /> 本週
                    </button>
                  )}
                  <button onClick={() => setCurrentWeek(addDays(currentWeek, -7))} className="p-2 hover:bg-slate-100 rounded-lg transition-colors" data-testid="button-prev-week">
                    <ChevronLeft className="w-5 h-5" />
                  </button>
                  <span className="text-sm font-bold min-w-[120px] text-center">
                    {format(weekDays[0], 'MM/dd')} - {format(weekDays[6], 'MM/dd')}
                  </span>
                  <button onClick={() => setCurrentWeek(addDays(currentWeek, 7))} className="p-2 hover:bg-slate-100 rounded-lg transition-colors" data-testid="button-next-week">
                    <ChevronRight className="w-5 h-5" />
                  </button>
                </div>
              </div>

              {renderWeekCalendar(tech.id, tech.color)}
            </div>
          )}
        </Card>
      </motion.div>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex gap-1 bg-slate-100 p-1 rounded-lg w-fit">
          <button 
            onClick={() => { setActiveTab('individual'); setSelectedTech(null); setIsEditing(false); }}
            className={cn(
              "px-6 py-2.5 rounded-md text-sm font-bold transition-all",
              activeTab === 'individual' ? "bg-white text-slate-900 shadow-sm" : "text-slate-500 hover:text-slate-700"
            )}
            data-testid="tab-individual"
          >
            師傅總覽
          </button>
          <button 
            onClick={() => setActiveTab('master')}
            className={cn(
              "px-6 py-2.5 rounded-md text-sm font-bold transition-all",
              activeTab === 'master' ? "bg-white text-slate-900 shadow-sm" : "text-slate-500 hover:text-slate-700"
            )}
            data-testid="tab-master"
          >
            全體行事曆
          </button>
        </div>

        {activeTab === 'individual' && !selectedTech && (
          <Button data-testid="button-add-tech" onClick={handleAddTech} className="py-2.5 px-4 text-sm">
            <Plus className="w-4 h-4" /> 新增師傅
          </Button>
        )}
      </div>

      {activeTab === 'individual' ? (
        <AnimatePresence mode="wait">
          {selectedTech ? (
            // 新增师傅（尚未保存到后端）时，跳过统计详情卡片，直接显示编辑器表单。
            isEditing && !technicians.some(t => t.id === selectedTech.id) ? (
              <motion.div
                initial={{ opacity: 0, x: 20 }}
                animate={{ opacity: 1, x: 0 }}
                className="space-y-6"
              >
                <div className="flex items-center gap-3">
                  <button
                    onClick={() => { setSelectedTech(null); setIsEditing(false); }}
                    className="p-2 hover:bg-slate-100 rounded-lg transition-colors"
                    data-testid="button-back-to-list"
                  >
                    <ArrowLeft className="w-5 h-5 text-slate-500" />
                  </button>
                  <span className="text-sm text-slate-400 font-medium">返回師傅列表</span>
                </div>
                <Card className="p-6">
                  <h3 className="font-bold text-lg mb-4 flex items-center gap-2">
                    <Plus className="w-5 h-5 text-blue-500" /> 新增師傅
                  </h3>
                  <TechEditor tech={selectedTech} onSave={handleSave} zones={zones} isNew={true} />
                </Card>
              </motion.div>
            ) : (
              renderTechDetail(selectedTech)
            )
          ) : (
            <motion.div
              key="grid"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
            >
              <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-5">
                {technicians.map(tech => renderTechCard(tech))}
              </div>

              {technicians.length === 0 && (
                <div className="text-center py-20 text-slate-400">
                  <Zap className="w-12 h-12 mx-auto mb-4 opacity-20" />
                  <p className="font-medium">尚未新增任何師傅</p>
                  <p className="text-sm mt-1">點擊上方「新增師傅」按鈕開始</p>
                </div>
              )}
            </motion.div>
          )}
        </AnimatePresence>
      ) : (
        renderMasterCalendar()
      )}
    </div>
  );
}
