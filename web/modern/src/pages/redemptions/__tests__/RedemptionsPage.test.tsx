import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BrowserRouter } from 'react-router-dom';

import { api } from '@/lib/api';
import { RedemptionsPage } from '../RedemptionsPage';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    delete: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
  },
}));

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}));

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<any>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RedemptionsPage action feedback', () => {
  beforeEach(() => {
    notify.mockReset();
    mockNavigate.mockReset();
    localStorage.clear();
    (api.get as any).mockReset();
    (api.delete as any).mockReset();
    (api.put as any).mockReset();
    (api.get as any).mockResolvedValue({
      data: {
        success: true,
        data: [{ id: 1, name: 'Redeem A', key: 'code-a', status: 1, created_time: 1, quota: 100 }],
        total: 1,
      },
    });
  });

  const renderPage = () =>
    render(
      <BrowserRouter>
        <RedemptionsPage />
      </BrowserRouter>
    );

  it('shows an error when delete returns success false', async () => {
    (api.delete as any).mockResolvedValue({ data: { success: false, message: 'cannot delete redemption' } });

    renderPage();
    const user = userEvent.setup();

    await screen.findByText('Redeem A');
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'cannot delete redemption' }));
    });
  });

  it('shows an error when status update returns success false', async () => {
    (api.put as any).mockResolvedValue({ data: { success: false, message: 'cannot disable redemption' } });

    renderPage();
    const user = userEvent.setup();

    await screen.findByText('Redeem A');
    await user.click(screen.getByRole('button', { name: 'Disable' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'cannot disable redemption' }));
    });
  });
});
