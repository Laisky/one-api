import type { TableSelectionSnapshot } from '@/hooks/useTableSelection';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import * as exports from '@/lib/export';
import { useAuthStore } from '@/lib/stores/auth';
import { LogsPage } from '../LogsPage';

const notify = vi.hoisted(() => vi.fn());
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
const rows = [1, 2, 3].map((i) => ({
  uuid: `018fcf6d-c484-7000-8000-${String(i).padStart(12, '0')}`,
  username: `user-${i}`,
  model_name: `model-${i}`,
  created_at: 1790000000,
  type: 2,
  quota: 1,
  content: 'usage',
  trace_id: `trace-${i}`,
}));
const get = vi.mocked(api.get),
  post = vi.mocked(api.post);

/** renderPage mounts real row selection and confirmation with only the remote API stubbed. */
function renderPage() {
  return render(
    <MemoryRouter>
      <LogsPage />
    </MemoryRouter>
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
  localStorage.clear();
  useAuthStore.setState({ user: { uuid: rows[0].uuid, username: 'admin', role: 10, status: 1 } as any, isAuthenticated: true });
  get.mockImplementation(async (url) =>
    url.startsWith('/api/trace/') ? { data: { success: true, data: {} } } : { data: { success: true, data: rows.slice(0, 2), total: 3 } }
  );
  post.mockImplementation(async (url, body) => {
    const selection = (body as { selection: TableSelectionSnapshot }).selection;
    return url === '/api/log/selection'
      ? {
          data: {
            success: true,
            data: rows.filter((row) =>
              selection.mode === 'ids' ? selection.ids.includes(row.uuid) : !selection.excluded_ids.includes(row.uuid)
            ),
          },
        }
      : { data: { success: true, data: { deleted: selection.mode === 'ids' ? selection.ids.length : 0 } } };
  });
});

describe('selected log actions', () => {
  it('exports only selected logs and fetches no traces for other records', async () => {
    const csv = vi.spyOn(exports, 'buildCsv');
    URL.createObjectURL = vi.fn(() => 'blob:selection');
    URL.revokeObjectURL = vi.fn();
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select user-1' }));
    await userEvent.click(screen.getByRole('button', { name: 'Export selected logs' }));
    await waitFor(() => expect(csv).toHaveBeenCalled());
    expect(csv.mock.calls[0][0]).toHaveLength(2); // Header plus one selected record.
    expect(JSON.stringify(csv.mock.calls[0][0])).toContain('model-1');
    expect(JSON.stringify(csv.mock.calls[0][0])).not.toContain('model-2');
    expect(get).toHaveBeenCalledWith(`/api/trace/log/${rows[0].uuid}`);
    expect(get).not.toHaveBeenCalledWith(`/api/trace/log/${rows[1].uuid}`);
    expect(post).toHaveBeenCalledWith('/api/log/selection', expect.objectContaining({ selection: { mode: 'ids', ids: [rows[0].uuid] } }), {
      timeout: 120_000,
    });
  });

  it('confirms a cross-page selection with exclusions and deletes only frozen UUIDs', async () => {
    renderPage();
    await screen.findByText('model-1');
    await userEvent.click(screen.getByRole('button', { name: 'Select all pages' }));
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select user-2' }));
    await userEvent.click(screen.getByRole('button', { name: 'Delete selected logs' }));
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('the 2 selected records');
    expect(post).toHaveBeenCalledTimes(1);
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/log/delete_selected',
        {
          selection: { mode: 'ids', ids: [rows[0].uuid, rows[2].uuid] },
        },
        { timeout: 120_000 }
      )
    );
    expect(api.delete).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith({ type: 'success', message: 'Deleted 2 selected logs.' });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Delete selected logs' })).toBeDisabled());
    expect(get.mock.calls.filter(([url]) => url.startsWith('/api/log/')).length).toBeGreaterThan(1);
  });

  it('does not delete when confirmation is canceled and shows mutation failures honestly', async () => {
    renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select user-1' }));
    await userEvent.click(screen.getByRole('button', { name: 'Delete selected logs' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(post).toHaveBeenCalledTimes(1);
    post.mockImplementation(async (url) =>
      url === '/api/log/selection'
        ? { data: { success: true, data: [rows[0]] } }
        : { data: { success: false, message: 'deletion rejected' } }
    );
    await userEvent.click(screen.getByRole('button', { name: 'Delete selected logs' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(notify).toHaveBeenCalledWith({ type: 'error', message: 'deletion rejected' }));
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('invalidates selection as soon as a filter changes, before Apply is pressed', async () => {
    const { container } = renderPage();
    await userEvent.click(await screen.findByRole('checkbox', { name: 'Select user-1' }));
    expect(screen.getByRole('button', { name: 'Export selected logs' })).toBeEnabled();
    fireEvent.change(container.querySelector('input[type="datetime-local"]')!, { target: { value: '2026-01-01T01:00' } });
    expect(screen.getByRole('button', { name: 'Export selected logs' })).toBeDisabled();
    expect(screen.getByRole('checkbox', { name: 'Select user-1' })).not.toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'Select user-1' })).toBeDisabled();
    expect(post).not.toHaveBeenCalled();
  });
});
