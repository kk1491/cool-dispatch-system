import { useState, useRef, useEffect } from 'react';
import { MessageSquare, Search, UserCheck, UserPlus, Copy, Check, X } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { zhTW } from 'date-fns/locale';
import { cn } from '../lib/utils';
import { toast } from 'react-hot-toast';
import { Card } from './shared';
import { LineFriend, Customer } from '../types';

interface LineDataViewProps {
  lineFriends: LineFriend[];
  customers: Customer[];
  onLinkCustomer?: (lineUid: string, customerId: string | null) => Promise<void>;
}

export default function LineDataView({ lineFriends, customers, onLinkCustomer }: LineDataViewProps) {
  const [search, setSearch] = useState('');
  const [copiedUid, setCopiedUid] = useState<string | null>(null);
  const [linkingUid, setLinkingUid] = useState<string | null>(null);

  const filtered = lineFriends.filter(f =>
    f.line_name.toLowerCase().includes(search.toLowerCase()) ||
    f.line_uid.toLowerCase().includes(search.toLowerCase()) ||
    (f.phone && f.phone.includes(search))
  );

  const linkedCount = lineFriends.filter(f => f.linked_customer_id).length;
  const unlinkedCount = lineFriends.length - linkedCount;

  const handleCopyUid = (uid: string) => {
    navigator.clipboard.writeText(uid).then(() => {
      setCopiedUid(uid);
      toast.success('已複製 UID');
      setTimeout(() => setCopiedUid(null), 2000);
    });
  };

  const getLinkedCustomer = (f: LineFriend) => {
    if (!f.linked_customer_id) return null;
    return customers.find(c => c.id === f.linked_customer_id) || null;
  };

  // handleLinkCustomer 统一处理绑定/解绑动作，并在按钮层展示进行中状态，避免重复提交。
  const handleLinkCustomer = async (lineUid: string, customerId: string | null) => {
    if (!onLinkCustomer) return;
    setLinkingUid(lineUid);
    try {
      await onLinkCustomer(lineUid, customerId);
    } finally {
      setLinkingUid(current => current === lineUid ? null : current);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold flex items-center gap-2">
          <MessageSquare className="w-6 h-6 text-[#06C755]" /> LINE 好友
        </h2>
        <div className="flex gap-3">
          <div className="flex items-center gap-1.5 text-xs text-slate-500 bg-slate-50 px-3 py-1.5 rounded-md" data-testid="stat-line-total">
            <UserPlus className="w-3.5 h-3.5" /> 共 {lineFriends.length} 位好友
          </div>
          <div className="flex items-center gap-1.5 text-xs text-emerald-600 bg-emerald-50 px-3 py-1.5 rounded-md" data-testid="stat-line-linked">
            <UserCheck className="w-3.5 h-3.5" /> 已綁定 {linkedCount}
          </div>
          {unlinkedCount > 0 && (
            <div className="flex items-center gap-1.5 text-xs text-amber-600 bg-amber-50 px-3 py-1.5 rounded-md" data-testid="stat-line-unlinked">
              未綁定 {unlinkedCount}
            </div>
          )}
        </div>
      </div>

      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400" />
        <input
          data-testid="input-search-line"
          className="w-full pl-10 pr-4 py-3 rounded-md border border-slate-200 text-sm focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          placeholder="搜尋 LINE 姓名、UID 或電話..."
          value={search}
          onChange={e => setSearch(e.target.value)}
        />
      </div>

      {filtered.length === 0 ? (
        <Card className="p-12 text-center text-slate-400">
          <MessageSquare className="w-12 h-12 mx-auto mb-4 opacity-20" />
          <p>{search ? '沒有找到符合的好友' : '目前尚無 LINE 好友'}</p>
        </Card>
      ) : (
        <div className="space-y-2">
          {filtered.map(friend => {
            const linked = getLinkedCustomer(friend);
            return (
              <Card 
                key={friend.line_uid}
                className="p-4 flex items-center gap-4 hover:shadow-md transition-shadow"
                data-testid={`line-friend-${friend.line_uid}`}
              >
                <img
                  src={friend.line_picture}
                  alt={friend.line_name}
                  className="w-12 h-12 rounded-full bg-slate-100 flex-shrink-0"
                  data-testid={`img-line-avatar-${friend.line_uid}`}
                />

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-slate-900 truncate" data-testid={`text-line-name-${friend.line_uid}`}>
                      {friend.line_name}
                    </span>
                    {linked && (
                      <span className="text-[10px] bg-emerald-50 text-emerald-600 border border-emerald-200 px-1.5 py-0.5 rounded-md font-medium flex items-center gap-0.5">
                        <UserCheck className="w-2.5 h-2.5" /> 已綁定
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-3 mt-1 text-xs text-slate-400">
                    <button
                      onClick={() => handleCopyUid(friend.line_uid)}
                      className="flex items-center gap-1 hover:text-slate-600 transition-colors font-mono"
                      data-testid={`button-copy-uid-${friend.line_uid}`}
                    >
                      {copiedUid === friend.line_uid ? (
                        <Check className="w-3 h-3 text-emerald-500" />
                      ) : (
                        <Copy className="w-3 h-3" />
                      )}
                      {friend.line_uid}
                    </button>
                    <span>加入 {format(parseISO(friend.line_joined_at || friend.joined_at), 'yyyy/MM/dd', { locale: zhTW })}</span>
                    {friend.status && (
                      <span className="px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">{friend.status}</span>
                    )}
                  </div>
                </div>

                <div className="text-right flex-shrink-0 space-y-2">
                  {linked ? (
                    <div className="text-xs text-slate-500">
                      <div className="font-medium text-slate-700">{linked.name}</div>
                      <div>{linked.phone}</div>
                    </div>
                  ) : (
                    <span className="text-xs text-amber-500 bg-amber-50 px-2 py-1 rounded-md">
                      未綁定客戶
                    </span>
                  )}
                  {onLinkCustomer && (
                    <div>
                      <select
                        data-testid={`select-link-customer-${friend.line_uid}`}
                        className="min-w-44 rounded-md border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                        value={friend.linked_customer_id || ''}
                        disabled={linkingUid === friend.line_uid}
                        onChange={(e) => {
                          const value = e.target.value || null;
                          void handleLinkCustomer(friend.line_uid, value);
                        }}
                      >
                        <option value="">未綁定客戶</option>
                        {customers.map(customer => (
                          <option key={customer.id} value={customer.id}>
                            {customer.name} / {customer.phone}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

interface LineFriendPickerProps {
  lineFriends: LineFriend[];
  selectedUid: string;
  onSelect: (friend: LineFriend | null) => void;
}

export function LineFriendPicker({ lineFriends, selectedUid, onSelect }: LineFriendPickerProps) {
  const [search, setSearch] = useState('');
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const selected = selectedUid ? lineFriends.find(f => f.line_uid === selectedUid) : null;

  const q = search.toLowerCase();
  const filtered = q
    ? lineFriends.filter(f =>
        f.line_name.toLowerCase().includes(q) ||
        f.line_uid.toLowerCase().includes(q) ||
        (f.phone && f.phone.includes(q))
      )
    : lineFriends;

  if (selected) {
    return (
      <div className="flex items-center gap-3 px-3 py-2 bg-[#06C755]/5 border border-[#06C755]/20 rounded-md" data-testid="line-friend-selected">
        <img src={selected.line_picture} alt="" className="w-8 h-8 rounded-full bg-slate-100 flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="text-sm font-medium text-slate-900 truncate">{selected.line_name}</div>
          <div className="text-[10px] text-slate-400 font-mono">{selected.line_uid}</div>
        </div>
        <button
          type="button"
          onClick={() => { onSelect(null); setSearch(''); }}
          className="text-slate-400 hover:text-red-500 transition-colors p-1"
          data-testid="button-clear-line-friend"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    );
  }

  return (
    <div ref={containerRef} className="relative">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-400" />
        <input
          data-testid="input-line-friend-search"
          className="w-full pl-9 pr-4 py-3 text-sm bg-white border border-slate-200 rounded-md focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          placeholder="搜尋 LINE 好友姓名或 UID..."
          value={search}
          onChange={e => { setSearch(e.target.value); setIsOpen(true); }}
          onFocus={() => setIsOpen(true)}
        />
      </div>

      {isOpen && (
        <div className="absolute z-50 top-full left-0 right-0 mt-1 bg-white border border-slate-200 rounded-lg shadow-xl max-h-60 overflow-y-auto" data-testid="dropdown-line-friends">
          {filtered.length === 0 && (
            <div className="p-4 text-center text-xs text-slate-400">沒有找到 LINE 好友</div>
          )}
          {filtered.map(f => (
            <button
              key={f.line_uid}
              type="button"
              className="w-full px-3 py-2.5 flex items-center gap-3 hover:bg-green-50 transition-colors text-left"
              onClick={() => { onSelect(f); setSearch(''); setIsOpen(false); }}
              data-testid={`dropdown-line-${f.line_uid}`}
            >
              <img src={f.line_picture} alt="" className="w-8 h-8 rounded-full bg-slate-100 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium text-slate-900 truncate">{f.line_name}</div>
                <div className="text-[10px] text-slate-400 font-mono">{f.line_uid}</div>
              </div>
              {f.linked_customer_id ? (
                <span className="text-[9px] bg-emerald-50 text-emerald-600 border border-emerald-200 px-1.5 py-0.5 rounded font-medium flex-shrink-0 flex items-center gap-0.5">
                  <UserCheck className="w-2.5 h-2.5" /> 已綁定
                </span>
              ) : (
                <span className="text-[9px] bg-amber-50 text-amber-600 px-1.5 py-0.5 rounded font-medium flex-shrink-0">新好友</span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
