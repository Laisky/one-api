import { useId } from 'react';
import type { EffectiveTariffSelection } from './effective-tariff';
import type { ModelDisplayTariffData } from './ModelPricingModal';
import { formatUsdForTokens, formatUsdRaw, type TrFn } from './pricing-format';

interface InactivePriceEntry {
  key: string;
  label: string;
  value: string;
  unit: string;
}

/**
 * CurrentTariffNotice labels the prices above as the time-window tariff in effect
 * now and lists the standard (base) prices that do not apply at this moment.
 * Parameters: selection is the current-tariff selection from selectEffectiveTariff
 * and tr translates keys in the model-detail namespace.
 * Returns: the notice, or null while the base schedule itself is in effect.
 */
export function CurrentTariffNotice({ selection, tr }: { selection: EffectiveTariffSelection; tr: TrFn }) {
  const standardTitleId = useId();
  if (!selection.inactiveBase) {
    return null;
  }
  const windowLabel =
    selection.windowName || (selection.windowIndex >= 0 ? `${tr('window_name', 'Window')} ${selection.windowIndex + 1}` : undefined);
  const currentLabel = windowLabel
    ? tr('current_price_window', 'Current price — {{window}} pricing applies', { window: windowLabel })
    : tr('current_price_time_window', 'Current price — time-window pricing applies');
  const entries = inactivePriceEntries(selection.inactiveBase, tr);

  return (
    <div className="space-y-2 rounded-lg border border-accent/30 bg-accent/[0.05] p-3">
      <p className="text-sm font-medium text-accent">{currentLabel}</p>
      {entries.length > 0 && (
        <div role="group" aria-labelledby={standardTitleId} className="rounded-md border border-dashed bg-muted/20 p-2.5">
          <div id={standardTitleId} className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
            {tr('standard_price_inactive', 'Standard price (not in effect now)')}
          </div>
          <dl className="mt-1.5 grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs sm:grid-cols-3">
            {entries.map((entry) => (
              <div key={entry.key} className="min-w-0">
                <dt className="text-muted-foreground">{entry.label}</dt>
                <dd className="text-foreground/80">
                  <span className="font-mono tabular-nums">{entry.value}</span>{' '}
                  <span className="text-[10px] text-muted-foreground/70">{entry.unit}</span>
                </dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </div>
  );
}

/**
 * inactivePriceEntries lists the headline prices of the inactive base tariff,
 * mirroring the dimensions a window card shows for its overlay.
 * Parameters: tariff is the inactive base tariff and tr translates labels.
 * Returns: the formatted entries in display order; an empty list when nothing is priced.
 */
function inactivePriceEntries(tariff: ModelDisplayTariffData, tr: TrFn): InactivePriceEntry[] {
  const perMillion = tr('per_1m', 'per 1M tokens');
  const entries: InactivePriceEntry[] = [];
  const tokenPriced = !tariff.per_call_pricing && !tariff.per_page_pricing;
  if (tokenPriced) {
    entries.push({ key: 'input', label: tr('input', 'Input'), value: formatUsdForTokens(tariff.input_price, tr), unit: perMillion });
    entries.push({ key: 'output', label: tr('output', 'Output'), value: formatUsdForTokens(tariff.output_price, tr), unit: perMillion });
  }
  if (tariff.cached_input_price !== undefined && tariff.cached_input_price !== tariff.input_price) {
    entries.push({
      key: 'cached_read',
      label: tr('cached_read', 'Cache Read'),
      value: formatUsdForTokens(tariff.cached_input_price, tr),
      unit: perMillion,
    });
  }
  if (tariff.cache_write_5m_price !== undefined && tariff.cache_write_5m_price > 0) {
    entries.push({
      key: 'cache_write_5m',
      label: tr('cache_write_5m', '5-min Cache Write'),
      value: formatUsdForTokens(tariff.cache_write_5m_price, tr),
      unit: perMillion,
    });
  }
  if (tariff.cache_write_1h_price !== undefined && tariff.cache_write_1h_price > 0) {
    entries.push({
      key: 'cache_write_1h',
      label: tr('cache_write_1h', '1-hour Cache Write'),
      value: formatUsdForTokens(tariff.cache_write_1h_price, tr),
      unit: perMillion,
    });
  }
  if (tariff.per_call_pricing?.usd_per_thousand_calls !== undefined) {
    entries.push({
      key: 'per_call',
      label: tr('per_call_pricing', 'Per-Call Pricing'),
      value: formatUsdRaw(tariff.per_call_pricing.usd_per_thousand_calls),
      unit: tr('per_1k_calls', 'per 1K calls'),
    });
  }
  if (tariff.per_page_pricing?.usd_per_thousand_pages !== undefined) {
    entries.push({
      key: 'per_page',
      label: tr('per_page_pricing', 'Per-Page Pricing'),
      value: formatUsdRaw(tariff.per_page_pricing.usd_per_thousand_pages),
      unit: tr('per_1k_pages', 'per 1K pages'),
    });
  }
  if (tariff.image_pricing?.price_per_image_usd !== undefined && tariff.image_pricing.price_per_image_usd > 0) {
    entries.push({
      key: 'image',
      label: tr('image_pricing', 'Image Pricing'),
      value: formatUsdRaw(tariff.image_pricing.price_per_image_usd),
      unit: tr('per_image', 'per image'),
    });
  }
  if (tariff.video_pricing?.per_second_usd !== undefined && tariff.video_pricing.per_second_usd > 0) {
    entries.push({
      key: 'video',
      label: tr('video_pricing', 'Video Pricing'),
      value: formatUsdRaw(tariff.video_pricing.per_second_usd),
      unit: tr('per_second', 'per second'),
    });
  }
  if (tariff.audio_pricing?.usd_per_second !== undefined && tariff.audio_pricing.usd_per_second > 0) {
    entries.push({
      key: 'audio',
      label: tr('audio_pricing', 'Audio Pricing'),
      value: formatUsdRaw(tariff.audio_pricing.usd_per_second),
      unit: tr('per_second', 'per second'),
    });
  }
  if (tariff.embedding_pricing?.text_token_price !== undefined && tariff.embedding_pricing.text_token_price > 0) {
    entries.push({
      key: 'embedding_text',
      label: tr('text_tokens', 'Text Token Pricing'),
      value: formatUsdForTokens(tariff.embedding_pricing.text_token_price, tr),
      unit: perMillion,
    });
  }
  return entries;
}
