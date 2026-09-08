import {
  advanceLogCursorTrail,
  buildLogCursorQuery,
  currentLogCursor,
  currentRowsBefore,
  logCursorPath,
  parseLogCursorResponse,
  rewindLogCursorTrail,
  startLogCursorTrail,
  type LogCount,
  type LogCursorFilters,
  type LogCursorTrail,
} from '@/lib/logCursor';
import { useCallback, useRef, useState } from 'react';

/**
 * Keyset pagination state for the log list (proposal W2.4).
 *
 * The hook owns the cursor trail and the capability decision; the page owns the
 * rows. Returning null from fetchPage means "this server cannot answer with a
 * cursor, use the legacy offset route" — the fallback is a decision the page
 * makes, not a silent empty result.
 */

/** LogCursorNavigation is the move the user asked for. */
export type LogCursorNavigation = 'first' | 'next' | 'previous' | 'reload';

/** LogCursorNotice reports a page the response budget cut short. */
export type LogCursorNotice = 'bytes_capped' | 'oversized_record' | null;

/** UseLogCursorPaginationOptions configures the hook. */
export interface UseLogCursorPaginationOptions {
  /** isAdminOrRoot selects the site-wide route and its extra filters. */
  isAdminOrRoot: boolean;
  filters: LogCursorFilters;
  pageSize: number;
  toUnixSeconds: (value: string) => number;
  /** get performs the HTTP request; injected so the hook stays testable. */
  get: (url: string) => Promise<{ data: unknown }>;
  /** onRestart reports that a cursor expired or stopped matching the query. */
  onRestart?: (code: string) => void;
}

/** UseLogCursorPaginationResult is the hook's public surface. */
export interface UseLogCursorPaginationResult<TRow> {
  /** supported is false once this server has proven it has no cursor capability. */
  supported: boolean;
  pageIndex: number;
  /** rowsBefore is how many rows precede the current page in the traversal. */
  rowsBefore: number;
  hasMore: boolean;
  hasPrevious: boolean;
  count: LogCount | null;
  notice: LogCursorNotice;
  /** reset returns to the first page; call whenever the query changes. */
  reset: () => void;
  /** fetchPage returns the rows for the requested move, or null to fall back. */
  fetchPage: (navigation: LogCursorNavigation) => Promise<TRow[] | null>;
}

/**
 * useLogCursorPagination drives a keyset traversal of the log list.
 *
 * @param options - the hook configuration.
 * @returns the traversal state and its navigation function.
 */
export function useLogCursorPagination<TRow>(options: UseLogCursorPaginationOptions): UseLogCursorPaginationResult<TRow> {
  const { isAdminOrRoot, filters, pageSize, toUnixSeconds, get, onRestart } = options;

  const [supported, setSupported] = useState(true);
  const [pageIndex, setPageIndex] = useState(0);
  const [rowsBefore, setRowsBefore] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [count, setCount] = useState<LogCount | null>(null);
  const [notice, setNotice] = useState<LogCursorNotice>(null);

  // The trail lives in a ref so a page request always reads the position it was
  // issued from, not the position a concurrent render had committed.
  const trailRef = useRef<LogCursorTrail>(startLogCursorTrail());
  const nextCursorRef = useRef('');
  // The row count of the page on screen, needed to place the page after it.
  const pageRowsRef = useRef(0);

  const reset = useCallback(() => {
    trailRef.current = startLogCursorTrail();
    nextCursorRef.current = '';
    pageRowsRef.current = 0;
    setPageIndex(0);
    setRowsBefore(0);
    setHasMore(false);
    setCount(null);
    setNotice(null);
  }, []);

  const fetchPage = useCallback(
    async (navigation: LogCursorNavigation): Promise<TRow[] | null> => {
      if (!supported) return null;

      let trail = trailRef.current;
      switch (navigation) {
        case 'first':
          trail = startLogCursorTrail();
          break;
        case 'next':
          trail = advanceLogCursorTrail(trail, nextCursorRef.current, pageRowsRef.current);
          break;
        case 'previous':
          trail = rewindLogCursorTrail(trail);
          break;
        case 'reload':
          break;
      }

      // One restart is attempted in place: an expired cursor is an ordinary
      // event, and making the user press the button again would tell them
      // nothing they could act on.
      for (let attempt = 0; attempt < 2; attempt++) {
        const params = buildLogCursorQuery({
          filters,
          pageSize,
          cursor: currentLogCursor(trail),
          isAdminOrRoot,
          toUnixSeconds,
        });

        let body: unknown;
        try {
          const response = await get(`${logCursorPath(isAdminOrRoot)}?${params}`);
          body = response?.data;
        } catch {
          // A transport or status failure is not proof the capability is
          // missing, but it is proof this page cannot be served by it now.
          setSupported(false);
          return null;
        }

        const outcome = parseLogCursorResponse<TRow>(body);
        if (outcome.kind === 'unsupported') {
          setSupported(false);
          return null;
        }
        if (outcome.kind === 'restart') {
          onRestart?.(outcome.code);
          trail = startLogCursorTrail();
          nextCursorRef.current = '';
          pageRowsRef.current = 0;
          continue;
        }

        trailRef.current = trail;
        nextCursorRef.current = outcome.page.nextCursor;
        pageRowsRef.current = outcome.page.items.length;
        setPageIndex(trail.index);
        setRowsBefore(currentRowsBefore(trail));
        setHasMore(outcome.page.hasMore);
        setCount(outcome.page.count);
        setNotice(outcome.page.oversizedRecord ? 'oversized_record' : outcome.page.bytesCapped ? 'bytes_capped' : null);
        return outcome.page.items;
      }

      // Two consecutive restarts mean the anchor cannot be established at all.
      setSupported(false);
      return null;
    },
    [supported, filters, pageSize, isAdminOrRoot, toUnixSeconds, get, onRestart]
  );

  return {
    supported,
    pageIndex,
    rowsBefore,
    hasMore,
    hasPrevious: pageIndex > 0,
    count,
    notice,
    reset,
    fetchPage,
  };
}
