import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>(), notify: vi.fn() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: mocks.notify }),
}));

vi.mock('@/components/ui/confirm-dialog', () => ({
  useConfirmDialog: () => [vi.fn().mockResolvedValue(true), () => null],
}));

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), delete: vi.fn(), post: vi.fn() },
}));

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}));

import { api } from '@/lib/api';
import { TokensPage } from '../TokensPage.impl';

const TOKEN_KEY = 'abcdef1234567890';

/** renderPage mounts the tokens list with one token row. */
async function renderPage() {
  render(
    <MemoryRouter>
      <TokensPage />
    </MemoryRouter>
  );
  await screen.findByText('Token A');
}

describe('TokensPage key copy', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
    mocks.notify.mockReset();
    localStorage.clear();
    (api.get as any).mockReset();
    (api.get as any).mockResolvedValue({
      data: {
        success: true,
        data: [
          {
            id: 7,
            name: 'Token A',
            key: TOKEN_KEY,
            status: 1,
            remain_quota: 100,
            unlimited_quota: false,
            used_quota: 0,
            created_time: 0,
            accessed_time: 0,
            expired_time: -1,
          },
        ],
        total: 1,
      },
    });
  });

  it('copies the key through the shared clipboard helper and shows success', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const user = userEvent.setup();
    await renderPage();

    await user.click(screen.getByRole('button', { name: 'Copy token' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied!' })).toBeInTheDocument());
    expect(mocks.copy).toHaveBeenCalledWith(expect.stringContaining(TOKEN_KEY.slice(-8)));
    expect(screen.queryByText('Manual copy required')).not.toBeInTheDocument();
  });

  it('opens the manual copy fallback instead of reporting success when the copy is rejected', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const user = userEvent.setup();
    await renderPage();

    await user.click(screen.getByRole('button', { name: 'Copy token' }));

    expect(await screen.findByText('Manual copy required')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Copied!' })).not.toBeInTheDocument();
  });
});
