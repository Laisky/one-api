import { formatNumber } from "@/lib/utils";
import { useTranslation } from "react-i18next";

interface InsightsProps {
  rangeInsights: {
    busiestDay: { date: string; requests: number } | null;
    tokenHeavyDay: { date: string; tokens: number } | null;
  };
  totalModels: number;
  totalRequests: number;
}

export function Insights({
  rangeInsights,
  totalModels,
  totalRequests,
}: InsightsProps) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.insights.busiest_day")}
        </div>
        <div className="text-lg font-semibold mt-1">
          {rangeInsights.busiestDay
            ? rangeInsights.busiestDay.date
            : t("dashboard.labels.no_data")}
        </div>
        {rangeInsights.busiestDay && (
          <div className="text-xs text-muted-foreground mt-2">
            {t("dashboard.labels.requests_value", {
              value: formatNumber(rangeInsights.busiestDay.requests),
            })}
          </div>
        )}
      </div>
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.insights.peak_token_day")}
        </div>
        <div className="text-lg font-semibold mt-1">
          {rangeInsights.tokenHeavyDay
            ? rangeInsights.tokenHeavyDay.date
            : t("dashboard.labels.no_data")}
        </div>
        {rangeInsights.tokenHeavyDay && (
          <div className="text-xs text-muted-foreground mt-2">
            {t("dashboard.labels.tokens_value", {
              value: formatNumber(rangeInsights.tokenHeavyDay.tokens),
            })}
          </div>
        )}
      </div>
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.insights.models_in_use")}
        </div>
        <div className="text-lg font-semibold mt-1">
          {formatNumber(totalModels)}
        </div>
        <div className="text-xs text-muted-foreground mt-2">
          {totalModels
            ? t("dashboard.insights.requests_per_model", {
                value: formatNumber(Math.round(totalRequests / totalModels)),
              })
            : t("dashboard.labels.no_value")}
        </div>
      </div>
    </div>
  );
}
