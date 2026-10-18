import { describe, expect, it } from 'vitest';

import {
  NO_ACTIVE_TIME_WINDOW,
  UNRESOLVED_TIME_WINDOWS,
  matchTimeWindow,
  isHistoricalTimeWindow,
  resolveTimeWindowSnapshot,
  resolveActiveTimeWindowIndex,
  timeWindowScheduleSignature,
  type TimeWindowSchedule,
} from './time-window';

// Mirrors the DeepSeek V4 off-peak / peak pair shipped by the backend.
const deepseekWindows: TimeWindowSchedule[] = [
  {
    timezone: 'Asia/Shanghai',
    date_from: '2026-08-17',
    ranges: [
      { start: '18:00', end: '09:00' },
      { start: '12:00', end: '14:00' },
    ],
  },
  {
    timezone: 'Asia/Shanghai',
    date_from: '2026-08-17',
    ranges: [
      { start: '09:00', end: '12:00' },
      { start: '14:00', end: '18:00' },
    ],
  },
];

// Asia/Shanghai is UTC+8 year round.
function shanghai(day: number, hour: number, minute = 0): Date {
  return new Date(Date.UTC(2026, 7, day, hour - 8, minute));
}

describe('resolveActiveTimeWindowIndex', () => {
  it('selects the off-peak window inside a wrapping range', () => {
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 20))).toBe(0);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 3))).toBe(0);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 13))).toBe(0);
  });

  it('selects the peak window inside a same-day range', () => {
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 9))).toBe(1);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 11, 59))).toBe(1);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 17, 59))).toBe(1);
  });

  it('switches at the range boundary', () => {
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 8, 59))).toBe(0);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 9, 0))).toBe(1);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 17, 59))).toBe(1);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(19, 18, 0))).toBe(0);
  });

  it('honours the date_from lower bound', () => {
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(16, 20))).toBe(NO_ACTIVE_TIME_WINDOW);
    expect(resolveActiveTimeWindowIndex(deepseekWindows, shanghai(17, 20))).toBe(0);
  });

  it('returns the first match when windows overlap', () => {
    const overlapping: TimeWindowSchedule[] = [
      { timezone: 'UTC', ranges: [{ start: '00:00', end: '12:00' }] },
      { timezone: 'UTC', ranges: [{ start: '06:00', end: '18:00' }] },
    ];
    expect(resolveActiveTimeWindowIndex(overlapping, new Date(Date.UTC(2026, 7, 19, 7)))).toBe(0);
    expect(resolveActiveTimeWindowIndex(overlapping, new Date(Date.UTC(2026, 7, 19, 13)))).toBe(1);
  });

  it('reports no active window for empty input', () => {
    expect(resolveActiveTimeWindowIndex(undefined, new Date())).toBe(NO_ACTIVE_TIME_WINDOW);
    expect(resolveActiveTimeWindowIndex([], new Date())).toBe(NO_ACTIVE_TIME_WINDOW);
  });

  it('reports unresolved when no window can be evaluated', () => {
    const broken: TimeWindowSchedule[] = [{ timezone: 'Not/AZone', ranges: [{ start: '00:00', end: '12:00' }] }];
    expect(resolveActiveTimeWindowIndex(broken, new Date())).toBe(UNRESOLVED_TIME_WINDOWS);
  });

  it('still evaluates valid windows when a sibling is broken', () => {
    const mixed: TimeWindowSchedule[] = [
      { timezone: 'Not/AZone', ranges: [{ start: '00:00', end: '23:59' }] },
      { timezone: 'UTC', ranges: [{ start: '06:00', end: '18:00' }] },
    ];
    expect(resolveActiveTimeWindowIndex(mixed, new Date(Date.UTC(2026, 7, 19, 7)))).toBe(1);
    expect(resolveActiveTimeWindowIndex(mixed, new Date(Date.UTC(2026, 7, 19, 19)))).toBe(NO_ACTIVE_TIME_WINDOW);
  });
});

describe('matchTimeWindow', () => {
  it('treats a zero-length range as a full day', () => {
    expect(matchTimeWindow({ timezone: 'UTC', ranges: [{ start: '00:00', end: '00:00' }] }, new Date(Date.UTC(2026, 7, 19, 3)))).toBe(
      'match'
    );
  });

  it('treats date_to as exclusive', () => {
    const window: TimeWindowSchedule = {
      timezone: 'UTC',
      date_to: '2026-08-19',
      ranges: [{ start: '00:00', end: '00:00' }],
    };
    expect(matchTimeWindow(window, new Date(Date.UTC(2026, 7, 18, 23, 59)))).toBe('match');
    expect(matchTimeWindow(window, new Date(Date.UTC(2026, 7, 19, 0, 0)))).toBe('miss');
  });

  it('filters on the weekday in the window timezone', () => {
    // 2026-08-19T23:00Z is Thursday in Asia/Shanghai.
    const window: TimeWindowSchedule = {
      timezone: 'Asia/Shanghai',
      days_of_week: [4],
      ranges: [{ start: '00:00', end: '00:00' }],
    };
    expect(matchTimeWindow(window, new Date(Date.UTC(2026, 7, 19, 23)))).toBe('match');
    expect(matchTimeWindow(window, new Date(Date.UTC(2026, 7, 19, 12)))).toBe('miss');
  });

  it('misses when no range is declared', () => {
    expect(matchTimeWindow({ timezone: 'UTC', ranges: [] }, new Date())).toBe('miss');
  });

  it('errors on malformed clock and date values', () => {
    expect(matchTimeWindow({ timezone: 'UTC', ranges: [{ start: '9:00', end: '12:00' }] }, new Date())).toBe('error');
    expect(matchTimeWindow({ timezone: 'UTC', ranges: [{ start: '25:00', end: '12:00' }] }, new Date())).toBe('error');
    expect(matchTimeWindow({ timezone: 'UTC', date_from: '2026-8-17', ranges: [{ start: '00:00', end: '12:00' }] }, new Date())).toBe(
      'error'
    );
  });
});

