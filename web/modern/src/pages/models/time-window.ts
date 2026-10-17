/**
 * Client-side mirror of the backend time-of-day pricing matcher
 * (relay/pricing/timewindow.go) so the UI can highlight the window that is
 * active right now instead of the one that was active when the page loaded.
 */

/** Clock range of a pricing window, expressed as HH:MM in the window timezone. */
export interface TimeWindowClockRange {
  start: string;
  end: string;
}

/** Schedule part of a pricing window; pricing overlays are irrelevant for matching. */
export interface TimeWindowSchedule {
  timezone?: string;
  ranges: TimeWindowClockRange[];
  days_of_week?: number[];
  date_from?: string;
  date_to?: string;
}

/** No window matches the evaluated instant. */
export const NO_ACTIVE_TIME_WINDOW = -1;

/** No window could be evaluated (e.g. the runtime lacks the requested timezones). */
export const UNRESOLVED_TIME_WINDOWS = -2;

type TimeWindowMatch = 'match' | 'miss' | 'error';

interface ZonedParts {
  year: number;
  month: number;
  day: number;
  weekday: number;
  minuteOfDay: number;
}

const zonedFormatterCache = new Map<string, Intl.DateTimeFormat | null>();

/**
 * resolveActiveTimeWindowIndex finds the first window matching an instant.
 * Parameters: windows is the ordered window list and at is the instant to evaluate.
 * Returns: the matching index, NO_ACTIVE_TIME_WINDOW when nothing matches, or
 * UNRESOLVED_TIME_WINDOWS when every window failed to evaluate.
 */
export function resolveActiveTimeWindowIndex(windows: TimeWindowSchedule[] | undefined, at: Date): number {
  if (!windows || windows.length === 0) {
    return NO_ACTIVE_TIME_WINDOW;
  }
  let evaluated = false;
  for (let index = 0; index < windows.length; index++) {
    const result = matchTimeWindow(windows[index], at);
    if (result === 'error') {
      continue;
    }
    evaluated = true;
    if (result === 'match') {
      return index;
    }
  }
  return evaluated ? NO_ACTIVE_TIME_WINDOW : UNRESOLVED_TIME_WINDOWS;
}

/**
 * matchTimeWindow reports whether a window covers an instant.
 * Parameters: window is the pricing window and at is the instant to evaluate.
 * Returns: 'match' when date, weekday, and clock ranges all match, 'miss' when
 * they do not, and 'error' when the window definition cannot be evaluated.
 */
export function matchTimeWindow(window: TimeWindowSchedule, at: Date): TimeWindowMatch {
  const parts = getZonedParts(at, window.timezone || 'UTC');
  if (!parts) {
    return 'error';
  }

  const localDate = parts.year * 10000 + parts.month * 100 + parts.day;
  if (window.date_from) {
    const from = parseDateNumber(window.date_from);
    if (from === null) {
      return 'error';
    }
    if (localDate < from) {
      return 'miss';
    }
  }
  if (window.date_to) {
    const to = parseDateNumber(window.date_to);
    if (to === null) {
      return 'error';
    }
    // The backend treats date_to as exclusive.
    if (localDate >= to) {
      return 'miss';
    }
  }

  if (window.days_of_week && window.days_of_week.length > 0 && !window.days_of_week.includes(parts.weekday)) {
    return 'miss';
  }

  if (!window.ranges || window.ranges.length === 0) {
    return 'miss';
  }
  for (const range of window.ranges) {
    const start = parseClockMinutes(range.start);
    const end = parseClockMinutes(range.end);
    if (start === null || end === null) {
      return 'error';
    }
    if (start === end) {
      // A zero-length range means the window covers the whole day.
      return 'match';
    }
    if (start < end) {
      if (parts.minuteOfDay >= start && parts.minuteOfDay < end) {
        return 'match';
      }
      continue;
    }
    // Range wraps past midnight.
    if (parts.minuteOfDay >= start || parts.minuteOfDay < end) {
      return 'match';
    }
  }
  return 'miss';
}

/**
 * timeWindowScheduleSignature builds a stable key for a window list schedule.
 * Parameters: windows is the window list rendered by the pricing modal.
 * Returns: a string that changes only when a schedule field changes, so effects
 * keyed on it do not restart on unrelated re-renders.
 */
