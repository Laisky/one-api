import { LogDetailsModal } from '@/components/LogDetailsModal';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useNotifications } from '@/components/ui/notifications';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { SearchableDropdown, type SearchOption } from '@/components/ui/searchable-dropdown';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { STORAGE_KEYS, usePageSize } from '@/hooks/usePersistentState';
import { api } from '@/lib/api';
import { LOG_TYPES, LOG_TYPE_OPTIONS } from '@/lib/constants/logs';
import type { LogCursorFilters } from '@/lib/logCursor';
import { useLogExport } from './useLogExport';
import { useAuthStore } from '@/lib/stores/auth';
import { cn, fromDateTimeLocal, renderQuota, toDateTimeLocal } from '@/lib/utils';
import { Eye, EyeOff, FileDown, Filter, RefreshCw } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';

import { createLogColumns, logRef, type LogRow } from './logs-page-columns';
import { LOG_TYPE_TRANSLATION_KEYS } from './log-types';
import { LogCursorPager } from './components/LogCursorPager';
import { useLogCursorPagination, type LogCursorNavigation } from './useLogCursorPagination';

/** LogStatistics describes the aggregate quota and request totals returned by log statistics APIs. */
interface LogStatistics {
  quota: number;
  token_count?: number;
  request_count?: number;
}

