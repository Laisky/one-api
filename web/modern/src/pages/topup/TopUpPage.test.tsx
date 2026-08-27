import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { TopUpPage } from './TopUpPage';

/** renderWithProviders mounts UI under notifications and a Router (required for useSearchParams). */
const renderWithProviders = (ui: ReactElement) =>
  render(
    <MemoryRouter initialEntries={['/topup']}>
      <NotificationsProvider>{ui}</NotificationsProvider>
    </MemoryRouter>
  );

vi.mock('@/lib/api', () => {
  const get = vi.fn();
  const post = vi.fn();
  return {
    api: {
      get,
      post,
      defaults: { withCredentials: true },
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
    },
  };
});

describe('TopUpPage', () => {
  beforeEach(() => {
    // Reset store
    useAuthStore.setState({
      user: {
        id: 1,
        username: 'testuser',
        role: 1,
        status: 1,
        quota: 1000,
        used_quota: 0,
        group: 'default',
      } as any,
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: vi.fn() as any,
    });

    // Clear and set localStorage defaults used by the page
    localStorage.clear();
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('display_in_currency', 'true');

    // Mock system status with a payment link
    localStorage.setItem(
      'status',
      JSON.stringify({ top_up_link: 'https://pay.example.com', stripe_enabled: false, min_topup_usd: 5 })
    );

    // Reset API mocks
    (api.get as any).mockReset();
    (api.post as any).mockReset();
    (api.get as any).mockImplementation((url: string) => {
      if (typeof url === 'string' && url.includes('/api/user/self')) {
        return Promise.resolve({
          data: { success: true, data: { id: 1, username: 'testuser', quota: 1000 } },
        });
      }
      if (typeof url === 'string' && url.includes('/api/status')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { top_up_link: 'https://pay.example.com', stripe_enabled: false, min_topup_usd: 5 },
          },
        });
      }
      if (typeof url === 'string' && url.includes('/topup/stripe/orders')) {
        return Promise.resolve({ data: { success: true, data: [] } });
      }
      return Promise.resolve({ data: { success: true, data: {} } });
    });
    (api.post as any).mockResolvedValue({ data: { success: true, data: 500 } });
  });

  it('renders and redeems a code', async () => {
    renderWithProviders(<TopUpPage />);

    // Field should be present
    const input = await screen.findByPlaceholderText(/enter your redemption code/i);

    // Type code and submit
    fireEvent.change(input, { target: { value: 'ABC-123' } });
    const redeemBtn = screen.getByRole('button', { name: /redeem/i });
    fireEvent.click(redeemBtn);

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/topup', {
        key: 'ABC-123',
      });
    });

    // Success message should appear
    await screen.findByText(/successfully redeemed/i);
  });

  it('opens the payment portal even though /api/user/self returns no numeric id', async () => {
    // Regression: dto.UserResponse exposes `uuid` only (no internal integer id).
    // The handler used to call `userData.id.toString()`, which threw and was
    // swallowed by its own try/catch, so the button silently did nothing.
    (api.get as any).mockImplementation((url: string) => {
      if (typeof url === 'string' && url.includes('/api/user/self')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { uuid: 'd0c9e0f2-1111-4222-8333-444455556666', username: 'testuser', quota: 1000 },
          },
        });
      }
      if (typeof url === 'string' && url.includes('/api/status')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { top_up_link: 'https://pay.example.com', stripe_enabled: false, min_topup_usd: 5 },
          },
        });
      }
      if (typeof url === 'string' && url.includes('/topup/stripe/orders')) {
        return Promise.resolve({ data: { success: true, data: [] } });
      }
      return Promise.resolve({ data: { success: true, data: {} } });
    });

    const openSpy = vi.spyOn(window, 'open').mockReturnValue(null);
    renderWithProviders(<TopUpPage />);

    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/self'));
    const portalBtn = await screen.findByRole('button', { name: /open payment portal|open$/i });

    fireEvent.click(portalBtn);

    expect(openSpy).toHaveBeenCalledTimes(1);
    const opened = new URL((openSpy.mock.calls[0] as any[])[0] as string);
    expect(opened.origin + opened.pathname).toBe('https://pay.example.com/');
    expect(opened.searchParams.get('username')).toBe('testuser');
    expect(opened.searchParams.get('user_id')).toBe('d0c9e0f2-1111-4222-8333-444455556666');
    expect(opened.searchParams.get('transaction_id')).toBeTruthy();

    openSpy.mockRestore();
  });

  it('loads user quota on mount', async () => {
    renderWithProviders(<TopUpPage />);

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith('/api/user/self');
    });

    // Shows current balance text
    await screen.findByText(/current balance/i);
  });
});
