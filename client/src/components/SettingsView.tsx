import { useEffect, useState } from 'react';
import { Trash2, Wrench, DollarSign, Plus, Webhook, Copy, CheckCircle2, AlertTriangle } from 'lucide-react';
import toast from 'react-hot-toast';
import { Button, Card } from './shared';
import { ExtraItem, ServiceItem } from '../types';
import { WebhookSettingsPayload } from '../lib/api';

interface SettingsViewProps {
  extraFeeProducts: ExtraItem[];
  onUpdateExtraFeeProducts: (items: ExtraItem[]) => Promise<unknown>;
  reminderDays: number;
  onUpdateReminderDays: (days: number) => Promise<unknown>;
  webhookSettings: WebhookSettingsPayload;
  onUpdateWebhookEnabled: (enabled: boolean) => Promise<void> | void;
  serviceItems: ServiceItem[];
  onUpdateServiceItems: (items: ServiceItem[]) => Promise<unknown>;
}

interface ServiceItemDraft {
  id: string;
  name: string;
  default_price: string;
  description: string;
}

interface ExtraItemDraft {
  id: string;
  name: string;
  price: string;
}

// toServiceItemDrafts 把后端读模型转换为可编辑草稿，允许金额输入框临时保持空字串。
const toServiceItemDrafts = (items: ServiceItem[]): ServiceItemDraft[] => (
  items.map(item => ({
    id: item.id,
    name: item.name,
    default_price: String(item.default_price),
    description: item.description || '',
  }))
);

// toExtraItemDrafts 让额外费用项也能先保留本地草稿，避免输入时立刻触发远端同步。
const toExtraItemDrafts = (items: ExtraItem[]): ExtraItemDraft[] => (
  items.map(item => ({
    id: item.id,
    name: item.name,
    price: String(item.price),
  }))
);

// normalizeIntegerInput 统一把数字输入框的草稿值转成非负整数；空字串按 0 处理。
const normalizeIntegerInput = (value: string): number => {
  const trimmed = value.trim();
  if (trimmed === '') {
    return 0;
  }
  const parsed = Number.parseInt(trimmed, 10);
  if (Number.isNaN(parsed)) {
    return 0;
  }
  return Math.max(0, parsed);
};

