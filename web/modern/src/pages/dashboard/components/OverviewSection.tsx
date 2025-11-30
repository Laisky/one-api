import type { TFunction } from "i18next";
import type { ReactNode } from "react";
import { TimestampDisplay } from "@/components/ui/timestamp";
import { formatNumber } from "@/lib/utils";
import { renderQuota } from "../services/chartConfig";

interface OverviewSectionProps {
  t: TFunction;
  lastUpdated: number | null;
  totals: {
    requests: number;
    quota: number;
    tokens: number;
    avgCostPerRequestRaw: number;
    avgTokensPerRequest: number;
    avgDailyRequests: number;
    avgDailyQuotaRaw: number;
    avgDailyTokens: number;
    uniqueModels: number;
  };
  modelLeaders: {
    mostRequested: { model: string; requests: number } | null;
    mostTokens: { model: string; tokens: number } | null;
    mostQuota: { model: string; quota: number } | null;
  };
  rangeInsights: {
    busiestDay: { date: string; requests: number } | null;
    tokenHeavyDay: { date: string; tokens: number } | null;
  };
}

export const OverviewSection = ({
  t,
  lastUpdated,
  totals,
  modelLeaders,
  rangeInsights,
}: OverviewSectionProps) => (
  <div className="mb-6">
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mb-6">
      <div>
        <h2 className="text-xl font-semibold">
          {t("dashboard.overview.title")}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t("dashboard.overview.subtitle")}
        </p>
      </div>
      {lastUpdated && (
        <span className="text-xs text-muted-foreground flex items-center gap-1">
          {t("dashboard.overview.updated")}
          <TimestampDisplay timestamp={lastUpdated} className="font-mono" />
        </span>
      )}
    </div>

    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
      <Card
        title={t("dashboard.cards.total_requests")}
        value={formatNumber(totals.requests)}
      >
        {t("dashboard.cards.avg_daily", {
          value: formatNumber(Math.round(totals.avgDailyRequests || 0)),
        })}
      </Card>
      <Card
        title={t("dashboard.cards.quota_used")}
        value={renderQuota(totals.quota)}
      >
        {t("dashboard.cards.avg_daily", {
          value: renderQuota(totals.avgDailyQuotaRaw),
        })}
      </Card>
      <Card
        title={t("dashboard.cards.tokens_consumed")}
        value={formatNumber(totals.tokens)}
      >
        {t("dashboard.cards.avg_daily", {
          value: formatNumber(Math.round(totals.avgDailyTokens || 0)),
        })}
      </Card>
      <Card
        title={t("dashboard.cards.avg_cost")}
        value={renderQuota(totals.avgCostPerRequestRaw, 4)}
      >
        {t("dashboard.cards.tokens_per_request", {
          value: Math.round(totals.avgTokensPerRequest || 0),
        })}
      </Card>
    </div>

    <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
      <h3 className="text-lg font-semibold mb-4">
        {t("dashboard.top_models.title")}
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <LeaderCard
          label={t("dashboard.top_models.most_requests")}
          value={
            modelLeaders.mostRequested?.model ?? t("dashboard.labels.no_data")
          }
          helper={
            modelLeaders.mostRequested
              ? t("dashboard.labels.requests_value", {
                  value: formatNumber(modelLeaders.mostRequested.requests),
                })
              : ""
          }
        />
        <LeaderCard
          label={t("dashboard.top_models.most_tokens")}
          value={
            modelLeaders.mostTokens?.model ?? t("dashboard.labels.no_data")
          }
          helper={
            modelLeaders.mostTokens
              ? t("dashboard.labels.tokens_value", {
                  value: formatNumber(modelLeaders.mostTokens.tokens),
                })
              : ""
          }
        />
        <LeaderCard
          label={t("dashboard.top_models.highest_cost")}
          value={modelLeaders.mostQuota?.model ?? t("dashboard.labels.no_data")}
          helper={
            modelLeaders.mostQuota
              ? t("dashboard.labels.quota_consumed", {
                  value: renderQuota(modelLeaders.mostQuota.quota),
                })
              : ""
          }
        />
      </div>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <InsightCard
        label={t("dashboard.insights.busiest_day")}
        value={rangeInsights.busiestDay?.date ?? t("dashboard.labels.no_data")}
        helper={
          rangeInsights.busiestDay
            ? t("dashboard.labels.requests_value", {
                value: formatNumber(rangeInsights.busiestDay.requests),
              })
            : ""
        }
      />
      <InsightCard
        label={t("dashboard.insights.peak_token_day")}
        value={
          rangeInsights.tokenHeavyDay?.date ?? t("dashboard.labels.no_data")
        }
        helper={
          rangeInsights.tokenHeavyDay
            ? t("dashboard.labels.tokens_value", {
                value: formatNumber(rangeInsights.tokenHeavyDay.tokens),
              })
            : ""
        }
      />
      <InsightCard
        label={t("dashboard.insights.models_in_use")}
        value={formatNumber(totals.uniqueModels)}
        helper={
          totals.uniqueModels
            ? t("dashboard.insights.requests_per_model", {
                value: formatNumber(
                  Math.round(totals.requests / totals.uniqueModels)
                ),
              })
            : t("dashboard.labels.no_value")
        }
      />
    </div>
  </div>
);

const Card = ({
  title,
  value,
  children,
}: {
  title: string;
  value: string;
  children: ReactNode;
}) => (
  <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
    <div className="text-sm text-muted-foreground">{title}</div>
    <div className="text-2xl font-bold mt-1">{value}</div>
    <div className="text-xs text-muted-foreground mt-2">{children}</div>
  </div>
);

const LeaderCard = ({
  label,
  value,
  helper,
}: {
  label: string;
  value: string;
  helper: string;
}) => (
  <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
    <div className="text-sm text-muted-foreground">{label}</div>
    <div className="text-xl font-semibold mt-1">{value}</div>
    {helper && (
      <div className="text-xs text-muted-foreground mt-2">{helper}</div>
    )}
  </div>
);

const InsightCard = ({
  label,
  value,
  helper,
}: {
  label: string;
  value: string;
  helper: string;
}) => (
  <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
    <div className="text-sm text-muted-foreground">{label}</div>
    <div className="text-lg font-semibold mt-1">{value}</div>
    {helper && (
      <div className="text-xs text-muted-foreground mt-2">{helper}</div>
    )}
  </div>
);
