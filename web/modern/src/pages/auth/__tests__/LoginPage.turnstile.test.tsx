import { describe, expect, test, beforeEach, beforeAll, afterAll, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { act } from 'react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { LoginPage } from '../LoginPage.impl';
import { api } from '@/lib/api';

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

  test('fetches system status on first load and renders Turnstile widget', async () => {
    const getSpy = vi.spyOn(api, 'get').mockResolvedValue({
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

    await waitFor(() => expect(getSpy).toHaveBeenCalledWith('/api/status'));

    const widget = await screen.findByTestId('turnstile-mock');
    expect(widget).toBeInTheDocument();
  });

  test('blocks login submission until Turnstile verification completes', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: {} },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    await screen.findByTestId('turnstile-mock');

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'password');

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    expect(submitButton).toBeDisabled();

    await act(async () => {
      fireEvent.submit(screen.getByTestId('login-form'));
    });

    expect(postSpy).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(screen.getByText('Please complete the Turnstile verification')).toBeInTheDocument();
    });
  });

  test('clears Turnstile warning after verification succeeds', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    const widget = await screen.findByTestId('turnstile-mock');

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'password');

    await act(async () => {
      fireEvent.submit(screen.getByTestId('login-form'));
    });
    await waitFor(() => {
      expect(screen.getByText('Please complete the Turnstile verification')).toBeInTheDocument();
    });

    await user.click(widget);

    await waitFor(() => {
      expect(screen.queryByText('Please complete the Turnstile verification')).not.toBeInTheDocument();
    });
  });

  test('submits login with Turnstile token once verification passes', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: true, turnstile_site_key: 'site-key' },
      },
    } as any);
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: {} },
    } as any);

    const user = userEvent.setup();

    render(
      <MemoryRouter initialEntries={['/login']}>
        <LoginPage />
      </MemoryRouter>
    );

    const widget = await screen.findByTestId('turnstile-mock');

    await user.type(screen.getByLabelText(/username/i), 'demo');
    await user.type(screen.getByLabelText(/password/i), 'password');

    await user.click(widget);

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    await waitFor(() => expect(submitButton).not.toBeDisabled());

    await user.click(submitButton);

    await waitFor(() => expect(postSpy).toHaveBeenCalledTimes(1));
    const [requestedPath] = postSpy.mock.calls[0];
    expect(requestedPath).toBe('/api/user/login?turnstile=mock-token');
  });
});
