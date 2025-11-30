import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { UserOption } from "../types";

interface DashboardFilterProps {
  filtersReady: boolean;
  fromDate: string;
  toDate: string;
  dashUser: string;
  userOptions: UserOption[];
  isAdmin: boolean;
  loading: boolean;
  dateError: string;
  getMinDate: () => string;
  getMaxDate: () => string;
  setFromDate: (date: string) => void;
  setToDate: (date: string) => void;
  setDashUser: (user: string) => void;
  applyPreset: (preset: "today" | "7d" | "30d") => void;
  loadStats: () => void;
}

export function DashboardFilter({
  filtersReady,
  fromDate,
  toDate,
  dashUser,
  userOptions,
  isAdmin,
  loading,
  dateError,
  getMinDate,
  getMaxDate,
  setFromDate,
  setToDate,
  setDashUser,
  applyPreset,
  loadStats,
}: DashboardFilterProps) {
  const { t } = useTranslation();

  if (!filtersReady) {
    return (
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4 mb-6">
        <div className="flex flex-col gap-3 animate-pulse">
          <div className="h-4 bg-muted/30 rounded w-24" />
          <div className="h-11 bg-muted/30 rounded" />
          <div className="h-11 bg-muted/30 rounded" />
          <div className="h-11 bg-muted/30 rounded" />
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-gray-900 rounded-lg border p-4 mb-6">
      <div className="flex flex-col lg:flex-row gap-4 items-start lg:items-end w-full">
        <div className="flex flex-col sm:flex-row gap-3 flex-1 w-full">
          <div className="flex-1 min-w-0">
            <label className="text-sm font-medium mb-2 block">
              {t("dashboard.filters.from")}
            </label>
            <Input
              type="date"
              value={fromDate}
              min={getMinDate()}
              max={getMaxDate()}
              onChange={(e) => setFromDate(e.target.value)}
              className={cn("h-10", dateError ? "border-red-500" : "")}
              aria-label={t("dashboard.filters.from_aria")}
            />
          </div>
          <div className="flex-1 min-w-0">
            <label className="text-sm font-medium mb-2 block">
              {t("dashboard.filters.to")}
            </label>
            <Input
              type="date"
              value={toDate}
              min={getMinDate()}
              max={getMaxDate()}
              onChange={(e) => setToDate(e.target.value)}
              className={cn("h-10", dateError ? "border-red-500" : "")}
              aria-label={t("dashboard.filters.to_aria")}
            />
          </div>
          {isAdmin && (
            <div className="flex-1 min-w-0">
              <label className="text-sm font-medium mb-2 block">
                {t("dashboard.filters.user")}
              </label>
              <select
                className="h-11 sm:h-10 w-full border rounded-md px-3 py-2 text-base sm:text-sm bg-background"
                value={dashUser}
                onChange={(e) => setDashUser(e.target.value)}
                aria-label={t("dashboard.filters.user_aria")}
              >
                <option value="all">{t("dashboard.filters.all_users")}</option>
                {userOptions.map((u) => (
                  <option key={u.id} value={String(u.id)}>
                    {u.display_name || u.username}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>

        <div className="flex flex-wrap sm:flex-nowrap gap-2 w-full sm:w-auto sm:justify-end">
          <Button
            variant="outline"
            size="sm"
            onClick={() => applyPreset("today")}
            className="h-10 flex-1 min-w-[6rem] sm:flex-none"
          >
            {t("dashboard.filters.today")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => applyPreset("7d")}
            className="h-10 flex-1 min-w-[6rem] sm:flex-none"
          >
            {t("dashboard.filters.seven_days")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => applyPreset("30d")}
            className="h-10 flex-1 min-w-[6rem] sm:flex-none"
          >
            {t("dashboard.filters.thirty_days")}
          </Button>
          <Button
            onClick={loadStats}
            disabled={loading}
            className="h-10 flex-1 min-w-[6rem] sm:flex-none sm:px-6"
          >
            {loading
              ? t("dashboard.filters.loading")
              : t("dashboard.filters.apply")}
          </Button>
        </div>
      </div>
    </div>
  );
}
