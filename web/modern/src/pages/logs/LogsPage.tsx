import { LogDetailsModal } from "@/components/LogDetailsModal";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { EnhancedDataTable } from "@/components/ui/enhanced-data-table";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  SearchableDropdown,
  type SearchOption,
} from "@/components/ui/searchable-dropdown";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { TimestampDisplay } from "@/components/ui/timestamp";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { usePageSize, STORAGE_KEYS } from "@/hooks/usePersistentState";
import { api } from "@/lib/api";
import { LOG_TYPES, LOG_TYPE_OPTIONS } from "@/lib/constants/logs";
import { useAuthStore } from "@/lib/stores/auth";
import {
  cn,
  formatTimestamp,
  fromDateTimeLocal,
  renderQuota,
  toDateTimeLocal,
} from "@/lib/utils";
import type { LogEntry, LogMetadata } from "@/types/log";
import type { ColumnDef } from "@tanstack/react-table";
import { Copy, Eye, EyeOff, FileDown, Filter, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

type LogRow = LogEntry;

interface LogStatistics {
  quota: number;
  token_count?: number;
  request_count?: number;
}

const LOG_TYPE_TRANSLATION_KEYS: Record<number, string> = {
  [LOG_TYPES.ALL]: "all",
  [LOG_TYPES.TOPUP]: "topup",
  [LOG_TYPES.CONSUME]: "consume",
  [LOG_TYPES.MANAGE]: "manage",
  [LOG_TYPES.SYSTEM]: "system",
  [LOG_TYPES.TEST]: "test",
};

const formatLatency = (ms?: number, fallback: string = "-") => {
  if (!ms) return fallback;
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
};

const getLatencyColor = (ms?: number) => {
  if (!ms) return "";
  if (ms < 1000) return "text-green-600";
  if (ms < 3000) return "text-yellow-600";
  return "text-red-600";
};

const coerceTokenCount = (value: unknown) => {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.trunc(value);
};

const getCacheWriteSummaries = (metadata?: LogMetadata) => {
  const details = metadata?.cache_write_tokens;
  if (!details) {
    return { fiveMinute: 0, oneHour: 0 };
  }

  return {
    fiveMinute: coerceTokenCount(details.ephemeral_5m),
    oneHour: coerceTokenCount(details.ephemeral_1h),
  };
};

export function LogsPage() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const [searchParams, setSearchParams] = useSearchParams();
  const [data, setData] = useState<LogRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(
    Math.max(0, parseInt(searchParams.get("p") || "1") - 1)
  );
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const mounted = useRef(false);

  // Determine if user is admin/root
  // Use strict equality for admin (10) and root (100)
  const isAdmin = useMemo(() => (user?.role ?? 0) === 10, [user]);
  const isRoot = useMemo(() => (user?.role ?? 0) === 100, [user]);
  const isAdminOrRoot = isAdmin || isRoot;

  // Filters: for admin/root, username is '', for others, username is self
  const [filters, setFilters] = useState(() => ({
    type: "0",
    model_name: "",
    token_name: "",
    username:
      user && (user.role === 10 || user.role === 100)
        ? ""
        : user?.username || "",
    channel: "",
    start_timestamp: toDateTimeLocal(
      Math.floor((Date.now() - 7 * 24 * 3600 * 1000) / 1000)
    ),
    end_timestamp: toDateTimeLocal(
      Math.floor((Date.now() + 3600 * 1000) / 1000)
    ),
  }));

  // Statistics
  const [stat, setStat] = useState<LogStatistics>({ quota: 0 });
  const [showStat, setShowStat] = useState(false);
  const [statLoading, setStatLoading] = useState(false);

  // Search
  const [searchKeyword, setSearchKeyword] = useState("");
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);

  // Sorting
  const [sortBy, setSortBy] = useState("created_at");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");

  // Tracing modal
  const [detailsModalOpen, setDetailsModalOpen] = useState(false);
  const [selectedLog, setSelectedLog] = useState<LogRow | null>(null);

  const getLogTypeLabelText = (typeValue: number) =>
    t(`logs.types.${LOG_TYPE_TRANSLATION_KEYS[typeValue] ?? "unknown"}`);

  const renderLogTypeBadge = (typeValue: number) => {
    const label = getLogTypeLabelText(typeValue);
    switch (typeValue) {
      case LOG_TYPES.TOPUP:
        return <Badge className="bg-green-100 text-green-800">{label}</Badge>;
      case LOG_TYPES.CONSUME:
        return <Badge className="bg-blue-100 text-blue-800">{label}</Badge>;
      case LOG_TYPES.MANAGE:
        return <Badge className="bg-purple-100 text-purple-800">{label}</Badge>;
      case LOG_TYPES.SYSTEM:
        return <Badge className="bg-gray-100 text-gray-800">{label}</Badge>;
      case LOG_TYPES.TEST:
        return <Badge className="bg-yellow-100 text-yellow-800">{label}</Badge>;
      default:
        return <Badge variant="outline">{label}</Badge>;
    }
  };

  // (removed duplicate isAdmin declaration)

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set("p", String(p));
      params.set("size", String(size));

      if (filters.type !== "0") params.set("type", filters.type);
      if (filters.model_name) params.set("model_name", filters.model_name);
      if (filters.token_name) params.set("token_name", filters.token_name);
      if (isAdminOrRoot && filters.username)
        params.set("username", filters.username);
      if (filters.channel && isAdminOrRoot)
        params.set("channel", filters.channel);
      if (filters.start_timestamp)
        params.set(
          "start_timestamp",
          String(fromDateTimeLocal(filters.start_timestamp))
        );
      if (filters.end_timestamp)
        params.set(
          "end_timestamp",
          String(fromDateTimeLocal(filters.end_timestamp))
        );
      if (sortBy) {
        params.set("sort", sortBy);
        params.set("order", sortOrder);
      }

      // Unified API call - complete URL with /api prefix
      const path = isAdminOrRoot
        ? `/api/log/?${params}`
        : `/api/log/self?${params}`;
      const res = await api.get(path);
      const { success, data: responseData, total: responseTotal } = res.data;

      if (success) {
        setData(responseData || []);
        setTotal(responseTotal || 0);
        setPageIndex(p);
        setPageSize(size);
      }
    } catch (error) {
      console.error("Failed to load logs:", error);
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  const loadStatistics = async () => {
    setStatLoading(true);
    try {
      const params = new URLSearchParams();
      if (filters.type !== "0") params.set("type", filters.type);
      if (filters.model_name) params.set("model_name", filters.model_name);
      if (filters.token_name) params.set("token_name", filters.token_name);
      if (isAdminOrRoot && filters.username)
        params.set("username", filters.username);
      if (filters.channel && isAdminOrRoot)
        params.set("channel", filters.channel);
      if (filters.start_timestamp)
        params.set(
          "start_timestamp",
          String(fromDateTimeLocal(filters.start_timestamp))
        );
      if (filters.end_timestamp)
        params.set(
          "end_timestamp",
          String(fromDateTimeLocal(filters.end_timestamp))
        );

      // Unified API call - complete URL with /api prefix
      const statPath = isAdminOrRoot ? "/api/log/stat" : "/api/log/self/stat";
      const res = await api.get(statPath + "?" + params.toString());

      if (res.data?.success) {
        setStat(res.data.data || { quota: 0 });
      }
    } catch (error) {
      console.error("Failed to load statistics:", error);
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
      const url = isAdminOrRoot ? "/api/log/search" : "/api/log/self/search";
      const res = await api.get(url + "?keyword=" + encodeURIComponent(query));
      const { success, data: responseData } = res.data;

      if (success && Array.isArray(responseData)) {
        const options: SearchOption[] = responseData
          .slice(0, 10)
          .map((log: LogRow) => ({
            key: log.id.toString(),
            value: log.content || log.model_name || t("logs.search.log_entry"),
            text: log.content || log.model_name || t("logs.search.log_entry"),
            content: (
              <div className="flex flex-col">
                <div className="font-medium">{log.model_name}</div>
                <div className="text-sm text-muted-foreground flex flex-wrap items-center gap-2">
                  <TimestampDisplay
                    timestamp={log.created_at}
                    className="font-mono text-xs"
                  />
                  <span>•</span>
                  {renderLogTypeBadge(log.type)}
                  <span>•</span>
                  <span>
                    {t("logs.search.quota", { value: renderQuota(log.quota) })}
                  </span>
                </div>
              </div>
            ),
          }));
        setSearchOptions(options);
      }
    } catch (error) {
      console.error("Search failed:", error);
      setSearchOptions([]);
    } finally {
      setSearchLoading(false);
    }
  };

  const performSearch = async () => {
    if (!searchKeyword.trim()) {
      return load(0, pageSize);
    }

    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const url = isAdminOrRoot ? "/api/log/search" : "/api/log/self/search";
      const res = await api.get(
        url + "?keyword=" + encodeURIComponent(searchKeyword)
      );
      const { success, data: responseData } = res.data;

      if (success) {
        setData(responseData || []);
        setPageIndex(0);
        setTotal(responseData?.length || 0);
      }
    } catch (error) {
      console.error("Search failed:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      load(pageIndex, pageSize);
      return;
    }
    load(0, pageSize);
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
    load(0, pageSize);
  };

  const handleClearLogs = async () => {
    const ts = fromDateTimeLocal(filters.end_timestamp);
    const confirmed = window.confirm(
      t("logs.confirm.delete_before", { timestamp: filters.end_timestamp })
    );
    if (!confirmed) return;

    try {
      // Unified API call - complete URL with /api prefix
      await api.delete("/api/log?target_timestamp=" + ts);
      load(0, pageSize);
    } catch (error) {
      console.error("Failed to clear logs:", error);
    }
  };

  const handleExportLogs = () => {
    // Implementation for exporting logs to CSV
    const csvHeaders = [
      t("logs.export.headers.time"),
      t("logs.export.headers.type"),
      t("logs.export.headers.model"),
      t("logs.export.headers.token"),
      t("logs.export.headers.username"),
      t("logs.export.headers.quota"),
      t("logs.export.headers.prompt_tokens"),
      t("logs.export.headers.completion_tokens"),
      t("logs.export.headers.cached_prompt_tokens"),
      t("logs.export.headers.cached_completion_tokens"),
      t("logs.export.headers.cache_write_5m"),
      t("logs.export.headers.cache_write_1h"),
      t("logs.export.headers.latency"),
      t("logs.export.headers.content"),
    ];
    const csvData = data.map((log) => {
      const { fiveMinute, oneHour } = getCacheWriteSummaries(log.metadata);
      return [
        formatTimestamp(log.created_at),
        log.type,
        log.model_name,
        log.token_name || "",
        log.username || "",
        log.quota,
        log.prompt_tokens || 0,
        log.completion_tokens || 0,
        log.cached_prompt_tokens || 0,
        log.cached_completion_tokens || 0,
        fiveMinute,
        oneHour,
        log.elapsed_time || 0,
        (log.content || "").replace(/,/g, ";").replace(/\n/g, " "),
      ];
    });

    const csv = [csvHeaders, ...csvData].map((row) => row.join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `logs_${new Date().toISOString().split("T")[0]}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const CopyButton = ({ text }: { text: string }) => (
    <Button
      size="sm"
      variant="ghost"
      className="h-6 w-6 p-0"
      onClick={() => navigator.clipboard.writeText(text)}
    >
      <Copy className="h-3 w-3" />
    </Button>
  );

  const columns: ColumnDef<LogRow>[] = [
    {
      accessorKey: "created_at",
      header: t("logs.table.time"),
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <TimestampDisplay
            timestamp={row.original.created_at}
            className="font-mono text-xs"
            title={row.original.request_id || undefined}
          />
          {row.original.request_id && (
            <CopyButton text={row.original.request_id} />
          )}
        </div>
      ),
    },
    ...(isAdminOrRoot
      ? [
          {
            accessorKey: "channel",
            header: t("logs.table.channel"),
            cell: ({ row }: { row: any }) => (
              <span className="font-mono text-sm">
                {row.original.channel || t("logs.labels.missing")}
              </span>
            ),
          } as ColumnDef<LogRow>,
        ]
      : []),
    {
      accessorKey: "type",
      header: t("logs.table.type"),
      cell: ({ row }) => renderLogTypeBadge(row.original.type),
    },
    {
      accessorKey: "model_name",
      header: t("logs.table.model"),
      cell: ({ row }) => (
        <span className="font-medium">{row.original.model_name}</span>
      ),
    },
    ...(Number(filters.type) !== LOG_TYPES.TEST
      ? [
          // Always show user column; for non-admins fall back to current user if username missing
          {
            accessorKey: "username",
            header: t("logs.table.user"),
            cell: ({ row }) => (
              <span className="text-sm">
                {row.original.username ||
                  user?.username ||
                  t("logs.labels.missing")}
              </span>
            ),
          } as ColumnDef<LogRow>,
          {
            accessorKey: "token_name",
            header: t("logs.table.token"),
            cell: ({ row }) => (
              <span className="text-sm">
                {row.original.token_name || t("logs.labels.missing")}
              </span>
            ),
          },
          {
            accessorKey: "prompt_tokens",
            header: t("logs.table.prompt"),
            cell: ({ row }) => (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="font-mono text-sm cursor-help">
                      {row.original.prompt_tokens || 0}
                    </span>
                  </TooltipTrigger>
                  <TooltipContent>
                    <div className="flex flex-col gap-1">
                      <div>
                        {t("logs.tooltip.input_tokens", {
                          value: row.original.prompt_tokens ?? 0,
                        })}
                      </div>
                      <div>
                        {t("logs.tooltip.cached_tokens", {
                          value: row.original.cached_prompt_tokens ?? 0,
                        })}
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ),
          },
          {
            accessorKey: "completion_tokens",
            header: t("logs.table.completion"),
            cell: ({ row }) => {
              const { fiveMinute, oneHour } = getCacheWriteSummaries(
                row.original.metadata
              );
              return (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="font-mono text-sm cursor-help">
                        {row.original.completion_tokens || 0}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <div className="flex flex-col gap-1">
                        <div>
                          {t("logs.tooltip.output_tokens", {
                            value: row.original.completion_tokens ?? 0,
                          })}
                        </div>
                        <div>
                          {t("logs.tooltip.cached_tokens", {
                            value: row.original.cached_completion_tokens ?? 0,
                          })}
                        </div>
                        <div>
                          {t("logs.tooltip.cache_write_5m", {
                            value: fiveMinute,
                          })}
                        </div>
                        <div>
                          {t("logs.tooltip.cache_write_1h", { value: oneHour })}
                        </div>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              );
            },
          },
          {
            accessorKey: "quota",
            header: t("logs.table.cost"),
            cell: ({ row }) => (
              <span
                className="font-mono text-sm"
                title={row.original.content || ""}
              >
                {renderQuota(row.original.quota)}
              </span>
            ),
          },
          {
            accessorKey: "elapsed_time",
            header: t("logs.table.latency"),
            cell: ({ row }) => (
              <span
                className={cn(
                  "font-mono text-sm",
                  getLatencyColor(row.original.elapsed_time)
                )}
              >
                {formatLatency(
                  row.original.elapsed_time,
                  t("logs.labels.not_available")
                )}
              </span>
            ),
          },
        ]
      : ([] as ColumnDef<LogRow>[])),
  ];

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    setSearchParams((prev) => {
      prev.set("p", (newPageIndex + 1).toString());
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
      load(0, newPageSize);
    }
  };

  const handleSortChange = (
    newSortBy: string,
    newSortOrder: "asc" | "desc"
  ) => {
    setSortBy(newSortBy);
    setSortOrder(newSortOrder);
    load(0, pageSize);
  };

  const handleRowClick = (log: LogRow) => {
    setSelectedLog(log);
    setDetailsModalOpen(true);
  };

  const handleDetailsModalChange = (open: boolean) => {
    setDetailsModalOpen(open);
    if (!open) {
      setSelectedLog(null);
    }
  };

  const refresh = () => {
    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
  };

  return (
    <div className="container mx-auto px-4 py-8">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                {t("logs.title")}
                {showStat && (
                  <div className="text-sm font-normal text-muted-foreground">
                    (
                    {t("logs.stats.total_quota", {
                      value: renderQuota(stat.quota),
                    })}
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={loadStatistics}
                      disabled={statLoading}
                      className="ml-2 h-6 w-6 p-0"
                    >
                      <RefreshCw
                        className={cn("h-3 w-3", statLoading && "animate-spin")}
                      />
                    </Button>
                    )
                  </div>
                )}
              </CardTitle>
              <CardDescription>{t("logs.description")}</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={toggleStatVisibility}
                className="gap-2"
              >
                {showStat ? (
                  <EyeOff className="h-4 w-4" />
                ) : (
                  <Eye className="h-4 w-4" />
                )}
                {showStat
                  ? t("logs.actions.hide_stats")
                  : t("logs.actions.show_stats")}
              </Button>
              <Button
                variant="outline"
                onClick={handleExportLogs}
                className="gap-2"
              >
                <FileDown className="h-4 w-4" />
                {t("logs.actions.export")}
              </Button>
              {isAdmin && (
                <Button variant="destructive" onClick={handleClearLogs}>
                  {t("logs.actions.clear")}
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Filters */}
          <div className="grid grid-cols-1 md:grid-cols-7 gap-3 md:gap-4 mb-6 p-4 border rounded-lg bg-muted/10">
            <div className="md:col-span-7 flex items-center gap-2 mb-1">
              <Filter className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">
                {t("logs.filters.title")}
              </span>
            </div>
            <div>
              <Label className="text-xs">{t("logs.filters.type")}</Label>
              <Select
                value={filters.type}
                onValueChange={(value) =>
                  setFilters({ ...filters, type: value })
                }
              >
                <SelectTrigger className="h-9">
                  <SelectValue
                    placeholder={t("logs.filters.type_placeholder")}
                  />
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
              <Label className="text-xs">{t("logs.filters.model")}</Label>
              <SearchableDropdown
                value={filters.model_name}
                placeholder={t("logs.filters.model_placeholder")}
                searchPlaceholder={t("logs.filters.model_placeholder")}
                options={[]}
                searchEndpoint="/api/models/display" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) => {
                  // /api/models/display returns a map; flatten to model names
                  const options: SearchOption[] = [];
                  if (data && typeof data === "object") {
                    Object.values<any>(data).forEach((entry: any) => {
                      if (entry?.models && typeof entry.models === "object") {
                        Object.keys(entry.models).forEach(
                          (modelName: string) => {
                            options.push({
                              key: modelName,
                              value: modelName,
                              text: modelName,
                            });
                          }
                        );
                      }
                    });
                  }
                  return options;
                }}
                onChange={(value) =>
                  setFilters({ ...filters, model_name: value })
                }
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">{t("logs.filters.token")}</Label>
              <SearchableDropdown
                value={filters.token_name}
                placeholder={t("logs.filters.token_placeholder")}
                searchPlaceholder={t("logs.filters.token_placeholder")}
                options={[]}
                searchEndpoint="/api/token/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) =>
                  Array.isArray(data)
                    ? data.map((t: any) => ({
                        key: String(t.id),
                        value: t.name,
                        text: t.name,
                      }))
                    : []
                }
                onChange={(value) =>
                  setFilters({ ...filters, token_name: value })
                }
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">{t("logs.filters.username")}</Label>
              <SearchableDropdown
                value={filters.username}
                placeholder={t("logs.filters.username_placeholder")}
                searchPlaceholder={t("logs.filters.username_placeholder")}
                options={[]}
                searchEndpoint="/api/user/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) =>
                  Array.isArray(data)
                    ? data.map((u: any) => ({
                        key: String(u.id),
                        value: u.username,
                        text: u.username,
                      }))
                    : []
                }
                onChange={(value) =>
                  setFilters({ ...filters, username: value })
                }
                clearable
              />
            </div>
            {isAdmin && (
              <>
                <div>
                  <Label className="text-xs">{t("logs.filters.channel")}</Label>
                  <Input
                    value={filters.channel}
                    onChange={(e) =>
                      setFilters({ ...filters, channel: e.target.value })
                    }
                    placeholder={t("logs.filters.channel_placeholder")}
                    className="h-9"
                  />
                </div>
              </>
            )}
            <div className="md:col-span-2 grid grid-cols-2 gap-3">
              <div>
                <Label className="text-xs">{t("logs.filters.start")}</Label>
                <Input
                  type="datetime-local"
                  value={filters.start_timestamp}
                  onChange={(e) =>
                    setFilters({ ...filters, start_timestamp: e.target.value })
                  }
                  className="h-9"
                />
              </div>
              <div>
                <Label className="text-xs">{t("logs.filters.end")}</Label>
                <Input
                  type="datetime-local"
                  value={filters.end_timestamp}
                  onChange={(e) =>
                    setFilters({ ...filters, end_timestamp: e.target.value })
                  }
                  className="h-9"
                />
              </div>
            </div>
            <div className="flex items-end md:justify-end md:col-span-1">
              <Button
                onClick={handleFilterSubmit}
                disabled={loading}
                className="w-full md:w-auto gap-2 px-4"
              >
                <Filter className="h-4 w-4" />
                {t("logs.filters.apply")}
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
            emptyMessage={t("logs.table.empty")}
          />
        </CardContent>
      </Card>

      <LogDetailsModal
        open={detailsModalOpen}
        onOpenChange={handleDetailsModalChange}
        log={selectedLog}
      />
    </div>
  );
}
