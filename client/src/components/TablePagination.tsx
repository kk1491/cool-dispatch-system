import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react';

import {
  TABLE_PAGINATION_SIZE_OPTIONS,
  normalizeTablePageSize,
} from '../lib/tablePagination';
import { cn } from '../lib/utils';

interface TablePaginationProps {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  className?: string;
  itemLabel?: string;
}

// buildVisiblePages 让页码按钮在大页数时仍保持紧凑，同时保留首尾页和当前页附近的导航。
function buildVisiblePages(totalPages: number, currentPage: number): Array<number | 'ellipsis'> {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index + 1);
  }

  if (currentPage <= 4) {
    return [1, 2, 3, 4, 5, 'ellipsis', totalPages];
  }

  if (currentPage >= totalPages - 3) {
    return [1, 'ellipsis', totalPages - 4, totalPages - 3, totalPages - 2, totalPages - 1, totalPages];
  }

  return [1, 'ellipsis', currentPage - 1, currentPage, currentPage + 1, 'ellipsis', totalPages];
}

export default function TablePagination({
  page,
  pageSize,
  totalItems,
  totalPages,
  onPageChange,
  onPageSizeChange,
  className,
  itemLabel = '筆',
}: TablePaginationProps) {
  if (totalItems <= 0) {
    return null;
  }

  const normalizedPage = Math.min(Math.max(1, page), totalPages);
  const normalizedPageSize = normalizeTablePageSize(pageSize);
  const startIndex = (normalizedPage - 1) * normalizedPageSize + 1;
  const endIndex = Math.min(totalItems, normalizedPage * normalizedPageSize);
  const visiblePages = buildVisiblePages(totalPages, normalizedPage);

  return (
    <div className={cn('flex flex-col gap-3 border-t border-slate-100 bg-slate-50/60 px-4 py-3 md:flex-row md:items-center md:justify-between', className)}>
      <div className="flex flex-col gap-2 text-xs text-slate-500 sm:flex-row sm:items-center sm:gap-4">
        <span>
          顯示第 <b className="text-slate-800">{startIndex}</b> - <b className="text-slate-800">{endIndex}</b> 筆，共 <b className="text-slate-800">{totalItems}</b> {itemLabel}
        </span>
        <label className="inline-flex items-center gap-2">
          <span>每頁</span>
          <select
            value={normalizedPageSize}
            onChange={event => onPageSizeChange(Number(event.target.value))}
            className="rounded-md border border-slate-200 bg-white px-2 py-1 text-xs text-slate-700 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {TABLE_PAGINATION_SIZE_OPTIONS.map(option => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
          <span>{itemLabel}</span>
        </label>
      </div>

      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <button
          type="button"
          onClick={() => onPageChange(1)}
          disabled={normalizedPage === 1}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="第一頁"
        >
          <ChevronsLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={() => onPageChange(normalizedPage - 1)}
          disabled={normalizedPage === 1}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="上一頁"
        >
          <ChevronLeft className="h-4 w-4" />
        </button>

        {visiblePages.map((pageItem, index) => (
          pageItem === 'ellipsis' ? (
            <span key={`ellipsis-${index}`} className="px-1 text-xs text-slate-300">
              ...
            </span>
          ) : (
            <button
              key={pageItem}
              type="button"
              onClick={() => onPageChange(pageItem)}
              className={cn(
                'inline-flex h-8 min-w-8 items-center justify-center rounded-md border px-2 text-xs font-medium transition-colors',
                pageItem === normalizedPage
                  ? 'border-blue-200 bg-blue-600 text-white'
                  : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-100',
              )}
            >
              {pageItem}
            </button>
          )
        ))}

        <button
          type="button"
          onClick={() => onPageChange(normalizedPage + 1)}
          disabled={normalizedPage === totalPages}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="下一頁"
        >
          <ChevronRight className="h-4 w-4" />
        </button>
        <button
          type="button"
          onClick={() => onPageChange(totalPages)}
          disabled={normalizedPage === totalPages}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="最後一頁"
        >
          <ChevronsRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
