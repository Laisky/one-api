import type { TFunction } from "i18next";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { chartConfig, GradientDefs } from "../services/chartConfig";

interface TrendSparklinesProps {
  t: TFunction;
  timeSeries: Array<{
    date: string;
    requests: number;
    quota: number;
    tokens: number;
  }>;
}

export const TrendSparklines = ({ t, timeSeries }: TrendSparklinesProps) => (
  <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
    <SparklineCard
      title={t("dashboard.labels.requests")}
      color={chartConfig.colors.requests}
      dataKey="requests"
      data={timeSeries}
    />
    <SparklineCard
      title={t("dashboard.labels.quota")}
      color={chartConfig.colors.quota}
      dataKey="quota"
      data={timeSeries}
    />
    <SparklineCard
      title={t("dashboard.labels.tokens")}
      color={chartConfig.colors.tokens}
      dataKey="tokens"
      data={timeSeries}
    />
  </div>
);

interface SparklineCardProps {
  title: string;
  color: string;
  dataKey: "requests" | "quota" | "tokens";
  data: Array<{
    date: string;
    requests: number;
    quota: number;
    tokens: number;
  }>;
}

const SparklineCard = ({ title, color, dataKey, data }: SparklineCardProps) => (
  <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
    <h3 className="font-medium mb-4" style={{ color }}>
      {title}
    </h3>
    <ResponsiveContainer width="100%" height={140}>
      <LineChart data={data}>
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
          dataKey={dataKey}
          stroke={color}
          strokeWidth={2}
          dot={false}
          activeDot={{ r: 4, fill: color }}
        />
      </LineChart>
    </ResponsiveContainer>
  </div>
);
