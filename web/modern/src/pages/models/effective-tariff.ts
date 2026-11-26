/**
 * Selection of the tariff the pricing page renders as "current".
 *
 * The server computes every complete tariff (the top-level display tariff,
 * inactive_base_pricing, and each window's effective_pricing). This module only
 * chooses among those server-computed tariffs using the live active-window index
 * from the client matcher; it never derives a price by itself.
 */
import type { ModelDisplayData, ModelDisplayTariffData } from './ModelPricingModal';
import { NO_ACTIVE_TIME_WINDOW, UNRESOLVED_TIME_WINDOWS, resolveActiveTimeWindowIndex } from './time-window';

// Typed as a Record so adding or removing a tariff field without updating this list fails type checking.
const TARIFF_FIELDS: Record<keyof ModelDisplayTariffData, true> = {
  input_price: true,
  cached_input_price: true,
  cache_write_5m_price: true,
  cache_write_1h_price: true,
  output_price: true,
  image_price: true,
  tiers: true,
  video_pricing: true,
  audio_pricing: true,
  image_pricing: true,
  embedding_pricing: true,
  per_call_pricing: true,
  per_page_pricing: true,
};

/** MODEL_DISPLAY_TARIFF_KEYS lists every price field of a complete display tariff. */
export const MODEL_DISPLAY_TARIFF_KEYS = Object.keys(TARIFF_FIELDS) as (keyof ModelDisplayTariffData)[];

/**
 * TariffSelectionSource names which server-computed tariff was selected:
 * 'server' is the top-level display tariff, 'window' is a live window's
 * effective_pricing, and 'base' is the base schedule while no window applies.
 */
export type TariffSelectionSource = 'server' | 'window' | 'base';

/**
 * EffectiveTariffSelection describes the tariff to render as current.
 * tariff is the selected complete tariff; inactiveBase is the base schedule that
 * is not in effect now and is set only while a window tariff applies;
 * windowIndex is the index of the window whose tariff applies, or
 * NO_ACTIVE_TIME_WINDOW when none is known; windowName is that window's name
 * when one is known; source records which server tariff was chosen.
 */
export interface EffectiveTariffSelection {
  tariff: ModelDisplayTariffData;
  inactiveBase?: ModelDisplayTariffData;
  windowIndex: number;
  windowName?: string;
  source: TariffSelectionSource;
}

/**
 * extractDisplayTariff copies the price fields of a display payload.
 * Parameters: data is a model display payload or tariff.
 * Returns: a tariff holding only the price fields present on data.
 */
export function extractDisplayTariff(data: ModelDisplayTariffData): ModelDisplayTariffData {
  const tariff: Partial<ModelDisplayTariffData> = {};
  for (const key of MODEL_DISPLAY_TARIFF_KEYS) {
    if (data[key] !== undefined) {
      Object.assign(tariff, { [key]: data[key] });
    }
  }
  return tariff as ModelDisplayTariffData;
}

/**
 * selectEffectiveTariff chooses which server-computed tariff is current.
 * Parameters: data is the model display payload and liveIndex is the active
 * window index from the client matcher (resolveActiveTimeWindowIndex), which may
 * be NO_ACTIVE_TIME_WINDOW or UNRESOLVED_TIME_WINDOWS.
 * Returns: the selected tariff and, while a window tariff applies, the inactive
 * base schedule. A missing effective_pricing (older server) falls back to the
 * server top-level tariff.
 */
export function selectEffectiveTariff(data: ModelDisplayData, liveIndex: number): EffectiveTariffSelection {
  const serverTariff = extractDisplayTariff(data);
  const windows = data.time_windows;

  if (!windows || windows.length === 0 || liveIndex === UNRESOLVED_TIME_WINDOWS) {
    return serverSelection(data, serverTariff);
  }

  if (liveIndex === NO_ACTIVE_TIME_WINDOW) {
    // Without a live window the base schedule applies; when the server saw a
    // window, its top-level is that window's tariff and the base is published apart.
    return { tariff: data.inactive_base_pricing ?? serverTariff, windowIndex: NO_ACTIVE_TIME_WINDOW, source: 'base' };
  }

  const window = liveIndex >= 0 ? windows[liveIndex] : undefined;
  if (!window?.effective_pricing) {
    return serverSelection(data, serverTariff);
  }

  // When the server saw no window, its top-level tariff is the base schedule.
  return {
    tariff: window.effective_pricing,
    inactiveBase: data.inactive_base_pricing ?? serverTariff,
    windowIndex: liveIndex,
    windowName: window.name || undefined,
    source: 'window',
  };
}

/**
 * serverSelection keeps the server's own evaluation of the display instant.
 * Parameters: data is the model display payload and serverTariff is its top-level tariff.
 * Returns: the server tariff, with the inactive base and the server-named window when the server saw one.
 */
function serverSelection(data: ModelDisplayData, serverTariff: ModelDisplayTariffData): EffectiveTariffSelection {
  if (!data.inactive_base_pricing) {
    return { tariff: serverTariff, windowIndex: NO_ACTIVE_TIME_WINDOW, source: 'server' };
  }
  const windowName = data.active_time_window || undefined;
  const windowIndex = windowName ? (data.time_windows ?? []).findIndex((window) => window.name === windowName) : NO_ACTIVE_TIME_WINDOW;
  return {
    tariff: serverTariff,
    inactiveBase: data.inactive_base_pricing,
    windowIndex: windowIndex >= 0 ? windowIndex : NO_ACTIVE_TIME_WINDOW,
    windowName,
    source: 'server',
  };
}

/**
 * applyDisplayTariff replaces every price field of a display payload with a tariff.
 * Parameters: data is the model display payload and tariff is the selected complete tariff.
 * Returns: a copy of data whose price fields are exactly the tariff's (fields the
 * tariff omits are removed) while all non-price metadata is kept.
 */
export function applyDisplayTariff(data: ModelDisplayData, tariff: ModelDisplayTariffData): ModelDisplayData {
  const metadata: Partial<ModelDisplayData> = { ...data };
  for (const key of MODEL_DISPLAY_TARIFF_KEYS) {
    delete metadata[key];
  }
  return { ...metadata, ...extractDisplayTariff(tariff) };
}

/**
 * currentDisplayTariff selects the tariff in effect at an instant without a live timer.
 * Parameters: data is the model display payload and at is the instant to evaluate.
 * Returns: the server-computed tariff that applies at that instant.
 */
export function currentDisplayTariff(data: ModelDisplayData, at: Date): ModelDisplayTariffData {
  return selectEffectiveTariff(data, resolveActiveTimeWindowIndex(data.time_windows, at)).tariff;
}
