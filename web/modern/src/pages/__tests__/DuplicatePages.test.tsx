import type { ReactNode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ChannelsPage } from '../channels/ChannelsPage';
import { TokensPage } from '../tokens/TokensPage.impl';

const { get, post, notify } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), notify: vi.fn() }));
vi.mock('@/lib/api', () => ({ api: { get, post } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('@/components/ui/confirm-dialog', () => ({ useConfirmDialog: () => [vi.fn(), () => null] }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false, isTablet: false }) }));
vi.mock('@/pages/channels/useChannelModelReset', () => ({
  useChannelModelReset: () => ({ busy: false, report: null, confirmation: null }),
  ChannelModelResetButton: () => null,
}));
vi.mock('@/pages/channels/useSelectedChannelActions', () => ({
  useSelectedChannelActions: () => ({ busy: false, report: null, confirmation: null }),
}));

interface Row {
  id?: number;
  uuid?: string;
  name: string;
}

vi.mock('@/components/ui/enhanced-data-table', () => ({
  // EnhancedDataTable exposes real page action renderers without depending on table virtualization.
  EnhancedDataTable: ({
    data, columns, floatingRowActions, onSearchValueChange, onSearchSubmit, onPageChange, pageIndex, pageSize,
  }: {
    data: Row[];
    columns: { header: unknown; cell?: (context: { row: { original: Row; index: number } }) => ReactNode }[];
    floatingRowActions: (row: Row) => ReactNode;
    onSearchValueChange: (value: string) => void;
    onSearchSubmit: () => void;
    onPageChange: (page: number, size: number) => void;
    pageIndex: number;
    pageSize: number;
  }) => (
    <div>
      <input aria-label="Search draft" onChange={(event) => onSearchValueChange(event.target.value)} />
      <button onClick={onSearchSubmit}>Apply search</button>
      <button onClick={() => onPageChange(pageIndex + 1, pageSize)}>Next page</button>
      {data.map((row, index) => (
        <div key={row.uuid || row.id}>
          <span>{row.name}</span>
          <div role="group" aria-label="inline actions">
            {columns.find((column) => column.header === 'Actions')?.cell?.({ row: { original: row, index } })}
          </div>
          <div role="group" aria-label="floating actions">{floatingRowActions(row)}</div>
        </div>
      ))}
    </div>
  ),
}));

const row = {
  id: 7, uuid: 'source-uuid', name: 'source-9', key: 'source-secret', type: 1, status: 1,
  remain_quota: 100, unlimited_quota: false, used_quota: 50,
  created_time: 0, accessed_time: 0, expired_time: -1,
};

/** deferred returns a controllable promise for cross-layout and navigation regressions. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

/** duplicateButton locates the action inside the requested real page action slot. */
function duplicateButton(layout: 'inline' | 'floating') {
  return within(screen.getByRole('group', { name: `${layout} actions` })).getByRole('button', { name: 'Duplicate' });
}

afterEach(cleanup);
beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  post.mockResolvedValue({ data: { success: true, data: { name: 'source-10' } } });
});

