import { useState, useEffect } from 'react';
import { 
  ClipboardList, User as UserIcon, Plus, ChevronRight, LogOut, Package, 
  DollarSign, Users, MessageSquare, MapPin, Phone, Calendar,
  CheckCircle2, X, Search, Clock, CalendarDays, Map, Star, LayoutDashboard,
  AlertTriangle, Download, Send, Link2, Copy, Check, CreditCard
} from 'lucide-react';
import { Switch, Route } from 'wouter';
import { motion, AnimatePresence } from 'motion/react';
import { format, parseISO, isAfter, isSameDay, addMinutes, subMinutes } from 'date-fns';
import { Toaster, toast } from 'react-hot-toast';
import { cn } from './lib/utils';
import { User, Appointment, AppointmentCreatePayload, AppointmentReadablePaymentMethod, ACType, ACUnit, Customer, ExtraItem, CashLedgerCreatePayload, CashLedgerEntry, ServiceZone, NotificationLog, NotificationLogDraft, Review, ReviewDraft, LineFriend, ServiceItem } from './types';
import { TAIPEI_DISTRICTS, NEW_TAIPEI_DISTRICTS } from './data/constants';
import { Button, Card, Badge } from './components/shared';
import LoginPage from './components/LoginPage';
import TechnicianDashboard from './components/TechnicianDashboard';
import AppointmentEditor from './components/AppointmentEditor';
import TechnicianManagement from './components/TechnicianManagement';
import CustomerManagement from './components/CustomerManagement';
import FinancialReportView from './components/FinancialReportView';
import LineDataView, { LineFriendPicker } from './components/LineDataView';
import SettingsView from './components/SettingsView';
import CashLedger from './components/CashLedger';
import ReminderSystem from './components/ReminderSystem';
import ZoneManagement from './components/ZoneManagement';
import { matchZoneByAddress } from './lib/zoneUtils';
import ScheduleGantt from './components/ScheduleGantt';
import NotificationSender from './components/NotificationSender';
import HeatMap from './components/HeatMap';
import ReviewPage from './components/ReviewPage';
import PaymentPage from './components/PaymentPage';
import PaymentManagement from './components/PaymentManagement';
import ReviewDashboard from './components/ReviewDashboard';
import DashboardView from './components/DashboardView';
import { getAutoDispatchSuggestions, DispatchScore } from './lib/autoDispatch';
import {
  CASH_LEDGER_RETURN_FAILURE_MESSAGE,
  CASH_LEDGER_TITLE,
  getAppointmentCollectedAmount,
  getChargeableAmount,
  getPaymentMethodLabel,
  getOutstandingAmount,
  getWritablePaymentMethod,
  isAppointmentFinished,
  isAppointmentRevenueCounted,
} from './lib/appointmentMetrics';
import {
  AUTH_REQUIRED_EVENT,
  AuthRequiredError,
  AppDataSnapshot,
  BootstrapPayload,
  createAppointment,
  createCashLedgerEntry as createCashLedgerEntryRequest,
  createNotificationLog as createNotificationLogRequest,
  createReview,
  deleteAppointment,
  fetchAppSnapshot,
  fetchAppointments,
  fetchAuthMe,
  fetchCashLedgerPageData,
  fetchCashLedgerEntries,
  fetchCustomerPageData,
  fetchCustomers,
  fetchDashboardPageData,
  fetchExtraItems,
  fetchLineData,
  fetchLinePageData,
  fetchFinancialReportPageData,
  fetchNotificationLogs,
  fetchReminderPageData,
  fetchReviewDashboardPageData,
  fetchReviews,
  fetchSettingsPageData,
  fetchServiceItems,
  fetchSettings,
  fetchTechnicianPageData,
  fetchTechnicians,
  fetchZonePageData,
  fetchZones,
  linkLineFriendCustomer,
  login as loginRequest,
  logoutRequest,
  replaceCustomers,
  replaceExtraItems,
  replaceServiceItems,
  replaceTechnicians,
  replaceZones,
  toAppointmentUpdatePayload,
  updateAppointment,
  updateReminderDays,
  updateWebhookEnabled as updateWebhookEnabledRequest,
  WebhookSettingsPayload,
} from './lib/api';

type ViewType = 'dashboard' | 'list' | 'create' | 'technicians' | 'customers' | 'line' | 'settings' | 'financials' | 'reminders' | 'cashLedger' | 'schedule' | 'zones' | 'heatmap' | 'reviews' | 'payments';

const EMPTY_APPOINTMENT_ITEMS: ACUnit[] = [];

type CreateFormDraft = {
  customer_name?: string;
  phone?: string;
  address?: string;
  line_uid?: string;
  scheduled_at?: string;
  technician_id?: number | null;
};

// toISOStringFromLocalInput 把日期 + 时间输入统一转成后端要求的 RFC3339 UTC 字符串。
const toISOStringFromLocalInput = (date: string, time: string): string => {
  return new Date(`${date}T${time}`).toISOString();
};

