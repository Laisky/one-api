/**
 * Client for the additive keyset log routes (proposal
 * docs/proposals/20260905_observability-data-tiering.md, W2.4).
 *
 * Modern uses the cursor capability explicitly. Offset pagination stays on the
 * legacy routes for the other frontends and as this one's fallback, so a server
 * that has the capability turned off, or an older server that has never heard
 * of it, still serves this page.
 *
 * Two properties are deliberate:
 *
 * - Keyset pagination cannot jump to an arbitrary page, so navigation is a
 *   trail of visited cursors rather than a page number. Offering a page-number
 *   jump would either be a lie or a full table walk.
 * - A count carries its own quality. A bounded probe that stopped at its limit
 *   establishes a lower bound, not a total, and an exhausted budget establishes
 *   nothing at all. Neither is rendered as if it were a total.
 */

/** LOG_CURSOR_VERSION is the capability version this client negotiates. */
export const LOG_CURSOR_VERSION = 1;

/** LogCountQuality states how much a reported count actually establishes. */
export type LogCountQuality = 'exact' | 'lower_bound' | 'estimate' | 'unavailable';

/** LogCount is a count together with what it establishes and when it was taken. */
export interface LogCount {
  /** value is null when nothing was established; it is never rendered as zero. */
  value: number | null;
  quality: LogCountQuality;
  /** asOf is the Unix second at which the count ran, not the time of display. */
  asOf: number;
  /** cached marks a count reused from an earlier identical request. */
  cached: boolean;
}

/** LogCursorPage is one page of a keyset traversal. */
export interface LogCursorPage<TRow> {
  items: TRow[];
  hasMore: boolean;
  nextCursor: string;
  count: LogCount;
  /** bytesCapped means the page stopped early to stay under the response budget. */
  bytesCapped: boolean;
  /** oversizedRecord means one record alone exceeded the whole budget. */
  oversizedRecord: boolean;
}

/**
 * LogCursorOutcome distinguishes the three things the route can say: here is a
 * page, your cursor is no longer usable so start over, or this server does not
 * offer the capability.
 */
export type LogCursorOutcome<TRow> =
  { kind: 'page'; page: LogCursorPage<TRow> } | { kind: 'restart'; code: string } | { kind: 'unsupported' };

/** UNAVAILABLE_COUNT is the count used when a response carries none. */
const UNAVAILABLE_COUNT: LogCount = { value: null, quality: 'unavailable', asOf: 0, cached: false };

/**
 * Codes the route returns when a cursor cannot be honoured and the caller must
 * restart. They mirror `logcursor.RejectReason`; anything else is recognised by
 * the `restart_required` flag instead, so a new server-side code still works.
 */
const RESTART_CODES = new Set(['cursor_invalid', 'cursor_expired', 'cursor_query_changed']);

/** LogCursorFilters are the user-selected filters shared with the legacy list. */
export interface LogCursorFilters {
  type: string;
  model_name: string;
  token_name: string;
  username: string;
  channel: string;
  start_timestamp: string;
  end_timestamp: string;
}

/** LogCursorQueryInput describes one page request. */
export interface LogCursorQueryInput {
  filters: LogCursorFilters;
  pageSize: number;
  cursor: string;
  /** isAdminOrRoot gates the site-wide-only filters, matching the server scope. */
  isAdminOrRoot: boolean;
  /** toUnixSeconds converts a datetime-local field into a Unix second. */
  toUnixSeconds: (value: string) => number;
}

/**
 * buildLogCursorQuery renders the query string for a cursor page request.
 *
 * The filters match the legacy list exactly so both routes select the same
 * rows; only the pagination differs.
 *
 * @param input - the page request.
 * @returns the query parameters, without a leading '?'.
 */
export function buildLogCursorQuery(input: LogCursorQueryInput): URLSearchParams {
  const { filters, isAdminOrRoot, toUnixSeconds } = input;
  const params = new URLSearchParams();

  params.set('v', String(LOG_CURSOR_VERSION));
  params.set('size', String(input.pageSize));
  if (input.cursor) params.set('cursor', input.cursor);

  if (filters.type !== '0') params.set('type', filters.type);
  if (filters.model_name) params.set('model_name', filters.model_name);
  if (filters.token_name) params.set('token_name', filters.token_name);
  if (isAdminOrRoot && filters.username) params.set('username', filters.username);
  if (isAdminOrRoot && filters.channel) params.set('channel', filters.channel);
  if (filters.start_timestamp) params.set('start_timestamp', String(toUnixSeconds(filters.start_timestamp)));
  if (filters.end_timestamp) params.set('end_timestamp', String(toUnixSeconds(filters.end_timestamp)));

  return params;
}

/**
 * logCursorPath returns the route for the caller's scope.
 *
 * @param isAdminOrRoot - whether the caller may list site-wide.
 * @returns the request path.
 */
export function logCursorPath(isAdminOrRoot: boolean): string {
  return isAdminOrRoot ? '/api/log/cursor' : '/api/log/self/cursor';
}

/**
 * parseLogCursorResponse interprets a cursor route response body.
 *
 * An unrecognized or unsuccessful body is reported as unsupported rather than
 * as an empty page, so the caller falls back instead of showing "no logs" for
 * what is really a server that cannot answer.
 *
 * @param body - the decoded response body.
 * @returns what the server said.
 */
