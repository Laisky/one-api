import type { TFunction } from "i18next";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  type TooltipProps,
  XAxis,
  YAxis,
} from "recharts";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { barColor } from "../services/chartConfig";

type UsageTooltipContent = TooltipProps<number, string>["content"];

interface BaseUsageProps {
  title: string;
  subtitle?: string;
  keys: string[];
  data: Array<Record<string, number | string>>;
  tickFormatter: (value: number) => string;
  tooltipContent: UsageTooltipContent;
}

interface ModelUsageProps extends BaseUsageProps {
  statisticsMetric: "tokens" | "requests" | "expenses";
  onMetricChange: (metric: "tokens" | "requests" | "expenses") => void;
  t: TFunction;
}

export const ModelUsageSection = ({
  title,
  keys,
  data,
  tickFormatter,
  tooltipContent,
  statisticsMetric,
  onMetricChange,
  t,
}: ModelUsageProps) => (
  <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
    <div className="flex items-center justify-between mb-6">
      <h3 className="text-lg font-semibold">{title}</h3>
      <Select
        value={statisticsMetric}
        onValueChange={(value) =>
          onMetricChange(value as "tokens" | "requests" | "expenses")
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
    <StackedChart
      keys={keys}
      data={data}
      tickFormatter={tickFormatter}
      tooltipContent={tooltipContent}
    />
  </div>
);

export const EntityUsageSection = ({
  title,
  subtitle,
  keys,
  data,
  tickFormatter,
  tooltipContent,
}: BaseUsageProps) => (
  <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
    <div className="flex items-center justify-between mb-6">
      <h3 className="text-lg font-semibold">{title}</h3>
      {subtitle && (
        <span className="text-xs text-muted-foreground">{subtitle}</span>
      )}
    </div>
    <StackedChart
      keys={keys}
      data={data}
      tickFormatter={tickFormatter}
      tooltipContent={tooltipContent}
    />
  </div>
);

const StackedChart = ({
  keys,
  data,
  tickFormatter,
  tooltipContent,
}: Pick<
  BaseUsageProps,
  "keys" | "data" | "tickFormatter" | "tooltipContent"
>) => (
  <ResponsiveContainer width="100%" height={300}>
    <BarChart data={data}>
      <CartesianGrid strokeOpacity={0.1} vertical={false} />
      <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
      <YAxis
        tickLine={false}
        axisLine={false}
        width={60}
        fontSize={12}
        tickFormatter={tickFormatter}
      />
      <Tooltip content={tooltipContent} />
      <Legend />
      {keys.map((key, idx) => (
        <Bar
          key={key}
          dataKey={key}
          stackId="usage"
          fill={barColor(idx)}
          radius={[2, 2, 0, 0]}
        />
      ))}
    </BarChart>
  </ResponsiveContainer>
);
