import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BaseMetricRow, getDisplayInCurrency, getQuotaPerUnit, ModelRow, TokenRow, ToolMetricRow, ToolRow, ToolTokenRow, ToolUserRow, UserRow } from '../types';

export type CacheHeatmapEntity = {
  name: string;
  totalVolume: number;
  cachedVolume: number;
  overallRate: number;
  perDay: Record<string, { numerator: number; denominator: number }>;
};

export type CacheHeatmap = {
  entities: CacheHeatmapEntity[];
  days: string[];
};

export const useDashboardCharts = (
  rows: ModelRow[],
  userRows: UserRow[],
  tokenRows: TokenRow[],
  toolRows: ToolRow[],
  toolUserRows: ToolUserRow[],
  toolTokenRows: ToolTokenRow[],
  statisticsMetric: 'tokens' | 'requests' | 'expenses',
  toolStatisticsMetric: 'requests' | 'expenses'
) => {
  const { t } = useTranslation();

  const dailyAgg = useMemo(() => {
    const map: Record<string, { date: string; requests: number; quota: number; tokens: number }> = {};
    for (const r of rows) {
      if (!map[r.day]) {
        map[r.day] = { date: r.day, requests: 0, quota: 0, tokens: 0 };
      }
      map[r.day].requests += r.request_count || 0;
      map[r.day].quota += r.quota || 0;
      map[r.day].tokens += (r.prompt_tokens || 0) + (r.completion_tokens || 0);
    }
    return Object.values(map).sort((a, b) => a.date.localeCompare(b.date));
  }, [rows]);

  const xAxisDays = useMemo(() => {
    const values = new Set<string>();
    for (const row of rows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    for (const row of userRows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    for (const row of tokenRows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [rows, userRows, tokenRows]);

  const toolXAxisDays = useMemo(() => {
    const values = new Set<string>();
    for (const row of toolRows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    for (const row of toolUserRows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    for (const row of toolTokenRows) {
      if (row.day) {
        values.add(row.day);
      }
    }
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [toolRows, toolUserRows, toolTokenRows]);

  const timeSeries = useMemo(() => {
    const quotaPerUnit = getQuotaPerUnit();
    const displayInCurrency = getDisplayInCurrency();
    return dailyAgg.map((day) => ({
      date: day.date,
      requests: day.requests,
      quota: displayInCurrency ? day.quota / quotaPerUnit : day.quota,
      tokens: day.tokens,
    }));
  }, [dailyAgg]);

  const computeStackedSeries = <T extends BaseMetricRow>(rowsSource: T[], daysList: string[], labelFn: (row: T) => string | null) => {
    const quotaPerUnit = getQuotaPerUnit();
    const displayInCurrency = getDisplayInCurrency();
    const dayToValues: Record<string, Record<string, number>> = {};
    for (const day of daysList) {
      dayToValues[day] = {};
    }

    const uniqueKeys: string[] = [];
    const seen = new Set<string>();

    for (const row of rowsSource) {
      const label = labelFn(row);
      if (!label) {
        continue;
      }
      if (!seen.has(label)) {
        uniqueKeys.push(label);
        seen.add(label);
      }

      const day = row.day;
      if (!dayToValues[day]) {
        dayToValues[day] = {};
      }

      let value: number;
      switch (statisticsMetric) {
        case 'requests':
          value = row.request_count || 0;
          break;
        case 'expenses':
          value = row.quota || 0;
          if (displayInCurrency) {
            value = value / quotaPerUnit;
          }
          break;
        case 'tokens':
        default:
          value = (row.prompt_tokens || 0) + (row.completion_tokens || 0);
          break;
      }

      dayToValues[day][label] = (dayToValues[day][label] || 0) + value;
    }

    const stackedData = daysList.map((day) => ({
      date: day,
      ...(dayToValues[day] || {}),
    }));

    return { uniqueKeys, stackedData };
  };

  const computeToolStackedSeries = <T extends ToolMetricRow>(rowsSource: T[], daysList: string[], labelFn: (row: T) => string | null) => {
    const quotaPerUnit = getQuotaPerUnit();
    const displayInCurrency = getDisplayInCurrency();
    const dayToValues: Record<string, Record<string, number>> = {};
    for (const day of daysList) {
      dayToValues[day] = {};
    }

    const uniqueKeys: string[] = [];
    const seen = new Set<string>();

    for (const row of rowsSource) {
      const label = labelFn(row);
      if (!label) {
        continue;
      }
      if (!seen.has(label)) {
        uniqueKeys.push(label);
        seen.add(label);
      }

      const day = row.day;
      if (!dayToValues[day]) {
        dayToValues[day] = {};
      }

      let value = row.request_count || 0;
      if (toolStatisticsMetric === 'expenses') {
        value = row.quota || 0;
        if (displayInCurrency) {
          value = value / quotaPerUnit;
        }
      }

      dayToValues[day][label] = (dayToValues[day][label] || 0) + value;
    }

    const stackedData = daysList.map((day) => ({
      date: day,
      ...(dayToValues[day] || {}),
    }));

    return { uniqueKeys, stackedData };
  };

  const { uniqueKeys: modelKeys, stackedData: modelStackedData } = useMemo(
    () => computeStackedSeries(rows, xAxisDays, (row) => (row.model_name ? row.model_name : t('dashboard.fallbacks.model'))),
    [rows, xAxisDays, statisticsMetric, t]
  );

  const { uniqueKeys: userKeys, stackedData: userStackedData } = useMemo(
    () => computeStackedSeries(userRows, xAxisDays, (row) => (row.username ? row.username : t('dashboard.fallbacks.user'))),
    [userRows, xAxisDays, statisticsMetric, t]
  );

  const { uniqueKeys: tokenKeys, stackedData: tokenStackedData } = useMemo(
    () =>
      computeStackedSeries(tokenRows, xAxisDays, (row) => {
        const token = row.token_name && row.token_name.trim().length > 0 ? row.token_name : t('dashboard.fallbacks.token');
        const owner = row.username && row.username.trim().length > 0 ? row.username : t('dashboard.fallbacks.owner');
        return `${token}(${owner})`;
      }),
    [tokenRows, xAxisDays, statisticsMetric, t]
  );

  const { uniqueKeys: toolKeys, stackedData: toolStackedData } = useMemo(
    () => computeToolStackedSeries(toolRows, toolXAxisDays, (row) => (row.tool_name ? row.tool_name : t('dashboard.fallbacks.tool'))),
    [toolRows, toolXAxisDays, toolStatisticsMetric, t]
  );

  const { uniqueKeys: toolUserKeys, stackedData: toolUserStackedData } = useMemo(
    () => computeToolStackedSeries(toolUserRows, toolXAxisDays, (row) => (row.username ? row.username : t('dashboard.fallbacks.user'))),
    [toolUserRows, toolXAxisDays, toolStatisticsMetric, t]
  );

  const { uniqueKeys: toolTokenKeys, stackedData: toolTokenStackedData } = useMemo(
    () =>
      computeToolStackedSeries(toolTokenRows, toolXAxisDays, (row) => {
        const token = row.token_name && row.token_name.trim().length > 0 ? row.token_name : t('dashboard.fallbacks.token');
        const owner = row.username && row.username.trim().length > 0 ? row.username : t('dashboard.fallbacks.owner');
        return `${token}(${owner})`;
      }),
    [toolTokenRows, toolXAxisDays, toolStatisticsMetric, t]
  );

  const buildCacheHeatmap = <T extends BaseMetricRow>(
    rowsSource: T[],
    daysList: string[],
    entityKey: (row: T) => string | null
  ): CacheHeatmap => {
    const quotaPerUnit = getQuotaPerUnit();
    const displayInCurrency = getDisplayInCurrency();

    type Bucket = { numerator: number; denominator: number };
    const byEntity: Record<string, { totals: Bucket; perDay: Record<string, Bucket> }> = {};

    for (const row of rowsSource) {
      const name = entityKey(row);
      if (!name) continue;
      if (!byEntity[name]) {
        byEntity[name] = { totals: { numerator: 0, denominator: 0 }, perDay: {} };
      }

      let numerator = 0;
      let denominator = 0;
      switch (statisticsMetric) {
        case 'requests':
          numerator = row.cache_hit_count || 0;
          denominator = row.request_count || 0;
          break;
        case 'expenses': {
          const cacheQuota = row.cache_hit_quota || 0;
          const totalQuota = row.quota || 0;
          numerator = displayInCurrency ? cacheQuota / quotaPerUnit : cacheQuota;
          denominator = displayInCurrency ? totalQuota / quotaPerUnit : totalQuota;
          break;
        }
        case 'tokens':
        default:
          numerator = row.cached_prompt_tokens || 0;
          denominator = row.prompt_tokens || 0;
          break;
      }

      const bucket = byEntity[name];
      bucket.totals.numerator += numerator;
      bucket.totals.denominator += denominator;
      if (!bucket.perDay[row.day]) {
        bucket.perDay[row.day] = { numerator: 0, denominator: 0 };
      }
      bucket.perDay[row.day].numerator += numerator;
      bucket.perDay[row.day].denominator += denominator;
    }

    const entities = Object.entries(byEntity)
      .map(([name, { totals, perDay }]) => ({
        name,
        totalVolume: totals.denominator,
        cachedVolume: totals.numerator,
        overallRate: totals.denominator > 0 ? totals.numerator / totals.denominator : 0,
        perDay,
      }))
      .filter((e) => e.totalVolume > 0)
      .sort((a, b) => b.totalVolume - a.totalVolume);

    return { entities, days: daysList };
  };

  const modelHeatmap = useMemo(
    () => buildCacheHeatmap(rows, xAxisDays, (row) => (row.model_name ? row.model_name : t('dashboard.fallbacks.model'))),
    [rows, xAxisDays, statisticsMetric, t]
  );

  const userHeatmap = useMemo(
    () => buildCacheHeatmap(userRows, xAxisDays, (row) => (row.username ? row.username : t('dashboard.fallbacks.user'))),
    [userRows, xAxisDays, statisticsMetric, t]
  );

  const tokenHeatmap = useMemo(
    () =>
      buildCacheHeatmap(tokenRows, xAxisDays, (row) => {
        const token = row.token_name && row.token_name.trim().length > 0 ? row.token_name : t('dashboard.fallbacks.token');
        const owner = row.username && row.username.trim().length > 0 ? row.username : t('dashboard.fallbacks.owner');
        return `${token} (${owner})`;
      }),
    [tokenRows, xAxisDays, statisticsMetric, t]
  );

  const rangeTotals = useMemo(() => {
    let requests = 0;
    let quota = 0;
    let tokens = 0;
    const modelSet = new Set<string>();

    for (const row of rows) {
      requests += row.request_count || 0;
      quota += row.quota || 0;
      tokens += (row.prompt_tokens || 0) + (row.completion_tokens || 0);
      if (row.model_name) {
        modelSet.add(row.model_name);
      }
    }

    const dayCount = dailyAgg.length;
    const avgCostPerRequestRaw = requests ? quota / requests : 0;
    const avgTokensPerRequest = requests ? tokens / requests : 0;
    const avgDailyRequests = dayCount ? requests / dayCount : 0;
    const avgDailyQuotaRaw = dayCount ? quota / dayCount : 0;
    const avgDailyTokens = dayCount ? tokens / dayCount : 0;

    return {
      requests,
      quota,
      tokens,
      avgCostPerRequestRaw,
      avgTokensPerRequest,
      avgDailyRequests,
      avgDailyQuotaRaw,
      avgDailyTokens,
      dayCount,
      uniqueModels: modelSet.size,
    };
  }, [rows, dailyAgg]);

  const byModel = useMemo(() => {
    const mm: Record<string, { model: string; requests: number; quota: number; tokens: number }> = {};
    for (const r of rows) {
      const key = r.model_name;
      if (!mm[key]) mm[key] = { model: key, requests: 0, quota: 0, tokens: 0 };
      mm[key].requests += r.request_count || 0;
      mm[key].quota += r.quota || 0;
      mm[key].tokens += (r.prompt_tokens || 0) + (r.completion_tokens || 0);
    }
    return Object.values(mm);
  }, [rows]);

  const modelLeaders = useMemo(() => {
    if (!byModel.length) {
      return {
        mostRequested: null,
        mostTokens: null,
        mostQuota: null,
      };
    }

    const mostRequested = [...byModel].sort((a, b) => b.requests - a.requests)[0];
    const mostTokens = [...byModel].sort((a, b) => b.tokens - a.tokens)[0];
    const mostQuota = [...byModel].sort((a, b) => b.quota - a.quota)[0];

    return { mostRequested, mostTokens, mostQuota };
  }, [byModel]);

  const rangeInsights = useMemo(() => {
    if (!dailyAgg.length) {
      return {
        busiestDay: null as {
          date: string;
          requests: number;
          quota: number;
          tokens: number;
        } | null,
        tokenHeavyDay: null as {
          date: string;
          requests: number;
          quota: number;
          tokens: number;
        } | null,
      };
    }

    let busiestDay = dailyAgg[0];
    let tokenHeavyDay = dailyAgg[0];

    for (const day of dailyAgg) {
      if (day.requests > busiestDay.requests) {
        busiestDay = day;
      }
      if (day.tokens > tokenHeavyDay.tokens) {
        tokenHeavyDay = day;
      }
    }

    return { busiestDay, tokenHeavyDay };
  }, [dailyAgg]);

  return {
    dailyAgg,
    timeSeries,
    modelKeys,
    modelStackedData,
    userKeys,
    userStackedData,
    tokenKeys,
    tokenStackedData,
    toolKeys,
    toolStackedData,
    toolUserKeys,
    toolUserStackedData,
    toolTokenKeys,
    toolTokenStackedData,
    modelHeatmap,
    userHeatmap,
    tokenHeatmap,
    rangeTotals,
    modelLeaders,
    rangeInsights,
  };
};

export const CACHE_SERIES_KEYS = {
  regular: 'regular' as const,
  cached: 'cached' as const,
};