export function parseLogCursorResponse<TRow>(body: unknown): LogCursorOutcome<TRow> {
  if (!body || typeof body !== 'object') return { kind: 'unsupported' };
  const payload = body as Record<string, unknown>;

  if (payload.success !== true) {
    const code = typeof payload.code === 'string' ? payload.code : '';
    if (payload.restart_required === true || RESTART_CODES.has(code)) {
      return { kind: 'restart', code: code || 'cursor_invalid' };
    }
    return { kind: 'unsupported' };
  }

  // A successful body without the version marker is not this capability.
  if (payload.version !== LOG_CURSOR_VERSION) return { kind: 'unsupported' };
  if (!Array.isArray(payload.data)) return { kind: 'unsupported' };

  return {
    kind: 'page',
    page: {
      items: payload.data as TRow[],
      hasMore: payload.has_more === true,
      nextCursor: typeof payload.next_cursor === 'string' ? payload.next_cursor : '',
      count: parseLogCount(payload.count),
      bytesCapped: payload.bytes_capped === true,
      oversizedRecord: payload.oversized_record === true,
    },
  };
}

/**
 * parseLogCount reads the count object defensively.
 *
 * @param raw - the count field of a response.
 * @returns the count, or an unavailable count when the field is unusable.
 */
function parseLogCount(raw: unknown): LogCount {
  if (!raw || typeof raw !== 'object') return UNAVAILABLE_COUNT;
  const source = raw as Record<string, unknown>;

  const quality = source.quality;
  if (quality !== 'exact' && quality !== 'lower_bound' && quality !== 'estimate' && quality !== 'unavailable') {
    return UNAVAILABLE_COUNT;
  }

  const value = typeof source.value === 'number' && Number.isFinite(source.value) ? source.value : null;
  return {
    // An unavailable count never carries a number, whatever the body claimed.
    value: quality === 'unavailable' ? null : value,
    quality,
    asOf: typeof source.as_of === 'number' ? source.as_of : 0,
    cached: source.cached === true,
  };
}

/** LogCursorTrail records the cursors of the pages visited since the last reset. */
export interface LogCursorTrail {
  /** cursors[i] is the cursor that produced page i; cursors[0] is always ''. */
  cursors: string[];
  /**
   * rowsBefore[i] is how many rows precede page i in the traversal.
   *
   * It is accumulated from the rows actually delivered rather than derived from
   * index * pageSize, because a keyset page is not always full: the response
   * byte budget can cut one short, and deriving the position from the page size
   * would then mislabel every page after it.
   */
  rowsBefore: number[];
  /** index is the page currently displayed. */
  index: number;
}

/**
 * startLogCursorTrail returns a trail positioned at the first page.
 *
 * @returns a fresh trail.
 */
export function startLogCursorTrail(): LogCursorTrail {
  return { cursors: [''], rowsBefore: [0], index: 0 };
}

/**
 * advanceLogCursorTrail moves the trail forward one page.
 *
 * A page already visited reuses its recorded cursor, so moving back and forward
 * again lands on the same rows rather than re-deriving a new anchor.
 *
 * @param trail - the current trail.
 * @param nextCursor - the cursor the current page returned.
 * @param rowsOnCurrentPage - how many rows the current page actually delivered.
 * @returns the trail positioned at the next page, or the input when there is none.
 */
export function advanceLogCursorTrail(trail: LogCursorTrail, nextCursor: string, rowsOnCurrentPage: number): LogCursorTrail {
  const nextIndex = trail.index + 1;
  if (nextIndex < trail.cursors.length) {
    return { cursors: trail.cursors, rowsBefore: trail.rowsBefore, index: nextIndex };
  }
  if (!nextCursor) return trail;
  return {
    cursors: [...trail.cursors.slice(0, nextIndex), nextCursor],
    rowsBefore: [...trail.rowsBefore.slice(0, nextIndex), (trail.rowsBefore[trail.index] ?? 0) + rowsOnCurrentPage],
    index: nextIndex,
  };
}

/**
 * rewindLogCursorTrail moves the trail back one page.
 *
 * @param trail - the current trail.
 * @returns the trail positioned at the previous page, or the input when at the first.
 */
export function rewindLogCursorTrail(trail: LogCursorTrail): LogCursorTrail {
  if (trail.index <= 0) return trail;
  return { cursors: trail.cursors, rowsBefore: trail.rowsBefore, index: trail.index - 1 };
}

/**
 * currentLogCursor returns the cursor for the trail's current page.
 *
 * @param trail - the trail.
 * @returns the cursor, empty for the first page.
 */
export function currentLogCursor(trail: LogCursorTrail): string {
  return trail.cursors[trail.index] ?? '';
}

/**
 * currentRowsBefore returns how many rows precede the trail's current page.
 *
 * @param trail - the trail.
 * @returns the number of preceding rows, 0 on the first page.
 */
export function currentRowsBefore(trail: LogCursorTrail): number {
  return trail.rowsBefore[trail.index] ?? 0;
}

/** LogCountLabel is an i18n key with its interpolation values. */
export interface LogCountLabel {
  key: string;
  params: Record<string, unknown>;
}

/**
 * describeLogCount maps a count onto the phrase that states exactly what it
 * establishes.
 *
 * A lower bound reads as "at least N", never as "N" and never as "N+", because
 * the second claims a total the probe did not establish and the third reads as
 * an estimate. An unavailable count says so instead of showing a number.
 *
 * @param count - the count to describe.
 * @param rangeStart - the 1-based index of the first row on the page.
 * @param rangeEnd - the 1-based index of the last row on the page.
 * @returns the translation key and its parameters.
 */
export function describeLogCount(count: LogCount, rangeStart: number, rangeEnd: number): LogCountLabel {
  const params: Record<string, unknown> = { from: rangeStart, to: rangeEnd, count: count.value ?? 0 };

  if (count.value === null || count.quality === 'unavailable') {
    return { key: 'logs.pagination.range_unknown_total', params };
  }
  if (count.quality === 'exact') {
    return { key: 'logs.pagination.range_exact_total', params };
  }
  return { key: 'logs.pagination.range_lower_bound_total', params };
}
