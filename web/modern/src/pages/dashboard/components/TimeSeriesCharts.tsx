import { useTranslation } from "react-i18next";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { GradientDefs } from "../services/chartConfig";
import { CHART_CONFIG } from "../types";

interface TimeSeriesChartsProps {
  timeSeries: any[];
}

export function TimeSeriesCharts({ timeSeries }: TimeSeriesChartsProps) {
  const { t } = useTranslation();
  const { requests, quota, tokens } = CHART_CONFIG.colors;

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
      <div className="bg-card rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-chart-1">
          {t("dashboard.labels.requests")}
        </h3>
        <ResponsiveContainer width="100%" height={140}>
          <LineChart data={timeSeries}>
            <GradientDefs />
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" hide />
            <YAxis hide />
            <Tooltip
              contentStyle={{
                backgroundColor: "var(--background)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                fontSize: "12px",
              }}
            />
            <Line
              type="monotone"
              dataKey="requests"
              stroke={requests}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: requests }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-chart-2">
          {t("dashboard.labels.quota")}
        </h3>
        <ResponsiveContainer width="100%" height={140}>
          <LineChart data={timeSeries}>
            <GradientDefs />
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" hide />
            <YAxis hide />
            <Tooltip
              contentStyle={{
                backgroundColor: "var(--background)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                fontSize: "12px",
              }}
            />
            <Line
              type="monotone"
              dataKey="quota"
              stroke={quota}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: quota }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-card rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-chart-3">
          {t("dashboard.labels.tokens")}
        </h3>
        <ResponsiveContainer width="100%" height={140}>
          <LineChart data={timeSeries}>
            <GradientDefs />
            <CartesianGrid strokeOpacity={0.1} vertical={false} />
            <XAxis dataKey="date" hide />
            <YAxis hide />
            <Tooltip
              contentStyle={{
                backgroundColor: "var(--background)",
                border: "1px solid var(--border)",
                borderRadius: "8px",
                fontSize: "12px",
              }}
            />
            <Line
              type="monotone"
              dataKey="tokens"
              stroke={tokens}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: tokens }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
