import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { api } from '@/lib/api';
import { TokensPage } from '../TokensPage.impl';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
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

describe('TokensPage action feedback', () => {
  beforeEach(() => {
    notify.mockReset();
    localStorage.clear();
    (api.get as any).mockReset();
    (api.put as any).mockReset();
    (api.delete as any).mockReset();
    (api.get as any).mockResolvedValue({
      data: {
        success: true,
        data: [
          {
            id: 7,
            name: 'Token A',
            key: 'abcdef1234567890',
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

  const renderPage = () =>
    render(
      <MemoryRouter>
        <TokensPage />
      </MemoryRouter>
    );

  it('shows an error when delete returns success false', async () => {
    (api.delete as any).mockResolvedValue({ data: { success: false, message: 'cannot delete token' } });
    const user = userEvent.setup();

    renderPage();
    await screen.findByText('Token A');
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'cannot delete token' }));
    });
  });

  it('shows an error when status update returns success false', async () => {
    (api.put as any).mockResolvedValue({ data: { success: false, message: 'cannot disable token' } });
    const user = userEvent.setup();

    renderPage();
    await screen.findByText('Token A');
    await user.click(screen.getByRole('button', { name: 'Disable' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'cannot disable token' }));
    });
  });
});
