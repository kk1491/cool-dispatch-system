import { useEffect, useMemo, useState } from 'react';

// TABLE_PAGINATION_* 统一约束所有表格页的分页规格，避免各页面自行定义不一致的条数上限。
export const TABLE_PAGINATION_MIN_PAGE_SIZE = 20;
export const TABLE_PAGINATION_DEFAULT_PAGE_SIZE = 20;
export const TABLE_PAGINATION_MAX_PAGE_SIZE = 1000;
export const TABLE_PAGINATION_SIZE_OPTIONS = [20, 50, 100, 200, 500, 1000] as const;
export const MOBILE_CARD_BATCH_SIZE = 20;

// normalizeTablePageSize 把外部传入的条数收敛到统一规格，确保默认值、最小值、最大值始终一致。
export function normalizeTablePageSize(value?: number): number {
  if (!value || Number.isNaN(value)) {
    return TABLE_PAGINATION_DEFAULT_PAGE_SIZE;
  }

  return Math.min(
    TABLE_PAGINATION_MAX_PAGE_SIZE,
    Math.max(TABLE_PAGINATION_MIN_PAGE_SIZE, Math.floor(value)),
  );
}

// getPaginatedItems 返回当前页的数据切片，供页面直接渲染表格行。
export function getPaginatedItems<T>(items: T[], page: number, pageSize: number): T[] {
  const normalizedPageSize = normalizeTablePageSize(pageSize);
  const normalizedPage = Math.max(1, Math.floor(page));
  const startIndex = (normalizedPage - 1) * normalizedPageSize;
  return items.slice(startIndex, startIndex + normalizedPageSize);
}

// useTablePagination 封装页码、每页条数与过滤后数据切片，统一各表格页的分页行为。
export function useTablePagination<T>(items: T[], resetDeps: readonly unknown[] = []) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSizeState] = useState(TABLE_PAGINATION_DEFAULT_PAGE_SIZE);

  const normalizedPageSize = normalizeTablePageSize(pageSize);
  const totalItems = items.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / normalizedPageSize));
  const paginatedItems = useMemo(
    () => getPaginatedItems(items, page, normalizedPageSize),
    [items, page, normalizedPageSize],
  );

  useEffect(() => {
    setPage(1);
  }, resetDeps);

  useEffect(() => {
    setPage(prev => Math.min(prev, totalPages));
  }, [totalPages]);

  const setPageSize = (value: number) => {
    setPageSizeState(normalizeTablePageSize(value));
    setPage(1);
  };

  return {
    page,
    pageSize: normalizedPageSize,
    totalItems,
    totalPages,
    paginatedItems,
    setPage,
    setPageSize,
  };
}

// useMobileInfiniteCards 为移动端卡片列表提供统一的首屏 20 条与下滑续载 20 条逻辑。
export function useMobileInfiniteCards<T>(items: T[], resetDeps: readonly unknown[] = []) {
  const [visibleCount, setVisibleCount] = useState(MOBILE_CARD_BATCH_SIZE);

  useEffect(() => {
    setVisibleCount(MOBILE_CARD_BATCH_SIZE);
  }, resetDeps);

  const visibleItems = useMemo(
    () => items.slice(0, visibleCount),
    [items, visibleCount],
  );
  const hasMore = visibleItems.length < items.length;

  const loadMore = () => {
    setVisibleCount(prev => Math.min(prev + MOBILE_CARD_BATCH_SIZE, items.length));
  };

  return {
    visibleCount,
    visibleItems,
    hasMore,
    loadMore,
  };
}
