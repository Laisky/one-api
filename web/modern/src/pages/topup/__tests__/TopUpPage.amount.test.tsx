import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { TopUpPage } from '../TopUpPage';

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

/** renderPage mounts TopUpPage inside Router + notifications for unit tests. */
const renderPage = (initialEntry = '/topup') =>
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NotificationsProvider>
        <TopUpPage />
      </NotificationsProvider>
    </MemoryRouter>
  );

describe('TopUpPage: Stripe checkout behavior', () => {
  beforeEach(() => {
    useAuthStore.setState({
      user: {
        id: 42,
        username: 'amountuser',
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

    localStorage.clear();
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('status', JSON.stringify({ stripe_enabled: true, min_topup_usd: 5, top_up_link: '' }));

    (api.get as any).mockReset();
    (api.post as any).mockReset();
    (api.post as any).mockResolvedValue({
      data: { success: false, message: 'test stops before redirect' },
    });
    (api.get as any).mockImplementation((url: string) => {
      if (url.includes('/api/user/self')) {
        return Promise.resolve({
          data: { success: true, data: { id: 42, username: 'amountuser', quota: 1000 } },
        });
      }
      if (url.includes('/api/status')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { stripe_enabled: true, min_topup_usd: 5, quota_per_unit: 500000 },
          },
        });
      }
      if (url.includes('/topup/stripe/orders/') && url.includes('cs_test')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { id: 1, status: 'paid', amount_cents: 500, currency: 'usd', session_id: 'cs_test' },
          },
        });
      }
      if (url.includes('/topup/stripe/orders')) {
        return Promise.resolve({ data: { success: true, data: [] } });
      }
      return Promise.resolve({ data: { success: true, data: {} } });
    });
  });

  it('submits the exact two-decimal USD amount', async () => {
    renderPage();

    const amount = await screen.findByLabelText(/amount \(usd\)/i);
    fireEvent.change(amount, { target: { value: '19.99' } });
    fireEvent.click(screen.getByRole('button', { name: /continue to stripe/i }));

    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/user/topup/stripe', { amount_usd: 19.99 }));
  });

  it('blocks amounts below the server-advertised minimum', async () => {
    renderPage();

    const amount = await screen.findByLabelText(/amount \(usd\)/i);
    fireEvent.change(amount, { target: { value: '4.99' } });
    fireEvent.click(screen.getByRole('button', { name: /continue to stripe/i }));

    await screen.findByText(/minimum is \$5/i);
    expect(api.post).not.toHaveBeenCalled();
  });

  it('shows the required message for an empty Stripe amount', async () => {
    renderPage();

    const amount = await screen.findByLabelText(/amount \(usd\)/i);
    fireEvent.change(amount, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /continue to stripe/i }));

    await screen.findByText(/enter an amount in usd/i);
    expect(screen.queryByText(/minimum is \$5/i)).not.toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('preserves the maximum validation for non-empty Stripe amounts', async () => {
    renderPage();

    const amount = await screen.findByLabelText(/amount \(usd\)/i);
    fireEvent.change(amount, { target: { value: '100000.01' } });
    fireEvent.click(screen.getByRole('button', { name: /continue to stripe/i }));

    await screen.findByText(/amount too large/i);
    expect(api.post).not.toHaveBeenCalled();
  });

  it('shows the cancel outcome from the query string', async () => {
    renderPage('/topup?stripe=cancel');
    await screen.findByText(/payment canceled/i);
    expect(screen.getByText(/you have not been charged/i)).toBeInTheDocument();
  });

  it('polls fulfillment and refreshes balance and history after paid status', async () => {
    renderPage('/topup?stripe=success&session_id=cs_test');

    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/topup/stripe/orders/cs_test'));
    await screen.findByText(/^credits added$/i);
    expect(screen.getByText(/your balance has been updated from the server order status/i)).toBeInTheDocument();

    await waitFor(() => {
      const calls = (api.get as any).mock.calls.map((args: unknown[]) => args[0]);
      expect(calls.filter((url: string) => url === '/api/user/self')).toHaveLength(2);
      expect(calls.filter((url: string) => url === '/api/user/topup/stripe/orders')).toHaveLength(2);
    });
  });
});
