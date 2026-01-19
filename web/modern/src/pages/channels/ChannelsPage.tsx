import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';
import { ListActionButton } from '@/components/ui/list-action-button';
import { useNotifications } from '@/components/ui/notifications';
import { ResponsiveActionGroup } from '@/components/ui/responsive-action-group';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { type SearchOption } from '@/components/ui/searchable-dropdown';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { STORAGE_KEYS, usePageSize } from '@/hooks/usePersistentState';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { cn, formatTimestamp } from '@/lib/utils';
import type { ColumnDef } from '@tanstack/react-table';
import { Ban, CheckCircle, FlaskConical, Plus, RefreshCw, Settings, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { resolveChannelColor } from './utils/colorGenerator';

interface Channel {
  id: number;
  name: string;
  type: number;
  status: number;
  response_time?: number;
  created_time: number;
  updated_time?: number;
  priority?: number;
  weight?: number;
  models?: string;
  group?: string;
  used_quota?: number;
  test_time?: number;
  testing_model?: string | null;
}

/**
 * Channel options defined at relay/channeltype/define.go
 */
const CHANNEL_TYPES: Record<number, { name: string; color: string }> = {
  1: { name: 'OpenAI', color: 'green' },
  50: { name: 'OpenAI Compatible', color: 'olive' },
  14: { name: 'Anthropic', color: 'black' },
  33: { name: 'AWS', color: 'orange' },
  3: { name: 'Azure', color: 'blue' },
  11: { name: 'PaLM2', color: 'orange' },
  24: { name: 'Gemini', color: 'orange' },
  51: { name: 'Gemini (OpenAI)', color: 'orange' },
  28: { name: 'Mistral AI', color: 'purple' },
  41: { name: 'Novita', color: 'purple' },
  40: { name: 'ByteDance Volcano', color: 'blue' },
  15: { name: 'Baidu Wenxin', color: 'blue' },
  47: { name: 'Baidu Wenxin V2', color: 'blue' },
  17: { name: 'Alibaba Qianwen', color: 'orange' },
  49: { name: 'Alibaba Bailian', color: 'orange' },
  18: { name: 'iFlytek Spark', color: 'blue' },
  48: { name: 'iFlytek Spark V2', color: 'blue' },
  16: { name: 'Zhipu ChatGLM', color: 'violet' },
  19: { name: '360 ZhiNao', color: 'blue' },
  25: { name: 'Moonshot AI', color: 'black' },
  23: { name: 'Tencent Hunyuan', color: 'teal' },
  26: { name: 'Baichuan', color: 'orange' },
  27: { name: 'MiniMax', color: 'red' },
  29: { name: 'Groq', color: 'orange' },
  30: { name: 'Ollama', color: 'black' },
  31: { name: '01.AI', color: 'green' },
  32: { name: 'StepFun', color: 'blue' },
  34: { name: 'Coze', color: 'blue' },
  35: { name: 'Cohere', color: 'blue' },
  36: { name: 'DeepSeek', color: 'black' },
  37: { name: 'Cloudflare', color: 'orange' },
  38: { name: 'DeepL', color: 'black' },
  39: { name: 'together.ai', color: 'blue' },
  42: { name: 'VertexAI', color: 'blue' },
  43: { name: 'Proxy', color: 'blue' },
  44: { name: 'SiliconFlow', color: 'blue' },
  45: { name: 'xAI', color: 'blue' },
  46: { name: 'Replicate', color: 'blue' },
  8: { name: 'Custom', color: 'pink' },
  22: { name: 'FastGPT', color: 'blue' },
  21: { name: 'AI Proxy KB', color: 'purple' },
  20: { name: 'OpenRouter', color: 'black' },
};

const formatResponseTime = (time?: number) => {
  if (!time) return '-';
  const color = time < 1000 ? 'text-green-600' : time < 3000 ? 'text-yellow-600' : 'text-red-600';
  return <span className={cn('font-mono text-sm', color)}>{time}ms</span>;
};

export function ChannelsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isMobile } = useResponsive();
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const [data, setData] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(false);
  const [pageIndex, setPageIndex] = useState(Math.max(0, parseInt(searchParams.get('p') || '1') - 1));
  const [pageSize, setPageSize] = usePageSize(STORAGE_KEYS.PAGE_SIZE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [sortBy, setSortBy] = useState('id');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [bulkTesting, setBulkTesting] = useState(false);
  const initializedRef = useRef(false);
  const skipFirstSortEffect = useRef(true);

  const renderChannelTypeBadge = (type: number) => {
    const channelType = CHANNEL_TYPES[type] || {
      name: t('channels.type_unknown', { type }),
      color: undefined,
    };
    const colorValue = resolveChannelColor(channelType.color, type);
    return (
      <Badge variant="outline" className="text-xs gap-1.5">
        <span className="inline-block w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: colorValue }} />
        {channelType.name}
      </Badge>
    );
  };

  const renderStatusBadge = (status: number, priority?: number) => {
    if (status === 2) {
      return <Badge variant="destructive">{t('channels.status.disabled')}</Badge>;
    }
    if ((priority ?? 0) < 0) {
      return (
        <Badge variant="secondary" className="bg-yellow-100 text-yellow-800">
          {t('channels.status.paused')}
        </Badge>
      );
    }
    return (
      <Badge variant="default" className="bg-green-100 text-green-800">
        {t('channels.status.active')}
      </Badge>
    );
  };
  const updateSearchParamPage = (nextPageIndex: number) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.set('p', (nextPageIndex + 1).toString());
      return params;
    });
  };

  const load = async (p = 0, size = pageSize) => {
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/channel/?p=${p}&size=${size}`;
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
      console.error('Failed to load channels:', error);
      setData([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  };

  const searchChannels = async (query: string) => {
    if (!query.trim()) {
      setSearchOptions([]);
      return;
    }

    setSearchLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/channel/search?keyword=${encodeURIComponent(query)}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      url += `&size=${pageSize}`;

      const res = await api.get(url);
      const { success, data: responseData } = res.data;

      if (success && Array.isArray(responseData)) {
        const options: SearchOption[] = responseData.map((channel: Channel) => ({
          key: channel.id.toString(),
          value: channel.name,
          text: channel.name,
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{channel.name}</div>
              <div className="text-sm text-muted-foreground flex items-center gap-2">
                {t('channels.search.id_label')}: {channel.id} • {renderChannelTypeBadge(channel.type)} •{' '}
                {renderStatusBadge(channel.status, channel.priority)}
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
      return load(0, pageSize);
    }

    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      let url = `/api/channel/search?keyword=${encodeURIComponent(searchKeyword)}`;
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

  // Load initial data
  useEffect(() => {
    load(pageIndex, pageSize);
    initializedRef.current = true;
  }, []);

  // Handle sort changes (only after initialization)
  useEffect(() => {
    // Skip the very first run to avoid duplicating the initial load
    if (skipFirstSortEffect.current) {
      skipFirstSortEffect.current = false;
      return;
    }

    if (!initializedRef.current) return;

    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
  }, [sortBy, sortOrder]);

  const manage = async (id: number, action: 'enable' | 'disable' | 'delete' | 'test', index?: number) => {
    try {
      if (action === 'delete') {
        if (!confirm(t('channels.confirm.delete'))) return;
        // Unified API call - complete URL with /api prefix
        const res = await api.delete(`/api/channel/${id}`);
        if (res.data?.success) {
          if (searchKeyword.trim()) {
            performSearch();
          } else {
            load(pageIndex, pageSize);
          }
        }
        return;
      }

      if (action === 'test') {
        // Unified API call - complete URL with /api prefix
        const res = await api.get(`/api/channel/test/${id}`);
        const { success, time, message } = res.data;
        if (index !== undefined) {
          const newData = [...data];
          newData[index] = {
            ...newData[index],
            response_time: time,
            test_time: Date.now(),
          };
          setData(newData);
        }
        if (success) {
          notify({
            type: 'success',
            message: t('channels.notifications.test_success'),
          });
        } else {
          notify({
            type: 'error',
            title: t('channels.notifications.test_failed_title'),
            message: message || t('channels.notifications.test_failed_message'),
          });
        }
        return;
      }

      // Enable/disable - send status_only to avoid overwriting other fields
      const payload = { id, status: action === 'enable' ? 1 : 2 };
      const res = await api.put('/api/channel/?status_only=1', payload);
      if (res.data?.success) {
        if (searchKeyword.trim()) {
          performSearch();
        } else {
          load(pageIndex, pageSize);
        }
      }
    } catch (error) {
      console.error(`Failed to ${action} channel:`, error);
    }
  };

  const updateTestingModel = async (id: number, testingModel: string | null) => {
    try {
      const current = data.find((c) => c.id === id);
      const payload: any = { id, name: current?.name };
      // When null, let backend clear it (auto-cheapest)
      if (testingModel === null) {
        payload.testing_model = null;
      } else {
        payload.testing_model = testingModel;
      }
      // Unified API call - complete URL with /api prefix
      const res = await api.put('/api/channel/', payload);
      if (res.data?.success) {
        // Update local row to reflect change
        setData((prev) => prev.map((ch) => (ch.id === id ? { ...ch, testing_model: testingModel } : ch)));
        notify({
          type: 'success',
          message: t('channels.notifications.testing_model_saved'),
        });
      } else {
        const msg = res.data?.message || t('channels.notifications.testing_model_failed_message');
        notify({
          type: 'error',
          title: t('channels.notifications.testing_model_failed_title'),
          message: msg,
        });
      }
    } catch (error) {
      console.error('Failed to update testing model:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.testing_model_failed_title'),
        message: t('channels.notifications.testing_model_failed_message'),
      });
    }
  };

  const handleBulkTest = async () => {
    setBulkTesting(true);
    try {
      // Unified API call - complete URL with /api prefix
      await api.get('/api/channel/test');
      load(pageIndex, pageSize);
      notify({
        type: 'info',
        message: t('channels.notifications.bulk_test_started'),
      });
    } catch (error) {
      console.error('Bulk test failed:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.bulk_test_failed_title'),
        message: error instanceof Error ? error.message : t('channels.notifications.test_failed_message'),
      });
    } finally {
      setBulkTesting(false);
    }
  };

  const handleDeleteDisabled = async () => {
    if (!confirm(t('channels.confirm.delete_disabled'))) return;

    try {
      // Unified API call - complete URL with /api prefix
      await api.delete('/api/channel/disabled');
      load(pageIndex, pageSize);
      notify({
        type: 'success',
        message: t('channels.notifications.delete_disabled_success'),
      });
    } catch (error) {
      console.error('Failed to delete disabled channels:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.delete_failed_title'),
        message: error instanceof Error ? error.message : t('channels.notifications.delete_failed_message'),
      });
    }
  };

  const columns: ColumnDef<Channel>[] = [
    {
      accessorKey: 'id',
      header: t('channels.columns.id'),
      cell: ({ row }) => <span className="font-mono text-sm">{row.original.id}</span>,
    },
    {
      accessorKey: 'name',
      header: t('channels.columns.name'),
      cell: ({ row }) => <div className="font-medium">{row.original.name}</div>,
    },
    {
      accessorKey: 'type',
      header: t('channels.columns.type'),
      cell: ({ row }) => renderChannelTypeBadge(row.original.type),
    },
    {
      accessorKey: 'status',
      header: t('channels.columns.status'),
      cell: ({ row }) => renderStatusBadge(row.original.status, row.original.priority),
    },
    {
      accessorKey: 'group',
      header: t('channels.columns.group'),
      cell: ({ row }) => <span className="text-sm">{row.original.group || t('channels.group_default')}</span>,
    },
    {
      accessorKey: 'priority',
      header: t('channels.columns.priority'),
      cell: ({ row }) => <span className="font-mono text-sm">{row.original.priority || 0}</span>,
    },
    {
      accessorKey: 'weight',
      header: t('channels.columns.weight'),
      cell: ({ row }) => <span className="font-mono text-sm">{row.original.weight || 0}</span>,
    },
    {
      accessorKey: 'response_time',
      header: t('channels.columns.response'),
      cell: ({ row }) => {
        const responseTime = row.original.response_time;
        const testTime = row.original.test_time;
        const responseTitle = `${t('channels.response.prefix')} ${responseTime ? `${responseTime}ms` : t('channels.response.not_tested')}${
          testTime
            ? ` (${t('channels.response.tested_at', {
                local: formatTimestamp(testTime),
                utc: formatTimestamp(testTime, { timeZone: 'UTC' }),
              })})`
            : ''
        }`;
        return (
          <div className="text-center" title={responseTitle}>
            {formatResponseTime(responseTime)}
            {testTime && (
              <div className="text-xs text-muted-foreground">
                <TimestampDisplay timestamp={testTime} className="font-mono" />
              </div>
            )}
          </div>
        );
      },
    },
    {
      accessorKey: 'testing_model',
      header: t('channels.columns.testing_model'),
      cell: ({ row }) => {
        const ch = row.original;
        const models = (ch.models || '')
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean)
          .sort();
        const value = ch.testing_model ?? ''; // empty => Auto (cheapest)
        return (
          <div className="w-[140px] md:w-[160px] max-w-[220px]">
            <select
              className="w-full border rounded px-2 py-1 text-sm bg-background"
              value={value}
              aria-label={t('channels.columns.testing_model')}
              onChange={(e) => {
                const v = e.target.value;
                updateTestingModel(ch.id, v === '' ? null : v);
              }}
            >
              <option value="">{t('channels.testing.auto')}</option>
              {models.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>
        );
      },
    },
    {
      accessorKey: 'created_time',
      header: t('channels.columns.created'),
      cell: ({ row }) => <TimestampDisplay timestamp={row.original.created_time} className="text-sm font-mono" />,
    },
    {
      header: t('channels.columns.actions'),
      cell: ({ row }) => {
        const channel = row.original;
        return (
          <ResponsiveActionGroup className="sm:items-center">
            <ListActionButton
              variant="outline"
              size="sm"
              onClick={() => navigate(`/channels/edit/${channel.id}`)}
              className="gap-1"
              icon={<Settings className="h-3 w-3" />}
            >
              {t('channels.actions.edit')}
            </ListActionButton>
            <ListActionButton
              variant="outline"
              size="sm"
              onClick={() => manage(channel.id, channel.status === 1 ? 'disable' : 'enable')}
              className={cn(
                'gap-1',
                channel.status === 1 ? 'text-orange-600 hover:text-orange-700' : 'text-green-600 hover:text-green-700'
              )}
            >
              {channel.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
            </ListActionButton>
            <ListActionButton
              variant="outline"
              size="sm"
              onClick={() => manage(channel.id, 'test', row.index)}
              className="gap-1"
              icon={<FlaskConical className="h-3 w-3" />}
            >
              {t('channels.actions.test')}
            </ListActionButton>
            <ListActionButton
              variant="destructive"
              size="sm"
              onClick={() => manage(channel.id, 'delete')}
              className="gap-1"
              icon={<Trash2 className="h-3 w-3" />}
            >
              {t('channels.actions.delete')}
            </ListActionButton>
          </ResponsiveActionGroup>
        );
      },
    },
  ];

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    updateSearchParamPage(newPageIndex);
    load(newPageIndex, newPageSize);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    // Don't call load here - let onPageChange handle it to avoid duplicate API calls
    setPageIndex(0);
  };

  const handleSortChange = (newSortBy: string, newSortOrder: 'asc' | 'desc') => {
    setSortBy(newSortBy);
    setSortOrder(newSortOrder);
    updateSearchParamPage(0);
    setPageIndex(0);
    // Let useEffect handle the reload to avoid double requests
  };

  const refresh = () => {
    if (searchKeyword.trim()) {
      performSearch();
    } else {
      load(pageIndex, pageSize);
    }
  };

  const toolbarActions = (
    <div className={cn('flex gap-2 flex-wrap max-w-full', isMobile ? 'flex-col w-full' : 'items-center')}>
      <div className="flex gap-2 w-full md:w-auto">
        <Button
          variant="outline"
          onClick={handleBulkTest}
          disabled={bulkTesting || loading}
          className={cn('gap-2 flex-1 md:flex-none whitespace-nowrap', isMobile ? 'touch-target' : '')}
          size="sm"
        >
          {bulkTesting ? <RefreshCw className="h-4 w-4 animate-spin" /> : <FlaskConical className="h-4 w-4" />}
          {isMobile ? t('channels.toolbar.test_all_mobile') : t('channels.toolbar.test_all')}
        </Button>
        <Button
          variant="destructive"
          onClick={handleDeleteDisabled}
          className={cn('gap-2 flex-1 md:flex-none whitespace-nowrap', isMobile ? 'touch-target' : '')}
          size="sm"
        >
          <Trash2 className="h-4 w-4" />
          {isMobile ? t('channels.toolbar.delete_disabled_mobile') : t('channels.toolbar.delete_disabled')}
        </Button>
      </div>
    </div>
  );

  return (
    <ResponsivePageContainer
      title={t('channels.title')}
      description={t('channels.description')}
      actions={
        <Button
          onClick={() => navigate('/channels/add')}
          className={cn('gap-2 whitespace-nowrap', isMobile ? 'w-full touch-target' : '')}
          size={isMobile ? 'sm' : 'md'}
        >
          <Plus className="h-4 w-4" />
          {isMobile ? t('channels.actions.add_mobile') : t('channels.actions.add')}
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
                  onClick={() => navigate(`/channels/edit/${row.id}`)}
                  title={t('channels.actions.edit')}
                  aria-label={t('channels.actions.edit')}
                  icon={<Settings className="h-4 w-4" />}
                />
                <ListActionButton
                  onClick={() => manage(row.id, row.status === 1 ? 'disable' : 'enable')}
                  title={row.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
                  aria-label={row.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
                  className={row.status === 1 ? 'text-orange-600 hover:text-orange-700' : 'text-green-600 hover:text-green-700'}
                  icon={row.status === 1 ? <Ban className="h-4 w-4" /> : <CheckCircle className="h-4 w-4" />}
                />
                <ListActionButton
                  onClick={() => {
                    const idx = data.findIndex((c) => c.id === row.id);
                    manage(row.id, 'test', idx !== -1 ? idx : undefined);
                  }}
                  title={t('channels.actions.test')}
                  aria-label={t('channels.actions.test')}
                  icon={<FlaskConical className="h-4 w-4" />}
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
            onSearchChange={searchChannels}
            onSearchValueChange={setSearchKeyword}
            onSearchSubmit={performSearch}
            searchPlaceholder={t('channels.search.placeholder')}
            allowSearchAdditions={true}
            toolbarActions={toolbarActions}
            onRefresh={refresh}
            loading={loading}
            emptyMessage={t('channels.empty')}
            mobileCardLayout={true}
            hideColumnsOnMobile={['created_time', 'response_time']}
            compactMode={isMobile}
          />
        </CardContent>
      </Card>
    </ResponsivePageContainer>
  );
}
