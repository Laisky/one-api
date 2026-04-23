import { formatNumber } from '@/lib/utils';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { CacheHeatmap, CacheHeatmapEntity } from '../hooks/useDashboardCharts';
import { getDisplayInCurrency, resolveChartVar } from '../types';

interface CacheHeatmapsProps {
  modelHeatmap: CacheHeatmap;
  userHeatmap: CacheHeatmap;
  tokenHeatmap: CacheHeatmap;
  statisticsMetric: 'tokens' | 'requests' | 'expenses';
}

const TOP_N_OPTIONS = [15, 30, 50] as const;

/** Compute cell background color. Uses alpha on the chart-1 HSL triplet. */
function cellBackground(rate: number, hslTriplet: string, hasData: boolean): string {
  if (!hasData) return 'transparent';
  const alpha = Math.max(0.08, Math.min(1, rate));
  if (!hslTriplet) return `rgba(34, 163, 146, ${alpha})`;
  return `hsl(${hslTriplet} / ${alpha})`;
}

/** Format the metric value for tooltip display. */
function formatMetricValue(value: number, metric: 'tokens' | 'requests' | 'expenses'): string {
  if (metric === 'expenses') {
    return getDisplayInCurrency() ? `$${value.toFixed(6)}` : formatNumber(value);
  }
  return formatNumber(value);
}

function ratePercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

function formatShortDate(day: string): string {
  const parts = day.split('-');
  return parts.length === 3 ? `${parts[1]}-${parts[2]}` : day;
}

