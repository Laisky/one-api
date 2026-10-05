/** TrFn translates a key under the model-detail namespace with an English fallback. */
export type TrFn = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

/**
 * formatUsd renders a per-1M-token USD price with precision that scales with magnitude.
 * @param price - USD amount to render.
 * @returns The formatted dollar string; zero renders as "$0".
 */
export function formatUsd(price: number): string {
  if (price === 0) return '$0';
  if (price < 0.001) return `$${parseFloat(price.toFixed(6))}`;
  if (price < 1) return `$${parseFloat(price.toFixed(4))}`;
  return `$${price.toFixed(2)}`;
}

/**
 * formatUsdRaw renders a flat (per unit) USD price, keeping small fractional amounts readable.
 * @param price - USD amount to render.
 * @returns The formatted dollar string; zero renders as "$0" because a present zero tariff is free.
 */
export function formatUsdRaw(price: number): string {
  if (price === 0) return '$0';
  if (price < 0.0001) return `$${parseFloat(price.toFixed(6))}`;
  if (price < 0.01) return `$${parseFloat(price.toFixed(4))}`;
  return `$${price.toFixed(2)}`;
}

/**
 * formatUsdForTokens renders a token price, using the translated "Free" label for zero.
 * @param price - USD amount per 1M tokens.
 * @param tr - Translation function for the model-detail namespace.
 * @returns The formatted price or the localized free label.
 */
export function formatUsdForTokens(price: number, tr: TrFn): string {
  if (price === 0) return tr('free', 'Free');
  return formatUsd(price);
}
