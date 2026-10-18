import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { useEffect, useMemo, useRef, useState } from 'react';
import { AudioTariffDetails } from './AudioTariffDetails';
import type { TimeWindowData } from './ModelPricingModal';
import { PerPageWindowCell } from './PerPagePricing';
import { PriceCell, PriceGrid, PricingSection } from './PricingPrimitives';
import { formatUsd, type TrFn } from './pricing-format';
import { formatTierThreshold } from './tier-threshold';
import { NO_ACTIVE_TIME_WINDOW, UNRESOLVED_TIME_WINDOWS, resolveTimeWindowSnapshot, timeWindowScheduleSignature } from './time-window';

/** useTimeWindowSnapshot tracks active/history membership from one instant and cleans up its timer. */
function useTimeWindowSnapshot(windows: TimeWindowData[] | undefined) {
  const windowsRef = useRef(windows);
  windowsRef.current = windows;
  const signature = useMemo(() => timeWindowScheduleSignature(windows), [windows]);
  const [snapshot, setSnapshot] = useState(() => ({ signature, ...resolveTimeWindowSnapshot(windows, new Date()) }));

  useEffect(() => {
    const evaluate = () => {
      const next = resolveTimeWindowSnapshot(windowsRef.current, new Date());
      setSnapshot((previous) =>
        previous.signature === signature &&
        previous.activeIndex === next.activeIndex &&
        previous.historicalIndices.length === next.historicalIndices.length &&
        previous.historicalIndices.every((index, position) => index === next.historicalIndices[position])
          ? previous
          : { signature, ...next }
      );
    };
    evaluate();
    const timer = setInterval(evaluate, 1000);
    return () => clearInterval(timer);
  }, [signature]);

  // A model/schedule replacement must never render indices from the previous list.
  return snapshot.signature === signature ? snapshot : resolveTimeWindowSnapshot(windows, new Date());
}

