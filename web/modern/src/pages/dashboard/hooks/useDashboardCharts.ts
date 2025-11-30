import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  BaseMetricRow,
  getDisplayInCurrency,
  getQuotaPerUnit,
  ModelRow,
  TokenRow,
  UserRow,
} from "../types";

export const useDashboardCharts = (
  rows: ModelRow[],
  userRows: UserRow[],
  tokenRows: TokenRow[],
  statisticsMetric: "tokens" | "requests" | "expenses"
) => {
  const { t } = useTranslation();

  const dailyAgg = useMemo(() => {
    const map: Record<
      string,
      { date: string; requests: number; quota: number; tokens: number }
    > = {};
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

  const computeStackedSeries = <T extends BaseMetricRow>(
    rowsSource: T[],
    daysList: string[],
    labelFn: (row: T) => string | null
  ) => {
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
        case "requests":
          value = row.request_count || 0;
          break;
        case "expenses":
          value = row.quota || 0;
          if (displayInCurrency) {
            value = value / quotaPerUnit;
          }
          break;
        case "tokens":
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

  const { uniqueKeys: modelKeys, stackedData: modelStackedData } = useMemo(
    () =>
      computeStackedSeries(rows, xAxisDays, (row) =>
        row.model_name ? row.model_name : t("dashboard.fallbacks.model")
      ),
    [rows, xAxisDays, statisticsMetric, t]
  );

  const { uniqueKeys: userKeys, stackedData: userStackedData } = useMemo(
    () =>
      computeStackedSeries(userRows, xAxisDays, (row) =>
        row.username ? row.username : t("dashboard.fallbacks.user")
      ),
    [userRows, xAxisDays, statisticsMetric, t]
  );

  const { uniqueKeys: tokenKeys, stackedData: tokenStackedData } = useMemo(
    () =>
      computeStackedSeries(tokenRows, xAxisDays, (row) => {
        const token =
          row.token_name && row.token_name.trim().length > 0
            ? row.token_name
            : t("dashboard.fallbacks.token");
        const owner =
          row.username && row.username.trim().length > 0
            ? row.username
            : t("dashboard.fallbacks.owner");
        return `${token}(${owner})`;
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
    const mm: Record<
      string,
      { model: string; requests: number; quota: number; tokens: number }
    > = {};
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

    const mostRequested = [...byModel].sort(
      (a, b) => b.requests - a.requests
    )[0];
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
    rangeTotals,
    modelLeaders,
    rangeInsights,
  };
};
