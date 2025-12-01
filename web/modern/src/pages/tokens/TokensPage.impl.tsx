import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EnhancedDataTable } from "@/components/ui/enhanced-data-table";
import { ResponsiveActionGroup } from "@/components/ui/responsive-action-group";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { type SearchOption } from "@/components/ui/searchable-dropdown";
import { TimestampDisplay } from "@/components/ui/timestamp";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { usePageSize, STORAGE_KEYS } from "@/hooks/usePersistentState";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/stores/auth";
import { cn, renderQuota } from "@/lib/utils";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  Check,
  CheckCircle,
  Copy,
  Eye,
  EyeOff,
  Plus,
  Settings,
  Trash2,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useClipboardManager } from "./useClipboardManager";

export interface Token {
  id: number;
  name: string;
  key: string;
  status: number;
  remain_quota: number;
  unlimited_quota: boolean;
  used_quota: number;
  created_time: number;
  accessed_time: number;
  expired_time: number;
  models?: string;
  subnet?: string;
}

// Status constants
const TOKEN_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  EXPIRED: 3,
  EXHAUSTED: 4,
} as const;

export const shouldHighlightTokenQuota = (
  token: Token,
  userQuota: number | null
): boolean => {
  if (userQuota === null || userQuota < 0) {
    return false;
  }
  if (token.unlimited_quota) {
    return true;
  }
  return token.remain_quota > userQuota;
};

/**
 * TokensPage renders the management interface for API tokens, including search, sorting, and key utilities.
 */
