import { formatNumber } from '@/lib/utils';
import { getChartColor, barColor as barColorFromTypes } from '../types';

export const getQuotaPerUnit = () => parseFloat(localStorage.getItem('quota_per_unit') || '500000');
export const getDisplayInCurrency = () => localStorage.getItem('display_in_currency') === 'true';

export const renderQuota = (quota: number, precision: number = 2): string => {
  const displayInCurrency = getDisplayInCurrency();
  const quotaPerUnit = getQuotaPerUnit();

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(precision);
    return `$${amount}`;
  }

  return formatNumber(quota);
};

export const chartConfig = {
  /** Resolved at render time from CSS variables --chart-1/2/3 */
  get colors() {
    return {
      requests: getChartColor(1),
      quota: getChartColor(2),
      tokens: getChartColor(3),
    };
  },
};

/** Re-export from types for backward compatibility */
export const barColor = barColorFromTypes;

export const GradientDefs = () => {
  const requests = getChartColor(1);
  const quota = getChartColor(2);
  const tokens = getChartColor(3);

  return (
    <defs>
      <linearGradient id="requestsGradient" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor={requests} stopOpacity={0.8} />
        <stop offset="100%" stopColor={requests} stopOpacity={0.1} />
      </linearGradient>
      <linearGradient id="quotaGradient" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor={quota} stopOpacity={0.8} />
        <stop offset="100%" stopColor={quota} stopOpacity={0.1} />
      </linearGradient>
      <linearGradient id="tokensGradient" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor={tokens} stopOpacity={0.8} />
        <stop offset="100%" stopColor={tokens} stopOpacity={0.1} />
      </linearGradient>
    </defs>
  );
};
