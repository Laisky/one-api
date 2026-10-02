import { chooseTableSelection, chooseTableAction } from '@/test/table-toolbar';
import type { TableSelectionSnapshot } from '@/hooks/useTableSelection';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';

const notify = vi.hoisted(() => vi.fn());
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
const rows = Array.from({ length: 25 }, (_, index) => ({
  uuid: `018fcf6d-c484-7000-8000-${String(index + 1).padStart(12, '0')}`,
  name: `Provider ${index + 1}`,
  type: 1,
  status: 1,
  models: 'old',
  group: 'default',
  created_time: 1,
}));
const get = vi.mocked(api.get);
const post = vi.mocked(api.post);

/** renderPage mounts the real list without bypassing selection or confirmation logic. */
function renderPage() {
  return render(
    <MemoryRouter>
      <ChannelsPage />
    </MemoryRouter>
  );
}
/** confirmBatch accepts the resolved selection snapshot through the real dialog. */
async function confirmBatch() {
  const dialog = await screen.findByRole('dialog');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  get.mockImplementation(async (url) => {
    if (url.startsWith('/api/channel/test/')) return { data: { success: true } };
    const params = new URL(url, 'https://example.com').searchParams;
    const p = Number(params.get('p') || 0),
      size = Number(params.get('size') || 10);
    return { data: { success: true, data: rows.slice(p * size, (p + 1) * size), total: 25 } };
  });
  post.mockImplementation(async (url, body) => {
    const selection = (body as { selection: TableSelectionSnapshot }).selection;
    const targets = rows.filter((row) =>
      selection.mode === 'ids' ? selection.ids.includes(row.uuid) : !selection.excluded_ids.includes(row.uuid)
    );
    if (url === '/api/channel/selection') return { data: { success: true, data: targets } };
    return { data: { success: true, data: { results: targets.map((row) => ({ ...row, success: true })) } } };
  });
});

describe('selected channel batch actions', () => {
  it('resets checked rows across pages, freezes UUIDs before confirmation, and never defaults to all', async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await user.click(screen.getByRole('button', { name: 'Page 2' }));
    await user.click(await screen.findByRole('checkbox', { name: 'Select Provider 11' }));
    await chooseTableAction('Reset selected models');
    expect(await screen.findByText(/the 2 selected records/)).toBeInTheDocument();
    expect(post).toHaveBeenCalledTimes(1);
    await confirmBatch();
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/channel/reset_models',
        {
          selection: { mode: 'ids', ids: [rows[0].uuid, rows[10].uuid] },
        },
        { timeout: 120_000 }
      )
    );
    await screen.findByText('2 selected: 2 succeeded, 0 skipped, 0 failed or rejected.');
  });

  it('resolves all matching pages minus exclusions and supports cancelling without a mutation', async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByText('Provider 1');
    await chooseTableSelection('Select all pages');
    await user.click(screen.getByRole('checkbox', { name: 'Select Provider 2' }));
    await chooseTableAction('Reset selected models');
    expect(await screen.findByText(/the 24 selected records/)).toBeInTheDocument();
    expect(post).toHaveBeenCalledWith('/api/channel/selection', {
      selection: { mode: 'all_matching', excluded_ids: [rows[1].uuid] },
      keyword: '',
    });
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(post).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('checkbox', { name: 'Select Provider 2' })).not.toBeChecked();
  });

  it('keeps confirmed target failures visible and does not treat partial reset as all success', async () => {
    post.mockImplementation(async (url) =>
      url === '/api/channel/selection'
        ? { data: { success: true, data: rows.slice(0, 2) } }
        : {
            data: {
              success: true,
              data: {
                results: [
                  { ...rows[0], success: true },
                  { ...rows[1], success: false, conflict: { code: 'mapping_conflict', field: 'model_mapping', models: ['private-alias'] } },
                ],
              },
            },
          }
    );
    renderPage();
    await screen.findByText('Provider 1');
    await chooseTableSelection('Select this page');
    await chooseTableAction('Reset selected models');
    await confirmBatch();
    const report = await screen.findByRole('region', { name: 'Selected action results' });
    expect(report).toHaveTextContent('2 selected: 1 succeeded, 0 skipped, 1 failed or rejected.');
    expect(report).toHaveTextContent('private-alias');
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('sends only selected IDs to disabled-channel deletion and disables duplicate submissions', async () => {
    let finish!: (value: unknown) => void;
    post.mockImplementation(async (url) =>
      url === '/api/channel/selection'
        ? { data: { success: true, data: [rows[0]] } }
        : new Promise((resolve) => {
            finish = resolve;
          })
    );
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await chooseTableAction('Delete selected disabled channels');
    await confirmBatch();
    const button = screen.getByRole('button', { name: 'Actions' });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(post).toHaveBeenCalledTimes(2);
    expect(post).toHaveBeenLastCalledWith(
      '/api/channel/delete_selected_disabled',
      {
        selection: { mode: 'ids', ids: [rows[0].uuid] },
      },
      { timeout: 120_000 }
    );
    finish({ data: { success: true, data: [{ ...rows[0], success: false, skipped: true }] } });
    await screen.findByText('1 selected: 0 succeeded, 1 skipped, 0 failed or rejected.');
    expect(api.delete).not.toHaveBeenCalled();
  });

  it('reports an already-removed selected channel as missing, not as enabled or failed', async () => {
    post.mockImplementation(async (url) =>
      url === '/api/channel/selection'
        ? { data: { success: true, data: [rows[0]] } }
        : { data: { success: true, data: [{ ...rows[0], success: false, skipped: true, message: 'This channel no longer exists.' }] } }
    );
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select Provider 1' }));
    await chooseTableAction('Delete selected disabled channels');
    await confirmBatch();
    const report = await screen.findByRole('region', { name: 'Selected action results' });
    expect(report).toHaveTextContent('1 selected: 0 succeeded, 1 skipped, 0 failed or rejected.');
    expect(report).toHaveTextContent('This channel no longer exists.');
    expect(report).not.toHaveTextContent('Failed to delete');
    expect(report).not.toHaveTextContent('enabled');
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
  });
});