describe('timeWindowScheduleSignature', () => {
  it('is stable across equal schedules and changes with them', () => {
    const clone = JSON.parse(JSON.stringify(deepseekWindows)) as TimeWindowSchedule[];
    expect(timeWindowScheduleSignature(clone)).toBe(timeWindowScheduleSignature(deepseekWindows));

    clone[0].ranges[0].start = '19:00';
    expect(timeWindowScheduleSignature(clone)).not.toBe(timeWindowScheduleSignature(deepseekWindows));
    expect(timeWindowScheduleSignature(undefined)).toBe('');
  });
});

describe('historical time windows', () => {
  const allDay: TimeWindowSchedule = { ranges: [{ start: '00:00', end: '00:00' }] };

  it('expires at exclusive midnight in the schedule timezone, defaulting to UTC', () => {
    const local = { ...allDay, timezone: 'Asia/Shanghai', date_to: '2026-08-20' };
    expect(isHistoricalTimeWindow(local, new Date('2026-08-19T15:59:59Z'))).toBe(false);
    expect(isHistoricalTimeWindow(local, new Date('2026-08-19T16:00:00Z'))).toBe(true);
    expect(isHistoricalTimeWindow({ ...allDay, date_to: '2026-08-20' }, new Date('2026-08-19T16:00:00Z'))).toBe(false);
    expect(isHistoricalTimeWindow({ ...allDay, date_to: '2026-08-20' }, new Date('2026-08-20T00:00:00Z'))).toBe(true);
  });

  it('uses calendar dates across DST instead of a fixed timezone offset', () => {
    const local = { ...allDay, timezone: 'America/New_York', date_to: '2026-11-02' };
    expect(isHistoricalTimeWindow(local, new Date('2026-11-02T04:59:59Z'))).toBe(false);
    expect(isHistoricalTimeWindow(local, new Date('2026-11-02T05:00:00Z'))).toBe(true);
  });

  it.each([
    { date_to: '2026-02-30' },
    { date_to: '2026-02-29' },
    { date_to: '2026-13-01' },
    { date_to: '2026-01-00' },
    { date_to: '2026-8-01' },
    { date_to: '2026-08-01T00:00:00Z' },
    { date_from: '2026-02-30', date_to: '2026-08-20' },
    { date_from: '2026-08-21', date_to: '2026-08-20' },
    { date_from: '2026-08-20', date_to: '2026-08-20' },
    { timezone: 'Not/AZone', date_to: '2026-08-20' },
    { ranges: [], date_to: '2026-08-20' },
    { ranges: [{ start: '24:00', end: '03:00' }], date_to: '2026-08-20' },
    {
      ranges: [
        { start: '00:00', end: '00:00' },
        { start: '00:00', end: 'bad' },
      ],
      date_to: '2026-08-20',
    },
    { days_of_week: [7], date_to: '2026-08-20' },
  ])('keeps malformed or unresolved schedules visible: %j', (fields) => {
    expect(isHistoricalTimeWindow({ ...allDay, ...fields }, new Date('2026-12-31T12:00:00Z'))).toBe(false);
  });

  it('accepts actual leap dates and valid overnight ranges', () => {
    expect(isHistoricalTimeWindow({ ...allDay, date_to: '2024-02-29' }, new Date('2024-02-29T00:00:00Z'))).toBe(true);
    expect(
      isHistoricalTimeWindow({ ranges: [{ start: '20:00', end: '04:00' }], date_to: '2026-08-20' }, new Date('2026-08-21T12:00:00Z'))
    ).toBe(true);
  });

  it('retains current/upcoming/open-ended schedules and does not infer history from a clock miss', () => {
    const at = new Date('2026-08-19T12:00:00Z');
    for (const window of [
      allDay,
      { ...allDay, date_from: '2026-08-21', date_to: '2026-08-22' },
      { ...allDay, date_to: '2026-08-20' },
      { ranges: [{ start: '01:00', end: '02:00' }] },
    ]) {
      expect(isHistoricalTimeWindow(window, at)).toBe(false);
    }
  });

  it('preserves original indices, array order and first-match precedence in one snapshot', () => {
    const windows: TimeWindowSchedule[] = [
      { ...allDay, date_to: '2026-08-18' },
      { ...allDay },
      { ...allDay },
      { ...allDay, date_to: '2026-08-19' },
      { ...allDay, date_from: '2026-08-22' },
    ];
    const original = JSON.stringify(windows);
    windows.forEach((window) => Object.freeze(window));
    Object.freeze(windows);
    const at = new Date('2026-08-20T00:00:00Z');
    expect(resolveTimeWindowSnapshot(windows, at)).toEqual({ activeIndex: 1, historicalIndices: [0, 3] });
    expect(resolveActiveTimeWindowIndex(windows, at)).toBe(1);
    expect(JSON.stringify(windows)).toBe(original);
  });

  it('updates history when midnight passes with the first active window unchanged', () => {
    const windows = [allDay, { ...allDay, date_to: '2026-08-20' }];
    expect(resolveTimeWindowSnapshot(windows, new Date('2026-08-19T23:59:59Z'))).toEqual({ activeIndex: 0, historicalIndices: [] });
    expect(resolveTimeWindowSnapshot(windows, new Date('2026-08-20T00:00:00Z'))).toEqual({ activeIndex: 0, historicalIndices: [1] });
  });
});
