import { describe, it, expect } from 'vitest';
import { toDateTimeLocal, fromDateTimeLocal, formatTimestamp } from '@/lib/utils';

const TORONTO_TZ = 'America/Toronto';

describe('datetime-local helpers', () => {
  it('round-trips epoch seconds via datetime-local', () => {
    const now = Math.floor(Date.now() / 1000);
    const str = toDateTimeLocal(now);
    const back = fromDateTimeLocal(str);
    // minutes precision due to formatting
    expect(Math.abs(back - now)).toBeLessThanOrEqual(60);
  });

  it('handles empty input', () => {
    expect(fromDateTimeLocal('')).toBe(0);
    expect(toDateTimeLocal(0)).toBe('');
  });

  it('toDateTimeLocal returns local timezone format', () => {
    // Test with a known timestamp: 2024-01-15 10:30:00 UTC (1705315800)
    const timestamp = 1705315800;
    const result = toDateTimeLocal(timestamp);

    // Should return YYYY-MM-DDTHH:MM format
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/);

    // Verify it's in local timezone by converting back
    const backToTimestamp = fromDateTimeLocal(result);
    expect(Math.abs(backToTimestamp - timestamp)).toBeLessThanOrEqual(60);
  });

  it('fromDateTimeLocal converts local time to UTC timestamp', () => {
    // Test with a datetime-local string
    const dateTimeLocal = '2024-01-15T10:30';
    const timestamp = fromDateTimeLocal(dateTimeLocal);

    // Should return a valid timestamp
    expect(timestamp).toBeGreaterThan(0);

    // Converting back should give us the same local time
    const backToLocal = toDateTimeLocal(timestamp);
    expect(backToLocal).toBe(dateTimeLocal);
  });
});

describe('formatTimestamp', () => {
  it('formats timestamp in local timezone', () => {
    // Test with a known timestamp: 2024-01-15 10:30:45 UTC
    const timestamp = 1705315845;
    const result = formatTimestamp(timestamp);

    // Should return YYYY-MM-DD HH:MM:SS format in local timezone
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);

    // The date object should match when we parse it
    const date = new Date(timestamp * 1000);
    const expectedYear = date.getFullYear();
    const expectedMonth = String(date.getMonth() + 1).padStart(2, '0');
    const expectedDay = String(date.getDate()).padStart(2, '0');

    expect(result).toContain(`${expectedYear}-${expectedMonth}-${expectedDay}`);
  });

  it('handles invalid timestamps', () => {
    expect(formatTimestamp(0)).toBe('-');
    expect(formatTimestamp(-1)).toBe('-');
    expect(formatTimestamp(undefined as any)).toBe('-');
    expect(formatTimestamp(null as any)).toBe('-');
  });
});

describe('timezone specific handling', () => {
  it('handles Toronto standard time correctly', () => {
    // 2024-11-12T18:21:00Z -> 13:21 EST (UTC-5)
    const timestamp = Math.floor(Date.UTC(2024, 10, 12, 18, 21, 0) / 1000);
    expect(formatTimestamp(timestamp, { timeZone: TORONTO_TZ })).toBe('2024-11-12 13:21:00');
    expect(toDateTimeLocal(timestamp, { timeZone: TORONTO_TZ })).toBe('2024-11-12T13:21');
    expect(fromDateTimeLocal('2024-11-12T13:21', { timeZone: TORONTO_TZ })).toBe(timestamp);
  });

  it('handles Toronto daylight time correctly', () => {
    // 2024-06-12T17:21:00Z -> 13:21 EDT (UTC-4)
    const timestamp = Math.floor(Date.UTC(2024, 5, 12, 17, 21, 0) / 1000);
    expect(formatTimestamp(timestamp, { timeZone: TORONTO_TZ })).toBe('2024-06-12 13:21:00');
    expect(toDateTimeLocal(timestamp, { timeZone: TORONTO_TZ })).toBe('2024-06-12T13:21');
    expect(fromDateTimeLocal('2024-06-12T13:21', { timeZone: TORONTO_TZ })).toBe(timestamp);
  });

  it('round trips across DST boundaries without drift', () => {
    const samples = [
      '2025-03-08T08:15', // before spring forward
      '2025-03-09T08:15', // after spring forward (DST in effect)
      '2025-11-02T08:15', // DST end day morning
      '2025-11-03T08:15', // after fallback
    ];

    for (const sample of samples) {
      const ts = fromDateTimeLocal(sample, { timeZone: TORONTO_TZ });
      expect(ts).toBeGreaterThan(0);
      expect(toDateTimeLocal(ts, { timeZone: TORONTO_TZ })).toBe(sample);
    }
  });
});
