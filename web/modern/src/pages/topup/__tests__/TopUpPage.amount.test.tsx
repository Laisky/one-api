import { render, screen, waitFor } from '@testing-library/react';
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

describe('TopUpPage: stripe status and router context', () => {
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
    localStorage.setItem(
      'status',
      JSON.stringify({ stripe_enabled: true, min_topup_usd: 5, top_up_link: '' })
    );

    (api.get as any).mockReset();
    (api.post as any).mockReset();
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

  it('renders inside a Router without useSearchParams crashes', async () => {
    renderPage();
    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/self'));
    expect(document.body.textContent || '').toMatch(/balance|token/i);
  });

  it('shows cancel outcome from query string', async () => {
    renderPage('/topup?stripe=cancel');
    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/user/self'));
    expect(document.body.textContent || '').toMatch(/cancel|not been charged/i);
  });

  it('polls order status after success return', async () => {
    renderPage('/topup?stripe=success&session_id=cs_test');
    await waitFor(() =>
      expect(api.get).toHaveBeenCalledWith(expect.stringContaining('/topup/stripe/orders/cs_test'))
    );
    expect(document.body.textContent || '').toMatch(/balance|token|credit/i);
  });
});
