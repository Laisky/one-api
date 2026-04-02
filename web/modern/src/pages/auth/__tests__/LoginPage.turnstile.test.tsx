import { api } from '@/lib/api';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterAll, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest';
import { LoginPage } from '../LoginPage.impl';

const mockLogin = vi.fn();

vi.mock('@/lib/stores/auth', () => ({
  useAuthStore: () => ({
    login: mockLogin,
  }),
}));

vi.mock('@/components/Turnstile', () => ({
  __esModule: true,
  default: ({ onVerify, className }: { onVerify?: (token: string) => void; className?: string }) => (
    <div data-testid="turnstile-mock" className={className} onClick={() => onVerify?.('mock-token')}>
      TurnstileMock
    </div>
  ),
}));

const originalLocalStorage = window.localStorage;
const storage: Record<string, string> = {};

const storageMock = {
  getItem: (key: string) => (key in storage ? storage[key] : null),
  setItem: (key: string, value: string) => {
    storage[key] = value;
  },
  removeItem: (key: string) => {
    delete storage[key];
  },
  clear: () => {
    for (const key of Object.keys(storage)) {
      delete storage[key];
    }
  },
};

describe('LoginPage Turnstile integration', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'localStorage', { value: storageMock, configurable: true });
  });

  afterAll(() => {
    Object.defineProperty(window, 'localStorage', { value: originalLocalStorage, configurable: true });
  });

  beforeEach(() => {
    storageMock.clear();
    mockLogin.mockReset();
    vi.restoreAllMocks();
  });

  test('does NOT render Turnstile widget on initial load even when enabled', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    // Wait for status to load, then verify no Turnstile widget
    await waitFor(() => expect(vi.spyOn(api, 'get')).toBeDefined());
    // Give it a tick to settle
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByTestId('turnstile-mock')).not.toBeInTheDocument();
  });

  test('shows Turnstile widget after a failed login returns turnstile_required', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: false,
        message: 'Invalid credentials',
        data: { turnstile_required: true },
      },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    // First login attempt — no Turnstile shown yet
    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'wrong');

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    // Button should NOT be disabled (no Turnstile gate yet)
    expect(submitButton).not.toBeDisabled();

    await user.click(submitButton);

    // After the failed response, Turnstile widget should now appear
    const widget = await screen.findByTestId('turnstile-mock');
    expect(widget).toBeInTheDocument();
  });

  test('blocks second login submission until Turnstile verification completes', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: false,
        message: 'Invalid credentials',
        data: { turnstile_required: true },
      },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'wrong');

    // First submit — triggers the failed response
    await user.click(screen.getByRole('button', { name: /sign in/i }));
    await screen.findByTestId('turnstile-mock');

    // Now the submit button should be disabled until Turnstile is completed
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /sign in/i })).toBeDisabled();
    });
  });

  test('submits login with Turnstile token after failed attempt and verification', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);
    const postSpy = vi
      .spyOn(api, 'post')
      .mockResolvedValueOnce({
        data: {
          success: false,
          message: 'Invalid credentials',
          data: { turnstile_required: true },
        },
      } as any)
      .mockResolvedValueOnce({
        data: { success: true, data: {} },
      } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'wrong');

    // First attempt fails
    await user.click(screen.getByRole('button', { name: /sign in/i }));
    const widget = await screen.findByTestId('turnstile-mock');

    // Complete Turnstile verification
    await user.click(widget);

    // Button should be re-enabled
    const submitButton = screen.getByRole('button', { name: /sign in/i });
    await waitFor(() => expect(submitButton).not.toBeDisabled());

    // Second attempt with Turnstile token
    await user.click(submitButton);

    await waitFor(() => expect(postSpy).toHaveBeenCalledTimes(2));
    const [secondPath] = postSpy.mock.calls[1];
    expect(secondPath).toBe('/api/user/login?turnstile=mock-token');
  });

  test('does not show Turnstile when turnstile_check is disabled', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: false, turnstile_site_key: '' },
      },
    } as any);
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: false,
        message: 'Invalid credentials',
        data: {},
      },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'wrong');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    // Even after failed login, no Turnstile because it's disabled
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByTestId('turnstile-mock')).not.toBeInTheDocument();
  });
});
