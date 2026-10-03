import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, MoreHorizontal } from 'lucide-react';
import * as React from 'react';
import { useTranslation } from 'react-i18next';
import './mobile-table.css';

/** AdvancedPaginationProps describes controlled page navigation and optional page-size changes. */
interface AdvancedPaginationProps {
  currentPage: number;
  totalPages: number;
  pageSize: number;
  totalItems: number;
  onPageChange: (page: number) => void;
  onPageSizeChange?: (pageSize: number) => void;
  showPageSizeSelector?: boolean;
  pageSizeOptions?: number[];
  className?: string;
  loading?: boolean;
}

/** AdvancedPagination keeps full desktop navigation and uses a compact, touch-friendly mobile control. */
export function AdvancedPagination({
  currentPage,
  totalPages,
  pageSize,
  totalItems,
  onPageChange,
  onPageSizeChange,
  showPageSizeSelector = true,
  pageSizeOptions = [10, 20, 50, 100],
  className,
  loading = false,
}: AdvancedPaginationProps) {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();

  /** getPageNumbers returns bounded desktop page links with ellipses around distant pages. */
  const getPageNumbers = () => {
    const pages: (number | 'ellipsis')[] = [];
    const maxPages = isTablet ? 5 : 7;
    if (totalPages <= maxPages) {
      for (let page = 1; page <= totalPages; page++) pages.push(page);
    } else {
      pages.push(1);
      const leftBoundary = Math.max(2, currentPage - 1);
      const rightBoundary = Math.min(totalPages - 1, currentPage + 1);
      if (leftBoundary > 2) pages.push('ellipsis');
      for (let page = leftBoundary; page <= rightBoundary; page++) pages.push(page);
      if (rightBoundary < totalPages - 1) pages.push('ellipsis');
      if (totalPages > 1) pages.push(totalPages);
    }
    return pages;
  };

  const startItem = Math.min((currentPage - 1) * pageSize + 1, totalItems);
  const endItem = Math.min(currentPage * pageSize, totalItems);
  const showing = t('common.pagination.showing', 'Showing {{start}}-{{end}} of {{total}} items', {
    start: startItem, end: endItem, total: totalItems,
  });
  const pageSizeLabel = t('common.pagination.rows_per_page', 'Rows per page:');

  /** handlePageChange delegates only valid, different page requests while the table is idle. */
  const handlePageChange = (page: number) => {
    if (Number.isInteger(page) && page >= 1 && page <= totalPages && page !== currentPage && !loading) onPageChange(page);
  };

  /** handlePageSizeChange validates the selected size and blocks stale interactions during loading. */
  const handlePageSizeChange = (value: string) => {
    const size = Number(value);
    if (!loading && onPageSizeChange && Number.isInteger(size) && size > 0 && size !== pageSize) onPageSizeChange(size);
  };

  const sizeSelector = showPageSizeSelector && onPageSizeChange ? (
    <Select value={pageSize.toString()} onValueChange={handlePageSizeChange} disabled={loading}>
      <SelectTrigger aria-label={pageSizeLabel} className={isMobile ? 'h-11 w-20' : 'h-8 w-20'}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {pageSizeOptions.map((size) => (
          <SelectItem key={size} value={size.toString()} aria-label={`${size}`}>{size}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  ) : null;

  if (totalPages <= 1 && !showPageSizeSelector) return null;

  if (isMobile) {
    return (
      <nav className={cn('mobile-pagination', className)} aria-label={t('mobile_table.pagination', 'Pagination')}>
        <div className="mobile-pagination__navigation">
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => handlePageChange(Math.min(currentPage - 1, totalPages))}
            disabled={currentPage <= 1 || totalPages < 1 || loading}
            aria-label={t('common.pagination.previous_page', 'Previous page')}
          >
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          </Button>
          <span className="mobile-pagination__position" aria-live="polite" aria-atomic="true">
            {t('mobile_table.page_status', 'Page {{page}} of {{pages}}', { page: totalPages > 0 ? currentPage : 0, pages: totalPages })}
          </span>
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => handlePageChange(currentPage + 1)}
            disabled={currentPage >= totalPages || totalPages < 1 || loading}
            aria-label={t('common.pagination.next_page', 'Next page')}
          >
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
        <div className="mobile-pagination__summary">
          <span>{showing}</span>
          {sizeSelector && (
            <div className="mobile-pagination__size">
              <span aria-hidden="true">{t('common.pagination.per_page', 'Per page:')}</span>
              {sizeSelector}
            </div>
          )}
        </div>
      </nav>
    );
  }

  return (
    <nav className={cn('flex px-2 py-2 md:py-4 items-center justify-between', className)} aria-label={t('mobile_table.pagination', 'Pagination')}>
      <div className="flex gap-4 items-center">
        <div className="text-muted-foreground text-sm">{showing}</div>
        {sizeSelector && (
          <div className="flex items-center gap-2">
            <span aria-hidden="true" className="text-muted-foreground whitespace-nowrap text-sm">{pageSizeLabel}</span>
            {sizeSelector}
          </div>
        )}
      </div>
      {totalPages > 1 && (
        <div className="flex items-center gap-1">
          <Button variant="outline" size="sm" onClick={() => handlePageChange(1)} disabled={currentPage <= 1 || loading} className="h-8 w-8 p-0 touch-target">
            <ChevronsLeft className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{t('common.pagination.first_page', 'First page')}</span>
          </Button>
          <Button variant="outline" size="sm" onClick={() => handlePageChange(Math.min(currentPage - 1, totalPages))} disabled={currentPage <= 1 || loading} className="h-8 w-8 p-0 touch-target">
            <ChevronLeft className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{t('common.pagination.previous_page', 'Previous page')}</span>
          </Button>
          {getPageNumbers().map((page, index) => (
            <React.Fragment key={index}>
              {page === 'ellipsis' ? (
                <span className="flex items-center justify-center h-8 w-8" aria-hidden="true"><MoreHorizontal className="h-4 w-4" /></span>
              ) : (
                <Button
                  variant={page === currentPage ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => handlePageChange(page)}
                  disabled={loading}
                  aria-current={page === currentPage ? 'page' : undefined}
                  aria-label={t('common.pagination.page_num', 'Page {{page}}', { page })}
                  className="h-8 p-0 touch-target w-8"
                >
                  {page}
                </Button>
              )}
            </React.Fragment>
          ))}
          <Button variant="outline" size="sm" onClick={() => handlePageChange(currentPage + 1)} disabled={currentPage >= totalPages || loading} className="h-8 w-8 p-0 touch-target">
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{t('common.pagination.next_page', 'Next page')}</span>
          </Button>
          <Button variant="outline" size="sm" onClick={() => handlePageChange(totalPages)} disabled={currentPage >= totalPages || loading} className="h-8 w-8 p-0 touch-target">
            <ChevronsRight className="h-4 w-4" aria-hidden="true" />
            <span className="sr-only">{t('common.pagination.last_page', 'Last page')}</span>
          </Button>
        </div>
      )}
    </nav>
  );
}
