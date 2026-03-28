import { useEffect, useMemo, useState } from 'react';
import { format, parseISO } from 'date-fns';
import { ArchiveRestore, CheckSquare, Loader2, RefreshCw, RotateCcw, Trash2 } from 'lucide-react';
import { toast } from 'react-hot-toast';

import {
  DeletedResourceItem,
  fetchRecycleBinPageData,
  RecycleBinEntityType,
  RecycleBinPageData,
  restoreRecycleBinItems,
} from '../lib/api';
import { Button, Card } from './shared';
import { cn } from '../lib/utils';

interface RecycleBinViewProps {
  onRestored?: () => Promise<unknown> | unknown;
}

type RecycleBinItem =
  | DeletedResourceItem;

type RecycleBinTabConfig = {
  key: RecycleBinEntityType;
  label: string;
  description: string;
};

const EMPTY_RECYCLE_BIN_DATA: RecycleBinPageData = {
  appointments: [],
  customers: [],
  technicians: [],
  zones: [],
  'service-items': [],
  'extra-items': [],
};

const RECYCLE_BIN_TABS: RecycleBinTabConfig[] = [
  { key: 'appointments', label: '預約', description: '已刪除的預約單與其基本資訊' },
  { key: 'customers', label: '顧客', description: '已刪除的顧客主檔' },
  { key: 'technicians', label: '師傅', description: '已刪除的師傅帳號與資料' },
  { key: 'zones', label: '區域', description: '已刪除的服務區域設定' },
  { key: 'service-items', label: '服務項目', description: '已刪除的服務項目設定' },
  { key: 'extra-items', label: '額外費用', description: '已刪除的額外費用設定' },
];