export default function SettingsView({ 
  extraFeeProducts, 
  onUpdateExtraFeeProducts,
  reminderDays,
  onUpdateReminderDays,
  webhookSettings,
  onUpdateWebhookEnabled,
  serviceItems,
  onUpdateServiceItems
}: SettingsViewProps) {
  const [copied, setCopied] = useState(false);
  const [serviceItemDrafts, setServiceItemDrafts] = useState<ServiceItemDraft[]>(() => toServiceItemDrafts(serviceItems));
  const [extraItemDrafts, setExtraItemDrafts] = useState<ExtraItemDraft[]>(() => toExtraItemDrafts(extraFeeProducts));
  const [reminderDaysDraft, setReminderDaysDraft] = useState(() => String(reminderDays));

  useEffect(() => {
    setServiceItemDrafts(toServiceItemDrafts(serviceItems));
  }, [serviceItems]);

  useEffect(() => {
    setExtraItemDrafts(toExtraItemDrafts(extraFeeProducts));
  }, [extraFeeProducts]);

  useEffect(() => {
    setReminderDaysDraft(String(reminderDays));
  }, [reminderDays]);

  // buildServiceItemsFromDrafts 在提交前统一做去空白与金额归一，避免 onBlur 分支各自复制转换逻辑。
  const buildServiceItemsFromDrafts = (drafts: ServiceItemDraft[]): ServiceItem[] | null => {
    const items = drafts.map(draft => ({
      id: draft.id,
      name: draft.name.trim(),
      default_price: normalizeIntegerInput(draft.default_price),
      description: draft.description.trim(),
    }));
    if (items.some(item => item.name === '')) {
      return null;
    }
    return items;
  };

  // buildExtraItemsFromDrafts 统一处理额外费用项草稿，避免清空金额时误把空字串直接打到后端。
  const buildExtraItemsFromDrafts = (drafts: ExtraItemDraft[]): ExtraItem[] | null => {
    const items = drafts.map(draft => ({
      id: draft.id,
      name: draft.name.trim(),
      price: normalizeIntegerInput(draft.price),
    }));
    if (items.some(item => item.name === '')) {
      return null;
    }
    return items;
  };

  const addProduct = () => {
    setExtraItemDrafts([...extraItemDrafts, { id: Date.now().toString(), name: '新項目', price: '0' }]);
  };

  const updateProductDraft = (id: string, field: keyof ExtraItemDraft, value: string) => {
    setExtraItemDrafts(prev => prev.map(item => item.id === id ? { ...item, [field]: value } : item));
  };

  const removeProduct = async (id: string) => {
    const nextDrafts = extraItemDrafts.filter(item => item.id !== id);
    setExtraItemDrafts(nextDrafts);
    const nextItems = buildExtraItemsFromDrafts(nextDrafts);
    if (!nextItems) {
      toast.error('額外費用項目名稱不能為空');
      setExtraItemDrafts(toExtraItemDrafts(extraFeeProducts));
      return;
    }
    await onUpdateExtraFeeProducts(nextItems);
  };

  const addServiceItem = () => {
    setServiceItemDrafts([...serviceItemDrafts, { id: Date.now().toString(), name: '新服務項目', default_price: '0', description: '' }]);
  };

  const updateServiceItemDraft = (id: string, field: keyof ServiceItemDraft, value: string) => {
    setServiceItemDrafts(prev => prev.map(item => item.id === id ? { ...item, [field]: value } : item));
  };

  const removeServiceItem = async (id: string) => {
    if (serviceItemDrafts.length <= 1) return;
    const nextDrafts = serviceItemDrafts.filter(item => item.id !== id);
    setServiceItemDrafts(nextDrafts);
    const nextItems = buildServiceItemsFromDrafts(nextDrafts);
    if (!nextItems) {
      toast.error('服務項目名稱不能為空');
      setServiceItemDrafts(toServiceItemDrafts(serviceItems));
      return;
    }
    await onUpdateServiceItems(nextItems);
  };

  // commitServiceItemOnBlur 只在输入框失焦时提交单行草稿，避免每次键入都打远端接口。
  const commitServiceItemOnBlur = async (id: string) => {
    // 服務項目名稱若仍為空，代表管理員還在編輯或暫時清空；此時只保留本地草稿，
    // 不觸發保存，也不提示錯誤，避免出現「空值保存不上」的干擾訊息。
    if (serviceItemDrafts.some(item => item.id === id && item.name.trim() === '')) {
      return;
    }
    const nextItems = buildServiceItemsFromDrafts(serviceItemDrafts);
    if (!nextItems) {
      // 若其他列仍有空名稱，也同樣跳過提交，避免單列失焦時把整個列表判成保存失敗。
      return;
    }
    if (JSON.stringify(nextItems) === JSON.stringify(serviceItems)) {
      setServiceItemDrafts(toServiceItemDrafts(nextItems));
      return;
    }
    setServiceItemDrafts(toServiceItemDrafts(nextItems));
    await onUpdateServiceItems(nextItems);
  };

  // commitExtraItemOnBlur 与服务项目保持同一交互策略：输入时只改本地，离焦后再保存。
  const commitExtraItemOnBlur = async (id: string) => {
    // 額外費用項目名稱若暫時為空，代表管理員仍在編輯；此時只保留本地草稿，
    // 不觸發保存，也不提示錯誤，避免空值編輯過程被干擾。
    if (extraItemDrafts.some(item => item.id === id && item.name.trim() === '')) {
      return;
    }
    const nextItems = buildExtraItemsFromDrafts(extraItemDrafts);
    if (!nextItems) {
      // 若其他列仍有空名稱，也跳過提交，避免單列失焦時誤觸整份設定保存失敗。
      return;
    }
    if (JSON.stringify(nextItems) === JSON.stringify(extraFeeProducts)) {
      setExtraItemDrafts(toExtraItemDrafts(nextItems));
      return;
    }
    setExtraItemDrafts(toExtraItemDrafts(nextItems));
    await onUpdateExtraFeeProducts(nextItems);
  };

  // commitReminderDaysOnBlur 仅在输入完成后提交回访天数，避免输入 1 -> 18 -> 180 时连续打三次接口。
  const commitReminderDaysOnBlur = async () => {
    const nextReminderDays = normalizeIntegerInput(reminderDaysDraft);
    if (nextReminderDays <= 0) {
      setReminderDaysDraft(String(reminderDays));
      toast.error('回訪提醒天數必須大於 0');
      return;
    }
    setReminderDaysDraft(String(nextReminderDays));
    if (nextReminderDays === reminderDays) {
      return;
    }
    await onUpdateReminderDays(nextReminderDays);
  };

  // handleCopyWebhookUrl 只复制后端回传的当前 URL，避免前端自行猜测生产地址。
  const handleCopyWebhookUrl = async () => {
    if (!webhookSettings.url) return;
    await navigator.clipboard.writeText(webhookSettings.url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  const webhookStatusTone = webhookSettings.effective_enabled
    ? 'bg-emerald-50 border-emerald-100 text-emerald-700'
    : webhookSettings.enabled
      ? 'bg-amber-50 border-amber-100 text-amber-700'
      : 'bg-slate-50 border-slate-100 text-slate-600';

  return (
    <div className="space-y-6">
      <h2 className="text-2xl font-bold">系統設定</h2>

      <Card className="p-6 space-y-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Webhook className="w-5 h-5 text-blue-500" />
              <h3 className="font-bold">LINE Webhook 設定</h3>
            </div>
            <p className="text-sm text-slate-500">
              管理員可在此切換 webhook 處理開關；LINE secret 與公開域名仍由後端環境配置決定，這裡僅展示狀態與可複製地址。
            </p>
          </div>
          <div className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-bold ${webhookStatusTone}`} data-testid="badge-webhook-effective-status">
            {webhookSettings.effective_enabled ? <CheckCircle2 className="w-3.5 h-3.5" /> : <AlertTriangle className="w-3.5 h-3.5" />}
            {webhookSettings.effective_enabled ? '已啟用' : webhookSettings.enabled ? '已開啟但未就緒' : '已停用'}
          </div>
        </div>

        <div className="grid gap-4 lg:grid-cols-[1.4fr,1fr]">
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 space-y-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm font-bold text-slate-800">Webhook 開關</p>
                <p className="text-xs text-slate-500">可切換，會寫入系統設定；不會直接改動伺服器環境變數。</p>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={webhookSettings.enabled}
                data-testid="switch-webhook-enabled"
                onClick={() => onUpdateWebhookEnabled(!webhookSettings.enabled)}
                className={`relative inline-flex h-7 w-14 items-center rounded-full transition-colors ${webhookSettings.enabled ? 'bg-blue-600' : 'bg-slate-300'}`}
              >
                <span
                  className={`inline-block h-5 w-5 transform rounded-full bg-white transition-transform ${webhookSettings.enabled ? 'translate-x-8' : 'translate-x-1'}`}
                />
              </button>
            </div>

            <div className="space-y-2">
              <p className="text-sm font-bold text-slate-800">Webhook URL</p>
              <div className="flex flex-col gap-2 md:flex-row">
                <input
                  readOnly
                  value={webhookSettings.url || '尚未生成可顯示的 webhook URL'}
                  data-testid="input-webhook-url"
                  className="flex-1 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700"
                />
                <Button
                  variant="outline"
                  onClick={handleCopyWebhookUrl}
                  disabled={!webhookSettings.url}
                  data-testid="button-copy-webhook-url"
                >
                  <Copy className="w-4 h-4" />
                  {copied ? '已複製' : '複製 URL'}
                </Button>
              </div>
              <p className="text-xs text-slate-500">
                只讀展示；來源：{webhookSettings.url_source}。{webhookSettings.url_is_public ? '目前為可對外公布的地址。' : '目前僅能作為本機/內網調試地址展示。'}
              </p>
            </div>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white p-4 space-y-3">
            <p className="text-sm font-bold text-slate-800">依賴狀態</p>
            <div className="space-y-2 text-sm text-slate-600">
              <div className="flex items-center justify-between gap-3">
                <span>LINE_CHANNEL_SECRET</span>
                <span className={`rounded-full px-2.5 py-1 text-xs font-bold ${webhookSettings.has_line_channel_secret ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'}`}>
                  {webhookSettings.has_line_channel_secret ? '已配置' : '未配置'}
                </span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span>公開域名 / 公網基址</span>
                <span className={`rounded-full px-2.5 py-1 text-xs font-bold ${webhookSettings.url_is_public ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700'}`}>
                  {webhookSettings.url_is_public ? '已就緒' : '僅本機地址'}
                </span>
              </div>
            </div>
            <div className="rounded-lg bg-slate-50 p-3 text-xs leading-6 text-slate-500" data-testid="text-webhook-status-message">
              <p>{webhookSettings.status_message}</p>
              <p className="mt-2">{webhookSettings.dependency_summary}</p>
            </div>
          </div>
        </div>
      </Card>

      <Card className="p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Wrench className="w-5 h-5 text-blue-500" />
            <h3 className="font-bold">服務項目設定</h3>
          </div>
          <Button data-testid="button-add-service-item" variant="outline" className="text-xs py-1 px-3" onClick={addServiceItem}>
            <Plus className="w-3 h-3" />
            新增項目
          </Button>
        </div>
        <p className="text-xs text-slate-400">設定清洗服務的項目種類與預設金額，新增預約時會自動顯示這些選項</p>
        <div className="space-y-3">
          {serviceItemDrafts.map(item => (
            <div key={item.id} className="flex items-center gap-4 bg-slate-50 p-4 rounded-md" data-testid={`service-item-row-${item.id}`}>
              <div className="flex-1 space-y-2">
                <div className="flex gap-3 items-center">
                  <input
                    data-testid={`input-service-item-name-${item.id}`}
                    className="flex-1 bg-white border border-slate-200 rounded-lg px-3 py-2 text-sm font-bold focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                    value={item.name}
                    onChange={e => updateServiceItemDraft(item.id, 'name', e.target.value)}
                    onBlur={() => void commitServiceItemOnBlur(item.id)}
                    placeholder="項目名稱"
                  />
                  <div className="flex items-center gap-1">
                    <DollarSign className="w-4 h-4 text-slate-400" />
                    <input
                      data-testid={`input-service-item-price-${item.id}`}
                      type="number"
                      className="w-28 bg-white border border-slate-200 rounded-lg px-3 py-2 text-sm text-right font-bold focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                      value={item.default_price}
                      onChange={e => updateServiceItemDraft(item.id, 'default_price', e.target.value)}
                      onBlur={() => void commitServiceItemOnBlur(item.id)}
                    />
                  </div>
                </div>
                <input
                  data-testid={`input-service-item-desc-${item.id}`}
                  className="w-full bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-[11px] text-slate-500 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                  value={item.description}
                  onChange={e => updateServiceItemDraft(item.id, 'description', e.target.value)}
                  onBlur={() => void commitServiceItemOnBlur(item.id)}
                  placeholder="項目說明（選填）"
                />
              </div>
              {serviceItemDrafts.length > 1 && (
                <button
                  onClick={() => void removeServiceItem(item.id)}
                  className="text-slate-300 hover:text-red-500 transition-colors p-1"
                  data-testid={`button-remove-service-item-${item.id}`}
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              )}
            </div>
          ))}
        </div>
      </Card>

      <Card className="p-6 space-y-6">
        <h3 className="font-bold">回訪提醒設定</h3>
        <div className="flex items-center gap-4">
          <label className="text-sm text-slate-600">客戶完工後幾天提醒回訪：</label>
          <input 
            data-testid="input-reminder-days"
            type="number"
            className="w-24 px-3 py-2 bg-white border border-slate-200 rounded-lg text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            value={reminderDaysDraft}
            onChange={e => setReminderDaysDraft(e.target.value)}
            onBlur={() => void commitReminderDaysOnBlur()}
          />
          <span className="text-sm text-slate-400">天</span>
        </div>
      </Card>

      <Card className="p-6 space-y-6">
        <div className="flex justify-between items-center">
          <h3 className="font-bold">額外費用產品設定</h3>
          <Button data-testid="button-add-product" onClick={addProduct}>+ 新增項目</Button>
        </div>
        <div className="space-y-3">
          {extraItemDrafts.map(p => (
            <div key={p.id} className="flex gap-3 items-center bg-slate-50 p-3 rounded-md">
              <input 
                className="flex-1 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={p.name}
                onChange={e => updateProductDraft(p.id, 'name', e.target.value)}
                onBlur={() => void commitExtraItemOnBlur(p.id)}
              />
              <input 
                type="number"
                className="w-24 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={p.price}
                onChange={e => updateProductDraft(p.id, 'price', e.target.value)}
                onBlur={() => void commitExtraItemOnBlur(p.id)}
              />
              <button onClick={() => void removeProduct(p.id)} className="text-slate-300 hover:text-red-500">
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
