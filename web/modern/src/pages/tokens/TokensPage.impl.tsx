import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';
import { ListActionButton } from '@/components/ui/list-action-button';
import { ResponsiveActionGroup } from '@/components/ui/responsive-action-group';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { type SearchOption } from '@/components/ui/searchable-dropdown';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { STORAGE_KEYS, usePageSize } from '@/hooks/usePersistentState';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { cn, renderQuota } from '@/lib/utils';
import type { ColumnDef } from '@tanstack/react-table';
import { Ban, Check, CheckCircle, Copy, ExternalLink, Eye, EyeOff, Plus, Settings, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useClipboardManager } from './useClipboardManager';

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

interface ThirdPartyClientContext {
  chatLink: string;
  serverAddress: string;
  encodedServerAddress: string;
}

export const resolveThirdPartyClientContext = (): ThirdPartyClientContext => {
  const chatLink = (typeof window !== 'undefined' && window.localStorage.getItem('chat_link')) || '';

  let serverAddress = '';
  if (typeof window !== 'undefined') {
    const rawStatus = window.localStorage.getItem('status');
    if (rawStatus) {
      try {
        const parsed = JSON.parse(rawStatus) as { server_address?: string };
        serverAddress = parsed?.server_address || '';
      } catch {
        // Ignore malformed status; fall back to window.location.origin below.
      }
    }
    if (!serverAddress) {
      serverAddress = window.localStorage.getItem('server_address') || '';
    }
    if (!serverAddress && typeof window.location !== 'undefined') {
      serverAddress = window.location.origin;
    }
  }

  return {
    chatLink,
    serverAddress,
    encodedServerAddress: encodeURIComponent(serverAddress),
  };
};

export type ThirdPartyClient = 'next' | 'ama' | 'opencat' | 'lobechat';

export const buildThirdPartyClientUrl = (client: ThirdPartyClient, key: string, ctx: ThirdPartyClientContext): string | null => {
  const { chatLink, serverAddress, encodedServerAddress } = ctx;
  switch (client) {
    case 'next':
      if (!chatLink) return null;
      return `${chatLink}/#/?settings={"key":"sk-${key}","url":"${serverAddress}"}`;
    case 'ama':
      return `ama://set-api-key?server=${encodedServerAddress}&key=sk-${key}`;
    case 'opencat':
      return `opencat://team/join?domain=${encodedServerAddress}&token=sk-${key}`;
    case 'lobechat':
      if (!chatLink) return null;
      return `${chatLink}/?settings={"keyVaults":{"openai":{"apiKey":"sk-${key}","baseURL":"${serverAddress}/v1"}}}`;
    default:
      return null;
  }
};

