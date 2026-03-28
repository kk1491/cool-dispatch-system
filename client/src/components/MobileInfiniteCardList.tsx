import { ReactNode, useEffect, useRef } from 'react';
import { Loader2 } from 'lucide-react';

import { useMobileInfiniteCards } from '../lib/tablePagination';

interface MobileInfiniteCardListProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => ReactNode;
  getKey: (item: T, index: number) => string | number;
  resetDeps?: readonly unknown[];
  emptyState?: ReactNode;
  endText?: string;
  className?: string;
}

// MobileInfiniteCardList 统一封装移动端卡片列表的自动续载逻辑，避免各页面重复写观察器与首批 20 条规则。
export default function MobileInfiniteCardList<T>({
  items,
  renderItem,
  getKey,
  resetDeps = [],
  emptyState = null,
  endText = '已載入全部資料',
  className = 'space-y-3',
}: MobileInfiniteCardListProps<T>) {
  const { visibleItems, hasMore, loadMore } = useMobileInfiniteCards(items, resetDeps);
  const sentinelRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!hasMore || !sentinelRef.current || typeof IntersectionObserver === 'undefined') {
      return;
    }

    const observer = new IntersectionObserver(entries => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          loadMore();
        }
      });
    }, {
      rootMargin: '120px 0px',
    });

    observer.observe(sentinelRef.current);
    return () => {
      observer.disconnect();
    };
  }, [hasMore, loadMore, visibleItems.length]);

  if (items.length === 0) {
    return <>{emptyState}</>;
  }

  return (
    <div className={className}>
      {visibleItems.map((item, index) => (
        <div key={getKey(item, index)}>
          {renderItem(item, index)}
        </div>
      ))}

      <div ref={sentinelRef} className="flex min-h-10 items-center justify-center py-2 text-xs text-slate-400">
        {hasMore ? (
          <span className="inline-flex items-center gap-2">
            <Loader2 className="h-3.5 w-3.5 animate-spin text-blue-500" />
            下滑載入更多
          </span>
        ) : (
          <span>{endText}</span>
        )}
      </div>
    </div>
  );
}
