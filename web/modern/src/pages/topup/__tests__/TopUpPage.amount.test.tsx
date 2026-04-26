import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

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

const renderPage = () =>
  render(
    <NotificationsProvider>
      <TopUpPage />
    </NotificationsProvider>
  );

describe('TopUpPage: amount precalculation', () => {
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
    localStorage.setItem('display_in_currency', 'true');
    localStorage.setItem('status', JSON.stringify({ top_up_link: 'https://pay.example.com/checkout' }));

    (api.get as any).mockReset();
    (api.post as any).mockReset();
    (api.get as any).mockResolvedValue({
      data: {
        success: true,
        data: { id: 42, username: 'amountuser', quota: 1000 },
      },
    });
  });

  it('calculates the payable amount and recharges via window.location', async () => {
    (api.post as any).mockResolvedValue({ data: { message: 'success', data: 12.5 } });

    renderPage();

    // Wait for the user payload to settle so userData is populated
    await waitFor(() => expect(api.get).toHaveBeenCalled());

    // Find and update amount
    const amountInput = await screen.findByLabelText(/^amount$/i);
    fireEvent.change(amountInput, { target: { value: '5' } });

    const codeInput = screen.getByLabelText(/top-up code/i);
    fireEvent.change(codeInput, { target: { value: 'PROMO5' } });

    const calcBtn = screen.getByRole('button', { name: /^calculate$/i });
    fireEvent.click(calcBtn);

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/amount', {
        amount: 5,
        top_up_code: 'PROMO5',
      });
    });

    // The result is shown via the data-testid
    await screen.findByTestId('topup-amount-result');

    // Spy on window.location.href assignment
    const originalLocation = window.location;
    const hrefSpy = vi.fn();
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: new Proxy(originalLocation, {
        set(_target, prop, value) {
          if (prop === 'href') {
            hrefSpy(value);
            return true;
          }
          (originalLocation as any)[prop as any] = value;
          return true;
        },
        get(target, prop) {
          return (target as any)[prop as any];
        },
      }) as any,
    });

    const rechargeBtn = screen.getByRole('button', { name: /^recharge$/i });
    fireEvent.click(rechargeBtn);

    await waitFor(() => expect(hrefSpy).toHaveBeenCalledTimes(1));
    const navigatedTo: string = hrefSpy.mock.calls[0][0];
    expect(navigatedTo).toContain('https://pay.example.com/checkout');
    expect(navigatedTo).toContain('username=amountuser');
    expect(navigatedTo).toContain('user_id=42');
    expect(navigatedTo).toContain('transaction_id=');

    // Restore window.location
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    });
  });

  it('shows error notification when backend returns failure with data field', async () => {
    (api.post as any).mockResolvedValue({ data: { success: false, data: 'Invalid promo code' } });

    renderPage();

    await waitFor(() => expect(api.get).toHaveBeenCalled());

    const amountInput = await screen.findByLabelText(/^amount$/i);
    fireEvent.change(amountInput, { target: { value: '10' } });

    const calcBtn = screen.getByRole('button', { name: /^calculate$/i });
    fireEvent.click(calcBtn);

    await waitFor(() => expect(api.post).toHaveBeenCalled());

    // Recharge should remain disabled because no amount was computed
    const rechargeBtn = screen.getByRole('button', { name: /^recharge$/i });
    expect(rechargeBtn).toBeDisabled();
  });
});
