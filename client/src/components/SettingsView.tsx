import { useState } from 'react';
import { Trash2, Wrench, DollarSign, Plus, Webhook, Copy, CheckCircle2, AlertTriangle } from 'lucide-react';
import { Button, Card } from './shared';
import { ExtraItem, ServiceItem } from '../types';
import { WebhookSettingsPayload } from '../lib/api';

interface SettingsViewProps {
  extraFeeProducts: ExtraItem[];
  onUpdateExtraFeeProducts: (items: ExtraItem[]) => void;
  reminderDays: number;
  onUpdateReminderDays: (days: number) => void;
  webhookSettings: WebhookSettingsPayload;
  onUpdateWebhookEnabled: (enabled: boolean) => void;
  serviceItems: ServiceItem[];
  onUpdateServiceItems: (items: ServiceItem[]) => void;
}

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
  const addProduct = () => {
    onUpdateExtraFeeProducts([...extraFeeProducts, { id: Date.now().toString(), name: '新項目', price: 0 }]);
  };

  const updateProduct = (id: string, field: keyof ExtraItem, value: string | number) => {
    onUpdateExtraFeeProducts(extraFeeProducts.map(p => p.id === id ? { ...p, [field]: value } : p));
  };

  const removeProduct = (id: string) => {
    onUpdateExtraFeeProducts(extraFeeProducts.filter(p => p.id !== id));
  };

  const addServiceItem = () => {
    onUpdateServiceItems([...serviceItems, { id: Date.now().toString(), name: '新服務項目', default_price: 0, description: '' }]);
  };

  const updateServiceItem = (id: string, field: keyof ServiceItem, value: string | number) => {
    onUpdateServiceItems(serviceItems.map(item => item.id === id ? { ...item, [field]: value } : item));
  };

  const removeServiceItem = (id: string) => {
    if (serviceItems.length <= 1) return;
    onUpdateServiceItems(serviceItems.filter(item => item.id !== id));
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
          {serviceItems.map(item => (
            <div key={item.id} className="flex items-center gap-4 bg-slate-50 p-4 rounded-md" data-testid={`service-item-row-${item.id}`}>
              <div className="flex-1 space-y-2">
                <div className="flex gap-3 items-center">
                  <input
                    data-testid={`input-service-item-name-${item.id}`}
                    className="flex-1 bg-white border border-slate-200 rounded-lg px-3 py-2 text-sm font-bold focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                    value={item.name}
                    onChange={e => updateServiceItem(item.id, 'name', e.target.value)}
                    placeholder="項目名稱"
                  />
                  <div className="flex items-center gap-1">
                    <DollarSign className="w-4 h-4 text-slate-400" />
                    <input
                      data-testid={`input-service-item-price-${item.id}`}
                      type="number"
                      className="w-28 bg-white border border-slate-200 rounded-lg px-3 py-2 text-sm text-right font-bold focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                      value={item.default_price}
                      onChange={e => updateServiceItem(item.id, 'default_price', parseInt(e.target.value) || 0)}
                    />
                  </div>
                </div>
                <input
                  data-testid={`input-service-item-desc-${item.id}`}
                  className="w-full bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-[11px] text-slate-500 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 focus:outline-none"
                  value={item.description || ''}
                  onChange={e => updateServiceItem(item.id, 'description', e.target.value)}
                  placeholder="項目說明（選填）"
                />
              </div>
              {serviceItems.length > 1 && (
                <button
                  onClick={() => removeServiceItem(item.id)}
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
            value={reminderDays}
            onChange={e => onUpdateReminderDays(parseInt(e.target.value) || 180)}
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
          {extraFeeProducts.map(p => (
            <div key={p.id} className="flex gap-3 items-center bg-slate-50 p-3 rounded-md">
              <input 
                className="flex-1 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={p.name}
                onChange={e => updateProduct(p.id, 'name', e.target.value)}
              />
              <input 
                type="number"
                className="w-24 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={p.price}
                onChange={e => updateProduct(p.id, 'price', parseInt(e.target.value) || 0)}
              />
              <button onClick={() => removeProduct(p.id)} className="text-slate-300 hover:text-red-500">
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
