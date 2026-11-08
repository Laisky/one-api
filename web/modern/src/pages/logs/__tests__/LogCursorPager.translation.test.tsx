import { type ComponentProps } from 'react';
import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';

import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import en from '@/i18n/locales/en';
import { LogCursorPager } from '../components/LogCursorPager';

it('updates a translated cursor warning when the response limit reason changes', () => {
  const onCaughtError = vi.fn();
  const props: ComponentProps<typeof LogCursorPager> = {
    rowsBefore: 0,
    pageSize: 20,
    rowCount: 1,
    hasMore: true,
    hasPrevious: false,
    count: null,
    notice: 'bytes_capped',
    loading: false,
    onPrevious: vi.fn(),
    onNext: vi.fn(),
    onPageSizeChange: vi.fn(),
  };
  const { rerender, unmount } = render(
    <DomMutationBoundary>
      <LogCursorPager {...props} />
    </DomMutationBoundary>,
    { onCaughtError }
  );
  const warning = screen.getByText(en.logs.pagination.bytes_capped);
  expect(translateText(warning)).toBe(1);

  rerender(
    <DomMutationBoundary>
      <LogCursorPager {...props} notice="oversized_record" />
    </DomMutationBoundary>
  );

  expect(onCaughtError.mock.calls.length).toBe(0);
  expect(screen.queryByText(en.logs.pagination.oversized_record)).toBeInTheDocument();
  expect(screen.queryByText(/Translated/)).not.toBeInTheDocument();

  rerender(
    <DomMutationBoundary>
      <LogCursorPager {...props} notice={null} />
    </DomMutationBoundary>
  );
  expect(screen.queryByText(en.logs.pagination.oversized_record)).not.toBeInTheDocument();
  unmount();
  expect(onCaughtError.mock.calls.length).toBe(0);
});
