import type { TableBatchAction } from '@/components/ui/table-toolbar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';
import { ListActionButton } from '@/components/ui/list-action-button';
import { useNotifications } from '@/components/ui/notifications';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { type SearchOption } from '@/components/ui/searchable-dropdown';
import { STORAGE_KEYS, usePageSize } from '@/hooks/usePersistentState';
import { useAuthStore } from '@/lib/stores/auth';
import { useTableSelection } from '@/hooks/useTableSelection';
import { useSelectedChannelActions } from './useSelectedChannelActions';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { cn } from '@/lib/utils';
import { Ban, CheckCircle, Copy, FlaskConical, Plus, RotateCcw, Settings, Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { CHANNEL_TYPE_LABELS as CHANNEL_TYPES } from './constants';
import { resolveChannelColor } from './utils/colorGenerator';
import { channelRef, channelRefPayload, createChannelColumns, sameChannelRef, type Channel } from './channels-page-columns';
import { ChannelModelResetButton, useChannelModelReset } from './useChannelModelReset';

/** ChannelsPage renders searchable channel administration, bulk actions, and pagination. */
export function ChannelsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isMobile } = useResponsive();
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const [confirmAction, ConfirmActionDialog] = useConfirmDialog();
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
  const [appliedKeyword, setAppliedKeyword] = useState('');
  const loadSequence = useRef(0);
  const { user } = useAuthStore();
  const selection = useTableSelection(JSON.stringify([user?.uuid || user?.username, user?.role, searchKeyword.trim(), appliedKeyword]));
  const [refreshingBalanceIds, setRefreshingBalanceIds] = useState<Set<string | number>>(new Set());
  const initializedRef = useRef(false);
  const skipFirstSortEffect = useRef(true);

  const getChannelTypeLabel = (type: number) => {
    return (
      CHANNEL_TYPES[type]?.name ||
      t('channels.type_unknown', {
        type,
      })
    );
  };

  const renderChannelTypeBadge = (type: number) => {
    const channelType = CHANNEL_TYPES[type] || {
      name: getChannelTypeLabel(type),
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
        <Badge variant="secondary" className="bg-warning-muted text-warning-foreground">
          {t('channels.status.paused')}
        </Badge>
      );
    }
    return (
      <Badge variant="default" className="bg-success-muted text-success-foreground">
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

  /** load binds rows to the applied keyword and keeps page navigation within that result set. */
  const load = async (p = 0, size = pageSize, keyword = appliedKeyword) => {
    const sequence = ++loadSequence.current;
    setLoading(true);
    try {
      let url = keyword ? `/api/channel/search?keyword=${encodeURIComponent(keyword)}` : `/api/channel/?p=${p}&size=${size}`;
      if (sortBy) url += `&sort=${sortBy}&order=${sortOrder}`;
      if (keyword) url += `&size=${size}`;
      const res = await api.get(url);
      if (sequence !== loadSequence.current) return;
      const { success, data: responseData, total: responseTotal, message } = res.data;
      if (!success) throw new Error(message || t('table_selection.failed'));
      const rows: Channel[] = responseData || [];
      // This search API returns the complete result set. Slice locally instead
      // of falling back to an unfiltered API when changing pages.
      setData(keyword ? rows.slice(p * size, (p + 1) * size) : rows);
      setTotal(keyword ? rows.length : responseTotal || 0);
      setAppliedKeyword(keyword);
      setPageIndex(p);
      setPageSize(size);
    } catch (error) {
      if (sequence !== loadSequence.current) return;
      console.error(`Failed to load channels: ${String(error)}`);
      setData([]);
      setTotal(0);
    } finally {
      if (sequence === loadSequence.current) setLoading(false);
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
          key: String(channelRef(channel)),
          value: channel.name,
          text: channel.name,
          // Keep the UUID matchable even though the visible label is the name.
          keywords: [String(channelRef(channel)), channel.uuid || '', String(channel.id ?? '')].filter(Boolean),
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{channel.name}</div>
              <div className="text-sm text-muted-foreground flex items-center gap-2">
                {t('channels.search.id_label')}: {String(channelRef(channel))} • {renderChannelTypeBadge(channel.type)} •{' '}
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

  const performSearch = () => load(0, pageSize, searchKeyword.trim());

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

  const manage = async (id: string | number, action: 'enable' | 'disable' | 'delete' | 'test', index?: number) => {
    try {
      if (action === 'delete') {
        const targetChannel = data.find((channel) => String(channelRef(channel)) === String(id) || String(channel.id) === String(id));
        const confirmed = await confirmAction({
          title: t('channels.confirm.delete_title', 'Delete Channel'),
          description: t('channels.confirm.delete'),
          details: [
            {
              label: t('channels.columns.name'),
              value: targetChannel?.name || '-',
            },
            {
              label: t('channels.columns.type'),
              value: targetChannel ? getChannelTypeLabel(targetChannel.type) : '-',
            },
            {
              label: t('channels.search.id_label'),
              value: id,
            },
          ],
        });
        if (!confirmed) return;
        // Unified API call - complete URL with /api prefix
        const res = await api.delete(`/api/channel/${id}`);
        if (!res.data?.success) {
          notify({
            type: 'error',
            title: t('channels.notifications.delete_failed_title', 'Delete failed'),
            message: res.data?.message || t('channels.notifications.delete_failed_message', 'Failed to delete channel.'),
          });
          return;
        }
        if (searchKeyword.trim()) {
          performSearch();
        } else {
          load(pageIndex, pageSize);
        }
        return;
      }

      if (action === 'test') {
        // Unified API call - complete URL with /api prefix
        const res = await api.get(`/api/channel/test/${id}`);
        const { success, time, message, skipped } = res.data;
        // A skipped channel was never probed (it serves no chat-capable
        // endpoint), so leave its recorded latency untouched rather than
        // stamping it as "tested just now, 0 ms".
        if (index !== undefined && !skipped) {
          const newData = [...data];
          newData[index] = {
            ...newData[index],
            response_time: time * 1000,
            test_time: Date.now(),
          };
          setData(newData);
        }
        if (success) {
          notify({
            type: 'success',
            message: t('channels.notifications.test_success'),
          });
        } else if (skipped) {
          notify({
            type: 'info',
            title: t('channels.notifications.test_skipped_title'),
            message: message || t('channels.notifications.test_skipped_message'),
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
      const payload = { ...channelRefPayload(id), status: action === 'enable' ? 1 : 2 };
      const res = await api.put('/api/channel/?status_only=1', payload);
      if (!res.data?.success) {
        notify({
          type: 'error',
          title: t('channels.notifications.status_failed_title', 'Update failed'),
          message: res.data?.message || t('channels.notifications.status_failed_message', 'Failed to update channel status.'),
        });
        return;
      }
      if (searchKeyword.trim()) {
        performSearch();
      } else {
        load(pageIndex, pageSize);
      }
    } catch (error) {
      console.error(`Failed to ${action} channel:`, error);
      notify({
        type: 'error',
        title:
          action === 'delete'
            ? t('channels.notifications.delete_failed_title', 'Delete failed')
            : t('channels.notifications.status_failed_title', 'Update failed'),
        message:
          error instanceof Error
            ? error.message
            : action === 'delete'
              ? t('channels.notifications.delete_failed_message', 'Failed to delete channel.')
              : t('channels.notifications.status_failed_message', 'Failed to update channel status.'),
      });
    }
  };

  const duplicateChannel = async (channel: Channel) => {
    try {
      const duplicateResponse = await api.post(`/api/channel/${channelRef(channel)}/duplicate`);
      if (duplicateResponse.data?.success) {
        notify({
          type: 'success',
          message: t('channels.notifications.duplicate_success', 'Channel duplicated.'),
        });

        if (searchKeyword.trim()) {
          await performSearch();
        } else {
          await load(pageIndex, pageSize);
        }
        return;
      }

      notify({
        type: 'error',
        title: t('channels.notifications.duplicate_failed_title', 'Duplicate failed'),
        message: duplicateResponse.data?.message || t('channels.notifications.duplicate_failed_message', 'Failed to duplicate channel.'),
      });
    } catch (error) {
      console.error('Failed to duplicate channel:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.duplicate_failed_title', 'Duplicate failed'),
        message:
          error instanceof Error ? error.message : t('channels.notifications.duplicate_failed_message', 'Failed to duplicate channel.'),
      });
    }
  };

  const updateTestingModel = async (channel: Channel, testingModel: string | null) => {
    try {
      const payload: any = { ...channelRefPayload(channelRef(channel)), name: channel.name };
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
        setData((prev) => prev.map((ch) => (sameChannelRef(ch, channel) ? { ...ch, testing_model: testingModel } : ch)));
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

  const handlePriorityUpdate = async (channel: Channel, newPriority: number) => {
    if ((channel.priority ?? 0) === newPriority) return;
    try {
      const res = await api.put('/api/channel/', {
        ...channelRefPayload(channelRef(channel)),
        name: channel.name,
        priority: newPriority,
      });
      if (res.data?.success) {
        setData((prev) => prev.map((row) => (sameChannelRef(row, channel) ? { ...row, priority: newPriority } : row)));
        notify({
          type: 'success',
          message: t('channels.notifications.priority_saved', 'Priority updated.'),
        });
      } else {
        notify({
          type: 'error',
          title: t('channels.notifications.priority_failed_title', 'Update failed'),
          message: res.data?.message || t('channels.notifications.priority_failed_message', 'Failed to update priority.'),
        });
      }
    } catch (error) {
      console.error('Failed to update priority:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.priority_failed_title', 'Update failed'),
        message: error instanceof Error ? error.message : t('channels.notifications.priority_failed_message', 'Failed to update priority.'),
      });
    }
  };

  const handleBalanceRefresh = async (channel: Channel) => {
    const ref = channelRef(channel);
    setRefreshingBalanceIds((prev) => {
      const next = new Set(prev);
      next.add(ref);
      return next;
    });
    try {
      const res = await api.get(`/api/channel/update_balance/${ref}`);
      const { success, message, balance, balance_updated_time } = res.data || {};
      if (success) {
        setData((prev) =>
          prev.map((row) =>
            sameChannelRef(row, channel)
              ? {
                  ...row,
                  balance: typeof balance === 'number' ? balance : row.balance,
                  balance_updated_time: typeof balance_updated_time === 'number' ? balance_updated_time : Math.floor(Date.now() / 1000),
                }
              : row
          )
        );
        notify({
          type: 'success',
          message: t('channels.notifications.balance_success', 'Balance refreshed.'),
        });
      } else {
        notify({
          type: 'error',
          title: t('channels.notifications.balance_failed_title', 'Balance refresh failed'),
          message: message || t('channels.notifications.balance_failed_message', 'Failed to refresh balance.'),
        });
      }
    } catch (error) {
      console.error('Failed to refresh balance:', error);
      notify({
        type: 'error',
        title: t('channels.notifications.balance_failed_title', 'Balance refresh failed'),
        message: error instanceof Error ? error.message : t('channels.notifications.balance_failed_message', 'Failed to refresh balance.'),
      });
    } finally {
      setRefreshingBalanceIds((prev) => {
        const next = new Set(prev);
        next.delete(ref);
        return next;
      });
    }
  };

  const resetModels = useChannelModelReset(() => load(pageIndex, pageSize));
  const selectedActions = useSelectedChannelActions(selection, searchKeyword.trim(), () => load(pageIndex, pageSize));
  const selectionDisabled = loading || selectedActions.busy || resetModels.busy || searchKeyword.trim() !== appliedKeyword;
  const batchDisabled = selectionDisabled || !selection.hasSelection || selection.selectedCount(total) === 0;
  const renderResetAction = (channel: Channel, compact = false) => (
    <ChannelModelResetButton
      channel={channel}
      compact={compact}
      disabled={loading || resetModels.busy || selectedActions.busy}
      onReset={resetModels.resetChannel}
    />
  );

  const columns = createChannelColumns({
    t,
    navigate,
    refreshingBalanceIds,
    renderChannelTypeBadge,
    renderStatusBadge,
    renderResetAction,
    onPriorityUpdate: handlePriorityUpdate,
    onBalanceRefresh: handleBalanceRefresh,
    onTestingModelUpdate: updateTestingModel,
    onDuplicate: duplicateChannel,
    onManage: manage,
  });

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

  const batchActions: TableBatchAction[] = [
    {
      id: 'reset',
      label: t('table_selection.reset'),
      icon: <RotateCcw className="h-4 w-4" />,
      onSelect: () => selectedActions.run('reset'),
    },
    {
      id: 'test',
      label: t('table_selection.test'),
      icon: <FlaskConical className="h-4 w-4" />,
      onSelect: () => selectedActions.run('test'),
    },
    {
      id: 'delete_disabled',
      label: t('table_selection.delete_disabled'),
      icon: <Trash2 className="h-4 w-4" />,
      onSelect: () => selectedActions.run('delete_disabled'),
      destructive: true,
    },
  ];

  return (
    <>
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
            {resetModels.report}
            {selectedActions.report}
            <EnhancedDataTable
              selection={selection}
              selectionDisabled={selectionDisabled}
              columns={columns}
              data={data}
              floatingRowActions={(row) => (
                <div className="flex items-center gap-1">
                  <ListActionButton
                    onClick={() => navigate(`/channels/edit/${channelRef(row)}`)}
                    title={t('channels.actions.edit')}
                    aria-label={t('channels.actions.edit')}
                    icon={<Settings className="h-4 w-4" />}
                  />
                  <ListActionButton
                    onClick={() => duplicateChannel(row)}
                    title={t('channels.actions.duplicate', 'Duplicate')}
                    aria-label={t('channels.actions.duplicate', 'Duplicate')}
                    icon={<Copy className="h-4 w-4" />}
                  />
                  {renderResetAction(row, true)}
                  <ListActionButton
                    onClick={() => manage(channelRef(row), row.status === 1 ? 'disable' : 'enable')}
                    title={row.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
                    aria-label={row.status === 1 ? t('channels.actions.disable') : t('channels.actions.enable')}
                    className={row.status === 1 ? 'text-warning hover:text-warning/80' : 'text-success hover:text-success/80'}
                    icon={row.status === 1 ? <Ban className="h-4 w-4" /> : <CheckCircle className="h-4 w-4" />}
                  />
                  <ListActionButton
                    onClick={() => {
                      const idx = data.findIndex((c) => sameChannelRef(c, row));
                      manage(channelRef(row), 'test', idx !== -1 ? idx : undefined);
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
              onSearchSelect={(key) => navigate(`/channels/edit/${key}`)}
              onSearchSubmit={performSearch}
              searchPlaceholder={t('channels.search.placeholder')}
              allowSearchAdditions={true}
              batchActions={batchActions}
              batchActionsDisabled={batchDisabled}
              batchActionsBusy={selectedActions.busy}
              onRefresh={refresh}
              loading={loading}
              emptyMessage={t('channels.empty')}
              mobileCardLayout={true}
              hideColumnsOnMobile={['created_time', 'response_time', 'balance']}
              compactMode={isMobile}
            />
          </CardContent>
        </Card>
      </ResponsivePageContainer>

      <ConfirmActionDialog />
      {resetModels.confirmation}
      {selectedActions.confirmation}
    </>
  );
}