export function timeWindowScheduleSignature(windows: TimeWindowSchedule[] | undefined): string {
  if (!windows || windows.length === 0) {
    return '';
  }
  return JSON.stringify(
    windows.map((window) => [
      window.timezone || '',
      (window.ranges || []).map((range) => `${range.start}-${range.end}`),
      window.days_of_week || [],
      window.date_from || '',
      window.date_to || '',
    ])
  );
}

/** TimeWindowSnapshot shares one instant for active highlighting and historical disclosure. */
export interface TimeWindowSnapshot {
  activeIndex: number;
  historicalIndices: number[];
}

/** resolveTimeWindowSnapshot preserves source indices and existing first-match precedence. */
export function resolveTimeWindowSnapshot(windows: TimeWindowSchedule[] | undefined, at: Date): TimeWindowSnapshot {
  const historicalIndices: number[] = [];
  windows?.forEach((window, index) => {
    if (isHistoricalTimeWindow(window, at)) historicalIndices.push(index);
  });
  return { activeIndex: resolveActiveTimeWindowIndex(windows, at), historicalIndices };
}

/** isHistoricalTimeWindow hides only valid schedules whose exclusive local date_to has passed. */
export function isHistoricalTimeWindow(window: TimeWindowSchedule, at: Date): boolean {
  if (!Number.isFinite(at.getTime()) || !window.date_to) return false;
  const to = parseHistoryDate(window.date_to);
  const from = window.date_from ? parseHistoryDate(window.date_from) : null;
  if (to === null || (window.date_from && (from === null || from >= to))) return false;
  if (
    !window.ranges?.length ||
    window.ranges.some((range) => !range || parseClockMinutes(range.start) === null || parseClockMinutes(range.end) === null)
  )
    return false;
  if (window.days_of_week?.some((day) => !Number.isInteger(day) || day < 0 || day > 6)) return false;
  const parts = getZonedParts(at, window.timezone || 'UTC');
  return parts !== null && parts.year * 10000 + parts.month * 100 + parts.day >= to;
}

/** parseHistoryDate validates calendar dates without changing the billing-mirror matcher. */
function parseHistoryDate(value: string): number | null {
  const parsed = parseDateNumber(value);
  if (parsed === null) return null;
  const year = Math.floor(parsed / 10000);
  const month = Math.floor(parsed / 100) % 100;
  const day = parsed % 100;
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return month >= 1 && month <= 12 && day >= 1 && day <= days[month - 1] ? parsed : null;
}

function getZonedParts(at: Date, timeZone: string): ZonedParts | null {
  const formatter = getZonedFormatter(timeZone);
  if (!formatter) {
    return null;
  }
  const parts = formatter.formatToParts(at);
  const read = (type: Intl.DateTimeFormatPartTypes): number => {
    const part = parts.find((candidate) => candidate.type === type);
    return part ? Number(part.value) : Number.NaN;
  };
  const year = read('year');
  const month = read('month');
  const day = read('day');
  const hour = read('hour');
  const minute = read('minute');
  if ([year, month, day, hour, minute].some((value) => !Number.isFinite(value))) {
    return null;
  }
  // Some engines render midnight as hour 24 under hour12:false.
  const normalizedHour = hour % 24;
  return {
    year,
    month,
    day,
    weekday: new Date(Date.UTC(year, month - 1, day)).getUTCDay(),
    minuteOfDay: normalizedHour * 60 + minute,
  };
}

function getZonedFormatter(timeZone: string): Intl.DateTimeFormat | null {
  if (zonedFormatterCache.has(timeZone)) {
    return zonedFormatterCache.get(timeZone) ?? null;
  }
  let formatter: Intl.DateTimeFormat | null = null;
  try {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hour12: false,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    formatter = null;
  }
  zonedFormatterCache.set(timeZone, formatter);
  return formatter;
}

function parseDateNumber(value: string): number | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) {
    return null;
  }
  return Number(match[1]) * 10000 + Number(match[2]) * 100 + Number(match[3]);
}

function parseClockMinutes(value: string): number | null {
  const match = /^(\d{2}):(\d{2})$/.exec(value);
  if (!match) {
    return null;
  }
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) {
    return null;
  }
  return hour * 60 + minute;
}