/** ModelPricingWindows preserves all source rules while collapsing expired valid schedules. */
export function ModelPricingWindows({
  windows,
  activeWindowName,
  tr,
  locale,
}: {
  windows: TimeWindowData[] | undefined;
  activeWindowName?: string;
  tr: TrFn;
  locale: string;
}) {
  const { activeIndex: activeTimeWindowIndex, historicalIndices } = useTimeWindowSnapshot(windows);
  if (!windows?.length) return null;
  const historical = new Set(historicalIndices);
  const renderWindow = (window: TimeWindowData, index: number) => {
    const label = window.name || `${tr('window_name', 'Window')} ${index + 1}`;
    // Fall back to the server-rendered name only when the browser cannot
    // evaluate the schedules itself (missing timezone data).
    const isActive =
      activeTimeWindowIndex === UNRESOLVED_TIME_WINDOWS
        ? Boolean(activeWindowName && window.name === activeWindowName)
        : activeTimeWindowIndex !== NO_ACTIVE_TIME_WINDOW && activeTimeWindowIndex === index;
    return (
      <div
        key={`${label}-${index}`}
        aria-current={isActive ? 'true' : undefined}
        className={cn(
          'rounded-lg border p-3 transition-colors',
          isActive ? 'border-accent/50 bg-accent/[0.07] ring-1 ring-accent/20' : 'bg-muted/20'
        )}
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className={cn('font-medium', isActive && 'text-accent')}>{label}</div>
          {isActive && (
            <Badge variant="outline" className="gap-1.5 border-accent/40 bg-accent/10 text-xs text-accent">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent opacity-60 reduce-motion:hidden" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-accent" />
              </span>
              {tr('window_active', 'Active now')}
            </Badge>
          )}
        </div>
        <div className="mt-2 flex flex-wrap gap-1.5 text-xs text-muted-foreground">
          <Badge variant="outline" className="font-mono text-[11px]">
            {formatWindowSchedule(window, locale)}
          </Badge>
          <Badge variant="outline" className="text-[11px]">
            {tr('window_timezone', 'Timezone')}: {window.timezone || 'UTC'}
          </Badge>
          {window.days_of_week && window.days_of_week.length > 0 && (
            <Badge variant="outline" className="text-[11px]">
              {tr('window_days', 'Days')}: {formatWeekdays(window.days_of_week, tr)}
            </Badge>
          )}
          {(window.date_from || window.date_to) && (
            <Badge variant="outline" className="text-[11px]">
              {tr('window_dates', 'Dates')}: {window.date_from || '...'} - {window.date_to || '...'}
            </Badge>
          )}
        </div>
        <div className="mt-3">
          <PriceGrid>
            {window.overlay.input_price !== undefined && window.overlay.input_price > 0 && (
              <PriceCell label={tr('input', 'Input')} sublabel={tr('per_1m', 'per 1M tokens')} value={window.overlay.input_price} tr={tr} />
            )}
            {window.overlay.output_price !== undefined && window.overlay.output_price > 0 && (
              <PriceCell
                label={tr('output', 'Output')}
                sublabel={tr('per_1m', 'per 1M tokens')}
                value={window.overlay.output_price}
                tr={tr}
              />
            )}
            {window.overlay.cached_input_price !== undefined && window.overlay.cached_input_price > 0 && (
              <PriceCell
                label={tr('cached_read', 'Cache Read')}
                sublabel={tr('per_1m', 'per 1M tokens')}
                value={window.overlay.cached_input_price}
                tr={tr}
              />
            )}
            {window.overlay.cache_write_5m_price !== undefined && window.overlay.cache_write_5m_price > 0 && (
              <PriceCell
                label={tr('cache_write_5m', '5-min Cache Write')}
                sublabel={tr('per_1m', 'per 1M tokens')}
                value={window.overlay.cache_write_5m_price}
                tr={tr}
              />
            )}
            {window.overlay.cache_write_1h_price !== undefined && window.overlay.cache_write_1h_price > 0 && (
              <PriceCell
                label={tr('cache_write_1h', '1-hour Cache Write')}
                sublabel={tr('per_1m', 'per 1M tokens')}
                value={window.overlay.cache_write_1h_price}
                tr={tr}
              />
            )}
            {window.overlay.per_call_pricing?.usd_per_thousand_calls !== undefined && (
              <PriceCell
                label={tr('per_call_pricing', 'Per-call Pricing')}
                sublabel={tr('per_1k_calls', 'per 1K calls')}
                value={window.overlay.per_call_pricing.usd_per_thousand_calls}
                tr={tr}
                raw
              />
            )}
            <PerPageWindowCell pricing={window.overlay.per_page_pricing} tr={tr} />
            {window.overlay.image_pricing?.price_per_image_usd !== undefined && window.overlay.image_pricing.price_per_image_usd > 0 && (
              <PriceCell
                label={tr('image_pricing', 'Image Pricing')}
                sublabel={tr('per_image', 'per image')}
                value={window.overlay.image_pricing.price_per_image_usd}
                tr={tr}
                raw
              />
            )}
            {window.overlay.video_pricing?.per_second_usd !== undefined && window.overlay.video_pricing.per_second_usd > 0 && (
              <PriceCell
                label={tr('video_pricing', 'Video Pricing')}
                sublabel={tr('per_second', 'per second')}
                value={window.overlay.video_pricing.per_second_usd}
                tr={tr}
                raw
              />
            )}
            <AudioTariffDetails pricing={window.overlay.audio_pricing} />
            {window.overlay.audio_pricing?.usd_per_second !== undefined && window.overlay.audio_pricing.usd_per_second > 0 && (
              <PriceCell
                label={tr('audio_pricing', 'Audio Pricing')}
                sublabel={tr('per_second', 'per second')}
                value={window.overlay.audio_pricing.usd_per_second}
                tr={tr}
                raw
              />
            )}
            {window.overlay.embedding_pricing?.text_token_price !== undefined && window.overlay.embedding_pricing.text_token_price > 0 && (
              <PriceCell
                label={tr('text_tokens', 'Text Token Pricing')}
                sublabel={tr('per_1m', 'per 1M tokens')}
                value={window.overlay.embedding_pricing.text_token_price}
                tr={tr}
              />
            )}
          </PriceGrid>
          {window.overlay.tiers && window.overlay.tiers.length > 0 && (
            <div className="mt-3 overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b text-muted-foreground">
                    <th className="py-1.5 pr-3 text-left font-medium">{tr('tier_threshold', 'Threshold')}</th>
                    <th className="px-3 py-1.5 text-left font-medium">{tr('input', 'Input')}</th>
                    <th className="px-3 py-1.5 text-left font-medium">{tr('output', 'Output')}</th>
                  </tr>
                </thead>
                <tbody>
                  {window.overlay.tiers.map((tier, tierIndex) => (
                    <tr key={tierIndex} className="border-b border-dashed last:border-0">
                      <td className="py-1.5 pr-3">
                        <Badge variant="outline" className="text-[11px]">
                          {formatTierThreshold(tier, tr('input', 'Input'), tr('output', 'Output'))}
                        </Badge>
                      </td>
                      <td className="px-3 py-1.5 font-mono">{formatUsd(tier.input_price)}</td>
                      <td className="px-3 py-1.5 font-mono">{formatUsd(tier.output_price)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <PricingSection title={tr('time_pricing', 'Time-of-day Pricing')} icon="time">
      <div className="space-y-3">
        {windows.map((window, index) => (historical.has(index) ? null : renderWindow(window, index)))}
        {historicalIndices.length > 0 && (
          <details className="rounded-lg border p-3">
            <summary className="cursor-pointer text-sm font-medium">
              {tr('historical_pricing', 'Historical pricing')} ({historicalIndices.length})
            </summary>
            <div className="mt-3 space-y-3">{historicalIndices.map((index) => renderWindow(windows[index], index))}</div>
          </details>
        )}
      </div>
    </PricingSection>
  );
}

function formatWindowSchedule(window: TimeWindowData, locale: string): string {
  return (window.ranges || []).map((range) => `${formatClockTime(range.start, locale)}-${formatClockTime(range.end, locale)}`).join(', ');
}

function formatClockTime(value: string, locale: string): string {
  const match = /^(\d{2}):(\d{2})$/.exec(value);
  if (!match) {
    return value;
  }
  const date = new Date(Date.UTC(2026, 0, 1, Number(match[1]), Number(match[2])));
  return new Intl.DateTimeFormat(locale || undefined, {
    hour: 'numeric',
    minute: '2-digit',
    timeZone: 'UTC',
  }).format(date);
}

function formatWeekdays(days: number[], tr: TrFn): string {
  return days.map((day) => tr(`weekday_${day}`, weekdayFallback(day))).join(', ');
}

function weekdayFallback(day: number): string {
  return ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'][day] || String(day);
}
