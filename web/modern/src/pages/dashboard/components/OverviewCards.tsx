import { formatNumber } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { getDisplayInCurrency, getQuotaPerUnit } from "../types";

interface OverviewCardsProps {
  totalRequests: number;
  totalQuota: number;
  totalTokens: number;
  avgDailyRequests: number;
  avgDailyQuotaRaw: number;
  avgDailyTokens: number;
  avgCostPerRequestRaw: number;
  avgTokensPerRequest: number;
}

const renderQuota = (quota: number, precision: number = 2): string => {
  const displayInCurrency = getDisplayInCurrency();
  const quotaPerUnit = getQuotaPerUnit();

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(precision);
    return `$${amount}`;
  }

  return formatNumber(quota);
};

export function OverviewCards({
  totalRequests,
  totalQuota,
  totalTokens,
  avgDailyRequests,
  avgDailyQuotaRaw,
  avgDailyTokens,
  avgCostPerRequestRaw,
  avgTokensPerRequest,
}: OverviewCardsProps) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.cards.total_requests")}
        </div>
        <div className="text-2xl font-bold mt-1">
          {formatNumber(totalRequests)}
        </div>
        <div className="text-xs text-muted-foreground mt-2">
          {t("dashboard.cards.avg_daily", {
            value: formatNumber(Math.round(avgDailyRequests || 0)),
          })}
        </div>
      </div>
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.cards.quota_used")}
        </div>
        <div className="text-2xl font-bold mt-1">{renderQuota(totalQuota)}</div>
        <div className="text-xs text-muted-foreground mt-2">
          {t("dashboard.cards.avg_daily", {
            value: renderQuota(avgDailyQuotaRaw),
          })}
        </div>
      </div>
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.cards.tokens_consumed")}
        </div>
        <div className="text-2xl font-bold mt-1">
          {formatNumber(totalTokens)}
        </div>
        <div className="text-xs text-muted-foreground mt-2">
          {t("dashboard.cards.avg_daily", {
            value: formatNumber(Math.round(avgDailyTokens || 0)),
          })}
        </div>
      </div>
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <div className="text-sm text-muted-foreground">
          {t("dashboard.cards.avg_cost")}
        </div>
        <div className="text-2xl font-bold mt-1">
          {renderQuota(avgCostPerRequestRaw, 4)}
        </div>
        <div className="text-xs text-muted-foreground mt-2">
          {t("dashboard.cards.tokens_per_request", {
            value: Math.round(avgTokensPerRequest || 0),
          })}
        </div>
      </div>
    </div>
  );
}
