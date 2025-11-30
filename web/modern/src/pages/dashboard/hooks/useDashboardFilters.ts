import type { TFunction } from "i18next";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
import { api } from "@/lib/api";

interface UserOption {
  id: number;
  username: string;
  display_name: string;
}

interface UseDashboardFiltersArgs {
  isAdmin: boolean;
  t: TFunction;
}

interface ApplyPresetResult {
  from: string;
  to: string;
}

export const useDashboardFilters = ({
  isAdmin,
  t,
}: UseDashboardFiltersArgs) => {
  const [filtersReady, setFiltersReady] = useState(false);
  const [dashUser, setDashUser] = useState<string>("all");
  const [userOptions, setUserOptions] = useState<UserOption[]>([]);
  const [dateError, setDateError] = useState("");

  const fmt = useCallback((d: Date) => d.toISOString().slice(0, 10), []);
  const today = useMemo(() => new Date(), []);
  const last7 = useMemo(() => {
    const clone = new Date();
    clone.setDate(clone.getDate() - 6);
    return clone;
  }, []);

  const [fromDate, setFromDate] = useState(fmt(last7));
  const [toDate, setToDate] = useState(fmt(today));

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
  }, [filtersReady]);

  const getMaxDate = useCallback(() => {
    const now = new Date();
    return now.toISOString().split("T")[0];
  }, []);

  const getMinDate = useCallback(() => {
    if (isAdmin) {
      const oneYearAgo = new Date();
      oneYearAgo.setFullYear(oneYearAgo.getFullYear() - 1);
      return oneYearAgo.toISOString().split("T")[0];
    }
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
    return sevenDaysAgo.toISOString().split("T")[0];
  }, [isAdmin]);

  const validateDateRange = useCallback(
    (from: string, to: string): string => {
      if (!from || !to) return "";

      const fromDateObj = new Date(from);
      const toDateObj = new Date(to);
      const todayObj = new Date();
      const minDate = new Date(getMinDate());

      if (fromDateObj > toDateObj) {
        return t("dashboard.errors.range_order");
      }

      if (toDateObj > todayObj) {
        return t("dashboard.errors.future");
      }

      if (fromDateObj < minDate) {
        return isAdmin
          ? t("dashboard.errors.too_old_admin")
          : t("dashboard.errors.too_old_user");
      }

      const daysDiff = Math.ceil(
        (toDateObj.getTime() - fromDateObj.getTime()) / (1000 * 60 * 60 * 24)
      );
      const maxDays = isAdmin ? 365 : 7;

      if (daysDiff > maxDays) {
        return isAdmin
          ? t("dashboard.errors.range_limit_admin")
          : t("dashboard.errors.range_limit_user");
      }

      return "";
    },
    [getMinDate, isAdmin, t]
  );

  const applyPreset = useCallback(
    (preset: "today" | "7d" | "30d"): ApplyPresetResult => {
      const now = new Date();
      const start = new Date(now);

      if (preset === "today") {
        start.setDate(now.getDate());
      } else if (preset === "7d") {
        start.setDate(now.getDate() - 6);
      } else {
        start.setDate(now.getDate() - 29);
      }

      const newRange = { from: fmt(start), to: fmt(now) };
      setFromDate(newRange.from);
      setToDate(newRange.to);
      return newRange;
    },
    [fmt]
  );

  const loadUsers = useCallback(async () => {
    if (!isAdmin) {
      return;
    }
    try {
      const res = await api.get("/api/user/dashboard/users");
      if (res.data?.success) {
        setUserOptions(res.data.data || []);
      }
    } catch {
      setUserOptions([]);
    }
  }, [isAdmin]);

  useEffect(() => {
    if (isAdmin) {
      loadUsers();
    }
  }, [isAdmin, loadUsers]);

  return {
    filtersReady,
    fromDate,
    toDate,
    dashUser,
    setFromDate,
    setToDate,
    setDashUser,
    userOptions,
    dateError,
    setDateError,
    getMinDate,
    getMaxDate,
    validateDateRange,
    applyPreset,
  };
};
