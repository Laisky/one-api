import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { TimestampDisplay } from "@/components/ui/timestamp";
import { useAuthStore } from "@/lib/stores/auth";
import { useLayoutEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DashboardFilter } from "./components/DashboardFilter";
import { Insights } from "./components/Insights";
import { OverviewCards } from "./components/OverviewCards";
import { TimeSeriesCharts } from "./components/TimeSeriesCharts";
import { TopModels } from "./components/TopModels";
import { UsageCharts } from "./components/UsageCharts";
import { useDashboardCharts } from "./hooks/useDashboardCharts";
import { useDashboardData } from "./hooks/useDashboardData";

export function DashboardPage() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const [filtersReady, setFiltersReady] = useState(false);
  const [statisticsMetric, setStatisticsMetric] = useState<
    "tokens" | "requests" | "expenses"
  >("tokens");

  useLayoutEffect(() => {
    if (typeof document === "undefined") {
      return;
    }

    const active = document.activeElement as HTMLElement | null;
    if (active && ["INPUT", "SELECT", "TEXTAREA"].includes(active.tagName)) {
      active.blur();
    }

    if (!filtersReady) {
      requestAnimationFrame(() => setFiltersReady(true));
    }
  }, []);

  const {
    isAdmin,
    fromDate,
    setFromDate,
    toDate,
    setToDate,
    dashUser,
    setDashUser,
    userOptions,
    loading,
    lastUpdated,
    dateError,
    rows,
    userRows,
    tokenRows,
    loadStats,
    applyPreset,
    getMinDate,
    getMaxDate,
  } = useDashboardData();

  const {
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
  } = useDashboardCharts(rows, userRows, tokenRows, statisticsMetric);

  if (!user) {
    return <div>{t("dashboard.login_required")}</div>;
  }

  return (
    <ResponsivePageContainer
      title={t("dashboard.title")}
      description={t("dashboard.description")}
    >
      {loading && (
        <div
          className="flex items-center gap-3 mb-4 p-3 rounded-md border bg-muted/30"
          role="status"
          aria-live="polite"
        >
          <span className="h-5 w-5 animate-spin rounded-full border-2 border-primary/20 border-t-primary flex-shrink-0" />
          <p className="text-sm text-muted-foreground">
            {t("dashboard.loading")}
          </p>
        </div>
      )}

      <DashboardFilter
        filtersReady={filtersReady}
        fromDate={fromDate}
        toDate={toDate}
        dashUser={dashUser}
        userOptions={userOptions}
        isAdmin={isAdmin}
        loading={loading}
        dateError={dateError}
        getMinDate={getMinDate}
        getMaxDate={getMaxDate}
        setFromDate={setFromDate}
        setToDate={setToDate}
        setDashUser={setDashUser}
        applyPreset={applyPreset}
        loadStats={loadStats}
      />

      {/* Error Message */}
      {dateError && (
        <div
          id="date-error"
          className="mb-4 p-3 bg-destructive/5 border border-destructive/20 rounded-md"
          role="alert"
          aria-live="polite"
        >
          <div className="flex items-center gap-2">
            <svg
              className="w-4 h-4 text-destructive"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span className="text-sm font-medium text-destructive">
              {t("dashboard.errors.label")}
            </span>
          </div>
          <p className="text-sm text-destructive/80 mt-1">
            {dateError}
          </p>
        </div>
      )}

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

        <OverviewCards
          totalRequests={rangeTotals.requests}
          totalQuota={rangeTotals.quota}
          totalTokens={rangeTotals.tokens}
          avgDailyRequests={rangeTotals.avgDailyRequests}
          avgDailyQuotaRaw={rangeTotals.avgDailyQuotaRaw}
          avgDailyTokens={rangeTotals.avgDailyTokens}
          avgCostPerRequestRaw={rangeTotals.avgCostPerRequestRaw}
          avgTokensPerRequest={rangeTotals.avgTokensPerRequest}
        />

        <TopModels modelLeaders={modelLeaders} />

        <Insights
          rangeInsights={rangeInsights}
          totalModels={rangeTotals.uniqueModels}
          totalRequests={rangeTotals.requests}
        />

        <TimeSeriesCharts timeSeries={timeSeries} />

        <UsageCharts
          modelStackedData={modelStackedData}
          modelKeys={modelKeys}
          userStackedData={userStackedData}
          userKeys={userKeys}
          tokenStackedData={tokenStackedData}
          tokenKeys={tokenKeys}
          statisticsMetric={statisticsMetric}
          setStatisticsMetric={setStatisticsMetric}
        />
      </div>
    </ResponsivePageContainer>
  );
}

export default DashboardPage;
