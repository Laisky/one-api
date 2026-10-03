import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { TableSelectionSnapshot } from '@/hooks/useTableSelection';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { chooseTableAction, chooseTableSelection } from '@/test/table-toolbar';
import { ChannelsPage } from '../ChannelsPage';

const { notify, responsive } = vi.hoisted(() => ({ notify: vi.fn(), responsive: { isMobile: false, isTablet: false } }));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => responsive }));
const rows = Array.from({ length: 25 }, (_, index) => ({
  uuid: `018fcf6d-c484-7000-8000-${String(index + 1).padStart(12, '0')}`,
  name: `Provider ${index + 1}`,
  type: 1,
  status: (index % 3) + 1,
  models: 'configured-model',
  group: 'default',
  created_time: 1,
}));
const get = vi.mocked(api.get);
const post = vi.mocked(api.post);
const put = vi.mocked(api.put);
const actions = [
  { label: 'Enable selected channels', status: 1 },
  { label: 'Disable selected channels', status: 2 },
];

/** renderPage mounts real selection, menus, and confirmation; only the network is mocked. */
function renderPage() {
  return render(
    <MemoryRouter>
      <ChannelsPage />
    </MemoryRouter>
  );
}

/** confirmBatch confirms the captured UUID snapshot through the visible dialog. */
async function confirmBatch() {
  const dialog = await screen.findByRole('dialog');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  get.mockReset();
  post.mockReset();
  put.mockReset();
  responsive.isMobile = false;
  localStorage.clear();
  useAuthStore.setState({ user: null, token: null, isAuthenticated: false });
  get.mockImplementation(async (url) => {
    const params = new URL(url, 'https://example.com').searchParams;
    const page = Number(params.get('p') || 0);
    const size = Number(params.get('size') || 10);
    return { data: { success: true, data: rows.slice(page * size, (page + 1) * size), total: rows.length } };
  });
  post.mockImplementation(async (url, body) => {
    if (url !== '/api/channel/selection') throw new Error('Unexpected batch mutation');
    const { selection } = body as { selection: TableSelectionSnapshot };
    return {
      data: {
        success: true,
        data: rows.filter((row) =>
          selection.mode === 'ids' ? selection.ids.includes(row.uuid) : !selection.excluded_ids.includes(row.uuid)
        ),
      },
    };
  });
  put.mockResolvedValue({ data: { success: true } });
});

describe('selected channel enable and disable', () => {
  it('hides batch actions without selection and opening the menu does not mutate', async () => {
    renderPage();
    await screen.findByText('Provider 1');
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 1' }));
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }));
    for (const { label } of actions) expect(screen.getByRole('menuitem', { name: label })).toBeEnabled();
    expect(post).not.toHaveBeenCalled();
    expect(put).not.toHaveBeenCalled();
  });

  it.each(actions)('$label changes only confirmed cross-page UUIDs with a status-only payload', async ({ label, status }) => {
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }));
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 11' }));
    await chooseTableAction(label);
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('2 selected channels');
    expect(put).not.toHaveBeenCalled();
    await confirmBatch();
    await screen.findByText('2 selected: 2 succeeded, 0 skipped, 0 failed or rejected.');
    expect(put).toHaveBeenCalledTimes(2);
    for (const uuid of [rows[0].uuid, rows[10].uuid]) {
      expect(put).toHaveBeenCalledWith('/api/channel/?status_only=1', { uuid, status });
    }
    expect(post).toHaveBeenCalledTimes(1);
    expect(api.delete).not.toHaveBeenCalled();
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining('/api/channel/?p=1&')));
  });

  it.each(actions)('$label respects all-page exclusions and cancellation', async ({ label }) => {
    renderPage();
    await screen.findByText('Provider 1');
    await chooseTableSelection('Select all pages');
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 2' }));
    await chooseTableAction(label);
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('24 selected channels');
    expect(post).toHaveBeenCalledWith('/api/channel/selection', {
      keyword: '',
      selection: { mode: 'all_matching', excluded_ids: [rows[1].uuid] },
    });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(put).not.toHaveBeenCalled();
    expect(screen.getByRole('checkbox', { name: 'Select Provider 2' })).not.toBeChecked();
  });

  it('disables all matching channels except excluded rows, including initially disabled rows', async () => {
    renderPage();
    await screen.findByText('Provider 1');
    await chooseTableSelection('Select all pages');
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 2' }));
    await chooseTableAction('Disable selected channels');
    await confirmBatch();
    await screen.findByText('24 selected: 24 succeeded, 0 skipped, 0 failed or rejected.');
    expect(put).toHaveBeenCalledTimes(24);
    expect(put.mock.calls.map(([, body]) => body)).toEqual(
      rows.filter((row) => row.uuid !== rows[1].uuid).map(({ uuid }) => ({ uuid, status: 2 }))
    );
  });

  it('reports HTTP and failure-envelope errors per channel without stopping other updates', async () => {
    put.mockResolvedValueOnce({ data: { success: true } });
    put.mockResolvedValueOnce({ data: { success: false, message: 'Channel no longer exists' } });
    put.mockRejectedValueOnce({ response: { data: { message: 'Failed to update channel status.' } } });
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 2' }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 3' }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider 4' }));
    await chooseTableAction('Enable selected channels');
    await confirmBatch();
    const report = await screen.findByRole('region', { name: 'Selected action results' });
    expect(report).toHaveTextContent('4 selected: 2 succeeded, 0 skipped, 2 failed or rejected.');
    expect(report).toHaveTextContent('Provider 2');
    expect(report).toHaveTextContent('Channel no longer exists');
    expect(report).toHaveTextContent('Provider 3');
    expect(report).toHaveTextContent('Failed to update channel status.');
    expect(put).toHaveBeenCalledTimes(4);
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2));
  });

  it.each(actions)('$label invalidates pending confirmation when the principal changes', async ({ label }) => {
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await chooseTableAction(label);
    await screen.findByRole('dialog');
    act(() =>
      useAuthStore.setState({
        user: { id: 'other', username: 'another-admin', role: 100, status: 1, quota: 0, used_quota: 0, group: 'default' },
      })
    );
    await confirmBatch();
    expect(put).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument());
  });

  it('bounds concurrent writes, blocks repeat clicks, and stops queued writes on a scope change', async () => {
    const pending: Array<() => void> = [];
    put.mockImplementation(() => new Promise((resolve) => pending.push(() => resolve({ data: { success: true } }))));
    renderPage();
    await screen.findByText('Provider 1');
    await chooseTableSelection('Select this page');
    await chooseTableAction('Disable selected channels');
    await confirmBatch();
    await waitFor(() => expect(put).toHaveBeenCalledTimes(3));
    const button = screen.getByRole('button', { name: 'Actions' });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(post).toHaveBeenCalledTimes(1);
    act(() =>
      useAuthStore.setState({
        user: { id: 'other', username: 'another-admin', role: 100, status: 1, quota: 0, used_quota: 0, group: 'default' },
      })
    );
    await act(async () => pending.forEach((finish) => finish()));
    await screen.findByText('10 selected: 3 succeeded, 0 skipped, 7 failed or rejected.');
    expect(put).toHaveBeenCalledTimes(3);
  });

  it('keeps status actions usable through the mobile card selection menu', async () => {
    responsive.isMobile = true;
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await chooseTableAction('Enable selected channels');
    await confirmBatch();
    await screen.findByText('1 selected: 1 succeeded, 0 skipped, 0 failed or rejected.');
    expect(put).toHaveBeenCalledWith('/api/channel/?status_only=1', { uuid: rows[0].uuid, status: 1 });
  });
});
