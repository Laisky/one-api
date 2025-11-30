import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatNumber } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { barColor, getDisplayInCurrency } from "../types";

interface UsageChartsProps {
  modelStackedData: any[];
  modelKeys: string[];
  userStackedData: any[];
  userKeys: string[];
  tokenStackedData: any[];
  tokenKeys: string[];
  statisticsMetric: "tokens" | "requests" | "expenses";
  setStatisticsMetric: (metric: "tokens" | "requests" | "expenses") => void;
}

export function UsageCharts({
  modelStackedData,
  modelKeys,
  userStackedData,
  userKeys,
  tokenStackedData,
  tokenKeys,
  statisticsMetric,
  setStatisticsMetric,
}: UsageChartsProps) {
  const { t } = useTranslation();

  const metricLabel = (() => {
    switch (statisticsMetric) {
      case "requests":
        return t("dashboard.metrics.requests");
      case "expenses":
        return t("dashboard.metrics.expenses");
      default:
        return t("dashboard.metrics.tokens");
    }
  })();

  const formatStackedTick = (value: number) => {
    switch (statisticsMetric) {
      case "requests":
        return formatNumber(value);
      case "expenses":
        return getDisplayInCurrency()
          ? `$${Number(value).toFixed(2)}`
          : formatNumber(value);
      case "tokens":
      default:
        return formatNumber(value);
    }
  };

  const stackedTooltip = ({ active, payload, label }: any) => {
    if (active && payload && payload.length) {
      const filtered = payload
        .filter(
          (entry: any) =>
            entry.value && typeof entry.value === "number" && entry.value > 0
        )
        .sort((a: any, b: any) => (b.value as number) - (a.value as number));

      if (!filtered.length) {
        return null;
      }

      const formatValue = (value: number) => {
        switch (statisticsMetric) {
          case "requests":
            return formatNumber(value);
          case "expenses":
            return getDisplayInCurrency()
              ? `$${value.toFixed(6)}`
              : formatNumber(value);
          case "tokens":
          default:
            return formatNumber(value);
        }
      };

      const isDark =
        typeof document !== "undefined" &&
        document.documentElement.classList.contains("dark");
      const tooltipBg = isDark ? "rgba(17,24,39,1)" : "rgba(255,255,255,1)";
      const tooltipText = isDark
        ? "rgba(255,255,255,0.95)"
        : "rgba(17,24,39,0.9)";

      return (
        <div
          style={{
            backgroundColor: tooltipBg,
            border: "1px solid var(--border)",
            borderRadius: "8px",
            padding: "12px 16px",
            fontSize: "12px",
            color: tooltipText,
            boxShadow: "0 8px 32px rgba(0, 0, 0, 0.12)",
          }}
        >
          <div
            style={{
              fontWeight: "600",
              marginBottom: "8px",
              color: "var(--foreground)",
            }}
          >
            {label}
          </div>
          {filtered.map((entry: any, index: number) => (
            <div
              key={`${entry.name ?? "series"}-${index}`}
              style={{
                marginBottom: "4px",
                display: "flex",
                alignItems: "center",
              }}
            >
              <span
                style={{
                  display: "inline-block",
                  width: "12px",
                  height: "12px",
                  backgroundColor: entry.color,
                  borderRadius: "50%",
                  marginRight: "8px",
                }}
              ></span>
              <span style={{ fontWeight: "600", color: "var(--foreground)" }}>
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
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">
            {t("dashboard.sections.model_usage")}
          </h3>
          <Select
            value={statisticsMetric}
            onValueChange={(value) =>
              setStatisticsMetric(value as "tokens" | "requests" | "expenses")
            }
          >
            <SelectTrigger className="w-32">
              <SelectValue
                placeholder={t("dashboard.sections.metric_placeholder")}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="tokens">
                {t("dashboard.metrics.tokens")}
              </SelectItem>
              <SelectItem value="requests">
                {t("dashboard.metrics.requests")}
              </SelectItem>
              <SelectItem value="expenses">
                {t("dashboard.metrics.expenses")}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={modelStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis
              dataKey="date"
              tickLine={false}
              axisLine={false}
              fontSize={12}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={60}
              fontSize={12}
              tickFormatter={formatStackedTick}
            />
            <Tooltip content={stackedTooltip} />
            <Legend />
            {modelKeys.map((m, idx) => (
              <Bar
                key={m}
                dataKey={m}
                stackId="statistics-models"
                fill={barColor(idx)}
                radius={[2, 2, 0, 0]}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">
            {t("dashboard.sections.user_usage")}
          </h3>
          <span className="text-xs text-muted-foreground">
            {t("dashboard.sections.metric_label", { metric: metricLabel })}
          </span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={userStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis
              dataKey="date"
              tickLine={false}
              axisLine={false}
              fontSize={12}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={60}
              fontSize={12}
              tickFormatter={formatStackedTick}
            />
            <Tooltip content={stackedTooltip} />
            <Legend />
            {userKeys.map((userKey, idx) => (
              <Bar
                key={userKey}
                dataKey={userKey}
                stackId="statistics-users"
                fill={barColor(idx)}
                radius={[2, 2, 0, 0]}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
        <div className="flex items-center justify-between mb-6">
          <h3 className="text-lg font-semibold">
            {t("dashboard.sections.token_usage")}
          </h3>
          <span className="text-xs text-muted-foreground">
            {t("dashboard.sections.metric_label", { metric: metricLabel })}
          </span>
        </div>
        <ResponsiveContainer width="100%" height={300}>
          <BarChart data={tokenStackedData}>
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis
              dataKey="date"
              tickLine={false}
              axisLine={false}
              fontSize={12}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={60}
              fontSize={12}
              tickFormatter={formatStackedTick}
            />
            <Tooltip content={stackedTooltip} />
            <Legend />
            {tokenKeys.map((tokenKey, idx) => (
              <Bar
                key={tokenKey}
                dataKey={tokenKey}
                stackId="statistics-tokens"
                fill={barColor(idx)}
                radius={[2, 2, 0, 0]}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
    </>
  );
}