export function TokensPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isMobile } = useResponsive();
  const userQuota = useAuthStore((state) => state.user?.quota ?? null);
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`tokens.page.${key}`, { defaultValue, ...options }),
    [t]
  );
  const [data, setData] = useState<Token[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(
    Math.max(0, parseInt(searchParams.get("p") || "1") - 1)
  );
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState("");
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [sortBy, setSortBy] = useState("id");
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("desc");
  const initializedRef = useRef(false);
  const [showKeys, setShowKeys] = useState<Record<number, boolean>>({});
  const {
    copiedTokens,
    manualCopyToken,
    handleCopySuccess,
    handleCopyFailure,
    clearManualCopyToken,
  } = useClipboardManager();
  const formatQuotaLabel = useCallback(
    (quota: number, unlimited = false) => {
      if (unlimited) {
        return tr("quota.unlimited", "Unlimited");
      }
      return renderQuota(quota);
    },
    [tr]
  );
  const renderStatusBadge = useCallback(
    (status: number) => {
      switch (status) {
        case TOKEN_STATUS.ENABLED:
          return (
            <Badge variant="default" className="bg-green-100 text-green-800">
              {tr("status.enabled", "Enabled")}
            </Badge>
          );
        case TOKEN_STATUS.DISABLED:
          return (
            <Badge variant="secondary" className="bg-gray-100 text-gray-800">
              {tr("status.disabled", "Disabled")}
            </Badge>
          );
        case TOKEN_STATUS.EXPIRED:
          return (
            <Badge variant="destructive" className="bg-red-100 text-red-800">
              {tr("status.expired", "Expired")}
            </Badge>
          );
        case TOKEN_STATUS.EXHAUSTED:
          return (
            <Badge
              variant="destructive"
              className="bg-yellow-100 text-yellow-800"
            >
              {tr("status.exhausted", "Exhausted")}
            </Badge>
          );
        default:
          return (
            <Badge variant="outline">{tr("status.unknown", "Unknown")}</Badge>
          );
      }
    },
    [tr]
  );
  const formatTokenLabel = useCallback(
    (token: Token) => {
      return (
        token.name ||
        tr("table.id_placeholder", "(ID {{id}})", { id: token.id })
      );
    },
    [tr]
  );

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/token/?p=${p}&size=${size}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;

      const res = await api.get(url);
      const { success, data: responseData, total: responseTotal } = res.data;

      if (success) {
        setData(responseData || []);
        setTotal(responseTotal || 0);
        setPageIndex(p);
        setPageSize(size);
      }
    } catch (error) {
      console.error("Failed to load tokens:", error);
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  // Load initial data
  useEffect(() => {
    load(pageIndex, pageSize);
    initializedRef.current = true;
  }, []);

  // Handle sort changes (only after initialization)
  useEffect(() => {
    if (!initializedRef.current) return;

    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
  }, [sortBy, sortOrder]);

  const searchTokens = async (query: string) => {
    if (!query.trim()) {
      setSearchOptions([]);
      return;
    }

    setSearchLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/token/search?keyword=${encodeURIComponent(query)}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      url += `&size=${pageSize}`;

      const res = await api.get(url);
      const { success, data: responseData } = res.data;

      if (success && Array.isArray(responseData)) {
        const options: SearchOption[] = responseData.map((token: Token) => ({
          key: token.id.toString(),
          value: formatTokenLabel(token),
          text: formatTokenLabel(token),
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{formatTokenLabel(token)}</div>
              <div className="text-sm text-muted-foreground flex items-center gap-2 flex-wrap">
                <span>
                  {tr("search.id_label", "ID: {{id}}", { id: token.id })}
                </span>
                {renderStatusBadge(token.status)}
                <span>
                  {tr("search.quota_label", "Quota: {{quota}}", {
                    quota: formatQuotaLabel(
                      token.remain_quota,
                      token.unlimited_quota
                    ),
                  })}
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
      let url = `/api/token/search?keyword=${encodeURIComponent(searchKeyword)}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      url += `&size=${pageSize}`;

      const res = await api.get(url);
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

  const manage = async (
    id: number,
    action: "enable" | "disable" | "delete"
  ) => {
    try {
      let res: any;
      if (action === "delete") {
        // Unified API call - complete URL with /api prefix
        res = await api.delete(`/api/token/${id}`);
      } else {
        // Use status_only to avoid overwriting other fields like name/models when toggling status
        res = await api.put("/api/token/?status_only=1", {
          id,
          status:
            action === "enable" ? TOKEN_STATUS.ENABLED : TOKEN_STATUS.DISABLED,
        });
      }

      if (res.data?.success) {
        if (searchKeyword.trim()) {
          performSearch();
        } else {
          load(pageIndex, pageSize);
        }
      }
    } catch (error) {
      console.error(`Failed to ${action} token:`, error);
    }
  };

  const copyToClipboard = async (token: Token) => {
    if (!navigator?.clipboard?.writeText) {
      handleCopyFailure({ id: token.id, key: token.key });
      return;
    }

    try {
      await navigator.clipboard.writeText(token.key);
      handleCopySuccess(token.id);
    } catch (error) {
      console.error("Failed to copy to clipboard:", error);
      handleCopyFailure({ id: token.id, key: token.key });
    }
  };

  const toggleKeyVisibility = (tokenId: number) => {
    setShowKeys((prev) => ({
      ...prev,
      [tokenId]: !prev[tokenId],
    }));
  };

  const maskKey = (key: string) => {
    if (key.length <= 8) return "***";
    return key.substring(0, 4) + "***" + key.substring(key.length - 4);
  };

  const columns: ColumnDef<Token>[] = [
    {
      accessorKey: "id",
      header: tr("columns.id", "ID"),
      cell: ({ row }) => (
        <span className="font-mono text-sm">{row.original.id}</span>
      ),
    },
    {
      accessorKey: "name",
      header: tr("columns.name", "Name"),
      cell: ({ row }) => (
        <div className="font-medium">{formatTokenLabel(row.original)}</div>
      ),
    },
    {
      accessorKey: "key",
      header: tr("columns.key", "Key"),
      cell: ({ row }) => {
        const token = row.original;
        const isVisible = showKeys[token.id];
        return (
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs">
              {isVisible ? token.key : maskKey(token.key)}
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => toggleKeyVisibility(token.id)}
              className="h-6 w-6 p-0"
            >
              {isVisible ? (
                <EyeOff className="h-3 w-3" />
              ) : (
                <Eye className="h-3 w-3" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => copyToClipboard(token)}
              className="h-6 w-6 p-0"
              disabled={!!copiedTokens[token.id]}
              title={
                copiedTokens[token.id]
                  ? tr("key.copied", "Copied!")
                  : tr("key.copy", "Copy token")
              }
            >
              {copiedTokens[token.id] ? (
                <Check className="h-3 w-3 text-green-600" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
            </Button>
          </div>
        );
      },
    },
    {
      accessorKey: "status",
      header: tr("columns.status", "Status"),
      cell: ({ row }) => renderStatusBadge(row.original.status),
    },
    {
      accessorKey: "remain_quota",
      header: tr("columns.remaining_quota", "Remaining Quota"),
      cell: ({ row }) => {
        const token = row.original;
        const quotaLabel = formatQuotaLabel(
          token.remain_quota,
          token.unlimited_quota
        );
        const highlight = shouldHighlightTokenQuota(token, userQuota);
        const quotaClasses = cn(
          "font-mono text-sm",
          highlight && "text-amber-600 font-semibold"
        );

        if (!highlight) {
          return (
            <span
              className={quotaClasses}
              title={tr("columns.remaining_title", "Remaining: {{label}}", {
                label: quotaLabel,
              })}
            >
              {quotaLabel}
            </span>
          );
        }

        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                className={quotaClasses}
                title={tr("columns.remaining_title", "Remaining: {{label}}", {
                  label: quotaLabel,
                })}
              >
                {quotaLabel}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {tr(
                "columns.remaining_tooltip",
                "This token retains more quota than your account allocation."
              )}
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: "used_quota",
      header: tr("columns.used_quota", "Used Quota"),
      cell: ({ row }) => (
        <span
          className="font-mono text-sm"
          title={tr("columns.used_title", "Used: {{label}}", {
            label: formatQuotaLabel(row.original.used_quota),
          })}
        >
          {formatQuotaLabel(row.original.used_quota)}
        </span>
      ),
    },
    {
      accessorKey: "created_time",
      header: tr("columns.created", "Created"),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={row.original.created_time}
          className="text-sm font-mono"
          fallback={tr("columns.no_value", "—")}
        />
      ),
    },
    {
      accessorKey: "accessed_time",
      header: tr("columns.last_access", "Last Access"),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={
            row.original.accessed_time > 0 ? row.original.accessed_time : null
          }
          className="text-sm font-mono"
          fallback={tr("columns.never", "Never")}
        />
      ),
    },
    {
      accessorKey: "expired_time",
      header: tr("columns.expires", "Expires"),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={
            row.original.expired_time > 0 ? row.original.expired_time : null
          }
          className="text-sm font-mono"
          fallback={tr("columns.never", "Never")}
        />
      ),
    },
    {
      header: tr("columns.actions", "Actions"),
      cell: ({ row }) => {
        const token = row.original;
        return (
          <ResponsiveActionGroup className="mobile-table-cell" justify="start">
            <Button
              variant="outline"
              size="sm"
              onClick={() => navigate(`/tokens/edit/${token.id}`)}
              className="touch-target"
            >
              {tr("actions.edit", "Edit")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                manage(
                  token.id,
                  token.status === TOKEN_STATUS.ENABLED ? "disable" : "enable"
                )
              }
              className={cn(
                "touch-target",
                token.status === TOKEN_STATUS.ENABLED
                  ? "text-orange-600 hover:text-orange-700"
                  : "text-green-600 hover:text-green-700"
              )}
            >
              {token.status === TOKEN_STATUS.ENABLED
                ? tr("actions.disable", "Disable")
                : tr("actions.enable", "Enable")}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                const label = formatTokenLabel(token);
                if (
                  confirm(
                    tr(
                      "confirm.delete",
                      'Are you sure you want to delete token "{{label}}"?',
                      { label }
                    )
                  )
                ) {
                  manage(token.id, "delete");
                }
              }}
              className="touch-target"
            >
              {tr("actions.delete", "Delete")}
            </Button>
          </ResponsiveActionGroup>
        );
      },
    },
  ];

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    setSearchParams((prev) => {
      prev.set("p", (newPageIndex + 1).toString());
      return prev;
    });
    load(newPageIndex, newPageSize);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    setPageIndex(0);
    // Don't call load here - let onPageChange handle it to avoid duplicate API calls
  };

  const handleSortChange = (
    newSortBy: string,
    newSortOrder: "asc" | "desc"
  ) => {
    setSortBy(newSortBy);
    setSortOrder(newSortOrder);
    // Let useEffect handle the reload to avoid double requests
  };

  const refresh = () => {
    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
  };

  return (
    <TooltipProvider delayDuration={150}>
      <ResponsivePageContainer
        title={tr("title", "Tokens")}
        description={tr("description", "Manage your API access tokens")}
        actions={
          <Button
            onClick={() => navigate("/tokens/add")}
            className={cn("gap-2", isMobile ? "w-full touch-target" : "")}
          >
            <Plus className="h-4 w-4" />
            {isMobile
              ? tr("actions.add_mobile", "Add New Token")
              : tr("actions.add", "Add Token")}
          </Button>
        }
      >
        <Card>
          <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
            <EnhancedDataTable
              columns={columns}
              data={data}
              floatingRowActions={(row) => (
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => navigate(`/tokens/edit/${row.id}`)}
                    title={tr("actions.edit", "Edit")}
                  >
                    <Settings className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() =>
                      manage(
                        row.id,
                        row.status === TOKEN_STATUS.ENABLED
                          ? "disable"
                          : "enable"
                      )
                    }
                    title={
                      row.status === TOKEN_STATUS.ENABLED
                        ? tr("actions.disable", "Disable")
                        : tr("actions.enable", "Enable")
                    }
                    className={
                      row.status === TOKEN_STATUS.ENABLED
                        ? "text-orange-600 hover:text-orange-700"
                        : "text-green-600 hover:text-green-700"
                    }
                  >
                    {row.status === TOKEN_STATUS.ENABLED ? (
                      <Ban className="h-4 w-4" />
                    ) : (
                      <CheckCircle className="h-4 w-4" />
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => {
                      const label =
                        row.name ||
                        tr("table.id_placeholder", "(ID {{id}})", {
                          id: row.id,
                        });
                      if (
                        confirm(
                          tr(
                            "confirm.delete",
                            'Are you sure you want to delete token "{{label}}"?',
                            { label }
                          )
                        )
                      ) {
                        manage(row.id, "delete");
                      }
                    }}
                    title={tr("actions.delete", "Delete")}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              )}
              pageIndex={pageIndex}
              pageSize={pageSize}
              total={total}
              onPageChange={handlePageChange}
              onPageSizeChange={handlePageSizeChange}
              sortBy={sortBy}
              sortOrder={sortOrder}
              onSortChange={handleSortChange}
              searchValue={searchKeyword}
              searchOptions={searchOptions}
              searchLoading={searchLoading}
              onSearchChange={searchTokens}
              onSearchValueChange={setSearchKeyword}
              onSearchSubmit={performSearch}
              searchPlaceholder={tr(
                "search.placeholder",
                "Search tokens by name..."
              )}
              allowSearchAdditions={true}
              onRefresh={refresh}
              loading={loading}
              emptyMessage={tr(
                "empty",
                "No tokens found. Create your first token to get started."
              )}
              mobileCardLayout={true}
              hideColumnsOnMobile={[
                "created_time",
                "accessed_time",
                "expired_time",
              ]}
              compactMode={isMobile}
            />
          </CardContent>
        </Card>
      </ResponsivePageContainer>

      <Dialog
        open={!!manualCopyToken}
        onOpenChange={(isOpen) => {
          if (!isOpen) {
            clearManualCopyToken();
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {tr("dialog.manual_copy_title", "Manual copy required")}
            </DialogTitle>
            <DialogDescription>
              {tr(
                "dialog.manual_copy_description",
                "Clipboard access is unavailable. Copy the API key below manually to continue using it."
              )}
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-dashed bg-muted/40 p-4">
            <span className="block font-mono text-sm break-all">
              {manualCopyToken?.key}
            </span>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={clearManualCopyToken}>
              {tr("dialog.close", "Close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </TooltipProvider>
  );
}

export default TokensPage;
