import { AdvancedPagination } from "@/components/ui/advanced-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useNotifications } from "@/components/ui/notifications";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { TimestampDisplay } from "@/components/ui/timestamp";
import { usePageSize, STORAGE_KEYS } from "@/hooks/usePersistentState";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  Activity,
  AlertCircle,
  Calendar,
  CheckCircle,
  Clock,
  RefreshCw,
  XCircle,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

interface ChannelStatus {
  name: string;
  status: string;
  enabled: boolean;
  response: {
    response_time_ms: number;
    test_time: number;
    created_time: number;
  };
}

interface StatusResponse {
  success: boolean;
  data: ChannelStatus[];
  total?: number;
  message?: string;
}

function StatusPageImpl() {
  const { t } = useTranslation();
  const { isMobile } = useResponsive();
  const { notify } = useNotifications();
  const [channelsData, setChannelsData] = useState<ChannelStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [refreshing, setRefreshing] = useState(false);

  // Pagination state
  const [currentPage, setCurrentPage] = useState(0);
  const [pageSize, setPageSize] = usePageSize(
    STORAGE_KEYS.PAGE_SIZE,
    10,
    [10, 20, 30, 50, 100]  // Use standard page size options for consistency
  );
  const [totalCount, setTotalCount] = useState(0);
  const [totalPages, setTotalPages] = useState(0);

  const fetchStatusData = useCallback(
    async (page: number, size: number) => {
      try {
        setLoading(true);
        const params = new URLSearchParams({
          p: page.toString(),
          size: size.toString(),
        });
        const res = await api.get(`/api/status/channel?${params}`);
        const { success, message, data, total }: StatusResponse = res.data;
        if (success) {
          setChannelsData(data || []);
          setTotalCount(total || 0);
          setTotalPages(Math.ceil((total || 0) / size));
        } else {
          notify({
            message: t("status.notifications.fetch_failed", {
              reason: message || t("status.notifications.unknown_error"),
            }),
            type: "error",
          });
        }
      } catch (error) {
        const reason = error instanceof Error ? error.message : String(error);
        notify({
          message: t("status.notifications.fetch_error", { reason }),
          type: "error",
        });
      } finally {
        setLoading(false);
      }
    },
    [notify, t]
  );

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchStatusData(currentPage, pageSize);
    setRefreshing(false);
  };

  const handlePageChange = (newPage: number) => {
    // AdvancedPagination uses 1-based page numbers, but our state uses 0-based
    const zeroBasedPage = newPage - 1;
    if (zeroBasedPage >= 0 && zeroBasedPage < totalPages) {
      setCurrentPage(zeroBasedPage);
    }
  };

  const handlePageSizeChange = (newSize: number) => {
    if (newSize !== pageSize) {
      setCurrentPage(0);
      setPageSize(newSize);
    }
  };

  useEffect(() => {
    fetchStatusData(currentPage, pageSize);
  }, [currentPage, pageSize, fetchStatusData]);

  const formatResponseTime = (responseTime: number): string => {
    if (responseTime === 0) return t("status.labels.not_available");
    if (responseTime < 1000) return `${responseTime}ms`;
    return `${(responseTime / 1000).toFixed(2)}s`;
  };

  const getStatusBadge = (status: string, enabled: boolean) => {
    if (enabled && status === "enabled") {
      return (
        <Badge
          variant="default"
          className="bg-success-muted text-success-foreground hover:bg-success-muted/80"
        >
          <CheckCircle className="w-3 h-3 mr-1" />
          {t("status.badges.enabled")}
        </Badge>
      );
    } else if (status === "manually_disabled") {
      return (
        <Badge
          variant="secondary"
          className="bg-warning-muted text-warning-foreground hover:bg-warning-muted/80"
        >
          <AlertCircle className="w-3 h-3 mr-1" />
          {t("status.badges.manually_disabled")}
        </Badge>
      );
    } else if (status === "auto_disabled") {
      return (
        <Badge
          variant="destructive"
          className="bg-destructive/10 text-destructive hover:bg-destructive/20"
        >
          <XCircle className="w-3 h-3 mr-1" />
          {t("status.badges.auto_disabled")}
        </Badge>
      );
    } else {
      return (
        <Badge
          variant="outline"
          className="bg-muted text-muted-foreground hover:bg-muted/80"
        >
          <AlertCircle className="w-3 h-3 mr-1" />
          {t("status.badges.unknown")}
        </Badge>
      );
    }
  };

  const getResponseTimeBadge = (responseTime: number) => {
    if (responseTime === 0) {
      return (
        <Badge variant="outline">{t("status.labels.not_available")}</Badge>
      );
    } else if (responseTime < 1000) {
      return (
        <Badge
          variant="default"
          className="bg-success-muted text-success-foreground"
        >
          {t("status.speed.fast")}
        </Badge>
      );
    } else if (responseTime < 3000) {
      return (
        <Badge
          variant="secondary"
          className="bg-warning-muted text-warning-foreground"
        >
          {t("status.speed.normal")}
        </Badge>
      );
    } else {
      return (
        <Badge
          variant="destructive"
          className="bg-destructive/10 text-destructive"
        >
          {t("status.speed.slow")}
        </Badge>
      );
    }
  };

  // Filter channels based on search term
  const filteredChannels = channelsData.filter((channel) => {
    if (!searchTerm) return true;
    const searchLower = searchTerm.toLowerCase();
    return (
      channel.name.toLowerCase().includes(searchLower) ||
      channel.status.toLowerCase().includes(searchLower) ||
      (channel.enabled ? "enabled" : "disabled").includes(searchLower)
    );
  });

  const enabledChannels = filteredChannels.filter(
    (channel) => channel.enabled
  ).length;
  const disabledChannels = filteredChannels.filter(
    (channel) => !channel.enabled
  ).length;
  const displayedChannels = filteredChannels.length;

  if (loading) {
    return (
      <ResponsivePageContainer>
        <div className="flex items-center justify-center py-16">
          <div className="text-center space-y-2">
            <span className="h-8 w-8 animate-spin rounded-full border-4 border-primary/20 border-t-primary inline-block" />
            <p className="text-muted-foreground">{t("status.loading")}</p>
          </div>
        </div>
      </ResponsivePageContainer>
    );
  }

  return (
    <ResponsivePageContainer
      title={t("status.title")}
      description={t("status.subtitle")}
      actions={
        <Button
          onClick={handleRefresh}
          disabled={refreshing}
          variant="outline"
          size="sm"
          className={cn("flex items-center gap-2", isMobile && "w-full touch-target")}
        >
          <RefreshCw
            className={cn("w-4 h-4", refreshing && "animate-spin")}
          />
          {refreshing ? t("status.refreshing") : t("status.refresh")}
        </Button>
      }
    >
      {/* Statistics Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card>
          <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
            <div className="flex items-center space-x-2">
              <CheckCircle className="w-5 h-5 text-success flex-shrink-0" />
              <div>
                <p className="text-2xl font-bold text-success">
                  {enabledChannels}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t("status.stats.enabled")}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
            <div className="flex items-center space-x-2">
              <XCircle className="w-5 h-5 text-destructive flex-shrink-0" />
              <div>
                <p className="text-2xl font-bold text-destructive">
                  {disabledChannels}
                </p>
                <p className="text-sm text-muted-foreground">
                  {t("status.stats.disabled")}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
            <div className="flex items-center space-x-2">
              <Activity className="w-5 h-5 text-info flex-shrink-0" />
              <div>
                <p className="text-2xl font-bold text-info">
                  {searchTerm ? displayedChannels : totalCount}
                </p>
                <p className="text-sm text-muted-foreground">
                  {searchTerm
                    ? t("status.stats.found")
                    : t("status.stats.total")}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Search and Controls */}
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="flex-1">
          <Input
            placeholder={t("status.search.placeholder")}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full"
          />
        </div>
        {searchTerm && (
          <Button
            variant="outline"
            onClick={() => setSearchTerm("")}
            className={cn("whitespace-nowrap", isMobile && "touch-target")}
          >
            {t("status.search.clear")}
          </Button>
        )}
      </div>

      {/* Channel Status Cards */}
      {filteredChannels.length === 0 ? (
        <Card>
          <CardContent className="p-8 text-center">
            <Activity className="w-12 h-12 mx-auto mb-4 text-muted-foreground" />
            <h2 className="text-lg font-semibold mb-2">
              {t("status.empty.title")}
            </h2>
            <p className="text-muted-foreground">
              {searchTerm
                ? t("status.empty.search", { term: searchTerm })
                : t("status.empty.none")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredChannels.map((channel) => (
            <Card key={channel.name} className="hover:shadow-md transition-shadow">
              <CardHeader className="pb-3">
                <div className="flex items-center justify-between gap-2">
                  <CardTitle className="text-lg truncate min-w-0">
                    {channel.name}
                  </CardTitle>
                  <div className="flex-shrink-0">
                    {getStatusBadge(channel.status, channel.enabled)}
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* Response Time */}
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center space-x-2 min-w-0">
                    <Clock className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                    <span className="text-sm text-muted-foreground truncate">
                      {t("status.details.response_time")}
                    </span>
                  </div>
                  <div className="flex items-center space-x-2 flex-shrink-0">
                    <span className="font-mono text-sm">
                      {formatResponseTime(
                        channel.response.response_time_ms
                      )}
                    </span>
                    {getResponseTimeBadge(
                      channel.response.response_time_ms
                    )}
                  </div>
                </div>

                {/* Test Time */}
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center space-x-2 min-w-0">
                    <Zap className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                    <span className="text-sm text-muted-foreground truncate">
                      {t("status.details.last_test")}
                    </span>
                  </div>
                  <TimestampDisplay
                    timestamp={channel.response.test_time || null}
                    className="text-sm font-mono flex-shrink-0"
                    fallback={t("status.labels.never")}
                  />
                </div>

                {/* Created Time */}
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center space-x-2 min-w-0">
                    <Calendar className="w-4 h-4 text-muted-foreground flex-shrink-0" />
                    <span className="text-sm text-muted-foreground truncate">
                      {t("status.details.created")}
                    </span>
                  </div>
                  <TimestampDisplay
                    timestamp={channel.response.created_time || null}
                    className="text-sm font-mono flex-shrink-0"
                    fallback={t("status.labels.never")}
                  />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Pagination - using standard AdvancedPagination component */}
      {!searchTerm && (
        <AdvancedPagination
          currentPage={currentPage + 1}
          totalPages={totalPages}
          pageSize={pageSize}
          totalItems={totalCount}
          onPageChange={handlePageChange}
          onPageSizeChange={handlePageSizeChange}
          loading={loading}
        />
      )}

      {/* Footer Info for search results */}
      {searchTerm && filteredChannels.length > 0 && (
        <div className="text-center text-sm text-muted-foreground">
          {t("status.pagination.showing_filtered", {
            displayed: filteredChannels.length,
            total: totalCount,
          })}
        </div>
      )}
    </ResponsivePageContainer>
  );
}

export function StatusPage() {
  return <StatusPageImpl />;
}

export default StatusPage;