export function CacheHeatmaps({ modelHeatmap, userHeatmap, tokenHeatmap, statisticsMetric }: CacheHeatmapsProps) {
  const { t } = useTranslation();
  const [topN, setTopN] = useState<number>(15);

  const resolved = resolveChartVar('--chart-1');
  const chartHsl = resolved.startsWith('hsl(') ? resolved.slice(4, -1) : '';

  const metricLabel = (() => {
    switch (statisticsMetric) {
      case 'requests':
        return t('dashboard.metrics.requests');
      case 'expenses':
        return t('dashboard.metrics.expenses');
      default:
        return t('dashboard.metrics.tokens');
    }
  })();

  return (
    <>
      <div className="flex items-center justify-between mb-4">
        <div>
          <h2 className="text-lg font-semibold">{t('dashboard.cache.section_title')}</h2>
          <p className="text-xs text-muted-foreground">{t('dashboard.cache.section_subtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">{t('dashboard.cache.top_n_label')}</span>
          <Select value={String(topN)} onValueChange={(v) => setTopN(Number(v))}>
            <SelectTrigger className="w-24">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TOP_N_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {`Top ${n}`}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <HeatmapCard
        title={t('dashboard.cache.model_title')}
        heatmap={modelHeatmap}
        statisticsMetric={statisticsMetric}
        metricLabel={metricLabel}
        topN={topN}
        chartHsl={chartHsl}
      />
      <HeatmapCard
        title={t('dashboard.cache.user_title')}
        heatmap={userHeatmap}
        statisticsMetric={statisticsMetric}
        metricLabel={metricLabel}
        topN={topN}
        chartHsl={chartHsl}
      />
      <HeatmapCard
        title={t('dashboard.cache.token_title')}
        heatmap={tokenHeatmap}
        statisticsMetric={statisticsMetric}
        metricLabel={metricLabel}
        topN={topN}
        chartHsl={chartHsl}
      />
    </>
  );
}

interface HeatmapCardProps {
  title: string;
  heatmap: CacheHeatmap;
  statisticsMetric: 'tokens' | 'requests' | 'expenses';
  metricLabel: string;
  topN: number;
  chartHsl: string;
}

function HeatmapCard({ title, heatmap, statisticsMetric, metricLabel, topN, chartHsl }: HeatmapCardProps) {
  const { t } = useTranslation();

  const visible = useMemo(() => heatmap.entities.slice(0, topN), [heatmap.entities, topN]);
  const maxVolume = useMemo(() => visible.reduce((max, entity) => (entity.totalVolume > max ? entity.totalVolume : max), 0), [visible]);

  // Dimming threshold: cells with very low absolute volume get an opacity hint
  const lowVolumeThreshold = statisticsMetric === 'requests' ? 3 : 100;

  return (
    <div className="bg-card rounded-lg border p-6 mb-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-base font-semibold">{title}</h3>
        <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
      </div>

      {!visible.length ? (
        <div className="text-sm text-muted-foreground py-8 text-center">{t('dashboard.labels.no_data')}</div>
      ) : (
        <div className="overflow-x-auto">
          <div
            className="grid gap-1"
            style={{
              gridTemplateColumns: `minmax(9rem, 14rem) repeat(${heatmap.days.length}, minmax(28px, 40px)) minmax(7rem, 9rem)`,
            }}
          >
            {/* Header row */}
            <div className="text-xs text-muted-foreground sticky left-0 bg-card" />
            {heatmap.days.map((day) => (
              <div key={day} className="text-[10px] text-muted-foreground text-center tabular-nums" title={day}>
                {formatShortDate(day)}
              </div>
            ))}
            <div className="text-xs text-muted-foreground text-right pr-1">{t('dashboard.cache.overall')}</div>

            {/* Entity rows */}
            {visible.map((entity) => (
              <EntityRow
                key={entity.name}
                entity={entity}
                days={heatmap.days}
                statisticsMetric={statisticsMetric}
                chartHsl={chartHsl}
                lowVolumeThreshold={lowVolumeThreshold}
                maxVolume={maxVolume}
              />
            ))}
          </div>
        </div>
      )}

      {heatmap.entities.length > topN && (
        <p className="text-xs text-muted-foreground mt-3">
          {t('dashboard.cache.showing_top', { shown: topN, total: heatmap.entities.length })}
        </p>
      )}
    </div>
  );
}

interface EntityRowProps {
  entity: CacheHeatmapEntity;
  days: string[];
  statisticsMetric: 'tokens' | 'requests' | 'expenses';
  chartHsl: string;
  lowVolumeThreshold: number;
  maxVolume: number;
}

function EntityRow({ entity, days, statisticsMetric, chartHsl, lowVolumeThreshold, maxVolume }: EntityRowProps) {
  const { t } = useTranslation();
  const volumeShare = maxVolume > 0 ? entity.totalVolume / maxVolume : 0;

  return (
    <>
      <div className="text-xs font-medium truncate sticky left-0 bg-card pr-2 flex items-center" title={entity.name}>
        {entity.name}
      </div>
      {days.map((day) => {
        const cell = entity.perDay[day];
        const hasData = !!cell && cell.denominator > 0;
        const rate = hasData ? cell!.numerator / cell!.denominator : 0;
        const lowVolume = hasData && cell!.denominator < lowVolumeThreshold;

        return (
          <CacheCell
            key={day}
            entityName={entity.name}
            day={day}
            cell={cell}
            hasData={hasData}
            rate={rate}
            lowVolume={lowVolume}
            chartHsl={chartHsl}
            statisticsMetric={statisticsMetric}
          />
        );
      })}

      {/* Trailing overall rate + volume bar */}
      <div className="flex items-center gap-2 pr-1">
        <span
          className="text-[11px] font-semibold tabular-nums rounded px-1.5 py-0.5"
          style={{ backgroundColor: `hsl(${chartHsl} / ${Math.max(0.15, entity.overallRate)})` }}
          title={`${t('dashboard.cache.overall')}: ${ratePercent(entity.overallRate)}`}
        >
          {ratePercent(entity.overallRate)}
        </span>
        <div
          className="flex-1 h-1.5 rounded-full overflow-hidden bg-muted"
          title={`${t('dashboard.cache.total')}: ${formatMetricValue(entity.totalVolume, statisticsMetric)}`}
        >
          <div
            className="h-full"
            style={{
              width: `${Math.max(2, volumeShare * 100)}%`,
              backgroundColor: `hsl(${chartHsl})`,
            }}
          />
        </div>
      </div>
    </>
  );
}

interface CacheCellProps {
  entityName: string;
  day: string;
  cell: { numerator: number; denominator: number } | undefined;
  hasData: boolean;
  rate: number;
  lowVolume: boolean;
  chartHsl: string;
  statisticsMetric: 'tokens' | 'requests' | 'expenses';
}

function CacheCell({ entityName, day, cell, hasData, rate, lowVolume, chartHsl, statisticsMetric }: CacheCellProps) {
  const { t } = useTranslation();

  const tooltip = hasData
    ? `${entityName}\n${day}\n` +
      `${t('dashboard.cache.hit_rate')}: ${ratePercent(rate)}\n` +
      `${t('dashboard.cache.cached')}: ${formatMetricValue(cell!.numerator, statisticsMetric)}\n` +
      `${t('dashboard.cache.total')}: ${formatMetricValue(cell!.denominator, statisticsMetric)}`
    : `${entityName}\n${day}\n${t('dashboard.cache.no_activity')}`;

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="rounded-sm transition-transform hover:scale-110 hover:ring-2 hover:ring-primary/40 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 cursor-pointer"
          style={{
            height: '24px',
            backgroundColor: cellBackground(rate, chartHsl, hasData),
            opacity: lowVolume ? 0.45 : 1,
            backgroundImage: !hasData
              ? 'repeating-linear-gradient(45deg, hsl(var(--muted)), hsl(var(--muted)) 2px, transparent 2px, transparent 6px)'
              : undefined,
          }}
          title={tooltip}
          aria-label={tooltip}
        />
      </PopoverTrigger>
      <PopoverContent side="top" className="w-auto max-w-xs p-3 text-xs">
        <div className="flex flex-col gap-1.5">
          <div className="font-semibold break-all">{entityName}</div>
          <div className="text-muted-foreground tabular-nums">{day}</div>
          {hasData ? (
            <div className="flex flex-col gap-1 pt-1 border-t">
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">{t('dashboard.cache.hit_rate')}</span>
                <span className="font-mono font-semibold">{ratePercent(rate)}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">{t('dashboard.cache.cached')}</span>
                <span className="font-mono tabular-nums">{formatMetricValue(cell!.numerator, statisticsMetric)}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">{t('dashboard.cache.total')}</span>
                <span className="font-mono tabular-nums">{formatMetricValue(cell!.denominator, statisticsMetric)}</span>
              </div>
            </div>
          ) : (
            <div className="text-muted-foreground italic pt-1 border-t">{t('dashboard.cache.no_activity')}</div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
