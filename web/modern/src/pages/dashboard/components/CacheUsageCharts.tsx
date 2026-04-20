import { formatNumber } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { getChartColor, getDisplayInCurrency } from '../types';

interface CacheUsageChartsProps {
  modelCacheData: Array<{ date: string; regular: number; cached: number }>;
  userCacheData: Array<{ date: string; regular: number; cached: number }>;
  tokenCacheData: Array<{ date: string; regular: number; cached: number }>;
  statisticsMetric: 'tokens' | 'requests' | 'expenses';
}

export function CacheUsageCharts({ modelCacheData, userCacheData, tokenCacheData, statisticsMetric }: CacheUsageChartsProps) {
  const { t } = useTranslation();

  const regularColor = getChartColor(1);
  const cachedColor = getChartColor(6);

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

  const regularLabel = t('dashboard.cache.regular');
  const cachedLabel = t('dashboard.cache.cached');

  const formatStackedTick = (value: number) => {
    switch (statisticsMetric) {
      case 'requests':
        return formatNumber(value);
      case 'expenses':
        return getDisplayInCurrency() ? `$${Number(value).toFixed(2)}` : formatNumber(value);
      case 'tokens':
      default:
        return formatNumber(value);
    }
  };

  const stackedTooltip = ({ active, payload, label }: any) => {
    if (active && payload && payload.length) {
      const filtered = payload
        .filter((entry: any) => entry.value && typeof entry.value === 'number' && entry.value > 0)
        .sort((a: any, b: any) => (b.value as number) - (a.value as number));

      if (!filtered.length) {
        return null;
      }

      const formatValue = (value: number) => {
        switch (statisticsMetric) {
          case 'requests':
            return formatNumber(value);
          case 'expenses':
            return getDisplayInCurrency() ? `$${value.toFixed(6)}` : formatNumber(value);
          case 'tokens':
          default:
            return formatNumber(value);
        }
      };

      const root = typeof document !== 'undefined' ? getComputedStyle(document.documentElement) : null;
      const tooltipBg = root ? `hsl(${root.getPropertyValue('--popover').trim()})` : '#fff';
      const tooltipText = root ? `hsl(${root.getPropertyValue('--popover-foreground').trim()})` : '#000';

      return (
        <div
          style={{
            backgroundColor: tooltipBg,
            border: '1px solid var(--border)',
            borderRadius: '8px',
            padding: '12px 16px',
            fontSize: '12px',
            color: tooltipText,
            boxShadow: '0 8px 32px hsl(0 0% 0% / 0.12)',
          }}
        >
          <div
            style={{
              fontWeight: '600',
              marginBottom: '8px',
              color: 'var(--foreground)',
            }}
          >
            {label}
          </div>
          {filtered.map((entry: any, index: number) => (
            <div
              key={`${entry.name ?? 'series'}-${index}`}
              style={{
                marginBottom: '4px',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <span
                style={{
                  display: 'inline-block',
                  width: '12px',
                  height: '12px',
                  backgroundColor: entry.color,
                  borderRadius: '50%',
                  marginRight: '8px',
                }}
              ></span>
              <span style={{ fontWeight: '600', color: 'var(--foreground)' }}>
                {entry.name}: {formatValue(entry.value as number)}
              </span>
            </div>
          ))}
        </div>
      );
    }

    return null;
  };

  return (
    <>
      <div className="bg-card rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">{t('dashboard.cache.model_title')}</h3>
          <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={modelCacheData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            <Bar dataKey="cached" name={cachedLabel} stackId="cache-models" fill={cachedColor} radius={[0, 0, 0, 0]} />
            <Bar dataKey="regular" name={regularLabel} stackId="cache-models" fill={regularColor} radius={[2, 2, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">{t('dashboard.cache.user_title')}</h3>
          <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={userCacheData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            <Bar dataKey="cached" name={cachedLabel} stackId="cache-users" fill={cachedColor} radius={[0, 0, 0, 0]} />
            <Bar dataKey="regular" name={regularLabel} stackId="cache-users" fill={regularColor} radius={[2, 2, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">{t('dashboard.cache.token_title')}</h3>
          <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={tokenCacheData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            <Bar dataKey="cached" name={cachedLabel} stackId="cache-tokens" fill={cachedColor} radius={[0, 0, 0, 0]} />
            <Bar dataKey="regular" name={regularLabel} stackId="cache-tokens" fill={regularColor} radius={[2, 2, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </>
  );
}
