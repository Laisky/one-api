import React, { useEffect, useMemo } from 'react';
import { Pagination } from '@douyinfe/semi-ui';
import './FixedPagination.css';

const FixedPagination = ({
  currentPage,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  pageSizeOpts = [10, 20, 50, 100],
  showSizeChanger = true,
  ...props
}) => {
  // Memoize calculations to prevent unnecessary re-renders
  const totalPages = useMemo(() => Math.ceil(total / pageSize), [total, pageSize]);
  const shouldShow = useMemo(() => total > 0 && totalPages > 1, [total, totalPages]);

  useEffect(() => {
    console.log(`[Air FixedPagination] Component rendered - currentPage: ${currentPage}, pageSize: ${pageSize}, total: ${total}, totalPages: ${totalPages}`);
  }, [currentPage, pageSize, total, totalPages]);


  const handlePageChange = useMemo(() => (page, pageSize) => {
    console.log(`[Air FixedPagination] Page change - from ${currentPage} to ${page}, pageSize: ${pageSize}`);
    if (onPageChange) {
      onPageChange(page, pageSize);
    }
  }, [currentPage, onPageChange]);

  const handlePageSizeChange = useMemo(() => (currentPage, pageSize) => {
    console.log(`[Air FixedPagination] Page size change - currentPage: ${currentPage}, new pageSize: ${pageSize}`);
    if (onPageSizeChange) {
      onPageSizeChange(currentPage, pageSize);
    }
  }, [onPageSizeChange]);

  if (!shouldShow) {
    console.log(`[Air FixedPagination] Hiding pagination - total: ${total}, totalPages: ${totalPages}`);
    return null;
  }

  console.log(`[Air FixedPagination] Rendering pagination`);
  return (
    <div className="fixed-pagination-container">
      <Pagination
        currentPage={currentPage}
        pageSize={pageSize}
        total={total}
        onPageChange={handlePageChange}
        onPageSizeChange={handlePageSizeChange}
        pageSizeOpts={pageSizeOpts}
        showSizeChanger={showSizeChanger}
        showQuickJumper={false}
        size="small"
        {...props}
      />
    </div>
  );
};

export default FixedPagination;
