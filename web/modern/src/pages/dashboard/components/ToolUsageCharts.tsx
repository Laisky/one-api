import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { formatNumber } from '@/lib/utils';
import { useTranslation } from 'react-i18next';
import { Bar, BarChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { barColor, getDisplayInCurrency } from '../types';

interface ToolUsageChartsProps {
  toolStackedData: any[];
  toolKeys: string[];
  toolUserStackedData: any[];
  toolUserKeys: string[];
  toolTokenStackedData: any[];
  toolTokenKeys: string[];
  toolStatisticsMetric: 'requests' | 'expenses';
  setToolStatisticsMetric: (metric: 'requests' | 'expenses') => void;
}

export function ToolUsageCharts({
  toolStackedData,
  toolKeys,
  toolUserStackedData,
  toolUserKeys,
  toolTokenStackedData,
  toolTokenKeys,
  toolStatisticsMetric,
  setToolStatisticsMetric,
}: ToolUsageChartsProps) {
  const { t } = useTranslation();

  const metricLabel = toolStatisticsMetric === 'expenses' ? t('dashboard.metrics.expenses') : t('dashboard.metrics.requests');

  const formatStackedTick = (value: number) => {
    if (toolStatisticsMetric === 'expenses') {
      return getDisplayInCurrency() ? `$${Number(value).toFixed(2)}` : formatNumber(value);
    }
    return formatNumber(value);
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
        if (toolStatisticsMetric === 'expenses') {
          return getDisplayInCurrency() ? `$${value.toFixed(6)}` : formatNumber(value);
        }
        return formatNumber(value);
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
          <h3 className="text-lg font-semibold">{t('dashboard.sections.tool_usage')}</h3>
          <Select value={toolStatisticsMetric} onValueChange={(value) => setToolStatisticsMetric(value as 'requests' | 'expenses')}>
            <SelectTrigger className="w-32">
              <SelectValue placeholder={t('dashboard.sections.metric_placeholder')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="requests">{t('dashboard.metrics.requests')}</SelectItem>
              <SelectItem value="expenses">{t('dashboard.metrics.expenses')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={toolStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            {toolKeys.map((toolKey, idx) => (
              <Bar key={toolKey} dataKey={toolKey} stackId="statistics-tools" fill={barColor(idx)} radius={[2, 2, 0, 0]} />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">{t('dashboard.sections.tool_user_usage')}</h3>
          <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={toolUserStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            {toolUserKeys.map((toolUserKey, idx) => (
              <Bar key={toolUserKey} dataKey={toolUserKey} stackId="statistics-tool-users" fill={barColor(idx)} radius={[2, 2, 0, 0]} />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">{t('dashboard.sections.tool_token_usage')}</h3>
          <span className="text-xs text-muted-foreground">{t('dashboard.sections.metric_label', { metric: metricLabel })}</span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={toolTokenStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
            <YAxis tickLine={false} axisLine={false} width={60} fontSize={12} tickFormatter={formatStackedTick} />
            <Tooltip content={stackedTooltip} />
            <Legend wrapperStyle={{ maxHeight: 80, overflowY: 'auto' }} />
            {toolTokenKeys.map((toolTokenKey, idx) => (
              <Bar key={toolTokenKey} dataKey={toolTokenKey} stackId="statistics-tool-tokens" fill={barColor(idx)} radius={[2, 2, 0, 0]} />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
    </>
  );
}
