import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';
import { useNotifications } from '@/components/ui/notifications';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { STORAGE_KEYS, usePageSize } from '@/hooks/usePersistentState';
import { api } from '@/lib/api';
import type { ColumnDef } from '@tanstack/react-table';
import { CheckCircle, Plus, RefreshCw, TestTube, Trash2, XCircle } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';

interface MCPServer {
  id: number;
  name: string;
  status: number;
  priority: number;
  base_url: string;
  protocol: string;
  auth_type: string;
  last_sync_at?: number;
  last_sync_status?: string;
  last_test_at?: number;
  last_test_status?: string;
  auto_sync_interval_minutes?: number;
}

interface MCPServerListItem {
  server: MCPServer;
  tool_count: number;
}

interface MCPServerRow extends MCPServer {
  tool_count: number;
}

export function MCPServersPage() {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [data, setData] = useState<MCPServerRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(Math.max(0, parseInt(searchParams.get('p') || '1') - 1));
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const [sortBy, setSortBy] = useState('id');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const initializedRef = useRef(false);

  const columns = useMemo<ColumnDef<MCPServerRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('mcp.list.columns.name', 'Name'),
      },
      {
        accessorKey: 'status',
        header: t('mcp.list.columns.status', 'Status'),
        cell: ({ row }) =>
          row.original.status === 1 ? (
            <span className="inline-flex items-center gap-1 text-green-600">
              <CheckCircle className="h-4 w-4" />
              {t('mcp.status.enabled', 'Enabled')}
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-red-600">
              <XCircle className="h-4 w-4" />
              {t('mcp.status.disabled', 'Disabled')}
            </span>
          ),
      },
      {
        accessorKey: 'priority',
        header: t('mcp.list.columns.priority', 'Priority'),
      },
      {
        accessorKey: 'base_url',
        header: t('mcp.list.columns.base_url', 'Base URL'),
      },
      {
        accessorKey: 'protocol',
        header: t('mcp.list.columns.protocol', 'Protocol'),
      },
      {
        accessorKey: 'auth_type',
        header: t('mcp.list.columns.auth_type', 'Auth'),
      },
      {
        accessorKey: 'tool_count',
        header: t('mcp.list.columns.tool_count', 'Tools'),
      },
      {
        accessorKey: 'last_sync_at',
        header: t('mcp.list.columns.last_sync', 'Last Sync'),
        cell: ({ row }) => (
          <TimestampDisplay
            timestamp={row.original.last_sync_at ? Math.floor(row.original.last_sync_at / 1000) : undefined}
            fallback={t('mcp.list.labels.never', 'Never')}
            className="text-xs"
          />
        ),
      },
      {
        id: 'actions',
        header: t('mcp.list.columns.actions', 'Actions'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" onClick={() => syncServer(row.original.id)} aria-label={t('mcp.list.actions.sync', 'Sync')}>
              <RefreshCw className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={() => testServer(row.original.id)} aria-label={t('mcp.list.actions.test', 'Test')}>
              <TestTube className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => deleteServer(row.original.id)}
              aria-label={t('mcp.list.actions.delete', 'Delete')}
              className="text-destructive"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ),
      },
    ],
    [t]
  );

  const updateSearchParamPage = (nextPageIndex: number) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.set('p', (nextPageIndex + 1).toString());
      return params;
    });
  };

  const handlePageChange = (nextPageIndex: number, nextPageSize: number) => {
    setPageIndex(nextPageIndex);
    if (nextPageSize !== pageSize) {
      setPageSize(nextPageSize);
    }
    updateSearchParamPage(nextPageIndex);
  };

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      const url = `/api/mcp_servers?p=${p}&size=${size}&sort=${sortBy}&order=${sortOrder}`;
      const response = await api.get(url);
      const { success, data: payload, total: totalCount, message } = response.data;
      if (success) {
        const rows = (payload as MCPServerListItem[]).map((item) => ({
          ...item.server,
          tool_count: item.tool_count,
        }));
        setData(rows);
        setTotal(totalCount ?? rows.length);
      } else {
        notify({
          type: 'error',
          title: t('mcp.notifications.fetch_failed', 'Failed to load MCP servers'),
          message: message || '',
        });
      }
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.notifications.fetch_failed', 'Failed to load MCP servers'),
        message: error instanceof Error ? error.message : String(error),
      });
    } finally {
      setLoading(false);
    }
  };

  const syncServer = async (id: number) => {
    try {
      const response = await api.post(`/api/mcp_servers/${id}/sync`);
      const { success, message } = response.data;
      if (!success) {
        notify({
          type: 'error',
          title: t('mcp.notifications.sync_failed', 'Sync failed'),
          message: message || '',
        });
      } else {
        notify({
          type: 'success',
          title: t('mcp.notifications.sync_success', 'Sync complete'),
          message: '',
        });
        load(pageIndex, pageSize);
      }
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.notifications.sync_failed', 'Sync failed'),
        message: error instanceof Error ? error.message : String(error),
      });
    }
  };

  const testServer = async (id: number) => {
    try {
      const response = await api.post(`/api/mcp_servers/${id}/test`);
      const { success, message, data: payload } = response.data;
      if (!success) {
        notify({
          type: 'error',
          title: t('mcp.notifications.test_failed', 'Test failed'),
          message: message || '',
        });
      } else {
        notify({
          type: 'success',
          title: t('mcp.notifications.test_success', 'Connection OK'),
          message: t('mcp.notifications.test_tools', 'Tools: {{count}}', {
            count: payload?.tool_count ?? 0,
          }),
        });
        load(pageIndex, pageSize);
      }
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.notifications.test_failed', 'Test failed'),
        message: error instanceof Error ? error.message : String(error),
      });
    }
  };

  const deleteServer = async (id: number) => {
    try {
      const response = await api.delete(`/api/mcp_servers/${id}`);
      const { success, message } = response.data;
      if (success) {
        notify({
          type: 'success',
          title: t('mcp.notifications.delete_success', 'Server deleted'),
          message: '',
        });
        load(pageIndex, pageSize);
      } else {
        notify({
          type: 'error',
          title: t('mcp.notifications.delete_failed', 'Delete failed'),
          message: message || '',
        });
      }
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.notifications.delete_failed', 'Delete failed'),
        message: error instanceof Error ? error.message : String(error),
      });
    }
  };

  useEffect(() => {
    const currentPage = Math.max(0, parseInt(searchParams.get('p') || '1') - 1);
    setPageIndex(currentPage);
  }, [searchParams]);

  useEffect(() => {
    if (!initializedRef.current) {
      initializedRef.current = true;
      load(pageIndex, pageSize);
      return;
    }
    load(pageIndex, pageSize);
  }, [pageIndex, pageSize, sortBy, sortOrder]);

  return (
    <ResponsivePageContainer
      title={t('mcp.list.title', 'MCP Servers')}
      description={t('mcp.list.subtitle', 'Manage MCP server registry and tool sync')}
      actions={
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => load(pageIndex, pageSize)}>
            <RefreshCw className="h-4 w-4 mr-2" />
            {t('mcp.list.actions.refresh', 'Refresh')}
          </Button>
          <Button onClick={() => navigate('/mcps/add')}>
            <Plus className="h-4 w-4 mr-2" />
            {t('mcp.list.actions.add', 'Add MCP Server')}
          </Button>
        </div>
      }
    >
      <Card>
        <EnhancedDataTable
          columns={columns}
          data={data}
          loading={loading}
          pageIndex={pageIndex}
          pageSize={pageSize}
          total={total}
          onPageChange={handlePageChange}
          onPageSizeChange={(size) => handlePageChange(0, size)}
          onRowClick={(row) => navigate(`/mcps/edit/${row.id}`)}
          sortBy={sortBy}
          sortOrder={sortOrder}
          onSortChange={(nextSortBy, nextSortOrder) => {
            setSortBy(nextSortBy);
            setSortOrder(nextSortOrder as 'asc' | 'desc');
          }}
        />
      </Card>
    </ResponsivePageContainer>
  );
}
