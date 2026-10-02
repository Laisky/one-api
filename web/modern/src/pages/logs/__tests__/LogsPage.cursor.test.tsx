import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { LogsPage } from '../LogsPage';

/**
 * End-to-end wiring of the keyset log routes into the modern log page
 * (proposal W2.4, "a versioned cursor capability used explicitly by Modern").
 *
 * These tests exercise the page, not the hook: what matters here is that the
 * page asks the cursor route, renders the keyset pager instead of the
 * page-number pager, and silently returns to the legacy route on a server that
 * does not offer the capability.
 */

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/components/ui/confirm-dialog', () => ({
  useConfirmDialog: () => [vi.fn().mockResolvedValue(true), () => null],
}));

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), delete: vi.fn() },
}));

/**
 * logRow builds a minimal log row for the table.
 *
 * @param id - the row id.
 * @param createdAt - the Unix second of the row.
 * @returns the row.
 */
function logRow(id: number, createdAt: number) {
  return {
    id,
    uuid: `018f0000-0000-7000-8000-0000000${String(id).padStart(5, '0')}`,
    user_id: 1,
    created_at: createdAt,
    type: 2,
    username: 'admin',
    token_name: 'prod',
    model_name: 'gpt-4.1',
    content: `row-${id}`,
    quota: 10,
    prompt_tokens: 1,
    completion_tokens: 1,
    channel: 1,
    elapsed_time: 100,
  };
}

/**
 * cursorPage builds a cursor route response body.
 *
 * @param ids - the row ids on the page.
 * @param nextCursor - the following page's cursor, empty when last.
 * @param count - the count object.
 * @returns the response envelope.
 */
function cursorPage(ids: number[], nextCursor: string, count: Record<string, unknown>) {
  return {
    data: {
      success: true,
      version: 1,
      data: ids.map((id) => logRow(id, 1767225540 - id)),
      has_more: Boolean(nextCursor),
      next_cursor: nextCursor,
      count,
    },
  };
}

describe('LogsPage keyset pagination', () => {
  beforeEach(() => {
    notify.mockReset();
    useAuthStore.setState({
      user: {
        id: 1,
        uuid: '018f0000-0000-7000-8000-000000000101',
        username: 'admin',
        role: 10,
        status: 1,
        quota: 0,
        used_quota: 0,
        group: 'default',
      } as any,
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: vi.fn() as any,
    });
    (api.get as any).mockReset();
    (api.delete as any).mockReset();
  });

  it('lists through the cursor route and shows the keyset pager', async () => {
    const urls: string[] = [];
    (api.get as any).mockImplementation((url: string) => {
      urls.push(url);
      if (url.startsWith('/api/log/cursor')) {
        if (url.includes('cursor=lc1.p2')) {
          return Promise.resolve(cursorPage([3, 4], '', { value: 4, quality: 'exact', as_of: 1767225540, cached: false }));
        }
        return Promise.resolve(cursorPage([1, 2], 'lc1.p2', { value: 4, quality: 'exact', as_of: 1767225540, cached: false }));
      }
      return Promise.resolve({ data: { success: true, data: [], total: 0 } });
    });

    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    // The admin route is used, the capability version is negotiated, and no
    // page number is ever sent.
    await waitFor(() => expect(urls.some((url) => url.startsWith('/api/log/cursor?'))).toBe(true));
    expect(urls[0]).toContain('v=1');
    expect(urls[0]).not.toMatch(/[?&]p=/);
    expect(urls.some((url) => url.startsWith('/api/log/?'))).toBe(false);

    const next = await screen.findByRole('button', { name: /next/i });
    expect(await screen.findByText(/showing 1–2 of 4/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled();

    await userEvent.click(next);

    await waitFor(() => expect(urls.some((url) => url.includes('cursor=lc1.p2'))).toBe(true));
    expect(await screen.findByText(/showing 3–4 of 4/i)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: /next/i })).toBeDisabled());
    expect(screen.getByRole('button', { name: /previous/i })).toBeEnabled();
  });

  it('states a bounded count as a lower bound rather than as a total', async () => {
    (api.get as any).mockImplementation((url: string) => {
      if (url.startsWith('/api/log/cursor')) {
        return Promise.resolve(cursorPage([1, 2], 'lc1.p2', { value: 10000, quality: 'lower_bound', as_of: 1767225540, cached: true }));
      }
      return Promise.resolve({ data: { success: true, data: [], total: 0 } });
    });

    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    expect(await screen.findByText(/of at least 10000/i)).toBeInTheDocument();
    // A reused count is labelled, so it is not read as a live figure.
    expect(screen.getByText(/reused from a recent identical query/i)).toBeInTheDocument();
  });

  it('falls back to the legacy route when the server has no cursor capability', async () => {
    const urls: string[] = [];
    (api.get as any).mockImplementation((url: string) => {
      urls.push(url);
      if (url.startsWith('/api/log/cursor')) {
        return Promise.resolve({
          data: { success: false, message: 'log cursor pagination is disabled', code: 'capability_disabled' },
        });
      }
      return Promise.resolve({
        data: { success: true, data: [{ ...logRow(9, 1767225540), token_name: 'served-by-legacy' }], total: 1 },
      });
    });

    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    // The legacy request follows the refused cursor request, and the rows show
    // up: a disabled capability must never look like an empty log list.
    await waitFor(() => expect(urls.some((url) => url.startsWith('/api/log/?'))).toBe(true));
    expect(await screen.findByText('served-by-legacy')).toBeInTheDocument();

    // The page-number pager is back, and the keyset one is gone.
    expect(screen.queryByRole('button', { name: /^next$/i })).not.toBeInTheDocument();
  });

  it('tells the user when a page was cut short by the response budget', async () => {
    (api.get as any).mockImplementation((url: string) => {
      if (url.startsWith('/api/log/cursor')) {
        return Promise.resolve({
          data: {
            success: true,
            version: 1,
            data: [logRow(1, 1767225540)],
            has_more: true,
            next_cursor: 'lc1.p2',
            count: { value: null, quality: 'unavailable', as_of: 1767225540, cached: false },
            bytes_capped: true,
          },
        });
      }
      return Promise.resolve({ data: { success: true, data: [], total: 0 } });
    });

    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    expect(await screen.findByText(/shortened to stay within the response size limit/i)).toBeInTheDocument();
    // An unavailable count is stated as unavailable, never rendered as zero.
    expect(await screen.findByText(/showing 1–1; the total is not available/i)).toBeInTheDocument();
    expect(screen.queryByText(/of 0$/)).not.toBeInTheDocument();
  });
});
