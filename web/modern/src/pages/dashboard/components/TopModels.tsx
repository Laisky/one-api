import { formatNumber } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { getDisplayInCurrency, getQuotaPerUnit } from "../types";

interface TopModelsProps {
  modelLeaders: {
    mostRequested: { model: string; requests: number } | null;
    mostTokens: { model: string; tokens: number } | null;
    mostQuota: { model: string; quota: number } | null;
  };
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

export function TopModels({ modelLeaders }: TopModelsProps) {
  const { t } = useTranslation();

  return (
    <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
      <h3 className="text-lg font-semibold mb-4">
        {t("dashboard.top_models.title")}
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
          <div className="text-sm text-muted-foreground">
            {t("dashboard.top_models.most_requests")}
          </div>
          <div className="text-xl font-semibold mt-1">
            {modelLeaders.mostRequested
              ? modelLeaders.mostRequested.model
              : t("dashboard.labels.no_data")}
          </div>
          {modelLeaders.mostRequested && (
            <div className="text-xs text-muted-foreground mt-2">
              {t("dashboard.labels.requests_value", {
                value: formatNumber(modelLeaders.mostRequested.requests),
              })}
            </div>
          )}
        </div>
        <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
          <div className="text-sm text-muted-foreground">
            {t("dashboard.top_models.most_tokens")}
          </div>
          <div className="text-xl font-semibold mt-1">
            {modelLeaders.mostTokens
              ? modelLeaders.mostTokens.model
              : t("dashboard.labels.no_data")}
          </div>
          {modelLeaders.mostTokens && (
            <div className="text-xs text-muted-foreground mt-2">
              {t("dashboard.labels.tokens_value", {
                value: formatNumber(modelLeaders.mostTokens.tokens),
              })}
            </div>
          )}
        </div>
        <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
          <div className="text-sm text-muted-foreground">
            {t("dashboard.top_models.highest_cost")}
          </div>
          <div className="text-xl font-semibold mt-1">
            {modelLeaders.mostQuota
              ? modelLeaders.mostQuota.model
              : t("dashboard.labels.no_data")}
          </div>
          {modelLeaders.mostQuota && (
            <div className="text-xs text-muted-foreground mt-2">
              {t("dashboard.labels.quota_consumed", {
                value: renderQuota(modelLeaders.mostQuota.quota),
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
