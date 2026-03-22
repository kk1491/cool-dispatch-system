import { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { isLegacyUncollectedAppointment, LEGACY_PAYMENT_METHOD_LABEL, STANDARD_PAYMENT_METHODS, shouldBackfillPaymentMethod } from '../lib/appointmentMetrics';
import { Button } from './shared';
import { Appointment, ACUnit, ExtraItem, PaymentMethod, ServiceItem } from '../types';

interface AppointmentEditorProps {
  appointment: Appointment;
  onSave: (updated: Appointment) => void;
  extraFeeProducts: ExtraItem[];
  serviceItems: ServiceItem[];
}

export default function AppointmentEditor({ appointment, onSave, extraFeeProducts, serviceItems }: AppointmentEditorProps) {
  const [edited, setEdited] = useState<Appointment>(JSON.parse(JSON.stringify(appointment)));
  const needsPaymentMethodBackfill = shouldBackfillPaymentMethod(edited);
  const hasLegacyPaymentMethod = isLegacyUncollectedAppointment(edited);

  const addItem = () => {
    const first = serviceItems[0];
    setEdited({
      ...edited,
      items: [...edited.items, { id: Date.now().toString(), type: first?.name || '', note: '', price: first?.default_price || 0 }]
    });
  };

  const removeItem = (id: string) => {
    setEdited({ ...edited, items: edited.items.filter(i => i.id !== id) });
  };

  // updateItem 只允许修改当前表单实际会编辑的文字/数字字段，避免继续使用 any。
  const updateItem = (id: string, field: 'note' | 'price', value: string | number) => {
    setEdited({ ...edited, items: edited.items.map(i => i.id === id ? { ...i, [field]: value } : i) });
  };

  const handleTypeChange = (id: string, typeName: string) => {
    const si = serviceItems.find(s => s.name === typeName);
    setEdited({
      ...edited,
      items: edited.items.map(i => i.id === id ? { ...i, type: typeName, price: si?.default_price || i.price } : i)
    });
  };

  const addExtra = () => {
    setEdited({
      ...edited,
      extra_items: [...(edited.extra_items || []), { id: Date.now().toString(), name: '', price: 0 }]
    });
  };

  const addExtraFromProduct = (product: ExtraItem) => {
    setEdited({
      ...edited,
      extra_items: [...(edited.extra_items || []), { ...product, id: Date.now().toString() }]
    });
  };

  const removeExtra = (id: string) => {
    setEdited({ ...edited, extra_items: (edited.extra_items || []).filter(i => i.id !== id) });
  };

  // updateExtra 与额外费用表单字段保持同样的窄类型约束。
  const updateExtra = (id: string, field: 'name' | 'price', value: string | number) => {
    setEdited({ ...edited, extra_items: (edited.extra_items || []).map(i => i.id === id ? { ...i, [field]: value } : i) });
  };

  const subtotal = (edited.items.reduce((sum, i) => sum + (i.price || 0), 0)) + 
                ((edited.extra_items || []).reduce((sum, i) => sum + (i.price || 0), 0));
  const discount = edited.discount_amount || 0;
  const total = Math.max(0, subtotal - discount);

  return (
    <div className="space-y-8">
      <div className="space-y-4">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">基本資訊</h4>
        <div className="grid gap-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-bold text-slate-400 mb-1">客戶姓名</label>
              <input 
                data-testid="input-edit-name"
                className="w-full px-4 py-2.5 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                value={edited.customer_name}
                onChange={e => setEdited({ ...edited, customer_name: e.target.value })}
              />
            </div>
            <div>
              <label className="block text-xs font-bold text-slate-400 mb-1">聯繫電話</label>
              <input 
                data-testid="input-edit-phone"
                className="w-full px-4 py-2.5 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                value={edited.phone}
                onChange={e => setEdited({ ...edited, phone: e.target.value })}
              />
            </div>
          </div>
          <div>
            <label className="block text-xs font-bold text-slate-400 mb-1">施工地址</label>
            <input 
              data-testid="input-edit-address"
              className="w-full px-4 py-2.5 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              value={edited.address}
              onChange={e => setEdited({ ...edited, address: e.target.value })}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-bold text-slate-400 mb-1">預約時間</label>
              <input 
                type="datetime-local"
                data-testid="input-edit-datetime"
                className="w-full px-4 py-2.5 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                value={edited.scheduled_at.slice(0, 16)}
                onChange={e => {
                  try {
                    const iso = new Date(e.target.value).toISOString();
                    setEdited({ ...edited, scheduled_at: iso });
                  } catch (err) {
                    setEdited({ ...edited, scheduled_at: e.target.value });
                  }
                }}
              />
            </div>
            <div>
              <label className="block text-xs font-bold text-slate-400 mb-1">收款方式</label>
              <select 
                data-testid="select-edit-payment"
                className="w-full px-4 py-2.5 bg-slate-50 border-none rounded-md text-sm focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                value={edited.payment_method}
                onChange={e => setEdited({ ...edited, payment_method: e.target.value as PaymentMethod })}
              >
                {hasLegacyPaymentMethod && (
                  <option value="未收款" disabled>{LEGACY_PAYMENT_METHOD_LABEL}</option>
                )}
                {STANDARD_PAYMENT_METHODS.map(method => (
                  <option key={method} value={method}>{method}</option>
                ))}
              </select>
              {needsPaymentMethodBackfill && (
                <p className="mt-1 text-[11px] text-amber-600">
                  這筆舊資料目前仍是占位值；若本次要補齊資料，請改成真實付款方式後再儲存。
                </p>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="space-y-4">
        <div className="flex justify-between items-center">
          <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">清洗內容</h4>
          <Button variant="outline" className="text-xs py-1 px-3" onClick={addItem} data-testid="button-edit-add-item">+ 新增項目</Button>
        </div>
        <div className="space-y-3">
          {edited.items.map((item, idx) => (
            <div key={item.id} className="bg-slate-50 rounded-lg p-4 space-y-4 relative">
              <div className="grid grid-cols-3 gap-3">
                <select 
                  className="bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                  value={item.type}
                  onChange={e => handleTypeChange(item.id, e.target.value)}
                  data-testid={`select-edit-item-type-${idx}`}
                >
                  {serviceItems.map(si => (
                    <option key={si.id} value={si.name}>{si.name}</option>
                  ))}
                </select>
                <input 
                  placeholder="備註"
                  className="bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                  value={item.note}
                  onChange={e => updateItem(item.id, 'note', e.target.value)}
                />
                <input 
                  type="number"
                  placeholder="單價"
                  className="bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                  value={item.price}
                  onChange={e => updateItem(item.id, 'price', parseInt(e.target.value) || 0)}
                />
              </div>
              {edited.items.length > 1 && (
                <button onClick={() => removeItem(item.id)} className="absolute top-4 right-4 text-slate-300 hover:text-red-500">
                  <Trash2 className="w-4 h-4" />
                </button>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-4">
        <div className="flex justify-between items-center">
          <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">額外費用</h4>
          <div className="flex gap-2">
            <Button variant="outline" className="text-xs py-1 px-3" onClick={addExtra}>+ 自訂項目</Button>
          </div>
        </div>
        <div className="flex gap-2 overflow-x-auto pb-2">
          {extraFeeProducts.map(p => (
            <Button key={p.id} variant="outline" className="text-xs py-1 px-3 whitespace-nowrap" onClick={() => addExtraFromProduct(p)}>
              + {p.name} (${p.price})
            </Button>
          ))}
        </div>
        <div className="space-y-3">
          {(edited.extra_items || []).map((item) => (
            <div key={item.id} className="flex gap-3 items-center bg-slate-50 p-3 rounded-md">
              <input 
                placeholder="項目名稱"
                className="flex-1 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={item.name}
                onChange={e => updateExtra(item.id, 'name', e.target.value)}
              />
              <input 
                type="number"
                placeholder="金額"
                className="w-24 bg-white border border-slate-200 rounded-lg px-3 py-1.5 text-sm"
                value={item.price}
                onChange={e => updateExtra(item.id, 'price', parseInt(e.target.value) || 0)}
              />
              <button onClick={() => removeExtra(item.id)} className="text-slate-300 hover:text-red-500">
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-4">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">優惠折扣</h4>
        <div className="flex items-center gap-3 bg-orange-50 p-4 rounded-lg border border-orange-100">
          <label className="text-sm font-medium text-orange-700 whitespace-nowrap">優惠金額</label>
          <div className="relative flex-1 max-w-[200px]">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-orange-400 text-sm">$</span>
            <input
              data-testid="input-edit-discount"
              type="number"
              min="0"
              className="w-full pl-7 pr-3 py-2 bg-white border border-orange-200 rounded-lg text-sm font-bold text-orange-700 focus:ring-1 focus:ring-orange-400 focus:border-orange-400 focus:outline-none"
              value={edited.discount_amount || 0}
              onChange={e => setEdited({ ...edited, discount_amount: parseInt(e.target.value) || 0 })}
            />
          </div>
          {discount > 0 && (
            <span className="text-xs text-orange-500">已折抵 ${discount.toLocaleString()}</span>
          )}
        </div>
      </div>

      <div className="pt-6 border-t border-slate-100 space-y-2">
        <div className="flex justify-between items-center text-sm text-slate-500">
          <span>小計</span>
          <span>${subtotal.toLocaleString()}</span>
        </div>
        {discount > 0 && (
          <div className="flex justify-between items-center text-sm text-orange-500">
            <span>優惠折扣</span>
            <span>-${discount.toLocaleString()}</span>
          </div>
        )}
        <div className="flex justify-between items-center pt-2">
          <div className="text-lg font-bold">
            總計金額: <span className="text-slate-900">${total.toLocaleString()}</span>
          </div>
          <Button
            data-testid="button-save-edit"
            className="px-10"
            // 管理员普通编辑保留读模型原始支付字段，真正的写 DTO 收口统一延后到 App/api 边界处理，
            // 避免 legacy `未收款` 旧单在“只改地址/品项”时被这里提前收敛成真实付款方式。
            onClick={() => onSave({ ...edited, total_amount: total, discount_amount: discount })}
          >
            儲存變更
          </Button>
        </div>
      </div>
    </div>
  );
}
