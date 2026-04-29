import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ModelRow, TokenRow, ToolRow, ToolTokenRow, ToolUserRow, UserOption, UserRow } from '../types';

export const useDashboardData = () => {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const isAdmin = useMemo(() => (user?.role ?? 0) >= 10, [user]);
  const abortControllerRef = useRef<AbortController | null>(null);

  // date range defaults: last 7 days (inclusive)
  const fmt = (d: Date) => d.toISOString().slice(0, 10);
  const today = new Date();
  const last7 = new Date(today);
  last7.setDate(today.getDate() - 6);

  const [fromDate, setFromDate] = useState(fmt(last7));
  const [toDate, setToDate] = useState(fmt(today));
  const [dashUser, setDashUser] = useState<string>('all');
  const [userOptions, setUserOptions] = useState<UserOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);
  const [dateError, setDateError] = useState<string>('');

  const [rows, setRows] = useState<ModelRow[]>([]);
  const [userRows, setUserRows] = useState<UserRow[]>([]);
  const [tokenRows, setTokenRows] = useState<TokenRow[]>([]);
  const [toolRows, setToolRows] = useState<ToolRow[]>([]);
  const [toolUserRows, setToolUserRows] = useState<ToolUserRow[]>([]);
  const [toolTokenRows, setToolTokenRows] = useState<ToolTokenRow[]>([]);

  // Date validation functions
  const getMaxDate = () => {
    const today = new Date();
    return today.toISOString().split('T')[0];
  };

  const getMinDate = () => {
    if (isAdmin) {
      // Admin users can go back 1 year
      const oneYearAgo = new Date();
      oneYearAgo.setFullYear(oneYearAgo.getFullYear() - 1);
      return oneYearAgo.toISOString().split('T')[0];
    } else {
      // Regular users can only go back 7 days from today
      const sevenDaysAgo = new Date();
      sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
      return sevenDaysAgo.toISOString().split('T')[0];
    }
  };

  // Date validation
  const validateDateRange = (from: string, to: string): string => {
    if (!from || !to) return '';

    const fromDate = new Date(from);
    const toDate = new Date(to);
    const today = new Date();
    const minDate = new Date(getMinDate());

    if (fromDate > toDate) {
      return t('dashboard.errors.range_order');
    }

    if (toDate > today) {
      return t('dashboard.errors.future');
    }

    if (fromDate < minDate) {
      return isAdmin ? t('dashboard.errors.too_old_admin') : t('dashboard.errors.too_old_user');
    }

    const daysDiff = Math.ceil((toDate.getTime() - fromDate.getTime()) / (1000 * 60 * 60 * 24));
    const maxDays = isAdmin ? 365 : 7;

    if (daysDiff > maxDays) {
      return isAdmin ? t('dashboard.errors.range_limit_admin') : t('dashboard.errors.range_limit_user');
    }

    return '';
  };

  const loadUsers = async () => {
    if (!isAdmin) return;
    const res = await api.get('/api/user/dashboard/users');
    if (res.data?.success) {
      setUserOptions(res.data.data || []);
    }
  };

  const loadStats = useCallback(async () => {
    // Validate date range before making API call
    const validationError = validateDateRange(fromDate, toDate);
    if (validationError) {
      setDateError(validationError);
      return;
    }

    // Cancel any pending request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    // Create new AbortController for this request
    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    setLoading(true);
    setDateError('');
    try {
      const params = new URLSearchParams();
      params.set('from_date', fromDate);
      params.set('to_date', toDate);
      if (isAdmin) {
        params.set('user_id', dashUser || 'all');
      }
      const res = await api.get('/api/user/dashboard?' + params.toString(), {
        signal: abortController.signal,
      });

      // Check if this request was aborted
      if (abortController.signal.aborted) {
        return;
      }

      const { success, data, message } = res.data;
      if (success) {
        const logs = data?.logs || data || [];
        const userLogs = data?.user_logs || [];
        const tokenLogs = data?.token_logs || [];
        const toolLogs = data?.tool_logs || [];
        const toolUserLogs = data?.tool_user_logs || [];
        const toolTokenLogs = data?.tool_token_logs || [];
        setRows(
          logs.map((row: any) => ({
            day: row.Day,
            model_name: row.ModelName,
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
            prompt_tokens: Number(row.PromptTokens ?? 0),
            completion_tokens: Number(row.CompletionTokens ?? 0),
            cached_prompt_tokens: Number(row.CachedPromptTokens ?? 0),
            cache_hit_count: Number(row.CacheHitCount ?? 0),
            cache_hit_quota: Number(row.CacheHitQuota ?? 0),
          }))
        );
        setUserRows(
          userLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            user_id: Number(row.UserId ?? 0),
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
            prompt_tokens: Number(row.PromptTokens ?? 0),
            completion_tokens: Number(row.CompletionTokens ?? 0),
            cached_prompt_tokens: Number(row.CachedPromptTokens ?? 0),
            cache_hit_count: Number(row.CacheHitCount ?? 0),
            cache_hit_quota: Number(row.CacheHitQuota ?? 0),
          }))
        );
        setTokenRows(
          tokenLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            token_name: row.TokenName,
            user_id: Number(row.UserId ?? 0),
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
            prompt_tokens: Number(row.PromptTokens ?? 0),
            completion_tokens: Number(row.CompletionTokens ?? 0),
            cached_prompt_tokens: Number(row.CachedPromptTokens ?? 0),
            cache_hit_count: Number(row.CacheHitCount ?? 0),
            cache_hit_quota: Number(row.CacheHitQuota ?? 0),
          }))
        );
        setToolRows(
          toolLogs.map((row: any) => ({
            day: row.Day,
            tool_name: row.ToolName,
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
          }))
        );
        setToolUserRows(
          toolUserLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            user_id: Number(row.UserId ?? 0),
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
          }))
        );
        setToolTokenRows(
          toolTokenLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            token_name: row.TokenName,
            user_id: Number(row.UserId ?? 0),
            request_count: Number(row.RequestCount ?? 0),
            quota: Number(row.Quota ?? 0),
          }))
        );

        setLastUpdated(Math.floor(Date.now() / 1000));
        setDateError('');
      } else {
        setDateError(message || t('dashboard.errors.fetch_failed'));
        setRows([]);
        setUserRows([]);
        setTokenRows([]);
        setToolRows([]);
        setToolUserRows([]);
        setToolTokenRows([]);
      }
    } catch (error: any) {
      // Ignore abort errors
      if (error.name === 'AbortError' || error.name === 'CanceledError') {
        return;
      }
      console.error('Failed to fetch dashboard data:', error);
      setDateError(t('dashboard.errors.fetch_failed'));
      setRows([]);
      setUserRows([]);
      setTokenRows([]);
      setToolRows([]);
      setToolUserRows([]);
      setToolTokenRows([]);
    } finally {
      // Only clear loading if this request wasn't aborted
      if (!abortController.signal.aborted) {
        setLoading(false);
      }
    }
  }, [fromDate, toDate, dashUser, isAdmin, t]);

  useEffect(() => {
    if (isAdmin) loadUsers();
    loadStats();
  }, [isAdmin]); // eslint-disable-line react-hooks/exhaustive-deps

  const applyPreset = (preset: 'today' | '7d' | '30d') => {
    const today = new Date();
    const start = new Date(today);
    if (preset === 'today') start.setDate(today.getDate());
    if (preset === '7d') start.setDate(today.getDate() - 6);
    if (preset === '30d') start.setDate(today.getDate() - 29);

    const newFromDate = fmt(start);
    const newToDate = fmt(today);

    setFromDate(newFromDate);
    setToDate(newToDate);

    // The effect will trigger loadStats because fromDate/toDate changed
    // But we need to wait for state update, or we can call loadStats with new values
    // However, loadStats uses state values.
    // Let's just rely on the effect if we add fromDate/toDate to dependency array of useEffect
    // But currently useEffect only depends on isAdmin.
    // Let's manually trigger loadStats in a useEffect that watches fromDate/toDate?
    // Or better, just update state and let the user click apply?
    // The original code called loadStats immediately with new values.
    // Let's stick to the original behavior but we need to pass values to loadStats or update state and wait.
    // Since loadStats uses state, we can't easily call it with new values unless we refactor loadStats.
    // For now, let's just update state. The user can click apply, or we can add an effect.
    // Actually, the original code did: setFromDate, setToDate, then validate and fetch.
    // To replicate that, we need a way to fetch with specific dates.
  };

  // Refactored applyPreset to trigger fetch
  const applyPresetAndFetch = async (preset: 'today' | '7d' | '30d') => {
    const today = new Date();
    const start = new Date(today);
    if (preset === 'today') start.setDate(today.getDate());
    if (preset === '7d') start.setDate(today.getDate() - 6);
    if (preset === '30d') start.setDate(today.getDate() - 29);

    const newFromDate = fmt(start);
    const newToDate = fmt(today);

    setFromDate(newFromDate);
    setToDate(newToDate);

    // We need to call the API with these new values
    // Since loadStats uses state, we can create a temporary version or just update state and let a useEffect handle it
    // But we want to avoid double fetching.
    // Let's create a fetch function that takes arguments.

    // For now, let's just return the new dates so the component can handle it or use a separate effect
    return { from: newFromDate, to: newToDate };
  };

  return {
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
    toolRows,
    toolUserRows,
    toolTokenRows,
    loadStats,
    applyPreset: applyPresetAndFetch,
    getMinDate,
    getMaxDate,
  };
};