// RecycleBinView 集中管理已软删除数据，支持按类型查看、单条恢复与批量恢复。
export default function RecycleBinView({ onRestored }: RecycleBinViewProps) {
  const [activeTab, setActiveTab] = useState<RecycleBinEntityType>('appointments');
  const [data, setData] = useState<RecycleBinPageData>(EMPTY_RECYCLE_BIN_DATA);
  const [loading, setLoading] = useState(true);
  const [restoringIds, setRestoringIds] = useState<string[]>([]);
  const [selectedByTab, setSelectedByTab] = useState<Record<RecycleBinEntityType, string[]>>({
    appointments: [],
    customers: [],
    technicians: [],
    zones: [],
    'service-items': [],
    'extra-items': [],
  });

  const loadData = async () => {
    try {
      const result = await fetchRecycleBinPageData();
      setData(result);
    } catch (error) {
      console.error(error);
      toast.error('載入回收站資料失敗');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const currentItems = useMemo(() => data[activeTab] || [], [activeTab, data]);
  const selectedIds = selectedByTab[activeTab] || [];
  const allCurrentIds = currentItems.map(item => String(getItemId(activeTab, item)));
  const allSelected = currentItems.length > 0 && allCurrentIds.every(id => selectedIds.includes(id));

  const summaryCards = useMemo(
    () => RECYCLE_BIN_TABS.map(tab => ({
      ...tab,
      count: data[tab.key].length,
    })),
    [data],
  );

  // toggleSelection 统一维护每个标签页的勾选状态，避免切换标签后丢失选择结果。
  const toggleSelection = (entityType: RecycleBinEntityType, id: string) => {
    setSelectedByTab(prev => {
      const current = prev[entityType] || [];
      return {
        ...prev,
        [entityType]: current.includes(id)
          ? current.filter(item => item !== id)
          : [...current, id],
      };
    });
  };

  // toggleSelectAll 仅作用于当前标签页，方便管理员对同类型数据做批量恢复。
  const toggleSelectAll = () => {
    setSelectedByTab(prev => ({
      ...prev,
      [activeTab]: allSelected ? [] : allCurrentIds,
    }));
  };

  const restoreItems = async (entityType: RecycleBinEntityType, ids: Array<string | number>) => {
    if (ids.length === 0) {
      return;
    }

    const pendingKeys = ids.map(id => `${entityType}:${String(id)}`);
    setRestoringIds(prev => [...prev, ...pendingKeys]);

    try {
      const result = await restoreRecycleBinItems({ resource: entityType, ids: ids.map(String) });
      toast.success(`已恢復 ${result.restored_count} 筆資料`);
      setSelectedByTab(prev => ({
        ...prev,
        [entityType]: prev[entityType].filter(id => !ids.map(String).includes(id)),
      }));
      await loadData();
      try {
        await onRestored?.();
      } catch (refreshError) {
        // 恢复成功后若主页快照刷新失败，不应覆盖恢复成功结果。
        console.error(refreshError);
      }
    } catch (error: any) {
      toast.error(error?.message || '恢復資料失敗');
    } finally {
      setRestoringIds(prev => prev.filter(key => !pendingKeys.includes(key)));
    }
  };

  const handleRestoreSingle = async (item: RecycleBinItem) => {
    await restoreItems(activeTab, [getItemId(activeTab, item)]);
  };

  const handleRestoreSelected = async () => {
    const ids = currentItems
      .filter(item => selectedIds.includes(String(getItemId(activeTab, item))))
      .map(item => getItemId(activeTab, item));
    await restoreItems(activeTab, ids);
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="space-y-1">
          <h2 className="text-2xl font-bold text-slate-900">回收站</h2>
          <p className="text-sm text-slate-500">
            所有軟刪除資料都會保留在回收站中，管理員可依需求單筆或批量恢復。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => { setLoading(true); void loadData(); }}>
            <RefreshCw className="w-4 h-4" />
            重新整理
          </Button>
          <Button
            variant="success"
            disabled={selectedIds.length === 0}
            onClick={() => void handleRestoreSelected()}
          >
            <ArchiveRestore className="w-4 h-4" />
            批量恢復 {selectedIds.length > 0 ? `(${selectedIds.length})` : ''}
          </Button>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-3 xl:grid-cols-6">
        {summaryCards.map(tab => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={cn(
              'rounded-2xl border px-4 py-4 text-left transition-all',
              activeTab === tab.key
                ? 'border-blue-200 bg-blue-50 shadow-sm'
                : 'border-slate-200 bg-white hover:border-slate-300',
            )}
          >
            <p className="text-xs font-bold uppercase tracking-wider text-slate-400">{tab.label}</p>
            <p className="mt-2 text-2xl font-black text-slate-900">{tab.count}</p>
            <p className="mt-1 text-[11px] leading-5 text-slate-500">{tab.description}</p>
          </button>
        ))}
      </div>

      <Card className="overflow-hidden border-slate-200/80">
        <div className="flex flex-col gap-3 border-b border-slate-100 px-5 py-4 md:flex-row md:items-center md:justify-between">
          <div className="space-y-1">
            <p className="text-sm font-bold text-slate-900">
              {RECYCLE_BIN_TABS.find(tab => tab.key === activeTab)?.label}回收站
            </p>
            <p className="text-xs text-slate-500">
              勾選後可批量恢復；單筆也可直接使用右側按鈕恢復。
            </p>
          </div>
          <label className="inline-flex items-center gap-2 text-sm text-slate-600">
            <input
              type="checkbox"
              checked={allSelected}
              onChange={toggleSelectAll}
              disabled={currentItems.length === 0}
              className="h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
            />
            <CheckSquare className="h-4 w-4 text-slate-400" />
            全選當前類型
          </label>
        </div>

        {loading ? (
          <div className="flex items-center justify-center gap-3 px-6 py-16 text-slate-500">
            <Loader2 className="h-5 w-5 animate-spin text-blue-500" />
            載入回收站資料中...
          </div>
        ) : currentItems.length === 0 ? (
          <div className="px-6 py-16 text-center">
            <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-slate-100">
              <Trash2 className="h-7 w-7 text-slate-300" />
            </div>
            <p className="text-sm font-medium text-slate-500">目前沒有這一類的軟刪除資料</p>
          </div>
        ) : (
          <div className="divide-y divide-slate-100">
            {currentItems.map(item => {
              const itemId = getItemId(activeTab, item);
              const selectionId = String(itemId);
              const restoring = restoringIds.includes(`${activeTab}:${selectionId}`);

              return (
                <div
                  key={`${activeTab}-${selectionId}`}
                  className="flex flex-col gap-4 px-5 py-4 lg:flex-row lg:items-center lg:justify-between"
                >
                  <div className="flex min-w-0 items-start gap-3">
                    <input
                      type="checkbox"
                      checked={selectedIds.includes(selectionId)}
                      onChange={() => toggleSelection(activeTab, selectionId)}
                      className="mt-1 h-4 w-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                    />
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-bold text-slate-900">{buildItemTitle(activeTab, item)}</p>
                        <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-bold text-slate-500">
                          {buildItemIdentifier(activeTab, item)}
                        </span>
                      </div>
                      <p className="text-xs leading-5 text-slate-500">{item.secondary_text || '—'}</p>
                      <div className="flex flex-wrap items-center gap-3 text-[11px] text-slate-400">
                        <span>刪除於 {format(parseISO(item.deleted_at), 'yyyy/MM/dd HH:mm')}</span>
                      </div>
                    </div>
                  </div>

                  <Button
                    variant="outline"
                    className="whitespace-nowrap"
                    disabled={restoring}
                    onClick={() => void handleRestoreSingle(item)}
                  >
                    {restoring ? (
                      <>
                        <Loader2 className="w-4 h-4 animate-spin" />
                        恢復中...
                      </>
                    ) : (
                      <>
                        <RotateCcw className="w-4 h-4" />
                        恢復
                      </>
                    )}
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </Card>
    </div>
  );
}

function getItemId(_entityType: RecycleBinEntityType, item: RecycleBinItem): string {
  return String(item.id);
}

function buildItemIdentifier(entityType: RecycleBinEntityType, item: RecycleBinItem): string {
  switch (entityType) {
    case 'appointments':
      return `預約 #${String(item.id)}`;
    case 'customers':
      return `顧客 ID ${String(item.id)}`;
    case 'technicians':
      return `師傅 #${String(item.id)}`;
    case 'zones':
      return `區域 ID ${String(item.id)}`;
    case 'service-items':
      return `項目 ID ${String(item.id)}`;
    case 'extra-items':
      return `費用 ID ${String(item.id)}`;
  }
}

function buildItemTitle(entityType: RecycleBinEntityType, item: RecycleBinItem): string {
  switch (entityType) {
    case 'appointments':
      return item.primary_text;
    case 'customers':
      return item.primary_text;
    case 'technicians':
      return item.primary_text;
    case 'zones':
      return item.primary_text;
    case 'service-items':
      return item.primary_text;
    case 'extra-items':
      return item.primary_text;
  }
}