export default function App() {
  const defaultWebhookSettings: WebhookSettingsPayload = {
    enabled: true,
    effective_enabled: false,
    url: '',
    url_source: 'UNAVAILABLE',
    url_is_public: false,
    has_line_channel_secret: false,
    status_message: '尚未載入 webhook 設定。',
    dependency_summary: '啟用條件：管理員開關、LINE_CHANNEL_SECRET、以及可從外網存取的 webhook URL。',
  };
  const [user, setUser] = useState<User | null>(null);
  const [allUsers, setAllUsers] = useState<User[]>([]);
  const [appointments, setAppointments] = useState<Appointment[]>([]);
  const [technicians, setTechnicians] = useState<User[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [lineFriends, setLineFriends] = useState<LineFriend[]>([]);
  const [view, setView] = useState<ViewType>('dashboard');
  const [extraFeeProducts, setExtraFeeProducts] = useState<ExtraItem[]>([]);
  const [selectedAppt, setSelectedAppt] = useState<Appointment | null>(null);
  const [statusFilter, setStatusFilter] = useState<Appointment['status'] | 'all'>('all');
  const [techFilter, setTechFilter] = useState<number | 'all'>('all');
  const [acTypeFilter, setAcTypeFilter] = useState<ACType | 'all'>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [dateRange, setDateRange] = useState<{ start: string; end: string }>({ start: '', end: '' });
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [reminderDays, setReminderDays] = useState(180);
  const [webhookSettings, setWebhookSettings] = useState<WebhookSettingsPayload>(defaultWebhookSettings);
  const [serviceItems, setServiceItems] = useState<ServiceItem[]>([]);
  const [newApptItems, setNewApptItems] = useState<ACUnit[]>(EMPTY_APPOINTMENT_ITEMS);
  const [newApptDiscount, setNewApptDiscount] = useState(0);
  const [newApptExtraItems, setNewApptExtraItems] = useState<ExtraItem[]>([]);
  const [createFormName, setCreateFormName] = useState('');
  const [createFormPhone, setCreateFormPhone] = useState('');
  const [createFormAddress, setCreateFormAddress] = useState('');
  const [createFormLineUid, setCreateFormLineUid] = useState('');
  const [createFormDate, setCreateFormDate] = useState('');
  const [createFormTimeStart, setCreateFormTimeStart] = useState('');
  const [createFormTimeEnd, setCreateFormTimeEnd] = useState('');
  const createFormScheduledAt = createFormDate && createFormTimeStart
    ? toISOStringFromLocalInput(createFormDate, createFormTimeStart)
    : '';
  const [createFormTechId, setCreateFormTechId] = useState<number | null>(null);
  const [createFormDistrict, setCreateFormDistrict] = useState('');
  const [cashLedgerEntries, setCashLedgerEntries] = useState<CashLedgerEntry[]>([]);
  const [selectedLedgerTechId, setSelectedLedgerTechId] = useState<number | null>(null);
  const [notificationLogs, setNotificationLogs] = useState<NotificationLog[]>([]);
  const [zones, setZones] = useState<ServiceZone[]>([]);
  const [reviews, setReviews] = useState<Review[]>([]);
  const [dispatchSuggestions, setDispatchSuggestions] = useState<DispatchScore[]>([]);
  const [showDispatch, setShowDispatch] = useState(false);
  const [snapshotLoaded, setSnapshotLoaded] = useState(false);
  const [snapshotError, setSnapshotError] = useState('');
  const [viewDataLoading, setViewDataLoading] = useState(false);
  const [viewDataError, setViewDataError] = useState('');
  // showLogoutConfirm 控制自定义登出确认弹窗的显示状态，替代原生 window.confirm。
  const [showLogoutConfirm, setShowLogoutConfirm] = useState(false);
  const currentMonthKey = format(new Date(), 'yyyy-MM');
  // currentMonthFinishedAppointments 统一提供“本月已结案”集合，避免列表页摘要卡片继续拿全量数据冒充月度指标。
  const currentMonthFinishedAppointments = appointments.filter(
    appt => isAppointmentFinished(appt) && appt.scheduled_at.startsWith(currentMonthKey)
  );
  // currentMonthCollectedAppointments 统一提供“本月真实已收款”集合，确保统计页/绩效页/财务页都以同一收款口径看营收。
  const currentMonthCollectedAppointments = currentMonthFinishedAppointments.filter(isAppointmentRevenueCounted);

  // mergeTechniciansIntoUsers 仅替换 allUsers 中的技师子集，避免页面级读取把管理员账号从登录态缓存里覆盖掉。
  const mergeTechniciansIntoUsers = (nextTechnicians: User[]) => {
    setAllUsers(prev => [
      ...prev.filter(item => item.role !== 'technician'),
      ...nextTechnicians,
    ].sort((a, b) => a.id - b.id));
  };

  // applyAppSnapshot 统一把资源级快照灌入页面状态，避免页面继续混用 bootstrap 语义和真实读模型。
  // 所有数组字段用 || [] 兜底，防止后端返回 null 时前端 .map()/.filter() 崩溃。
  const applyAppSnapshot = (data: AppDataSnapshot) => {
    const users = data.users || [];
    const customers = data.customers || [];
    const appointments = data.appointments || [];
    const lineFriends = data.line_friends || [];
    const extraFeeProducts = data.extra_fee_products || [];
    const cashLedgerEntries = data.cash_ledger_entries || [];
    const zones = data.zones || [];
    const reviews = data.reviews || [];
    const notificationLogs = data.notification_logs || [];
    const svcItems = data.service_items || [];
    const reminderDaysVal = data.settings?.reminder_days ?? 180;
    const nextWebhookSettings = data.settings?.webhook || defaultWebhookSettings;

    setAllUsers(users);
    setTechnicians(users.filter((u: User) => u.role === 'technician'));
    setCustomers(customers);
    setAppointments(appointments);
    setLineFriends(lineFriends);
    setExtraFeeProducts(extraFeeProducts);
    setCashLedgerEntries(cashLedgerEntries);
    setZones(zones);
    setReviews(reviews);
    setNotificationLogs(notificationLogs);
    setServiceItems(svcItems);
    setReminderDays(reminderDaysVal);
    setWebhookSettings(nextWebhookSettings);
    setNewApptItems(svcItems.length > 0 ? [{
      id: '1',
      type: svcItems[0].name,
      note: '',
      price: svcItems[0].default_price,
    }] : []);
    setUser(prev => prev ? (users.find((item: User) => item.id === prev.id) || prev) : prev);
  };

  // clearAppSnapshotState 在登录失效或切换用户前清空内存中的业务数据，避免旧账号快照残留到下一次登录会话。
  const clearAppSnapshotState = () => {
    setAllUsers([]);
    setAppointments([]);
    setTechnicians([]);
    setCustomers([]);
    setLineFriends([]);
    setExtraFeeProducts([]);
    setCashLedgerEntries([]);
    setZones([]);
    setReviews([]);
    setNotificationLogs([]);
    setServiceItems([]);
    setReminderDays(180);
    setWebhookSettings(defaultWebhookSettings);
    setNewApptItems(EMPTY_APPOINTMENT_ITEMS);
    setNewApptExtraItems([]);
    setNewApptDiscount(0);
    setSelectedLedgerTechId(null);
  };

  // handleAuthRequired 统一处理后端返回 401 且明确表示登录失效的场景，避免各页面各自判断字符串。
  const handleAuthRequired = () => {
    setUser(null);
    clearAppSnapshotState();
    setSnapshotError('');
    setViewDataError('');
    setViewDataLoading(false);
    setSnapshotLoaded(true);
    setIsDrawerOpen(false);
    setSelectedAppt(null);
    setShowLogoutConfirm(false);
  };

  useEffect(() => {
    const onAuthRequired = () => {
      handleAuthRequired();
    };

    window.addEventListener(AUTH_REQUIRED_EVENT, onAuthRequired);
    return () => {
      window.removeEventListener(AUTH_REQUIRED_EVENT, onAuthRequired);
    };
  }, []);

  // refreshAppSnapshot 在初始化、进入快照型页面和写操作后统一回读真实资源级数据。
  const refreshAppSnapshot = async () => {
    const data = await fetchAppSnapshot();
    applyAppSnapshot(data);
    return data;
  };

  useEffect(() => {
    let cancelled = false;

    const loadInitialSnapshot = async () => {
      try {
        // 初始化时先通过 cookie token 恢复登录态，支持页面刷新/服务重启后自动保持登录。
        const savedUser = await fetchAuthMe();
        if (cancelled) {
          return;
        }
        if (!savedUser) {
          setUser(null);
          setSnapshotLoaded(true);
          return;
        }
        setUser(savedUser);

        const data = await fetchAppSnapshot();
        if (!cancelled) {
          applyAppSnapshot(data);
          setSnapshotLoaded(true);
        }
      } catch (err) {
        if (!cancelled) {
          if (err instanceof AuthRequiredError) {
            handleAuthRequired();
            return;
          }
          console.error('Failed to fetch app snapshot.', err);
          setSnapshotError('載入後端資料失敗，請確認 Go API 與資料庫已啟動。');
          setSnapshotLoaded(true);
        }
      }
    };

    void loadInitialSnapshot();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    // 管理页进入目标视图后，按页面职责请求真实页面级/资源级接口，逐步替代初始化时的全量快照直出。
    if (!snapshotLoaded || snapshotError || user?.role !== 'admin') {
      return;
    }

    const targetViews: ViewType[] = ['dashboard', 'list', 'create', 'schedule', 'technicians', 'customers', 'line', 'settings', 'financials', 'reminders', 'cashLedger', 'zones', 'heatmap', 'reviews'];
    if (!targetViews.includes(view)) {
      setViewDataError('');
      setViewDataLoading(false);
      return;
    }
    if (view === 'cashLedger' && !selectedLedgerTechId) {
      return;
    }

    let cancelled = false;

    const syncViewData = async () => {
      setViewDataLoading(true);
      setViewDataError('');

      try {
        // list/create/schedule/heatmap 仍复用全局快照，但数据来源已统一切到真实资源级接口并发读取。
        if (view === 'list' || view === 'create' || view === 'schedule' || view === 'heatmap') {
          const data = await refreshAppSnapshot();
          if (cancelled) return;
          applyAppSnapshot(data);
          return;
        }

        if (view === 'dashboard') {
          const data = await fetchDashboardPageData();
          if (cancelled) return;
          setAppointments(data.appointments);
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          setCustomers(data.customers);
          setReviews(data.reviews);
          return;
        }

        if (view === 'customers') {
          const data = await fetchCustomerPageData();
          if (cancelled) return;
          setCustomers(data.customers);
          setAppointments(data.appointments);
          setReviews(data.reviews);
          return;
        }

        if (view === 'technicians') {
          const data = await fetchTechnicianPageData();
          if (cancelled) return;
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          setAppointments(data.appointments);
          setReviews(data.reviews);
          setZones(data.zones);
          return;
        }

        if (view === 'reminders') {
          const data = await fetchReminderPageData();
          if (cancelled) return;
          setCustomers(data.customers);
          setAppointments(data.appointments);
          setReminderDays(data.settings.reminder_days);
          return;
        }

        if (view === 'line') {
          const data = await fetchLinePageData();
          if (cancelled) return;
          setLineFriends(data.line_friends);
          setCustomers(data.customers);
          return;
        }

        if (view === 'zones') {
          const data = await fetchZonePageData();
          if (cancelled) return;
          setZones(data.zones);
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          return;
        }

        if (view === 'settings') {
          const data = await fetchSettingsPageData();
          if (cancelled) return;
          setExtraFeeProducts(data.extra_fee_products);
          setServiceItems(data.service_items);
          setReminderDays(data.settings.reminder_days);
          setWebhookSettings(data.settings.webhook || defaultWebhookSettings);
          return;
        }

        if (view === 'financials') {
          const data = await fetchFinancialReportPageData();
          if (cancelled) return;
          setAppointments(data.appointments);
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          return;
        }

        if (view === 'reviews') {
          const data = await fetchReviewDashboardPageData();
          if (cancelled) return;
          setReviews(data.reviews);
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          setAppointments(data.appointments);
          return;
        }

        if (view === 'cashLedger') {
          const data = await fetchCashLedgerPageData();
          if (cancelled) return;
          setTechnicians(data.technicians);
          mergeTechniciansIntoUsers(data.technicians);
          setAppointments(data.appointments);
          setCashLedgerEntries(data.cash_ledger_entries);
        }
      } catch (error) {
        if (cancelled) {
          return;
        }
        if (error instanceof AuthRequiredError) {
          handleAuthRequired();
          return;
        }
        console.error(error);
        setViewDataError('載入當前頁面資料失敗，已保留上一份資料。');
      } finally {
        if (!cancelled) {
          setViewDataLoading(false);
        }
      }
    };

    void syncViewData();

    return () => {
      cancelled = true;
    };
  }, [selectedLedgerTechId, snapshotError, snapshotLoaded, user?.role, view]);

  // replaceAppointmentInState 统一维护预约列表与当前抽屉选中项，避免不同操作分支重复写同一段状态更新。
  const replaceAppointmentInState = (next: Appointment) => {
    setAppointments(prev => prev.map(appt => appt.id === next.id ? next : appt));
    setSelectedAppt(prev => prev?.id === next.id ? next : prev);
  };

  // createDefaultAppointmentItem 在新建预约清空表单时复用第一个服务项目，避免每个重置分支手写同样逻辑。
  const createDefaultAppointmentItem = (): ACUnit[] => (
    serviceItems.length > 0
      ? [{ id: '1', type: serviceItems[0].name, note: '', price: serviceItems[0].default_price }]
      : []
  );

  // applyCreateFormDraft 用统一状态更新预填新建预约表单，避免通过 DOM 赋值导致 React 状态与画面脱节。
  const applyCreateFormDraft = (draft: CreateFormDraft) => {
    setCreateFormName(draft.customer_name ?? '');
    setCreateFormPhone(draft.phone ?? '');
    setCreateFormAddress(draft.address ?? '');
    setCreateFormLineUid(draft.line_uid ?? '');
    setCreateFormTechId(draft.technician_id ?? null);

    if (draft.scheduled_at) {
      const scheduled = new Date(draft.scheduled_at);
      const nextHour = addMinutes(scheduled, 60);
      setCreateFormDate(format(scheduled, 'yyyy-MM-dd'));
      setCreateFormTimeStart(format(scheduled, 'HH:mm'));
      setCreateFormTimeEnd(format(nextHour, 'HH:mm'));
    } else {
      setCreateFormDate('');
      setCreateFormTimeStart('');
      setCreateFormTimeEnd('');
    }

    // createFormDistrict 仍使用地址文本参与既有区划匹配逻辑；若无地址则清空。
    setCreateFormDistrict(draft.address ?? '');
    setNewApptItems(createDefaultAppointmentItem());
    setNewApptExtraItems([]);
    setNewApptDiscount(0);
  };

  const handleLogin = async (phone: string, password: string) => {
    const { user: loggedInUser } = await loginRequest(phone, password);
    setUser(loggedInUser);
    // 登录成功后总是刷新一次快照，确保会话失效后重登不会继续沿用上一位用户残留的内存数据。
    await refreshAppSnapshot();
    return loggedInUser;
  };

  const handleAssign = async (apptId: number, techId: number) => {
    const tech = technicians.find(t => t.id === techId);
    const updatedAppt = appointments.find(a => a.id === apptId);
    if (!updatedAppt) return;
    const nextAppt = { ...updatedAppt, technician_id: techId, technician_name: tech?.name, status: 'assigned' as const };

    try {
      const saved = await updateAppointment(nextAppt.id, toAppointmentUpdatePayload(nextAppt));
      replaceAppointmentInState(saved);
      await refreshAppSnapshot();
      toast.success('指派成功');
    } catch (err) {
      console.error(err);
      toast.error('指派失敗');
    }
  };

  const handleUpdateAppointment = async (updated: Appointment) => {
    try {
      // 普通编辑链路直接交给 toAppointmentUpdatePayload 判断是否保留 legacy payment_*，
      // 避免 UI 侧提前把 `未收款` 旧资料错误归一成真实付款方式。
      const saved = await updateAppointment(updated.id, toAppointmentUpdatePayload(updated));
      replaceAppointmentInState(saved);
      await refreshAppSnapshot();
      toast.success('資料已更新');
    } catch (err) {
      console.error(err);
      toast.error('資料更新失敗');
    }
  };

  const handleStatusUpdate = async (
    target: Appointment,
    status: Appointment['status'],
    patch: Partial<Appointment> = {},
  ) => {
    try {
      let lat = patch.lat;
      let lng = patch.lng;
      if (status === 'arrived') {
        try {
          const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
            navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 5000 });
          });
          lat = pos.coords.latitude;
          lng = pos.coords.longitude;
        } catch (e) {
          console.warn("GPS failed, proceeding without coordinates");
        }
      }
      // 所有业务关键时间戳（出发/签到/完工/结案时间）由后端服务器自动填充，
      // 不依赖客户端本地时钟，避免时区偏差和手机时间不准的问题。
      const payload: Appointment = {
        ...target,
        ...patch,
        status,
        lat: lat ?? patch.lat ?? target.lat,
        lng: lng ?? patch.lng ?? target.lng,
      };
      const saved = await updateAppointment(target.id, toAppointmentUpdatePayload(payload));
      replaceAppointmentInState(saved);
      await refreshAppSnapshot();
      toast.success(status === 'arrived' ? '簽到成功' : '任務完成');
      if (status === 'completed') setIsDrawerOpen(false);
    } catch (err) {
      console.error(err);
      toast.error('更新失敗');
    }
  };

  const handleCreateAppointment = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const customer_name = createFormName;
    const phone = createFormPhone;
    const address = createFormAddress;
    // 新建表单也必须收敛到可写付款方式，避免未来表单复用读模型选项时把 legacy 占位值再次写回后端。
    const payment_method = getWritablePaymentMethod(
      String(formData.get('payment_method') || '現金') as AppointmentReadablePaymentMethod,
    );
    const line_uid = createFormLineUid || undefined;

    const assignedTech = createFormTechId ? technicians.find(t => t.id === createFormTechId) : null;

    // newAppt 只提交创建预约所需业务字段，總額、狀態、區域等派生字段统一交给后端生成。
    const newAppt: AppointmentCreatePayload = {
      customer_name, address, phone,
      items: [...newApptItems],
      extra_items: newApptExtraItems.map(({ id, name, price }) => ({ id, name, price })),
      payment_method,
      discount_amount: newApptDiscount || 0,
      scheduled_at: createFormScheduledAt,
      scheduled_end: createFormDate && createFormTimeEnd
        ? toISOStringFromLocalInput(createFormDate, createFormTimeEnd)
        : undefined,
      technician_id: assignedTech?.id,
      line_uid,
    };

    try {
      const saved = await createAppointment(newAppt);
      await refreshAppSnapshot();

      if (assignedTech) {
        toast.success(`預約單已建立，已指派給 ${assignedTech.name}`);
      } else if (saved.zone_id) {
        const zoneName = zones.find(z => z.id === saved.zone_id)?.name;
        toast.success(`預約單已建立，自動匹配區域：${zoneName}`);
      } else {
        toast.success('預約單已建立');
      }
      setView('list');
      setNewApptItems(createDefaultAppointmentItem());
      setNewApptExtraItems([]);
      setNewApptDiscount(0);
      setCreateFormName('');
      setCreateFormPhone('');
      setCreateFormAddress('');
      setCreateFormLineUid('');
      setCreateFormDate('');
      setCreateFormTimeStart('');
      setCreateFormTimeEnd('');
      setCreateFormTechId(null);
      setCreateFormDistrict('');
    } catch (err) {
      console.error(err);
      toast.error('建立預約失敗');
    }
  };

  const getAvailableTechs = () => {
    if (!createFormDistrict || !createFormScheduledAt) return [];
    const districtAddr = createFormDistrict;
    const matchedZoneId = matchZoneByAddress(districtAddr, zones);
    const tempAppt: Appointment = {
      id: 0, customer_name: '', address: districtAddr, phone: '',
      items: [...newApptItems], extra_items: [], payment_method: '現金',
      total_amount: 0, scheduled_at: createFormScheduledAt, status: 'pending',
      photos: [], zone_id: matchedZoneId
    };
    return getAutoDispatchSuggestions(tempAppt, technicians, appointments, zones)
      .filter(s => s.totalScore > 0);
  };

  const getConflicts = (techId: number) => {
    if (!createFormScheduledAt || !createFormDate) return [];
    const newStart = parseISO(createFormScheduledAt);
    const newEnd = createFormTimeEnd ? parseISO(`${createFormDate}T${createFormTimeEnd}`) : addMinutes(newStart, 60);
    const bufferStart = subMinutes(newStart, 30);
    const bufferEnd = addMinutes(newEnd, 30);
    return appointments.filter(a => {
      if (a.technician_id !== techId) return false;
      const apptStart = parseISO(a.scheduled_at);
      const apptEnd = a.scheduled_end ? parseISO(a.scheduled_end) : addMinutes(apptStart, 60);
      return apptStart < bufferEnd && apptEnd > bufferStart;
    });
  };

  const exportCSV = (data: Appointment[], filename: string) => {
    const statusMap: Record<string, string> = { pending: '待指派', assigned: '已分派', arrived: '清洗中', completed: '已完成', cancelled: '已取消' };
    const header = '客戶姓名,電話,地址,預約日期,開始時間,結束時間,清洗內容,師傅,金額,收款方式,狀態';
    const rows = data.map(a => {
      const date = a.scheduled_at ? format(parseISO(a.scheduled_at), 'yyyy-MM-dd') : '';
      const startTime = a.scheduled_at ? format(parseISO(a.scheduled_at), 'HH:mm') : '';
      const endTime = a.scheduled_end ? format(parseISO(a.scheduled_end), 'HH:mm') : '';
      const items = a.items.map(i => i.type).join('+');
      const tech = a.technician_name || '';
      const status = statusMap[a.status] || a.status;
      // 匯出報表也沿用统一付款方式展示文案，避免 CSV 再把历史 `未收款` 旧值冒充成真实付款方式。
      return [a.customer_name, a.phone, a.address, date, startTime, endTime, items, tech, a.total_amount, getPaymentMethodLabel(a), status]
        .map(v => `"${String(v).replace(/"/g, '""')}"`).join(',');
    });
    const bom = '\uFEFF';
    const csv = bom + header + '\n' + rows.join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
    toast.success('匯出成功');
  };

  // handleReviewSubmit 返回后端保存结果，供公开评价页回填真实主键与分享状态。
  const handleReviewSubmit = async (reviewToken: string, review: ReviewDraft): Promise<Review> => {
    try {
      const saved = await createReview(reviewToken, review);
      setReviews(prev => {
        const existingIndex = prev.findIndex(item => item.appointment_id === saved.appointment_id);
        if (existingIndex === -1) {
          return [saved, ...prev];
        }
        return prev.map(item => item.appointment_id === saved.appointment_id ? saved : item);
      });
      await refreshAppSnapshot();
      return saved;
    } catch (err) {
      console.error(err);
      toast.error('送出評價失敗');
      throw err;
    }
  };

  const syncTechnicians = async (next: User[]) => {
    try {
      const saved = await replaceTechnicians(next);
      setTechnicians(saved);
      setAllUsers(prev => [...prev.filter(item => item.role !== 'technician'), ...saved].sort((a, b) => a.id - b.id));
      await refreshAppSnapshot();
    } catch (err) {
      console.error(err);
      toast.error('同步師傅資料失敗');
    }
  };

  const syncZones = async (next: ServiceZone[]) => {
    try {
      const saved = await replaceZones(next);
      setZones(saved);
      await refreshAppSnapshot();
    } catch (err) {
      console.error(err);
      toast.error('同步區域資料失敗');
    }
  };

  const syncExtraFeeProducts = async (next: ExtraItem[]) => {
    // 設定頁輸入框屬於受控元件；先即時回寫本地狀態，避免每次鍵入都卡在 API 往返上，
    // 導致新增項目看起來「無法輸入」或輸入中的文字被舊值覆蓋。
    setExtraFeeProducts(next);
    try {
      const saved = await replaceExtraItems(next);
      setExtraFeeProducts(saved);
    } catch (err) {
      console.error(err);
      await refreshAppSnapshot();
      toast.error('同步額外費用失敗');
    }
  };

  const syncServiceItems = async (next: ServiceItem[]) => {
    // 服務項目設定同樣採用受控輸入；必須先更新前端狀態，才能讓管理員在新增項目後立即打字。
    setServiceItems(next);
    // 新增預約表單若還沒有任何服務項目，沿用設定頁最新草稿作為預設值，避免畫面仍停留在舊項目。
    setNewApptItems(prev => prev.length > 0 ? prev : (next.length > 0 ? [{
      id: '1',
      type: next[0].name,
      note: '',
      price: next[0].default_price,
    }] : []));
    try {
      const saved = await replaceServiceItems(next);
      setServiceItems(saved);
      setNewApptItems(prev => prev.length > 0 ? prev : (saved.length > 0 ? [{
        id: '1',
        type: saved[0].name,
        note: '',
        price: saved[0].default_price,
      }] : []));
    } catch (err) {
      console.error(err);
      await refreshAppSnapshot();
      toast.error('同步服務項目失敗');
    }
  };

  const syncReminderDays = async (next: number) => {
    setReminderDays(next);
    try {
      await updateReminderDays(next);
      await refreshAppSnapshot();
    } catch (err) {
      console.error(err);
      toast.error('同步回訪提醒設定失敗');
    }
  };

  // syncWebhookEnabled 只切换管理员持久化开关；实际是否生效仍由后端按 secret 与公网 URL 依赖判定。
  const syncWebhookEnabled = async (enabled: boolean) => {
    setWebhookSettings(prev => ({ ...prev, enabled }));
    try {
      const saved = await updateWebhookEnabledRequest(enabled);
      setWebhookSettings(saved);
      await refreshAppSnapshot();
      toast.success(enabled ? 'Webhook 開關已啟用' : 'Webhook 開關已停用');
    } catch (err) {
      console.error(err);
      toast.error('同步 webhook 設定失敗');
      await refreshAppSnapshot();
    }
  };

  // syncCustomers 将客户列表变更同步到后端，与 syncTechnicians/syncZones 保持一致。
  const syncCustomers = async (next: Customer[]) => {
    try {
      const saved = await replaceCustomers(next);
      setCustomers(saved);
      await refreshAppSnapshot();
    } catch (err) {
      console.error(err);
      toast.error('同步顧客資料失敗');
    }
  };

  // createCashLedgerEntry 只接收现金账写模型，请求成功后再用后端返回的真实记录刷新页面。
  const createCashLedgerEntry = async (entry: CashLedgerCreatePayload) => {
    try {
      const saved = await createCashLedgerEntryRequest(entry);
      setCashLedgerEntries(prev => [...prev, saved]);
      await refreshAppSnapshot();
    } catch (err) {
      console.error(err);
      toast.error(CASH_LEDGER_RETURN_FAILURE_MESSAGE);
    }
  };

  // createNotificationLog 返回后端写入结果，供通知组件在需要时使用真实 sent_at / id。
  const createNotificationLog = async (log: NotificationLogDraft): Promise<NotificationLog> => {
    try {
      const saved = await createNotificationLogRequest(log);
      setNotificationLogs(prev => [saved, ...prev]);
      await refreshAppSnapshot();
      toast.success(`通知已發送 (${saved.type === 'line' ? 'LINE' : '簡訊'})`);
      return saved;
    } catch (err) {
      console.error(err);
      toast.error('發送通知失敗');
      throw err;
    }
  };

  // handleLinkLineFriend 统一处理 LINE 好友与客户的绑定/解绑，并同步刷新客户与好友状态。
  const handleLinkLineFriend = async (lineUid: string, customerId: string | null) => {
    try {
      const saved = await linkLineFriendCustomer(lineUid, customerId);
      setLineFriends(prev => prev.map(item => item.line_uid === saved.line_uid ? { ...item, ...saved } : item));
      await refreshAppSnapshot();
      toast.success(customerId ? '已綁定顧客' : '已解除綁定');
    } catch (error) {
      console.error(error);
      toast.error(customerId ? '綁定顧客失敗' : '解除綁定失敗');
      throw error;
    }
  };

  if (!snapshotLoaded) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg border border-slate-200 p-8 text-center space-y-3">
          <div className="w-12 h-12 rounded-full border-4 border-slate-200 border-t-blue-600 animate-spin mx-auto" />
          <p className="text-sm text-slate-500">正在載入後端資料...</p>
        </div>
      </div>
    );
  }

  if (snapshotError) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white rounded-lg border border-red-100 p-8 text-center space-y-4 max-w-md">
          <div className="w-14 h-14 rounded-full bg-red-50 flex items-center justify-center mx-auto">
            <AlertTriangle className="w-7 h-7 text-red-500" />
          </div>
          <p className="text-sm text-slate-600">{snapshotError}</p>
        </div>
      </div>
    );
  }

  if (typeof window !== 'undefined' && window.location.pathname.startsWith('/review/')) {
    return (
      <>
        <Toaster position="top-center" />
        <Switch>
          <Route path="/review/:reviewToken">
            <ReviewPage onSubmit={handleReviewSubmit} />
          </Route>
        </Switch>
      </>
    );
  }

  // 支付页面：客户凭 PaymentToken 无需登录直接访问支付页面。
  if (typeof window !== 'undefined' && window.location.pathname.startsWith('/pay/')) {
    return (
      <>
        <Toaster position="top-center" />
        <Switch>
          <Route path="/pay/:payToken">
            <PaymentPage />
          </Route>
        </Switch>
      </>
    );
  }

  if (!user) {
    return <LoginPage onLogin={handleLogin} />;
  }

  const filteredAppointments = (user.role === 'admin' 
    ? appointments 
    : appointments.filter(appt => appt.technician_id === user.id))
    .filter(appt => {
      const matchesStatus = statusFilter === 'all' ? true : appt.status === statusFilter;
      const matchesTech = techFilter === 'all' ? true : appt.technician_id === techFilter;
      const matchesAcType = acTypeFilter === 'all' ? true : appt.items.some(item => item.type === acTypeFilter);
      const matchesSearch = 
        appt.customer_name.includes(searchQuery) || 
        appt.phone.includes(searchQuery) || 
        appt.address.includes(searchQuery);
      const apptDate = appt.scheduled_at.split('T')[0];
      const matchesDate = 
        (!dateRange.start || apptDate >= dateRange.start) && 
        (!dateRange.end || apptDate <= dateRange.end);
      return matchesStatus && matchesTech && matchesAcType && matchesSearch && matchesDate;
    });

  const headerTitle: Record<ViewType, string> = {
    dashboard: '首頁總覽',
    list: '任務清單', create: '新增預約單', technicians: '師傅管理',
    customers: '顧客管理', line: 'LINE 紀錄', settings: '系統設定', financials: '財務報表',
    reminders: '回訪提醒',
    cashLedger: CASH_LEDGER_TITLE,
    schedule: '排程表',
    zones: '區域管理',
    heatmap: '熱區地圖',
    reviews: '客戶評價',
    payments: '支付管理'
  };

  return (
    <div className="min-h-screen bg-slate-50 pb-24 md:pb-0 md:pl-56">
      <Toaster position="top-center" />

      {/* 自定义登出确认弹窗，替代原生 window.confirm */}
      <AnimatePresence>
        {showLogoutConfirm && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[100] flex items-center justify-center bg-black/40 backdrop-blur-sm"
            onClick={() => setShowLogoutConfirm(false)}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9, y: 20 }}
              transition={{ type: 'spring', damping: 25, stiffness: 300 }}
              className="bg-white rounded-2xl shadow-2xl p-6 w-[340px] space-y-5"
              onClick={e => e.stopPropagation()}
            >
              <div className="text-center space-y-2">
                <div className="w-14 h-14 rounded-full bg-red-50 flex items-center justify-center mx-auto">
                  <LogOut className="w-7 h-7 text-red-500" />
                </div>
                <h3 className="text-lg font-bold text-slate-900">確定要登出嗎？</h3>
                <p className="text-sm text-slate-500">登出後需要重新輸入帳號密碼登入。</p>
              </div>
              <div className="flex gap-3">
                <button
                  onClick={() => setShowLogoutConfirm(false)}
                  className="flex-1 px-4 py-2.5 rounded-xl text-sm font-medium bg-slate-100 text-slate-700 hover:bg-slate-200 transition-colors"
                  data-testid="button-logout-cancel"
                >
                  取消
                </button>
                <button
                  onClick={() => {
                    setShowLogoutConfirm(false);
                    logoutRequest().catch(() => {});
                    setUser(null);
                    setView('dashboard');
                  }}
                  className="flex-1 px-4 py-2.5 rounded-xl text-sm font-medium bg-red-500 text-white hover:bg-red-600 transition-colors"
                  data-testid="button-logout-confirm"
                >
                  確定登出
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
      
      {user.role === 'technician' ? (
        <TechnicianDashboard 
          user={user} 
          appointments={appointments} 
          onStatusUpdate={handleStatusUpdate}
          onUpdateAppointment={handleUpdateAppointment}
          onLogout={() => setShowLogoutConfirm(true)}
        />
      ) : (
        <>
          <nav className="fixed bottom-0 left-0 right-0 bg-white border-t border-slate-200 px-4 py-2 flex justify-around items-center z-50 md:top-0 md:bottom-0 md:left-0 md:w-56 md:flex-col md:justify-start md:py-5 md:px-3 md:border-r md:border-t-0 md:shadow-sm">
            <div className="hidden md:flex items-center gap-2.5 mb-8 px-3">
              <div className="w-8 h-8 bg-blue-600 rounded-md flex items-center justify-center">
                <Package className="text-white w-4.5 h-4.5" />
              </div>
              <span className="font-semibold text-base text-slate-800">CoolDispatch</span>
            </div>

            <div className="flex md:flex-col gap-0.5 w-full">
              {([
                { key: 'dashboard' as ViewType, icon: LayoutDashboard, label: '首頁總覽' },
                { key: 'list' as ViewType, icon: ClipboardList, label: '任務清單' },
                { key: 'create' as ViewType, icon: Plus, label: '新增預約' },
                { key: 'schedule' as ViewType, icon: CalendarDays, label: '排程表' },
                { key: 'technicians' as ViewType, icon: UserIcon, label: '師傅管理' },
                { key: 'customers' as ViewType, icon: Users, label: '顧客管理' },
                { key: 'line' as ViewType, icon: MessageSquare, label: 'LINE 紀錄' },
                { key: 'zones' as ViewType, icon: MapPin, label: '區域管理' },
                { key: 'reminders' as ViewType, icon: Clock, label: '回訪提醒' },
                { key: 'settings' as ViewType, icon: Package, label: '系統設定' },
                { key: 'financials' as ViewType, icon: DollarSign, label: '財務報表' },
                { key: 'heatmap' as ViewType, icon: Map, label: '熱區地圖' },
                { key: 'reviews' as ViewType, icon: Star, label: '客戶評價' },
                { key: 'payments' as ViewType, icon: CreditCard, label: '支付管理' },
              ]).map(item => (
                <button
                  key={item.key}
                  onClick={() => setView(item.key)}
                  data-testid={`nav-${item.key}`}
                  className={cn(
                    "flex flex-col md:flex-row items-center gap-1 md:gap-2.5 px-3 py-2 rounded transition-all w-full text-left text-sm",
                    view === item.key 
                      ? "text-blue-600 md:bg-blue-50 font-medium" 
                      : "text-slate-500 hover:text-slate-700 hover:bg-slate-50"
                  )}
                >
                  <item.icon className="w-5 h-5" />
                  <span className="text-[10px] md:text-[13px]">{item.label}</span>
                </button>
              ))}

              <button 
                onClick={() => setShowLogoutConfirm(true)}
                data-testid="button-logout"
                className="flex flex-col md:flex-row items-center gap-1 md:gap-2.5 px-3 py-2 rounded text-red-400 hover:text-red-500 hover:bg-red-50 transition-all w-full md:mt-auto text-left"
              >
                <LogOut className="w-5 h-5" />
                <span className="text-[10px] md:text-[13px]">登出</span>
              </button>
            </div>
          </nav>

          <header className="bg-white border-b border-slate-100 px-6 py-4 flex items-center justify-between md:px-10 md:py-6">
            <div>
              <h2 className="text-xl font-bold text-slate-900" data-testid="text-header-title">{headerTitle[view]}</h2>
              <p className="text-sm text-slate-500">歡迎回來, {user.name} ({user.role === 'admin' ? '管理員' : '師傅'})</p>
            </div>
            <div className="w-10 h-10 bg-slate-100 rounded-full flex items-center justify-center">
              <UserIcon className="w-5 h-5 text-slate-600" />
            </div>
          </header>

          <main className="p-6 md:p-10 max-w-6xl mx-auto">
            {viewDataLoading && (
              <div className="mb-4 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm text-slate-500">
                正在同步當前頁面的後端資料...
              </div>
            )}
            {viewDataError && (
              <div className="mb-4 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">
                {viewDataError}
              </div>
            )}
            <AnimatePresence mode="wait">
              {view === 'dashboard' && (
                <motion.div key="dashboard" initial={{ opacity: 0, x: -20 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 20 }}>
                  <DashboardView
                    appointments={appointments}
                    technicians={technicians}
                    customers={customers}
                    reviews={reviews}
                  />
                </motion.div>
              )}
              {view === 'list' && (
                <motion.div key="list" initial={{ opacity: 0, x: -20 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 20 }} className="space-y-8">
                  {user.role === 'admin' && (
                    <div className="grid grid-cols-2 md:grid-cols-5 gap-4">
                      <Card className="p-4 bg-blue-50 border-blue-100/50">
                        <p className="text-[10px] font-bold text-blue-400 uppercase tracking-wider mb-1">今日預約</p>
                        <p className="text-2xl font-bold text-blue-900" data-testid="text-today-appts">{appointments.filter(a => a.scheduled_at.startsWith(new Date().toISOString().split('T')[0])).length}</p>
                      </Card>
                      <Card className="p-4 bg-amber-50 border-amber-100/50">
                        <p className="text-[10px] font-bold text-amber-400 uppercase tracking-wider mb-1">待處理</p>
                        <p className="text-2xl font-bold text-amber-900" data-testid="text-pending-count">{appointments.filter(a => a.status === 'pending').length}</p>
                      </Card>
                      <Card className="p-4 bg-emerald-50 border-emerald-100/50">
                        <p className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider mb-1">已完成 (本月)</p>
                        <p className="text-2xl font-bold text-emerald-900">{currentMonthFinishedAppointments.length}</p>
                        <p className="text-[10px] text-emerald-600 font-medium mt-1">
                          實收: ${currentMonthCollectedAppointments.reduce((sum, a) => sum + getAppointmentCollectedAmount(a), 0).toLocaleString()}
                        </p>
                      </Card>
                      <Card className="p-4 bg-rose-50 border-rose-100/50 cursor-pointer" onClick={() => setView('reminders')} data-testid="card-reminder-stats">
                        <p className="text-[10px] font-bold text-rose-400 uppercase tracking-wider mb-1">待回訪客戶</p>
                        <p className="text-2xl font-bold text-rose-900" data-testid="text-reminder-count">{(() => {
                          const today = new Date();
                          return customers.filter(c => {
                            const completedAppts = appointments.filter(a => a.status === 'completed' && (a.phone === c.phone || a.customer_name === c.name));
                            if (completedAppts.length === 0) return false;
                            const sorted = completedAppts.sort((a, b) => new Date(b.checkout_time || b.scheduled_at).getTime() - new Date(a.checkout_time || a.scheduled_at).getTime());
                            const lastDate = sorted[0].checkout_time || sorted[0].scheduled_at;
                            const daysSince = Math.floor((today.getTime() - new Date(lastDate).getTime()) / (1000 * 60 * 60 * 24));
                            return daysSince >= reminderDays;
                          }).length;
                        })()}</p>
                      </Card>
                      <Card className="p-4 bg-blue-700 border-blue-600">
                        <p className="text-[10px] font-bold text-blue-200 uppercase tracking-wider mb-1">應收總額 (本月)</p>
                        <p className="text-2xl font-bold text-white">
                          ${currentMonthFinishedAppointments.reduce((sum, a) => sum + getChargeableAmount(a), 0).toLocaleString()}
                        </p>
                        <p className="text-[10px] text-blue-300 font-medium mt-1">
                          未收餘額: ${currentMonthFinishedAppointments.reduce((sum, a) => sum + getOutstandingAmount(a), 0).toLocaleString()}
                        </p>
                      </Card>
                    </div>
                  )}

                  <div className="flex flex-col gap-4">
                    <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
                      {(['all', 'pending', 'assigned', 'arrived', 'completed'] as const).map((s) => (
                        <button
                          key={s}
                          onClick={() => setStatusFilter(s)}
                          data-testid={`filter-status-${s}`}
                          className={cn(
                            "px-4 py-2 rounded-full text-sm font-medium transition-all whitespace-nowrap",
                            statusFilter === s 
                              ? "bg-blue-600 text-white shadow-sm" 
                              : "bg-white text-slate-500 border border-slate-200 hover:border-blue-300 hover:text-blue-600"
                          )}
                        >
                          {s === 'all' ? '全部' : s === 'pending' ? '待指派' : s === 'assigned' ? '已分派' : s === 'arrived' ? '清洗中' : '已完成'}
                        </button>
                      ))}
                    </div>

                    <div className="flex flex-col md:flex-row gap-3">
                      <div className="flex-1 relative">
                        <input 
                          data-testid="input-search"
                          type="text" 
                          placeholder="搜尋客戶姓名、電話或地址..." 
                          className="w-full pl-10 pr-4 py-2.5 bg-white border border-slate-100 rounded-md text-sm focus:outline-none focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all"
                          value={searchQuery}
                          onChange={e => setSearchQuery(e.target.value)}
                        />
                        <ClipboardList className="w-4 h-4 text-slate-300 absolute left-3.5 top-1/2 -translate-y-1/2" />
                      </div>
                      <div className="flex flex-wrap gap-2 items-center">
                        <select 
                          data-testid="select-tech-filter"
                          className="px-3 py-2.5 bg-white border border-slate-100 rounded-md text-sm focus:outline-none"
                          value={techFilter}
                          onChange={e => setTechFilter(e.target.value === 'all' ? 'all' : parseInt(e.target.value))}
                        >
                          <option value="all">所有師傅</option>
                          {technicians.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
                        </select>
                        <select 
                          data-testid="select-type-filter"
                          className="px-3 py-2.5 bg-white border border-slate-100 rounded-md text-sm focus:outline-none"
                          value={acTypeFilter}
                          onChange={e => setAcTypeFilter(e.target.value as ACType | 'all')}
                        >
                          <option value="all">所有種類</option>
                          {serviceItems.map(si => (
                            <option key={si.id} value={si.name}>{si.name}</option>
                          ))}
                        </select>
                        <div className="flex gap-1 items-center bg-white border border-slate-100 rounded-md px-2">
                          <input type="date" className="px-2 py-2 text-sm focus:outline-none bg-transparent" value={dateRange.start} onChange={e => setDateRange({ ...dateRange, start: e.target.value })} />
                          <span className="text-slate-300">~</span>
                          <input type="date" className="px-2 py-2 text-sm focus:outline-none bg-transparent" value={dateRange.end} onChange={e => setDateRange({ ...dateRange, end: e.target.value })} />
                        </div>
                        <Button 
                          variant="outline" 
                          className="px-3 py-2.5 rounded-md text-xs"
                          data-testid="button-reset-filters"
                          onClick={() => {
                            setSearchQuery('');
                            setTechFilter('all');
                            setAcTypeFilter('all');
                            setDateRange({ start: '', end: '' });
                            setStatusFilter('all');
                          }}
                        >
                          重設
                        </Button>
                        <Button
                          variant="outline"
                          className="px-3 py-2.5 rounded-md text-xs"
                          data-testid="button-export-csv"
                          onClick={() => exportCSV(filteredAppointments, `預約清單_${format(new Date(), 'yyyyMMdd')}.csv`)}
                        >
                          <Download className="w-3.5 h-3.5 mr-1" /> 匯出
                        </Button>
                      </div>
                    </div>
                  </div>

                  <Card className="border-none shadow-none bg-transparent">
                    <div className="space-y-2 mt-2">
                      {filteredAppointments.length === 0 ? (
                        <div className="text-center py-20 bg-white rounded-lg border border-slate-100">
                          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
                            <ClipboardList className="text-slate-300 w-8 h-8" />
                          </div>
                          <p className="text-slate-500">目前沒有符合條件的預約單</p>
                        </div>
                      ) : (
                        <div className="overflow-x-auto">
                          <table className="w-full text-sm text-left text-slate-600">
                            <thead className="text-xs text-slate-700 uppercase bg-slate-50">
                              <tr>
                                <th className="px-4 py-3">姓名</th>
                                <th className="px-4 py-3">行動電話</th>
                                <th className="px-4 py-3">施工地址</th>
                                <th className="px-4 py-3">預約時間</th>
                                <th className="px-4 py-3">清洗內容</th>
                                <th className="px-4 py-3">付款方式</th>
                                <th className="px-4 py-3">狀態</th>
                              </tr>
                            </thead>
                            <tbody>
                              {filteredAppointments.map(appt => {
                                const isLate = appt.status !== 'completed' && isAfter(new Date(), parseISO(appt.scheduled_at));
                                return (
                                  <tr 
                                    key={appt.id} 
                                    onClick={() => { setSelectedAppt(appt); setIsDrawerOpen(true); setIsEditing(false); }}
                                    className="bg-white border-b hover:bg-slate-50 cursor-pointer"
                                    data-testid={`row-appointment-${appt.id}`}
                                  >
                                    <td className="px-4 py-3 font-medium text-slate-900">{appt.customer_name}</td>
                                    <td className="px-4 py-3">{appt.phone}</td>
                                    <td className="px-4 py-3">{appt.address}</td>
                                    <td className={cn("px-4 py-3", isLate ? "text-red-500 font-medium" : "")}>
                                      {format(parseISO(appt.scheduled_at), 'MM/dd HH:mm')}
                                    </td>
                                    <td className="px-4 py-3">{appt.items.length} 台</td>
                                    <td className="px-4 py-3">{getPaymentMethodLabel(appt)}</td>
                                    <td className="px-4 py-3"><Badge status={appt.status} /></td>
                                  </tr>
                                );
                              })}
                            </tbody>
                          </table>
                        </div>
                      )}
                    </div>
                  </Card>
                </motion.div>
              )}

              {view === 'create' && user.role === 'admin' && (
                <motion.div key="create" initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -20 }}>
                  <Card className="p-8">
                    <form className="space-y-8" onSubmit={handleCreateAppointment} data-testid="form-create-appointment">
                      <div className="space-y-6">
                        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">基本資訊</h4>
                        <div>
                          <label className="block text-sm font-medium text-slate-700 mb-1">LINE 好友</label>
                          <LineFriendPicker
                            lineFriends={lineFriends}
                            selectedUid={createFormLineUid}
                            onSelect={(f) => {
                              if (f) {
                                setCreateFormLineUid(f.line_uid);
                              } else {
                                setCreateFormLineUid('');
                              }
                            }}
                          />
                          <p className="text-[10px] text-slate-400 mt-1">選填 - 可關聯 LINE 好友到此預約單</p>
                        </div>
                        <div className="grid md:grid-cols-2 gap-6">
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">客戶姓名</label>
                            <input data-testid="input-create-name" name="customer_name" required className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500" value={createFormName} onChange={e => setCreateFormName(e.target.value)} />
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">聯繫電話</label>
                            <input data-testid="input-create-phone" name="phone" required className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500" value={createFormPhone} onChange={e => setCreateFormPhone(e.target.value)} />
                          </div>
                        </div>
                        <div className="grid md:grid-cols-3 gap-4">
                          <div className="md:col-span-2">
                            <label className="block text-sm font-medium text-slate-700 mb-1">施工地址</label>
                            <div className="relative">
                              <MapPin className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
                              <input
                                data-testid="input-create-address"
                                name="address"
                                required
                                className="w-full pl-10 pr-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                                value={createFormAddress}
                                onChange={e => setCreateFormAddress(e.target.value)}
                                placeholder="輸入完整地址..."
                              />
                            </div>
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">縣市區域</label>
                            <select
                              data-testid="select-create-district"
                              className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                              value={createFormDistrict}
                              onChange={e => { setCreateFormDistrict(e.target.value); setCreateFormTechId(null); }}
                            >
                              <option value="">請選擇</option>
                              <optgroup label="台北市">
                                {TAIPEI_DISTRICTS.map(d => (
                                  <option key={d} value={d}>{d}</option>
                                ))}
                              </optgroup>
                              <optgroup label="新北市">
                                {NEW_TAIPEI_DISTRICTS.map(d => (
                                  <option key={d} value={d}>{d}</option>
                                ))}
                              </optgroup>
                            </select>
                          </div>
                        </div>
                        <div className="grid md:grid-cols-4 gap-4">
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">預約日期</label>
                            <input
                              data-testid="input-create-date"
                              type="date"
                              required
                              className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                              value={createFormDate}
                              onChange={e => { setCreateFormDate(e.target.value); setCreateFormTechId(null); }}
                            />
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">開始時間</label>
                            <select
                              data-testid="select-create-time-start"
                              required
                              className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                              value={createFormTimeStart}
                              onChange={e => {
                                const val = e.target.value;
                                setCreateFormTimeStart(val);
                                setCreateFormTechId(null);
                                if (val && (!createFormTimeEnd || createFormTimeEnd <= val)) {
                                  const idx = Array.from({ length: 21 }, (_, i) => {
                                    const h = Math.floor(i / 2) + 8;
                                    const m = i % 2 === 0 ? '00' : '30';
                                    return `${String(h).padStart(2, '0')}:${m}`;
                                  }).indexOf(val);
                                  if (idx >= 0 && idx < 20) {
                                    const nextH = Math.floor((idx + 1) / 2) + 8;
                                    const nextM = (idx + 1) % 2 === 0 ? '00' : '30';
                                    setCreateFormTimeEnd(`${String(nextH).padStart(2, '0')}:${nextM}`);
                                  }
                                }
                              }}
                            >
                              <option value="">開始</option>
                              {Array.from({ length: 21 }, (_, i) => {
                                const hour = Math.floor(i / 2) + 8;
                                const min = i % 2 === 0 ? '00' : '30';
                                const val = `${String(hour).padStart(2, '0')}:${min}`;
                                return <option key={val} value={val}>{val}</option>;
                              })}
                            </select>
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">結束時間</label>
                            <select
                              data-testid="select-create-time-end"
                              required
                              className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                              value={createFormTimeEnd}
                              onChange={e => { setCreateFormTimeEnd(e.target.value); setCreateFormTechId(null); }}
                            >
                              <option value="">結束</option>
                              {Array.from({ length: 21 }, (_, i) => {
                                const hour = Math.floor(i / 2) + 8;
                                const min = i % 2 === 0 ? '00' : '30';
                                const val = `${String(hour).padStart(2, '0')}:${min}`;
                                return <option key={val} value={val} disabled={!!createFormTimeStart && val <= createFormTimeStart}>{val}</option>;
                              })}
                            </select>
                          </div>
                          <div>
                            <label className="block text-sm font-medium text-slate-700 mb-1">收款方式</label>
                            <select data-testid="select-create-payment" name="payment_method" className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500">
                              <option value="現金">現金</option>
                              <option value="轉帳">轉帳</option>
                              <option value="無收款">無收款</option>
                            </select>
                          </div>
                        </div>
                        {createFormDistrict && createFormScheduledAt && (() => {
                          const available = getAvailableTechs();
                          const matchedZone = matchZoneByAddress(createFormDistrict, zones);
                          const zoneName = matchedZone ? zones.find(z => z.id === matchedZone)?.name : null;
                          return (
                            <div className="space-y-3">
                              <div className="flex items-center justify-between">
                                <label className="block text-sm font-medium text-slate-700">指派師傅</label>
                                {zoneName && (
                                  <span className="text-[10px] text-blue-600 bg-blue-50 px-2 py-0.5 rounded-md flex items-center gap-1" data-testid="text-matched-zone">
                                    <MapPin className="w-3 h-3" /> {zoneName}
                                  </span>
                                )}
                              </div>
                              {available.length > 0 ? (
                                <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
                                  {available.map(s => {
                                    const isSelected = createFormTechId === s.technician.id;
                                    const conflicts = getConflicts(s.technician.id);
                                    const hasConflict = conflicts.length > 0;
                                    return (
                                      <button
                                        key={s.technician.id}
                                        type="button"
                                        onClick={() => setCreateFormTechId(isSelected ? null : s.technician.id)}
                                        className={cn(
                                          "p-3 rounded-md border text-left transition-all flex items-center gap-2.5 relative",
                                          isSelected
                                            ? "bg-blue-50 border-blue-300 ring-1 ring-blue-300"
                                            : hasConflict
                                              ? "bg-amber-50/50 border-amber-200 hover:border-amber-300"
                                              : "bg-white border-slate-200 hover:border-slate-300 hover:shadow-sm"
                                        )}
                                        data-testid={`button-assign-tech-${s.technician.id}`}
                                      >
                                        {hasConflict && <AlertTriangle className="w-4 h-4 text-amber-500 absolute top-1.5 right-1.5" />}
                                        <div
                                          className="w-9 h-9 rounded-md flex items-center justify-center font-bold text-white text-xs flex-shrink-0"
                                          style={{ backgroundColor: s.technician.color }}
                                        >
                                          {s.technician.name[0]}
                                        </div>
                                        <div className="flex-1 min-w-0">
                                          <div className="text-sm font-bold truncate">{s.technician.name}</div>
                                          <div className="flex items-center gap-1 mt-0.5 flex-wrap">
                                            {s.reasons.zoneMatch && <span className="text-[9px] bg-emerald-50 text-emerald-600 px-1 py-0.5 rounded">區域</span>}
                                            {s.reasons.timeAvailable && <span className="text-[9px] bg-blue-50 text-blue-600 px-1 py-0.5 rounded">有空</span>}
                                            {s.reasons.skillMatch && <span className="text-[9px] bg-violet-50 text-violet-600 px-1 py-0.5 rounded">技能</span>}
                                            {s.reasons.loadBalance === 0 && <span className="text-[9px] bg-amber-50 text-amber-600 px-1 py-0.5 rounded">無排程</span>}
                                          </div>
                                        </div>
                                        {isSelected && <CheckCircle2 className="w-5 h-5 text-blue-600 flex-shrink-0" />}
                                      </button>
                                    );
                                  })}
                                </div>
                              ) : (
                                <div className="text-sm text-slate-400 bg-slate-50 rounded-md p-4 text-center">
                                  此時段無可用師傅
                                </div>
                              )}
                              {createFormTechId && (() => {
                                const conflicts = getConflicts(createFormTechId);
                                if (conflicts.length === 0) return null;
                                return (
                                  <div className="bg-amber-50 border border-amber-200 rounded-md p-3 flex items-start gap-2" data-testid="warning-conflict">
                                    <AlertTriangle className="w-4 h-4 text-amber-500 mt-0.5 flex-shrink-0" />
                                    <div className="text-xs text-amber-700 space-y-1">
                                      {conflicts.map(c => (
                                        <div key={c.id}>⚠ 此師傅在 {format(parseISO(c.scheduled_at), 'HH:mm')} 有另一筆預約（{c.customer_name}），間隔不足 30 分鐘交通時間</div>
                                      ))}
                                    </div>
                                  </div>
                                );
                              })()}
                              <p className="text-[10px] text-slate-400">選填 - 不選擇則建立為待指派狀態</p>
                            </div>
                          );
                        })()}
                      </div>

                      <div className="space-y-6">
                        <div className="space-y-4">
                          <div className="flex justify-between items-center">
                            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">額外費用</h4>
                          </div>
                          <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
                            {extraFeeProducts.map(p => (
                              <Button key={p.id} type="button" variant="outline" className="text-xs py-1 px-3 whitespace-nowrap" onClick={() => {
                                setNewApptExtraItems(prev => [...prev, { ...p, id: Date.now().toString() }]);
                              }}>
                                + {p.name} (${p.price})
                              </Button>
                            ))}
                          </div>
                          {newApptExtraItems.length > 0 && (
                            <div className="space-y-2">
                              {newApptExtraItems.map((item, idx) => (
                                <div key={item.id} className="flex flex-col md:flex-row md:items-center justify-between bg-slate-50 p-3 rounded-md border border-slate-100 gap-3">
                                  <span className="text-sm text-slate-600 font-medium">{item.name}</span>
                                  <div className="flex items-center gap-3">
                                    <div className="relative">
                                      <span className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 text-xs">$</span>
                                      <input 
                                        type="number" value={item.price}
                                        onChange={e => {
                                          const newPrice = parseInt(e.target.value) || 0;
                                          setNewApptExtraItems(prev => prev.map((it, i) => i === idx ? { ...it, price: newPrice } : it));
                                        }}
                                        className="w-24 pl-6 pr-3 py-1.5 rounded-lg border-none text-sm font-bold text-slate-900 focus:outline-none focus:ring-1 focus:ring-blue-500"
                                      />
                                    </div>
                                    <button type="button" onClick={() => setNewApptExtraItems(prev => prev.filter((_, i) => i !== idx))} className="text-slate-400 hover:text-red-500 transition-colors p-1">
                                      <X className="w-4 h-4" />
                                    </button>
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>

                        <div className="flex justify-between items-center">
                          <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">清洗內容</h4>
                          <Button type="button" variant="outline" className="text-xs py-1 px-3"
                            onClick={() => {
                              const first = serviceItems[0];
                              setNewApptItems([...newApptItems, { id: Date.now().toString(), type: first?.name || '', note: '', price: first?.default_price || 0 }]);
                            }}
                          >
                            + 新增項目
                          </Button>
                        </div>
                        <div className="space-y-4">
                          {newApptItems.map((item, idx) => (
                            <div key={item.id} className="bg-slate-50 rounded-lg p-4 space-y-4 relative">
                              <div className="grid md:grid-cols-3 gap-4">
                                <div>
                                  <label className="block text-[10px] font-bold text-slate-400 mb-1">種類</label>
                                  <select 
                                    value={item.type}
                                    onChange={e => { 
                                      const typeName = e.target.value; 
                                      const si = serviceItems.find(s => s.name === typeName);
                                      setNewApptItems(newApptItems.map(i => i.id === item.id ? { ...i, type: typeName, price: si?.default_price || i.price } : i)); 
                                    }}
                                    className="w-full px-3 py-2 rounded-lg border-none text-sm"
                                  >
                                    {serviceItems.map(si => (
                                      <option key={si.id} value={si.name}>{si.name}</option>
                                    ))}
                                  </select>
                                </div>
                                <div>
                                  <label className="block text-[10px] font-bold text-slate-400 mb-1">備註</label>
                                  <input value={item.note} onChange={e => setNewApptItems(newApptItems.map(i => i.id === item.id ? { ...i, note: e.target.value } : i))} className="w-full px-3 py-2 rounded-lg border-none text-sm" />
                                </div>
                                <div>
                                  <label className="block text-[10px] font-bold text-slate-400 mb-1">單價</label>
                                  <input type="number" value={item.price} onChange={e => setNewApptItems(newApptItems.map(i => i.id === item.id ? { ...i, price: parseInt(e.target.value) || 0 } : i))} className="w-full px-3 py-2 rounded-lg border-none text-sm" />
                                </div>
                              </div>
                              {newApptItems.length > 1 && (
                                <button type="button" onClick={() => setNewApptItems(newApptItems.filter(i => i.id !== item.id))} className="absolute top-4 right-4 text-slate-300 hover:text-red-500 transition-colors">
                                  <LogOut className="w-4 h-4 rotate-45" />
                                </button>
                              )}
                            </div>
                          ))}
                        </div>
                      </div>

                      <div className="space-y-6">
                        <div className="space-y-4">
                          <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">優惠折扣</h4>
                          <div className="flex items-center gap-3 bg-orange-50 p-4 rounded-lg border border-orange-100">
                            <label className="text-sm font-medium text-orange-700 whitespace-nowrap">優惠金額</label>
                            <div className="relative flex-1 max-w-[200px]">
                              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-orange-400 text-sm">$</span>
                              <input
                                data-testid="input-create-discount"
                                type="number"
                                min="0"
                                className="w-full pl-7 pr-3 py-2 bg-white border border-orange-200 rounded-lg text-sm font-bold text-orange-700 focus:ring-1 focus:ring-orange-400 focus:border-orange-400 focus:outline-none"
                                value={newApptDiscount}
                                onChange={e => setNewApptDiscount(parseInt(e.target.value) || 0)}
                              />
                            </div>
                            {newApptDiscount > 0 && (
                              <span className="text-xs text-orange-500">已折抵 ${newApptDiscount.toLocaleString()}</span>
                            )}
                          </div>
                        </div>

                        <div className="pt-6 border-t border-slate-100 space-y-2">
                          {(() => {
                            const createSubtotal = newApptItems.reduce((sum, item) => sum + item.price, 0) + newApptExtraItems.reduce((sum, item) => sum + item.price, 0);
                            const createTotal = Math.max(0, createSubtotal - (newApptDiscount || 0));
                            return (
                              <>
                                <div className="flex justify-between items-center text-sm text-slate-500">
                                  <span>小計</span>
                                  <span>${createSubtotal.toLocaleString()}</span>
                                </div>
                                {newApptDiscount > 0 && (
                                  <div className="flex justify-between items-center text-sm text-orange-500">
                                    <span>優惠折扣</span>
                                    <span>-${newApptDiscount.toLocaleString()}</span>
                                  </div>
                                )}
                                <div className="flex justify-between items-center pt-2">
                                  <div className="text-lg font-bold">
                                    總計: <span className="text-slate-900">${createTotal.toLocaleString()}</span>
                                  </div>
                                  <Button data-testid="button-submit-appointment" type="submit" className="px-12 py-4 text-lg shadow-xl shadow-slate-200">
                                    建立預約單
                                  </Button>
                                </div>
                              </>
                            );
                          })()}
                        </div>
                      </div>
                    </form>
                  </Card>
                </motion.div>
              )}

              {view === 'technicians' && user.role === 'admin' && (
                <motion.div key="technicians" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <TechnicianManagement technicians={technicians} appointments={appointments} onUpdate={syncTechnicians} onViewLedger={(techId) => { setSelectedLedgerTechId(techId); setView('cashLedger'); }} reviews={reviews} zones={zones} />
                </motion.div>
              )}

              {view === 'customers' && user.role === 'admin' && (
                <motion.div key="customers" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <CustomerManagement customers={customers} onUpdate={syncCustomers} appointments={appointments} reviews={reviews} />
                </motion.div>
              )}

              {view === 'settings' && user.role === 'admin' && (
                <motion.div key="settings" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <SettingsView 
                    extraFeeProducts={extraFeeProducts} 
                    onUpdateExtraFeeProducts={syncExtraFeeProducts}
                    reminderDays={reminderDays}
                    onUpdateReminderDays={syncReminderDays}
                    webhookSettings={webhookSettings}
                    onUpdateWebhookEnabled={syncWebhookEnabled}
                    serviceItems={serviceItems}
                    onUpdateServiceItems={syncServiceItems}
                  />
                </motion.div>
              )}

              {view === 'financials' && user.role === 'admin' && (
                <motion.div key="financials" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <FinancialReportView appointments={appointments} technicians={technicians} />
                </motion.div>
              )}

              {view === 'reminders' && user.role === 'admin' && (
                <motion.div key="reminders" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <ReminderSystem
                    customers={customers}
                    appointments={appointments}
                    reminderDays={reminderDays}
                    onCreateAppointment={(customer) => {
                      setView('create');
                      applyCreateFormDraft({
                        customer_name: customer.name,
                        phone: customer.phone,
                        address: customer.address,
                        line_uid: customer.line_uid,
                      });
                    }}
                  />
                </motion.div>
              )}

              {view === 'line' && user.role === 'admin' && (
                <motion.div key="line" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <LineDataView lineFriends={lineFriends} customers={customers} onLinkCustomer={handleLinkLineFriend} />
                </motion.div>
              )}

              {view === 'zones' && user.role === 'admin' && (
                <motion.div key="zones" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <ZoneManagement zones={zones} technicians={technicians} onUpdateZones={syncZones} />
                </motion.div>
              )}

              {view === 'heatmap' && user.role === 'admin' && (
                <motion.div key="heatmap" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <HeatMap appointments={appointments} />
                </motion.div>
              )}

              {view === 'reviews' && user.role === 'admin' && (
                <motion.div key="reviews" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <ReviewDashboard reviews={reviews} technicians={technicians} appointments={appointments} />
                </motion.div>
              )}

              {view === 'payments' && user.role === 'admin' && (
                <motion.div key="payments" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <PaymentManagement />
                </motion.div>
              )}

              {view === 'schedule' && user.role === 'admin' && (
                <motion.div key="schedule" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <ScheduleGantt
                    technicians={technicians}
                    appointments={appointments}
                    onSelectAppointment={(appt) => { setSelectedAppt(appt); setIsDrawerOpen(true); setIsEditing(false); }}
                    onQuickCreate={(techId, dateTime) => {
                      setView('create');
                      applyCreateFormDraft({
                        scheduled_at: dateTime,
                        technician_id: techId,
                      });
                    }}
                  />
                </motion.div>
              )}

              {view === 'cashLedger' && user.role === 'admin' && selectedLedgerTechId && (
                <motion.div key="cashLedger" initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }}>
                  <CashLedger
                    technician={technicians.find(t => t.id === selectedLedgerTechId)!}
                    appointments={appointments}
                    ledgerEntries={cashLedgerEntries}
                    onAddEntry={createCashLedgerEntry}
                    onBack={() => setView('technicians')}
                  />
                </motion.div>
              )}
            </AnimatePresence>
          </main>

          <AnimatePresence>
            {isDrawerOpen && selectedAppt && (
              <>
                <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
                  onClick={() => { setIsDrawerOpen(false); setIsEditing(false); setShowDispatch(false); }}
                  className="fixed inset-0 bg-black/20 backdrop-blur-sm z-[60]"
                />
                <motion.div 
                  initial={{ x: '100%' }} animate={{ x: 0 }} exit={{ x: '100%' }}
                  transition={{ type: 'spring', damping: 25, stiffness: 200 }}
                  className="fixed top-0 right-0 bottom-0 w-full max-w-xl bg-white shadow-2xl z-[70] overflow-y-auto"
                >
                  <div className="p-6 md:p-8 space-y-8">
                    <div className="flex justify-between items-center">
                      <div className="flex gap-2">
                        <Button variant="outline" onClick={() => { setIsDrawerOpen(false); setIsEditing(false); setShowDispatch(false); }} className="rounded-full w-10 h-10 p-0" data-testid="button-close-drawer">
                          <ChevronRight className="w-5 h-5 rotate-180" />
                        </Button>
                        {user.role === 'admin' && (
                          <div className="flex gap-2">
                            <Button variant={isEditing ? 'primary' : 'outline'} onClick={() => setIsEditing(!isEditing)} data-testid="button-toggle-edit">
                              {isEditing ? '取消編輯' : '編輯資料'}
                            </Button>
                            <Button variant="danger" data-testid="button-delete-appt" onClick={() => {
                              if (confirm('確定要刪除這筆預約嗎？')) {
                                deleteAppointment(selectedAppt.id)
                                  .then(() => {
                                    setAppointments(prev => prev.filter(a => a.id !== selectedAppt.id));
                                    setIsDrawerOpen(false);
                                    refreshAppSnapshot().catch((err: unknown) => console.error(err));
                                    toast.success('預約已刪除');
                                  })
                                  .catch((err) => {
                                    console.error(err);
                                    toast.error('刪除預約失敗');
                                  });
                              }
                            }}>
                              刪除
                            </Button>
                          </div>
                        )}
                      </div>
                      <Badge status={selectedAppt.status} />
                    </div>

                    {isEditing ? (
                      <AppointmentEditor 
                        appointment={selectedAppt} 
                        onSave={(updated) => { handleUpdateAppointment(updated); setIsEditing(false); }}
                        extraFeeProducts={extraFeeProducts}
                        serviceItems={serviceItems}
                      />
                    ) : (
                      <>
                        <div>
                          <h3 className="text-3xl font-bold text-slate-900 mb-2" data-testid="text-drawer-customer">{selectedAppt.customer_name}</h3>
                          <div className="flex items-center gap-2 text-slate-400">
                            <Calendar className="w-4 h-4" />
                            <span className="text-sm font-medium">
                              {format(parseISO(selectedAppt.scheduled_at), 'yyyy/MM/dd HH:mm')}
                            </span>
                          </div>
                        </div>

                        <div className="space-y-6">
                          <div className="space-y-4">
                            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">聯絡與地點</h4>
                            <div className="bg-slate-50 rounded-lg p-4 space-y-4">
                              <div className="flex items-start gap-3">
                                <MapPin className="w-5 h-5 text-slate-400 mt-0.5" />
                                <div>
                                  <p className="text-sm font-medium text-slate-900">{selectedAppt.address}</p>
                                  <a href={`https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(selectedAppt.address)}`} target="_blank" rel="noreferrer" className="text-xs text-blue-500 hover:underline mt-1 inline-block">
                                    在 Google 地圖中開啟
                                  </a>
                                </div>
                              </div>
                              <div className="flex items-center gap-3">
                                <Phone className="w-5 h-5 text-slate-400" />
                                <p className="text-sm font-medium text-slate-900">{selectedAppt.phone}</p>
                              </div>
                            </div>
                          </div>

                          <div className="space-y-4">
                            <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">清洗內容 ({selectedAppt.items.length} 台)</h4>
                            <div className="space-y-3">
                              {selectedAppt.items.map((item, idx) => (
                                <div key={item.id} className="bg-slate-50 rounded-lg p-4 flex items-center justify-between">
                                  <div className="flex items-center gap-4">
                                    <div className="w-10 h-10 bg-white rounded-md flex items-center justify-center shadow-sm text-xs font-bold">{idx + 1}</div>
                                    <div>
                                      <p className="text-sm font-bold text-slate-900">{item.type}</p>
                                      {item.note && <p className="text-xs text-slate-500">{item.note}</p>}
                                    </div>
                                  </div>
                                  <span className="text-sm font-bold text-slate-900">${item.price}</span>
                                </div>
                              ))}
                            </div>
                          </div>

                          {selectedAppt.extra_items && selectedAppt.extra_items.length > 0 && (
                            <div className="space-y-4">
                              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">額外項目</h4>
                              <div className="space-y-2">
                                {selectedAppt.extra_items.map(item => (
                                  <div key={item.id} className="flex justify-between items-center bg-slate-50 p-4 rounded-lg">
                                    <span className="text-sm font-medium text-slate-700">{item.name}</span>
                                    <span className="text-sm font-bold text-slate-900">${item.price}</span>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          <div className="bg-blue-600 text-white p-6 rounded-lg flex justify-between items-center">
                            <div>
                              <p className="text-[10px] font-bold text-blue-200 uppercase tracking-wider">應收總額 ({getPaymentMethodLabel(selectedAppt)})</p>
                              <p className="text-2xl font-bold">${getChargeableAmount(selectedAppt)}</p>
                            </div>
                            <DollarSign className="w-8 h-8 text-blue-300" />
                          </div>

                          {user.role === 'admin' && selectedAppt.status !== 'completed' && (
                            <div className="space-y-4">
                              <div className="flex items-center justify-between">
                                <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">
                                  {selectedAppt.technician_id ? '重新指派師傅' : '指派師傅'}
                                </h4>
                                <Button
                                  variant="outline"
                                  className="text-xs py-1 px-3"
                                  data-testid="button-auto-dispatch"
                                  onClick={() => {
                                    const suggestions = getAutoDispatchSuggestions(selectedAppt, technicians, appointments, zones);
                                    setDispatchSuggestions(suggestions);
                                    setShowDispatch(!showDispatch);
                                  }}
                                >
                                  {showDispatch ? '收起推薦' : '智能推薦'}
                                </Button>
                              </div>
                              <div className="flex gap-2">
                                <select 
                                  data-testid="select-assign-tech"
                                  className="flex-1 bg-slate-50 border-none rounded-md px-4 py-3 text-sm focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all"
                                  onChange={(e) => handleAssign(selectedAppt.id, parseInt(e.target.value))}
                                  value={selectedAppt.technician_id || ""}
                                >
                                  <option value="" disabled>選擇師傅...</option>
                                  {technicians.map(t => {
                                    const apptDate = parseISO(selectedAppt.scheduled_at);
                                    const day = apptDate.getDay();
                                    const time = format(apptDate, 'HH:00');
                                    const isAvailable = t.availability?.find(a => a.day === day)?.slots.includes(time);
                                    return (
                                      <option key={t.id} value={t.id}>
                                        {t.name} {isAvailable ? '(可預約)' : '(非上班時段)'}
                                      </option>
                                    );
                                  })}
                                </select>
                              </div>
                              {showDispatch && dispatchSuggestions.length > 0 && (
                                <div className="space-y-2" data-testid="dispatch-suggestions">
                                  <p className="text-xs text-slate-500 font-medium">推薦排序（分數由高到低）：</p>
                                  {dispatchSuggestions.map((ds, idx) => (
                                    <div
                                      key={ds.technician.id}
                                      className={cn(
                                        "p-3 rounded-md border cursor-pointer transition-all hover:shadow-sm",
                                        idx === 0 ? "bg-emerald-50 border-emerald-200" : "bg-white border-slate-100"
                                      )}
                                      onClick={() => {
                                        handleAssign(selectedAppt.id, ds.technician.id);
                                        setShowDispatch(false);
                                      }}
                                      data-testid={`dispatch-suggestion-${ds.technician.id}`}
                                    >
                                      <div className="flex justify-between items-center mb-2">
                                        <span className="text-sm font-bold text-slate-900">
                                          {idx === 0 && '⭐ '}{ds.technician.name}
                                        </span>
                                        <span className={cn(
                                          "text-xs font-bold px-2 py-0.5 rounded-full",
                                          ds.totalScore >= 60 ? "bg-emerald-100 text-emerald-700" :
                                          ds.totalScore >= 30 ? "bg-amber-100 text-amber-700" :
                                          "bg-red-100 text-red-700"
                                        )}>
                                          {ds.totalScore} 分
                                        </span>
                                      </div>
                                      <div className="flex flex-wrap gap-1.5">
                                        <span className={cn("text-[10px] px-2 py-0.5 rounded-full font-medium",
                                          ds.reasons.zoneMatch ? "bg-emerald-100 text-emerald-700" : "bg-slate-100 text-slate-400"
                                        )}>
                                          區域{ds.reasons.zoneMatch ? ' ✓' : ' ✗'}
                                        </span>
                                        <span className={cn("text-[10px] px-2 py-0.5 rounded-full font-medium",
                                          ds.reasons.timeAvailable ? "bg-emerald-100 text-emerald-700" : "bg-slate-100 text-slate-400"
                                        )}>
                                          時段{ds.reasons.timeAvailable ? ' ✓' : ' ✗'}
                                        </span>
                                        <span className={cn("text-[10px] px-2 py-0.5 rounded-full font-medium",
                                          ds.reasons.skillMatch ? "bg-emerald-100 text-emerald-700" : "bg-slate-100 text-slate-400"
                                        )}>
                                          技能{ds.reasons.skillMatch ? ' ✓' : ' ✗'}
                                        </span>
                                        <span className="text-[10px] px-2 py-0.5 rounded-full font-medium bg-slate-100 text-slate-500">
                                          今日 {ds.reasons.loadBalance} 單
                                        </span>
                                      </div>
                                    </div>
                                  ))}
                                </div>
                              )}
                            </div>
                          )}

                          {user.role === 'admin' && (
                            <NotificationSender
                              appointment={selectedAppt}
                              technicians={technicians}
                              notificationLogs={notificationLogs}
                              onSend={createNotificationLog}
                            />
                          )}

                          {selectedAppt.status === 'completed' && (
                            <div className="space-y-2" data-testid="section-review-link">
                              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">評價連結</h4>
                              {reviews.find(r => r.appointment_id === selectedAppt.id) ? (
                                <div className="bg-emerald-50 border border-emerald-100 rounded-lg p-4">
                                  <div className="flex items-center gap-2 text-emerald-700 text-sm font-medium">
                                    <CheckCircle2 className="w-4 h-4" />
                                    <span>客戶已完成評價</span>
                                    <span className="ml-auto flex gap-0.5">
                                      {[1, 2, 3, 4, 5].map(s => (
                                        <Star key={s} className={cn("w-3.5 h-3.5", (reviews.find(r => r.appointment_id === selectedAppt.id)?.rating ?? 0) >= s ? "text-amber-400 fill-amber-400" : "text-slate-200")} />
                                      ))}
                                    </span>
                                  </div>
                                  {reviews.find(r => r.appointment_id === selectedAppt.id)?.comment && (
                                    <p className="text-xs text-emerald-600 mt-2">「{reviews.find(r => r.appointment_id === selectedAppt.id)?.comment}」</p>
                                  )}
                                </div>
                              ) : (
                                <div className="bg-slate-50 border border-slate-100 rounded-lg p-4 space-y-3">
                                  <div className="flex items-center gap-2 text-slate-600 text-sm">
                                    <Link2 className="w-4 h-4 text-blue-500" />
                                    <span className="font-medium">傳送此連結給客戶進行評價</span>
                                  </div>
                                  <div className="flex gap-2">
                                    <input
                                      readOnly
                                      value={selectedAppt.review_token ? `${window.location.origin}/review/${selectedAppt.review_token}` : ''}
                                      data-testid="input-review-url"
                                      className="flex-1 px-3 py-2 bg-white rounded-md border border-slate-200 text-xs text-slate-600 truncate"
                                    />
                                    <button
                                      data-testid="button-copy-review-url"
                                      onClick={() => {
                                        if (!selectedAppt.review_token) {
                                          toast.error('評價連結尚未就緒，請重新整理後再試');
                                          return;
                                        }
                                        navigator.clipboard.writeText(`${window.location.origin}/review/${selectedAppt.review_token}`);
                                        toast.success('已複製評價連結');
                                      }}
                                      className="px-3 py-2 bg-blue-600 text-white rounded-md text-xs font-medium hover:bg-blue-700 transition-colors flex items-center gap-1"
                                    >
                                      <Copy className="w-3 h-3" />
                                      複製
                                    </button>
                                  </div>
                                </div>
                              )}
                            </div>
                          )}

                          {selectedAppt.photos && selectedAppt.photos.length > 0 && (
                            <div className="space-y-4">
                              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">施工照片</h4>
                              <div className="grid grid-cols-2 gap-4">
                                {selectedAppt.photos.map((p, i) => (
                                  <img key={i} src={p} alt={`Photo ${i}`} className="w-full aspect-square object-cover rounded-lg border border-slate-100 shadow-sm" referrerPolicy="no-referrer" />
                                ))}
                              </div>
                            </div>
                          )}

                          {(selectedAppt.departed_time || selectedAppt.checkin_time || selectedAppt.completed_time || selectedAppt.payment_time) && (
                            <div className="space-y-4">
                              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">工作時間軸</h4>
                              <div className="bg-slate-50 rounded-lg p-4 space-y-3">
                                {selectedAppt.departed_time && (
                                  <div className="flex items-center gap-3">
                                    <div className="w-2 h-2 rounded-full bg-blue-500 flex-shrink-0" />
                                    <span className="text-sm text-slate-500 flex-1">出發</span>
                                    <span className="text-sm font-medium" data-testid="text-departed-time">{format(parseISO(selectedAppt.departed_time), 'HH:mm:ss')}</span>
                                  </div>
                                )}
                                {selectedAppt.checkin_time && (
                                  <div className="flex items-center gap-3">
                                    <div className="w-2 h-2 rounded-full bg-violet-500 flex-shrink-0" />
                                    <span className="text-sm text-slate-500 flex-1">到達簽到</span>
                                    <span className="text-sm font-medium" data-testid="text-checkin-time">{format(parseISO(selectedAppt.checkin_time), 'HH:mm:ss')}</span>
                                  </div>
                                )}
                                {selectedAppt.completed_time && (
                                  <div className="flex items-center gap-3">
                                    <div className="w-2 h-2 rounded-full bg-emerald-500 flex-shrink-0" />
                                    <span className="text-sm text-slate-500 flex-1">清洗完成</span>
                                    <span className="text-sm font-medium" data-testid="text-completed-time">{format(parseISO(selectedAppt.completed_time), 'HH:mm:ss')}</span>
                                  </div>
                                )}
                                {selectedAppt.checkout_time && (
                                  <div className="flex items-center gap-3">
                                    <div className="w-2 h-2 rounded-full bg-emerald-500 flex-shrink-0" />
                                    <span className="text-sm text-slate-500 flex-1">結案</span>
                                    <span className="text-sm font-medium" data-testid="text-checkout-time">{format(parseISO(selectedAppt.checkout_time), 'HH:mm:ss')}</span>
                                  </div>
                                )}
                                {selectedAppt.payment_time && (
                                  <div className="flex items-center gap-3">
                                    <div className="w-2 h-2 rounded-full bg-amber-500 flex-shrink-0" />
                                    <span className="text-sm text-slate-500 flex-1">收款確認</span>
                                    <span className="text-sm font-medium" data-testid="text-payment-time">{format(parseISO(selectedAppt.payment_time), 'HH:mm:ss')}</span>
                                  </div>
                                )}
                              </div>
                            </div>
                          )}

                          {selectedAppt.signature_data && (
                            <div className="space-y-4">
                              <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">客戶簽名</h4>
                              <div className="border border-slate-100 rounded-lg overflow-hidden bg-slate-50 p-2">
                                <img src={selectedAppt.signature_data} alt="客戶簽名" className="w-full h-auto rounded-md" data-testid="img-admin-signature" />
                              </div>
                            </div>
                          )}
                        </div>
                      </>
                    )}
                  </div>
                </motion.div>
              </>
            )}
          </AnimatePresence>
        </>
      )}
    </div>
  );
}
