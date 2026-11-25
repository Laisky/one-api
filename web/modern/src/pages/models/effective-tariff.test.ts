import { describe, expect, it } from 'vitest';

import {
  MODEL_DISPLAY_TARIFF_KEYS,
  applyDisplayTariff,
  currentDisplayTariff,
  extractDisplayTariff,
  selectEffectiveTariff,
} from './effective-tariff';
import type { ModelDisplayData, ModelDisplayTariffData, TimeWindowData } from './ModelPricingModal';
import { NO_ACTIVE_TIME_WINDOW, UNRESOLVED_TIME_WINDOWS } from './time-window';

const SOL_WINDOW_NAME = 'gpt-5.6-sol-standard-from-december-2026';

/** solBase returns the GPT-5.6 Sol promotional base schedule. */
function solBase(): ModelDisplayTariffData {
  return {
    input_price: 4,
    cached_input_price: 0.4,
    cache_write_5m_price: 5,
    output_price: 20,
    tiers: [{ input_price: 8, output_price: 30, input_token_threshold: 272001 }],
  };
}

/** solStandard returns the complete tariff billed once the December window applies. */
function solStandard(): ModelDisplayTariffData {
  return {
    input_price: 5,
    cached_input_price: 0.5,
    cache_write_5m_price: 6.25,
    output_price: 30,
    tiers: [{ input_price: 10, output_price: 45, input_token_threshold: 272001 }],
  };
}

/** solWindow returns the dated all-day window, optionally without effective_pricing (older server). */
function solWindow(withEffective = true): TimeWindowData {
  return {
    name: SOL_WINDOW_NAME,
    timezone: 'UTC',
    ranges: [{ start: '00:00', end: '00:00' }],
    date_from: '2026-12-01',
    overlay: {
      input_price: 5,
      cached_input_price: 0.5,
      cache_write_5m_price: 6.25,
      output_price: 30,
      tiers: [{ input_price: 10, output_price: 45, input_token_threshold: 272001 }],
    },
    ...(withEffective ? { effective_pricing: solStandard() } : {}),
  };
}

/** fetchedBeforeCutoff mirrors a payload the server rendered while no window applied. */
function fetchedBeforeCutoff(): ModelDisplayData {
  return { ...solBase(), description: 'Sol', context_length: 1050000, time_windows: [solWindow()] };
}

/** fetchedAfterCutoff mirrors a payload the server rendered while the window applied. */
function fetchedAfterCutoff(): ModelDisplayData {
  return {
    ...solStandard(),
    description: 'Sol',
    time_windows: [solWindow()],
    active_time_window: SOL_WINDOW_NAME,
    inactive_base_pricing: solBase(),
  };
}

