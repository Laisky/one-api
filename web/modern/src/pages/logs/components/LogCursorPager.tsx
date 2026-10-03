import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { describeLogCount, type LogCount } from '@/lib/logCursor';
import { cn } from '@/lib/utils';
import { AlertTriangle, ChevronLeft, ChevronRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { LogCursorNotice } from '../useLogCursorPagination';

/** LogCursorPagerProps configures the keyset pager. */
export interface LogCursorPagerProps {
  /** rowsBefore is how many rows precede this page in the traversal. */
  rowsBefore: number;
  pageSize: number;
  rowCount: number;
  hasMore: boolean;
  hasPrevious: boolean;
  count: LogCount | null;
  notice: LogCursorNotice;
  loading: boolean;
  onPrevious: () => void;
  onNext: () => void;
  onPageSizeChange: (pageSize: number) => void;
  pageSizeOptions?: number[];
  className?: string;
}

/**
 * LogCursorPager renders sequential navigation for a keyset traversal.
 *
 * There is deliberately no page-number jump: a keyset cursor cannot address an
 * arbitrary page, and offering the control would either mislead or force a full
 * scan of everything before it. The range label states what the count actually
 * established rather than presenting every number as a total.
 *
 * @param props - the pager configuration.
 * @returns the pager element.
 */
export function LogCursorPager({
  rowsBefore,
  pageSize,
  rowCount,
  hasMore,
  hasPrevious,
  count,
  notice,
  loading,
  onPrevious,
  onNext,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50, 100],
  className,
}: LogCursorPagerProps) {
  const { t } = useTranslation();

  // The range is taken from the rows actually delivered, not from the page
  // size: a page cut short by the response budget would otherwise make every
  // later page claim a position it does not hold.
  const rangeStart = rowCount === 0 ? 0 : rowsBefore + 1;
  const rangeEnd = rowsBefore + rowCount;
  const label = describeLogCount(count ?? { value: null, quality: 'unavailable', asOf: 0, cached: false }, rangeStart, rangeEnd);

  return (
    <div className={cn('flex flex-col gap-3 px-2 py-3 sm:flex-row sm:items-center sm:justify-between', className)}>
      <div className="flex flex-col gap-1 text-sm text-muted-foreground">
        <span>{t(label.key, label.params)}</span>
        {count?.cached && (
          <span className="text-xs" title={t('logs.pagination.count_cached_hint')}>
            {t('logs.pagination.count_cached')}
          </span>
        )}
        {notice && (
          <span className="flex items-center gap-1 text-xs text-warning-foreground">
            <AlertTriangle className="h-3 w-3" />
            {t(notice === 'oversized_record' ? 'logs.pagination.oversized_record' : 'logs.pagination.bytes_capped')}
          </span>
        )}
      </div>

      <div className="flex items-center gap-2">
        <Select value={String(pageSize)} onValueChange={(value) => onPageSizeChange(Number(value))}>
          <SelectTrigger className="w-[110px]" aria-label={t('logs.pagination.page_size')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {pageSizeOptions.map((size) => (
              <SelectItem key={size} value={String(size)}>
                {t('logs.pagination.page_size_option', { size })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button variant="outline" size="sm" className="gap-1" onClick={onPrevious} disabled={loading || !hasPrevious}>
          <ChevronLeft className="h-4 w-4" />
          {t('logs.pagination.previous')}
        </Button>
        <Button variant="outline" size="sm" className="gap-1" onClick={onNext} disabled={loading || !hasMore}>
          {t('logs.pagination.next')}
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
