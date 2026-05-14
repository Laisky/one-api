import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { LogsPage } from '../LogsPage';

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

describe('LogsPage action feedback', () => {
  beforeEach(() => {
    notify.mockReset();
    useAuthStore.setState({
      user: {
        id: 1,
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
    (api.get as any).mockResolvedValue({ data: { success: true, data: [], total: 0 } });
  });

  it('shows an error when clear logs returns success false', async () => {
    (api.delete as any).mockResolvedValue({ data: { success: false, message: 'clear logs rejected' } });

    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    const clearButton = await screen.findByRole('button', { name: 'Clear' });
    await user.click(clearButton);

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error', message: 'clear logs rejected' })
      );
    });
  });
});