describe('selectEffectiveTariff', () => {
  it('uses the server tariff without an inactive base when the model has no windows', () => {
    const data: ModelDisplayData = { input_price: 3, cached_input_price: 0.3, output_price: 15, description: 'operator tariff' };
    for (const index of [NO_ACTIVE_TIME_WINDOW, UNRESOLVED_TIME_WINDOWS, 0]) {
      const selection = selectEffectiveTariff(data, index);
      expect(selection.source).toBe('server');
      expect(selection.tariff).toEqual({ input_price: 3, cached_input_price: 0.3, output_price: 15 });
      expect(selection.inactiveBase).toBeUndefined();
      expect(selection.windowIndex).toBe(NO_ACTIVE_TIME_WINDOW);
    }
  });

  it('keeps a server-published inactive base even without windows', () => {
    const data: ModelDisplayData = { ...solStandard(), inactive_base_pricing: solBase() };
    const selection = selectEffectiveTariff(data, NO_ACTIVE_TIME_WINDOW);
    expect(selection.source).toBe('server');
    expect(selection.tariff.input_price).toBe(5);
    expect(selection.inactiveBase).toBe(data.inactive_base_pricing);
  });

  it('falls back to the server evaluation when the browser cannot evaluate windows', () => {
    const inWindow = fetchedAfterCutoff();
    const unresolved = selectEffectiveTariff(inWindow, UNRESOLVED_TIME_WINDOWS);
    expect(unresolved.source).toBe('server');
    expect(unresolved.tariff).toEqual(solStandard());
    expect(unresolved.inactiveBase).toBe(inWindow.inactive_base_pricing);
    expect(unresolved.windowName).toBe(SOL_WINDOW_NAME);
    expect(unresolved.windowIndex).toBe(0);

    const beforeWindow = selectEffectiveTariff(fetchedBeforeCutoff(), UNRESOLVED_TIME_WINDOWS);
    expect(beforeWindow.tariff).toEqual(solBase());
    expect(beforeWindow.inactiveBase).toBeUndefined();
    expect(beforeWindow.windowName).toBeUndefined();
  });

  it('selects the live window tariff and treats a no-window server top-level as the base', () => {
    const data = fetchedBeforeCutoff();
    const selection = selectEffectiveTariff(data, 0);
    expect(selection.source).toBe('window');
    expect(selection.tariff).toBe(data.time_windows![0].effective_pricing);
    expect(selection.inactiveBase).toEqual(solBase());
    expect(selection.windowIndex).toBe(0);
    expect(selection.windowName).toBe(SOL_WINDOW_NAME);
  });

  it('prefers inactive_base_pricing over a top-level that is another window tariff', () => {
    const peak: TimeWindowData = { ...solWindow(), name: 'peak', effective_pricing: { input_price: 7, output_price: 40 } };
    const data: ModelDisplayData = {
      input_price: 7,
      output_price: 40,
      time_windows: [solWindow(), peak],
      active_time_window: 'peak',
      inactive_base_pricing: solBase(),
    };
    const selection = selectEffectiveTariff(data, 0);
    expect(selection.tariff).toBe(data.time_windows![0].effective_pricing);
    expect(selection.inactiveBase).toBe(data.inactive_base_pricing);
  });

  it('renders the base schedule without an inactive base while no window is live', () => {
    const inWindow = fetchedAfterCutoff();
    const ended = selectEffectiveTariff(inWindow, NO_ACTIVE_TIME_WINDOW);
    expect(ended.source).toBe('base');
    expect(ended.tariff).toBe(inWindow.inactive_base_pricing);
    expect(ended.inactiveBase).toBeUndefined();

    const notStarted = selectEffectiveTariff(fetchedBeforeCutoff(), NO_ACTIVE_TIME_WINDOW);
    expect(notStarted.tariff).toEqual(solBase());
    expect(notStarted.inactiveBase).toBeUndefined();
  });

  it('falls back to the old server behavior when effective_pricing is missing', () => {
    const legacy: ModelDisplayData = { ...solBase(), time_windows: [solWindow(false)], active_time_window: SOL_WINDOW_NAME };
    const selection = selectEffectiveTariff(legacy, 0);
    expect(selection.source).toBe('server');
    expect(selection.tariff).toEqual(solBase());
    expect(selection.inactiveBase).toBeUndefined();
  });

  it('falls back to the server tariff for an out-of-range live index', () => {
    const selection = selectEffectiveTariff(fetchedBeforeCutoff(), 3);
    expect(selection.source).toBe('server');
    expect(selection.tariff).toEqual(solBase());
  });

  it('reports the index of an unnamed live window', () => {
    const unnamed = { ...solWindow(), name: undefined };
    const selection = selectEffectiveTariff({ ...solBase(), time_windows: [unnamed] }, 0);
    expect(selection.windowIndex).toBe(0);
    expect(selection.windowName).toBeUndefined();
  });
});

describe('applyDisplayTariff', () => {
  it('replaces every price field, drops omitted ones, and keeps metadata', () => {
    const data = fetchedBeforeCutoff();
    const before = JSON.stringify(data);
    const tariff: ModelDisplayTariffData = { input_price: 5, output_price: 30 };
    const merged = applyDisplayTariff(data, tariff);
    expect(merged.input_price).toBe(5);
    expect(merged.output_price).toBe(30);
    expect(merged.cached_input_price).toBeUndefined();
    expect(merged.cache_write_5m_price).toBeUndefined();
    expect(merged.tiers).toBeUndefined();
    expect(merged.description).toBe('Sol');
    expect(merged.context_length).toBe(1050000);
    expect(merged.time_windows).toBe(data.time_windows);
    expect(JSON.stringify(data)).toBe(before);
  });
});

describe('extractDisplayTariff', () => {
  it('copies only the price fields', () => {
    const tariff = extractDisplayTariff(fetchedAfterCutoff());
    expect(Object.keys(tariff).every((key) => (MODEL_DISPLAY_TARIFF_KEYS as string[]).includes(key))).toBe(true);
    expect(tariff).toEqual(solStandard());
    expect(MODEL_DISPLAY_TARIFF_KEYS).toHaveLength(13);
  });
});

describe('currentDisplayTariff', () => {
  it('follows the dated cutoff at 2026-12-01 00:00 UTC', () => {
    const data = fetchedBeforeCutoff();
    expect(currentDisplayTariff(data, new Date('2026-11-30T23:59:59Z')).input_price).toBe(4);
    expect(currentDisplayTariff(data, new Date('2026-12-01T00:00:00Z')).input_price).toBe(5);
    expect(currentDisplayTariff(fetchedAfterCutoff(), new Date('2026-11-30T23:59:59Z')).output_price).toBe(20);
  });
});