/** LogsPage renders log filters, statistics, export actions, pagination, and trace details. */
export function LogsPage() {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const { user } = useAuthStore();
  const [confirmAction, ConfirmActionDialog] = useConfirmDialog();
  const [searchParams, setSearchParams] = useSearchParams();
  const [data, setData] = useState<LogRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(Math.max(0, parseInt(searchParams.get('p') || '1') - 1));
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  // cursorActive reflects the source of the rows on screen, not the eligibility
  // of the next request, so the pager never describes data it did not produce.
  const [cursorActive, setCursorActive] = useState(false);
  const mounted = useRef(false);

  // Determine if user is admin/root
  // Use strict equality for admin (10) and root (100)
  const isAdmin = useMemo(() => (user?.role ?? 0) === 10, [user]);
  const isRoot = useMemo(() => (user?.role ?? 0) === 100, [user]);
  const isAdminOrRoot = isAdmin || isRoot;

  // Filters: for admin/root, username is '', for others, username is self
  const [filters, setFilters] = useState(() => ({
    type: '0',
    model_name: '',
    token_name: '',
    username: user && (user.role === 10 || user.role === 100) ? '' : user?.username || '',
    channel: '',
    start_timestamp: toDateTimeLocal(Math.floor((Date.now() - 7 * 24 * 3600 * 1000) / 1000)),
    end_timestamp: toDateTimeLocal(Math.floor((Date.now() + 3600 * 1000) / 1000)),
  }));

  // Statistics
  const [stat, setStat] = useState<LogStatistics>({ quota: 0 });
  const [showStat, setShowStat] = useState(false);
  const [statLoading, setStatLoading] = useState(false);

  // Search
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);

  // Sorting
  const [sortBy, setSortBy] = useState('created_at');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  // Tracing modal — driven by URL ?id=xxx
  const selectedLog = useMemo(() => {
    const idStr = searchParams.get('id');
    if (!idStr) return null;
    return data.find((row) => String(logRef(row)) === idStr || String(row.id) === idStr) ?? null;
  }, [searchParams, data]);
  const detailsModalOpen = selectedLog !== null;

  const getLogTypeLabelText = (typeValue: number) => t(`logs.types.${LOG_TYPE_TRANSLATION_KEYS[typeValue] ?? 'unknown'}`);

  const renderLogTypeBadge = (typeValue: number) => {
    const label = getLogTypeLabelText(typeValue);
    switch (typeValue) {
      case LOG_TYPES.TOPUP:
        return <Badge className="bg-success-muted text-success-foreground">{label}</Badge>;
      case LOG_TYPES.CONSUME:
        return <Badge className="bg-info-muted text-info-foreground">{label}</Badge>;
      case LOG_TYPES.MANAGE:
        return <Badge className="bg-accent text-accent-foreground">{label}</Badge>;
      case LOG_TYPES.SYSTEM:
        return <Badge className="bg-muted text-muted-foreground">{label}</Badge>;
      case LOG_TYPES.TEST:
        return <Badge className="bg-warning-muted text-warning-foreground">{label}</Badge>;
      case LOG_TYPES.TOOL:
        return <Badge className="bg-secondary text-secondary-foreground">{label}</Badge>;
      default:
        return <Badge variant="outline">{label}</Badge>;
    }
  };

  // (removed duplicate isAdmin declaration)

  const load = async (p = 0, size = pageSize, sortOverride?: { by: string; order: 'asc' | 'desc' }) => {
    // The sort is passed explicitly because a caller that has just called
    // setSortBy still sees the previous value in this closure.
    const activeSortBy = sortOverride?.by ?? sortBy;
    const activeSortOrder = sortOverride?.order ?? sortOrder;
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('p', String(p));
      params.set('size', String(size));

      if (filters.type !== '0') params.set('type', filters.type);
      if (filters.model_name) params.set('model_name', filters.model_name);
      if (filters.token_name) params.set('token_name', filters.token_name);
      if (isAdminOrRoot && filters.username) params.set('username', filters.username);
      if (filters.channel && isAdminOrRoot) params.set('channel', filters.channel);
      if (filters.start_timestamp) params.set('start_timestamp', String(fromDateTimeLocal(filters.start_timestamp)));
      if (filters.end_timestamp) params.set('end_timestamp', String(fromDateTimeLocal(filters.end_timestamp)));
      if (activeSortBy) {
        params.set('sort', activeSortBy);
        params.set('order', activeSortOrder);
      }

      // Unified API call - complete URL with /api prefix
      const path = isAdminOrRoot ? `/api/log/?${params}` : `/api/log/self?${params}`;
      const res = await api.get(path);
      const { success, data: responseData, total: responseTotal } = res.data;

      if (success) {
        setData(responseData || []);
        setTotal(responseTotal || 0);
        setPageIndex(p);
        setPageSize(size);
        setCursorActive(false);
      }
    } catch (error) {
      console.error('Failed to load logs:', error);
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  const { exporting, exportLogs: handleExportLogs } = useLogExport({ filters, isAdminOrRoot, sortBy, sortOrder });

  const cursor = useLogCursorPagination<LogRow>({
    isAdminOrRoot,
    filters: filters as LogCursorFilters,
    pageSize,
    toUnixSeconds: fromDateTimeLocal,
    get: (url) => api.get(url),
    onRestart: () =>
      notify({
        type: 'info',
        title: t('logs.notifications.cursor_restart_title'),
        message: t('logs.notifications.cursor_restart_message'),
      }),
  });

  /**
   * loadPage fetches a page, preferring the keyset route.
   *
   * The keyset route only answers the view it can answer: the default
   * created_at DESC listing with no keyword search. Anything else — a different
   * sort, a keyword search, or a server without the capability — uses the
   * legacy offset route unchanged, so no existing capability is lost.
   *
   * @param navigation - the move being made.
   * @param options - per-call overrides for state that has not committed yet.
   * @returns nothing; the page state is updated in place.
   */
  const loadPage = async (
    navigation: LogCursorNavigation,
    options: { size?: number; sortBy?: string; sortOrder?: 'asc' | 'desc'; offsetPage?: number } = {}
  ) => {
    const size = options.size ?? pageSize;
    const activeSortBy = options.sortBy ?? sortBy;
    const activeSortOrder = options.sortOrder ?? sortOrder;
    const eligible = cursor.supported && !searchKeyword.trim() && activeSortBy === 'created_at' && activeSortOrder === 'desc';

    if (eligible) {
      setLoading(true);
      try {
        if (navigation === 'first') cursor.reset();
        const rows = await cursor.fetchPage(navigation);
        if (rows) {
          setData(rows);
          setPageSize(size);
          setCursorActive(true);
          return;
        }
      } finally {
        setLoading(false);
      }
      // rows === null means this server cannot answer with a cursor; fall
      // through to the legacy route rather than showing an empty list.
    }

    setCursorActive(false);
    await load(options.offsetPage ?? 0, size, { by: activeSortBy, order: activeSortOrder });
  };

  const loadStatistics = async () => {
    setStatLoading(true);
    try {
      const params = new URLSearchParams();
      if (filters.type !== '0') params.set('type', filters.type);
      if (filters.model_name) params.set('model_name', filters.model_name);
      if (filters.token_name) params.set('token_name', filters.token_name);
      if (isAdminOrRoot && filters.username) params.set('username', filters.username);
      if (filters.channel && isAdminOrRoot) params.set('channel', filters.channel);
      if (filters.start_timestamp) params.set('start_timestamp', String(fromDateTimeLocal(filters.start_timestamp)));
      if (filters.end_timestamp) params.set('end_timestamp', String(fromDateTimeLocal(filters.end_timestamp)));

      // Unified API call - complete URL with /api prefix
      const statPath = isAdminOrRoot ? '/api/log/stat' : '/api/log/self/stat';
      const res = await api.get(statPath + '?' + params.toString());

      if (res.data?.success) {
        setStat(res.data.data || { quota: 0 });
      }
    } catch (error) {
      console.error('Failed to load statistics:', error);
    } finally {
      setStatLoading(false);
    }
  };

  // Search functionality
  const searchLogs = async (query: string) => {
    if (!query.trim()) {
      setSearchOptions([]);
      return;
    }

    setSearchLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const url = isAdminOrRoot ? '/api/log/search' : '/api/log/self/search';
      const res = await api.get(url + '?keyword=' + encodeURIComponent(query));
      const { success, data: responseData } = res.data;

      if (success && Array.isArray(responseData)) {
        const options: SearchOption[] = responseData.slice(0, 10).map((log: LogRow) => ({
          key: String(logRef(log)),
          value: log.content || log.model_name || t('logs.search.log_entry'),
          text: log.content || log.model_name || t('logs.search.log_entry'),
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{log.model_name}</div>
              <div className="text-sm text-muted-foreground flex flex-wrap items-center gap-2">
                <TimestampDisplay timestamp={log.created_at} className="font-mono text-xs" />
                <span>•</span>
                {renderLogTypeBadge(log.type)}
                <span>•</span>
                <span>{t('logs.search.quota', { value: renderQuota(log.quota) })}</span>
              </div>
            </div>
          ),
        }));
        setSearchOptions(options);
      }
    } catch (error) {
      console.error('Search failed:', error);
      setSearchOptions([]);
    } finally {
      setSearchLoading(false);
    }
  };

  const performSearch = async () => {
    if (!searchKeyword.trim()) {
      return loadPage('first');
    }

    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const url = isAdminOrRoot ? '/api/log/search' : '/api/log/self/search';
      const res = await api.get(url + '?keyword=' + encodeURIComponent(searchKeyword));
      const { success, data: responseData } = res.data;

      if (success) {
        setData(responseData || []);
        setPageIndex(0);
        setTotal(responseData?.length || 0);
        // Keyword search is a separate route with its own result set; the
        // keyset pager describes nothing about it.
        setCursorActive(false);
      }
    } catch (error) {
      console.error('Search failed:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      // A ?p= deep link is an offset address; it has no keyset equivalent, so
      // it is honoured only by the route that understands it.
      if (pageIndex > 0) {
        load(pageIndex, pageSize);
      } else {
        loadPage('first');
      }
      return;
    }
    loadPage('first');
  }, [pageSize]);

  useEffect(() => {
    if (showStat) {
      loadStatistics();
    }
  }, [showStat, filters]);

  const toggleStatVisibility = () => {
    setShowStat(!showStat);
  };

  const handleFilterSubmit = () => {
    loadPage('first');
  };

  const handleClearLogs = async () => {
    const ts = fromDateTimeLocal(filters.end_timestamp);
    const confirmed = await confirmAction({
      title: t('logs.actions.clear'),
      description: t('logs.confirm.delete_before', { timestamp: filters.end_timestamp }),
      details: [
        {
          label: t('logs.filters.end'),
          value: filters.end_timestamp,
        },
      ],
      variant: 'destructive',
    });
    if (!confirmed) return;

    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.delete('/api/log?target_timestamp=' + ts);
      if (!res.data?.success) {
        notify({
          type: 'error',
          title: t('logs.notifications.clear_failed_title', 'Clear failed'),
          message: res.data?.message || t('logs.notifications.clear_failed_message', 'Failed to clear logs.'),
        });
        return;
      }
      loadPage('first');
      notify({
        type: 'success',
        title: t('logs.notifications.clear_success_title', 'Logs cleared'),
        message: t('logs.notifications.clear_success_message', 'Logs cleared successfully.'),
      });
    } catch (error) {
      console.error('Failed to clear logs:', error);
      notify({
        type: 'error',
        title: t('logs.notifications.clear_failed_title', 'Clear failed'),
        message:
          (error as any)?.response?.data?.message ||
          (error as Error)?.message ||
          t('logs.notifications.clear_failed_message', 'Failed to clear logs.'),
      });
    }
  };

  const columns = createLogColumns({
    t,
    isAdminOrRoot,
    filterType: filters.type,
    currentUsername: user?.username,
    testLogType: LOG_TYPES.TEST,
    renderLogTypeBadge,
  });

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    setSearchParams((prev) => {
      prev.set('p', (newPageIndex + 1).toString());
      return prev;
    });
    if (searchKeyword.trim()) {
      setPageIndex(newPageIndex);
    } else {
      load(newPageIndex, newPageSize);
    }
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    if (searchKeyword.trim()) {
      performSearch();
    } else {
      loadPage('first', { size: newPageSize });
    }
  };

  const handleSortChange = (newSortBy: string, newSortOrder: 'asc' | 'desc') => {
    setSortBy(newSortBy);
    setSortOrder(newSortOrder);
    loadPage('first', { sortBy: newSortBy, sortOrder: newSortOrder });
  };

  const handleRowClick = (log: LogRow) => {
    setSearchParams((prev) => {
      prev.set('id', String(logRef(log)));
      return prev;
    });
  };

  const handleDetailsModalChange = (open: boolean) => {
    if (!open) {
      setSearchParams((prev) => {
        prev.delete('id');
        return prev;
      });
    }
  };

  const refresh = () => {
    if (searchKeyword.trim()) {
      performSearch();
    } else if (cursorActive) {
      loadPage('reload');
    } else {
      load(pageIndex, pageSize);
    }
  };

  return (
    <ResponsivePageContainer
      title={t('logs.title')}
      description={t('logs.description')}
      actions={
        <div className="flex w-full flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
          {showStat && (
            <div className="flex items-center gap-2 rounded-lg border border-border/70 bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
              <span>
                {t('logs.stats.total_quota', {
                  value: renderQuota(stat.quota),
                })}
              </span>
              <Button size="sm" variant="ghost" onClick={loadStatistics} disabled={statLoading} className="h-7 w-7 p-0">
                <RefreshCw className={cn('h-3 w-3', statLoading && 'animate-spin')} />
              </Button>
            </div>
          )}
          <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:justify-end">
            <Button variant="outline" onClick={toggleStatVisibility} className="gap-2 whitespace-nowrap w-full sm:w-auto" size="sm">
              {showStat ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              {showStat ? t('logs.actions.hide_stats') : t('logs.actions.show_stats')}
            </Button>
            <Button
              variant="outline"
              onClick={handleExportLogs}
              className="gap-2 whitespace-nowrap w-full sm:w-auto"
              size="sm"
              disabled={exporting}
            >
              {exporting ? <RefreshCw className="h-4 w-4 animate-spin" /> : <FileDown className="h-4 w-4" />}
              {t('logs.actions.export')}
            </Button>
            {isAdmin && (
              <Button variant="destructive" onClick={handleClearLogs} size="sm" className="w-full sm:w-auto">
                {t('logs.actions.clear')}
              </Button>
            )}
          </div>
        </div>
      }
    >
      <Card className="border-0 md:border shadow-none md:shadow-sm">
        <CardContent className="px-2 pt-3 md:px-6 md:pt-6">
          {/* Filters */}
          <div className="grid grid-cols-1 md:grid-cols-7 gap-3 md:gap-4 mb-6 p-3 md:p-4 border-x-0 md:border border-y md:rounded-lg bg-muted/5 md:bg-muted/10">
            <div className="md:col-span-7 flex items-center gap-2 mb-1">
              <Filter className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">{t('logs.filters.title')}</span>
            </div>
            <div>
              <Label className="text-xs">{t('logs.filters.type')}</Label>
              <Select value={filters.type} onValueChange={(value) => setFilters({ ...filters, type: value })}>
                <SelectTrigger className="h-9">
                  <SelectValue placeholder={t('logs.filters.type_placeholder')} />
                </SelectTrigger>
                <SelectContent>
                  {LOG_TYPE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {getLogTypeLabelText(Number(option.value))}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="text-xs">{t('logs.filters.model')}</Label>
              <SearchableDropdown
                value={filters.model_name}
                placeholder={t('logs.filters.model_placeholder')}
                searchPlaceholder={t('logs.filters.model_placeholder')}
                options={[]}
                searchEndpoint="/api/models/display" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) => {
                  // /api/models/display returns a map; flatten to model names
                  const options: SearchOption[] = [];
                  if (data && typeof data === 'object') {
                    Object.values<any>(data).forEach((entry: any) => {
                      if (entry?.models && typeof entry.models === 'object') {
                        Object.keys(entry.models).forEach((modelName: string) => {
                          options.push({
                            key: modelName,
                            value: modelName,
                            text: modelName,
                          });
                        });
                      }
                    });
                  }
                  return options;
                }}
                onChange={(value) => setFilters({ ...filters, model_name: value })}
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">{t('logs.filters.token')}</Label>
              <SearchableDropdown
                value={filters.token_name}
                placeholder={t('logs.filters.token_placeholder')}
                searchPlaceholder={t('logs.filters.token_placeholder')}
                options={[]}
                searchEndpoint="/api/token/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) =>
                  Array.isArray(data)
                    ? data.map((t: any) => ({
                        key: String(t.uuid ?? t.id ?? t.name),
                        value: t.name,
                        text: t.name,
                      }))
                    : []
                }
                onChange={(value) => setFilters({ ...filters, token_name: value })}
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">{t('logs.filters.username')}</Label>
              <SearchableDropdown
                value={filters.username}
                placeholder={t('logs.filters.username_placeholder')}
                searchPlaceholder={t('logs.filters.username_placeholder')}
                options={[]}
                searchEndpoint="/api/user/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) =>
                  Array.isArray(data)
                    ? data.map((u: any) => ({
                        key: String(u.uuid ?? u.id ?? u.username),
                        value: u.username,
                        text: u.username,
                      }))
                    : []
                }
                onChange={(value) => setFilters({ ...filters, username: value })}
                clearable
              />
            </div>
            {isAdmin && (
              <>
                <div>
                  <Label className="text-xs">{t('logs.filters.channel')}</Label>
                  <Input
                    value={filters.channel}
                    onChange={(e) => setFilters({ ...filters, channel: e.target.value })}
                    placeholder={t('logs.filters.channel_placeholder')}
                    className="h-9"
                  />
                </div>
              </>
            )}
            <div className="md:col-span-2 grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <Label className="text-xs">{t('logs.filters.start')}</Label>
                <Input
                  type="datetime-local"
                  value={filters.start_timestamp}
                  onChange={(e) => setFilters({ ...filters, start_timestamp: e.target.value })}
                  className="h-9"
                />
              </div>
              <div>
                <Label className="text-xs">{t('logs.filters.end')}</Label>
                <Input
                  type="datetime-local"
                  value={filters.end_timestamp}
                  onChange={(e) => setFilters({ ...filters, end_timestamp: e.target.value })}
                  className="h-9"
                />
              </div>
            </div>
            <div className="flex items-end md:justify-end md:col-span-1">
              <Button onClick={handleFilterSubmit} disabled={loading} className="w-full md:w-auto gap-2 px-4">
                <Filter className="h-4 w-4" />
                {t('logs.filters.apply')}
              </Button>
            </div>
          </div>

          <EnhancedDataTable
            columns={columns}
            data={data}
            pageIndex={pageIndex}
            pageSize={pageSize}
            total={total}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
            sortBy={sortBy}
            sortOrder={sortOrder}
            onSortChange={handleSortChange}
            onRowClick={handleRowClick}
            onRefresh={refresh}
            loading={loading}
            emptyMessage={t('logs.table.empty')}
            paginationSlot={
              cursorActive ? (
                <LogCursorPager
                  rowsBefore={cursor.rowsBefore}
                  pageSize={pageSize}
                  rowCount={data.length}
                  hasMore={cursor.hasMore}
                  hasPrevious={cursor.hasPrevious}
                  count={cursor.count}
                  notice={cursor.notice}
                  loading={loading}
                  onPrevious={() => loadPage('previous')}
                  onNext={() => loadPage('next')}
                  onPageSizeChange={handlePageSizeChange}
                />
              ) : undefined
            }
          />
        </CardContent>
      </Card>

      <ConfirmActionDialog />
      <LogDetailsModal open={detailsModalOpen} onOpenChange={handleDetailsModalChange} log={selectedLog} />
    </ResponsivePageContainer>
  );
}