describe.each(['token', 'channel'] as const)('%s page duplicate UX', (kind) => {
  const Page = kind === 'token' ? TokensPage : ChannelsPage;

  /** renderPage returns the actual page with existing requests stubbed and both action layouts visible. */
  const renderPage = (hasUUID = true) => {
    const source = { ...row, uuid: hasUUID ? row.uuid : '' };
    get.mockImplementation(async (url: string) => ({
      data: url === `/api/token/${hasUUID ? row.uuid : row.id}`
        ? { success: true, data: source }
        : { success: true, data: [source], total: 30 },
    }));
    return render(<MemoryRouter initialEntries={[`/${kind}s`]}><Page /></MemoryRouter>);
  };

  it.each([
    ['inline', true], ['floating', true], ['inline', false], ['floating', false],
  ] as const)('creates one independent copy from %s actions with UUID availability %s', async (layout, hasUUID) => {
    renderPage(hasUUID);
    await screen.findByText('source-9');
    fireEvent.click(duplicateButton(layout));
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    if (kind === 'channel') {
      expect(post).toHaveBeenCalledWith(`/api/channel/${hasUUID ? row.uuid : row.id}/duplicate`);
      expect(get.mock.calls.some(([url]) => String(url).startsWith('/api/channel/source-uuid'))).toBe(false);
    } else {
      expect(post).toHaveBeenCalledWith('/api/token/', expect.objectContaining({ name: 'source-10' }));
      expect(JSON.stringify(post.mock.calls)).not.toContain('source-secret');
    }
    await waitFor(() => expect(notify).toHaveBeenCalledWith({ type: 'success', message: `Created ${kind} "source-10".` }));
  });

  it('shares progress and the double-click guard between real inline and floating actions', async () => {
    const creation = deferred<{ data: { success: boolean; data: { name: string } } }>();
    post.mockReturnValue(creation.promise);
    renderPage();
    await screen.findByText('source-9');
    fireEvent.click(duplicateButton('inline'));
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    const buttons = screen.getAllByRole('button', { name: 'Duplicating...' });
    expect(buttons).toHaveLength(2);
    for (const button of buttons) {
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('aria-busy', 'true');
      fireEvent.click(button);
    }
    expect(post).toHaveBeenCalledTimes(1);
    await act(async () => creation.resolve({ data: { success: true, data: { name: 'source-10' } } }));
    await waitFor(() => expect(duplicateButton('floating')).toBeEnabled());
  });

  it('refreshes only the applied filter, never an unsubmitted search draft', async () => {
    renderPage();
    await screen.findByText('source-9');
    fireEvent.change(screen.getByRole('textbox', { name: 'Search draft' }), { target: { value: 'source' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply search' }));
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining('search?keyword=source')));
    fireEvent.change(screen.getByRole('textbox', { name: 'Search draft' }), { target: { value: 'unsubmitted' } });
    get.mockClear();
    fireEvent.click(duplicateButton('floating'));
    await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' })));
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining(`/api/${kind}/search?keyword=source`)));
    expect(get.mock.calls.some(([url]) => String(url).includes('unsubmitted'))).toBe(false);
  });

  it('keeps the page reached while creation is in flight', async () => {
    const creation = deferred<{ data: { success: boolean; data: { name: string } } }>();
    post.mockReturnValue(creation.promise);
    renderPage();
    await screen.findByText('source-9');
    fireEvent.click(duplicateButton('inline'));
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining(`/${kind}/?p=1&size=`)));
    get.mockClear();
    await act(async () => creation.resolve({ data: { success: true, data: { name: 'source-10' } } }));
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining(`/${kind}/?p=1&size=`)));
  });

  it.each(['HTTP failure', 'rejected envelope'])('preserves visible rows and distinguishes a refresh %s from failed creation', async (failure) => {
    renderPage();
    await screen.findByText('source-9');
    get.mockImplementation(async (url: string) => {
      if (url === '/api/token/source-uuid') return { data: { success: true, data: row } };
      if (failure === 'HTTP failure') throw new Error('list unavailable');
      return { data: { success: false, message: 'list unavailable' } };
    });
    fireEvent.click(duplicateButton('inline'));
    await waitFor(() => expect(notify).toHaveBeenCalledTimes(2));
    expect(notify).toHaveBeenNthCalledWith(1, { type: 'success', message: `Created ${kind} "source-10".` });
    expect(notify).toHaveBeenNthCalledWith(2, {
      type: 'error',
      message: `The ${kind} was created, but the list could not be refreshed. Refresh the page.`,
    });
    expect(screen.getByText('source-9')).toBeInTheDocument();
    expect(post).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(duplicateButton('inline')).toBeEnabled());
  });
});