export const shouldHighlightTokenQuota = (token: Token, userQuota: number | null): boolean => {
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
  const [confirmDelete, ConfirmDeleteDialog] = useConfirmDialog();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`tokens.page.${key}`, { defaultValue, ...options }),
    [t]
  );
  const [data, setData] = useState<Token[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(Math.max(0, parseInt(searchParams.get('p') || '1') - 1));
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState(searchParams.get('keyword') || '');
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [sortBy, setSortBy] = useState('id');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const initializedRef = useRef(false);
  const [showKeys, setShowKeys] = useState<Record<number, boolean>>({});
  const { copiedTokens, manualCopyToken, handleCopySuccess, handleCopyFailure, clearManualCopyToken } = useClipboardManager();
  const formatQuotaLabel = useCallback(
    (quota: number, unlimited = false) => {
      if (unlimited) {
        return tr('quota.unlimited', 'Unlimited');
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
            <Badge variant="default" className="bg-success-muted text-success-foreground">
              {tr('status.enabled', 'Enabled')}
            </Badge>
          );
        case TOKEN_STATUS.DISABLED:
          return (
            <Badge variant="secondary" className="bg-muted text-muted-foreground">
              {tr('status.disabled', 'Disabled')}
            </Badge>
          );
        case TOKEN_STATUS.EXPIRED:
          return (
            <Badge variant="destructive" className="bg-destructive/10 text-destructive">
              {tr('status.expired', 'Expired')}
            </Badge>
          );
        case TOKEN_STATUS.EXHAUSTED:
          return (
            <Badge variant="destructive" className="bg-warning-muted text-warning-foreground">
              {tr('status.exhausted', 'Exhausted')}
            </Badge>
          );
        default:
          return <Badge variant="outline">{tr('status.unknown', 'Unknown')}</Badge>;
      }
    },
    [tr]
  );
  const formatTokenLabel = useCallback(
    (token: Token) => {
      return token.name || tr('table.id_placeholder', '(ID {{id}})', { id: token.id });
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
      console.error('Failed to load tokens:', error);
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  // Load initial data (perform search if keyword is pre-filled from URL)
  useEffect(() => {
    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
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
                <span>{tr('search.id_label', 'ID: {{id}}', { id: token.id })}</span>
                {renderStatusBadge(token.status)}
                <span>
                  {tr('search.quota_label', 'Quota: {{quota}}', {
                    quota: formatQuotaLabel(token.remain_quota, token.unlimited_quota),
                  })}
                </span>
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
      setSearchParams((prev) => {
        prev.delete('keyword');
        return prev;
      });
      return load(0, pageSize);
    }

    setSearchParams((prev) => {
      prev.set('keyword', searchKeyword);
      return prev;
    });
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
      console.error('Search failed:', error);
    } finally {
      setLoading(false);
    }
  };

  const manage = async (id: number, action: 'enable' | 'disable' | 'delete') => {
    try {
      let res: any;
      if (action === 'delete') {
        // Unified API call - complete URL with /api prefix
        res = await api.delete(`/api/token/${id}`);
      } else {
        // Use status_only to avoid overwriting other fields like name/models when toggling status
        res = await api.put('/api/token/?status_only=1', {
          id,
          status: action === 'enable' ? TOKEN_STATUS.ENABLED : TOKEN_STATUS.DISABLED,
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
      console.error('Failed to copy to clipboard:', error);
      handleCopyFailure({ id: token.id, key: token.key });
    }
  };

  const openInClient = useCallback((token: Token, client: ThirdPartyClient) => {
    const ctx = resolveThirdPartyClientContext();
    const url = buildThirdPartyClientUrl(client, token.key, ctx);
    if (!url) return;
    if (typeof window !== 'undefined' && typeof window.open === 'function') {
      window.open(url, '_blank');
    }
  }, []);

  const renderClientDropdown = (token: Token, label?: string) => {
    const chatLink = (typeof window !== 'undefined' && window.localStorage.getItem('chat_link')) || '';
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" className="touch-target gap-1" aria-label={tr('share.open_in_client', 'Open in client')}>
            <ExternalLink className="h-3.5 w-3.5" />
            {label ?? tr('share.open_in_client', 'Open in client')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {chatLink ? (
            <DropdownMenuItem onSelect={() => openInClient(token, 'next')} data-testid={`token-share-next-${token.id}`}>
              {tr('share.next_chat', 'ChatGPT Next Web')}
            </DropdownMenuItem>
          ) : null}
          <DropdownMenuItem onSelect={() => openInClient(token, 'ama')} data-testid={`token-share-ama-${token.id}`}>
            {tr('share.ama', 'AMA 问天')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => openInClient(token, 'opencat')} data-testid={`token-share-opencat-${token.id}`}>
            {tr('share.opencat', 'OpenCat')}
          </DropdownMenuItem>
          {chatLink ? (
            <DropdownMenuItem onSelect={() => openInClient(token, 'lobechat')} data-testid={`token-share-lobechat-${token.id}`}>
              {tr('share.lobechat', 'LobeChat')}
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    );
  };

  const toggleKeyVisibility = (tokenId: number) => {
    setShowKeys((prev) => ({
      ...prev,
      [tokenId]: !prev[tokenId],
    }));
  };

  const maskKey = (key: string) => {
    if (key.length <= 8) return '***';
    return key.substring(0, 4) + '***' + key.substring(key.length - 4);
  };

  const columns: ColumnDef<Token>[] = [
    {
      accessorKey: 'id',
      header: tr('columns.id', 'ID'),
      cell: ({ row }) => <span className="font-mono text-sm">{row.original.id}</span>,
    },
    {
      accessorKey: 'name',
      header: tr('columns.name', 'Name'),
      cell: ({ row }) => <div className="font-medium">{formatTokenLabel(row.original)}</div>,
    },
    {
      accessorKey: 'key',
      header: tr('columns.key', 'Key'),
      cell: ({ row }) => {
        const token = row.original;
        const isVisible = showKeys[token.id];
        return (
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs">{isVisible ? token.key : maskKey(token.key)}</span>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => toggleKeyVisibility(token.id)}
              className="h-8 w-8 touch-target"
              aria-label={isVisible ? tr('key.hide', 'Hide key') : tr('key.show', 'Show key')}
            >
              {isVisible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => copyToClipboard(token)}
              className="h-8 w-8 touch-target"
              disabled={!!copiedTokens[token.id]}
              aria-label={copiedTokens[token.id] ? tr('key.copied', 'Copied!') : tr('key.copy', 'Copy token')}
              title={copiedTokens[token.id] ? tr('key.copied', 'Copied!') : tr('key.copy', 'Copy token')}
            >
              {copiedTokens[token.id] ? <Check className="h-3.5 w-3.5 text-success" /> : <Copy className="h-3.5 w-3.5" />}
            </Button>
          </div>
        );
      },
    },
    {
      accessorKey: 'status',
      header: tr('columns.status', 'Status'),
      cell: ({ row }) => renderStatusBadge(row.original.status),
    },
    {
      accessorKey: 'remain_quota',
      header: tr('columns.remaining_quota', 'Remaining Quota'),
      cell: ({ row }) => {
        const token = row.original;
        const quotaLabel = formatQuotaLabel(token.remain_quota, token.unlimited_quota);
        const highlight = shouldHighlightTokenQuota(token, userQuota);
        const quotaClasses = cn('font-mono text-sm', highlight && 'text-warning font-semibold');

        if (!highlight) {
          return (
            <span
              className={quotaClasses}
              title={tr('columns.remaining_title', 'Remaining: {{label}}', {
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
                title={tr('columns.remaining_title', 'Remaining: {{label}}', {
                  label: quotaLabel,
                })}
              >
                {quotaLabel}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {tr('columns.remaining_tooltip', 'This token retains more quota than your account allocation.')}
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'used_quota',
      header: tr('columns.used_quota', 'Used Quota'),
      cell: ({ row }) => (
        <span
          className="font-mono text-sm"
          title={tr('columns.used_title', 'Used: {{label}}', {
            label: formatQuotaLabel(row.original.used_quota),
          })}
        >
          {formatQuotaLabel(row.original.used_quota)}
        </span>
      ),
    },
    {
      accessorKey: 'created_time',
      header: tr('columns.created', 'Created'),
      cell: ({ row }) => (
        <TimestampDisplay timestamp={row.original.created_time} className="text-sm font-mono" fallback={tr('columns.no_value', '—')} />
      ),
    },
    {
      accessorKey: 'accessed_time',
      header: tr('columns.last_access', 'Last Access'),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={row.original.accessed_time > 0 ? row.original.accessed_time : null}
          className="text-sm font-mono"
          fallback={tr('columns.never', 'Never')}
        />
      ),
    },
    {
      accessorKey: 'expired_time',
      header: tr('columns.expires', 'Expires'),
      cell: ({ row }) => (
        <TimestampDisplay
          timestamp={row.original.expired_time > 0 ? row.original.expired_time : null}
          className="text-sm font-mono"
          fallback={tr('columns.never', 'Never')}
        />
      ),
    },
    {
      header: tr('columns.actions', 'Actions'),
      cell: ({ row }) => {
        const token = row.original;
        return (
          <ResponsiveActionGroup className="mobile-table-cell" justify="start">
            <Button variant="outline" size="sm" onClick={() => navigate(`/tokens/edit/${token.id}`)} className="touch-target">
              {tr('actions.edit', 'Edit')}
            </Button>
            {renderClientDropdown(token)}
            <Button
              variant="outline"
              size="sm"
              onClick={() => manage(token.id, token.status === TOKEN_STATUS.ENABLED ? 'disable' : 'enable')}
              className={cn(
                'touch-target',
                token.status === TOKEN_STATUS.ENABLED ? 'text-warning hover:text-warning/80' : 'text-success hover:text-success/80'
              )}
            >
              {token.status === TOKEN_STATUS.ENABLED ? tr('actions.disable', 'Disable') : tr('actions.enable', 'Enable')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={async () => {
                const label = formatTokenLabel(token);
                const confirmed = await confirmDelete({
                  title: tr('confirm.delete_title', 'Delete Token'),
                  description: tr('confirm.delete', 'Are you sure you want to delete token "{{label}}"?', { label }),
                });
                if (confirmed) manage(token.id, 'delete');
              }}
              className="touch-target"
            >
              {tr('actions.delete', 'Delete')}
            </Button>
          </ResponsiveActionGroup>
        );
      },
    },
  ];

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    setSearchParams((prev) => {
      prev.set('p', (newPageIndex + 1).toString());
      return prev;
    });
    load(newPageIndex, newPageSize);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    setPageIndex(0);
    // Don't call load here - let onPageChange handle it to avoid duplicate API calls
  };

  const handleSortChange = (newSortBy: string, newSortOrder: 'asc' | 'desc') => {
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
        title={tr('title', 'Tokens')}
        description={tr('description', 'Manage your API access tokens')}
        actions={
          <Button
            onClick={() => navigate('/tokens/add')}
            className={cn('gap-2 whitespace-nowrap', isMobile ? 'w-full touch-target' : '')}
            size={isMobile ? 'sm' : 'md'}
          >
            <Plus className="h-4 w-4" />
            {isMobile ? tr('actions.add_mobile', 'Add New Token') : tr('actions.add', 'Add Token')}
          </Button>
        }
      >
        <Card className="border-0 md:border shadow-none md:shadow-sm">
          <CardContent className={cn(isMobile ? 'p-2' : 'p-6')}>
            <EnhancedDataTable
              columns={columns}
              data={data}
              floatingRowActions={(row) => (
                <div className="flex items-center gap-1">
                  <ListActionButton
                    onClick={() => navigate(`/tokens/edit/${row.id}`)}
                    title={tr('actions.edit', 'Edit')}
                    icon={<Settings className="h-4 w-4" />}
                  />
                  <ListActionButton
                    onClick={() => manage(row.id, row.status === TOKEN_STATUS.ENABLED ? 'disable' : 'enable')}
                    title={row.status === TOKEN_STATUS.ENABLED ? tr('actions.disable', 'Disable') : tr('actions.enable', 'Enable')}
                    className={
                      row.status === TOKEN_STATUS.ENABLED ? 'text-warning hover:text-warning/80' : 'text-success hover:text-success/80'
                    }
                    icon={row.status === TOKEN_STATUS.ENABLED ? <Ban className="h-4 w-4" /> : <CheckCircle className="h-4 w-4" />}
                  />
                  <ListActionButton
                    onClick={async () => {
                      const label =
                        row.name ||
                        tr('table.id_placeholder', '(ID {{id}})', {
                          id: row.id,
                        });
                      const confirmed = await confirmDelete({
                        title: tr('confirm.delete_title', 'Delete Token'),
                        description: tr('confirm.delete', 'Are you sure you want to delete token "{{label}}"?', { label }),
                      });
                      if (confirmed) manage(row.id, 'delete');
                    }}
                    title={tr('actions.delete', 'Delete')}
                    icon={<Trash2 className="h-4 w-4" />}
                  />
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
              onSearchSelect={(key) => navigate(`/tokens/edit/${key}`)}
              searchPlaceholder={tr('search.placeholder', 'Search tokens by name...')}
              allowSearchAdditions={true}
              onRefresh={refresh}
              loading={loading}
              emptyMessage={tr('empty', 'No tokens found. Create your first token to get started.')}
              mobileCardLayout={true}
              hideColumnsOnMobile={['created_time', 'accessed_time', 'expired_time']}
              compactMode={isMobile}
            />
          </CardContent>
        </Card>
      </ResponsivePageContainer>

      <ConfirmDeleteDialog />

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
            <DialogTitle>{tr('dialog.manual_copy_title', 'Manual copy required')}</DialogTitle>
            <DialogDescription>
              {tr(
                'dialog.manual_copy_description',
                'Clipboard access is unavailable. Copy the API key below manually to continue using it.'
              )}
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-dashed bg-muted/40 p-4">
            <span className="block font-mono text-sm break-all">{manualCopyToken?.key}</span>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={clearManualCopyToken}>
              {tr('dialog.close', 'Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </TooltipProvider>
  );
}

export default TokensPage;
