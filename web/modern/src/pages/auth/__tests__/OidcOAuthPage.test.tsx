import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { vi } from 'vitest';
import { OidcOAuthPage } from '../OidcOAuthPage';

vi.mock('@/lib/stores/auth');
vi.mock('@/lib/api');

const mockLogin = vi.fn();
const mockUseAuthStore = vi.mocked(useAuthStore);
const mockApiGet = vi.mocked(api.get);

const renderOidcOAuthPage = (search = '?code=abc&state=xyz') => {
  return render(
    <MemoryRouter initialEntries={[`/oauth/oidc${search}`]}>
      <Routes>
        <Route path="/oauth/oidc" element={<OidcOAuthPage />} />
        <Route path="/" element={<div>home</div>} />
        <Route path="/settings" element={<div>settings</div>} />
        <Route path="/login" element={<div>login</div>} />
      </Routes>
    </MemoryRouter>
  );
};

describe('OidcOAuthPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockReset();
    mockUseAuthStore.mockReturnValue({
      login: mockLogin,
    } as any);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('navigates to / on successful OAuth login', async () => {
    mockApiGet.mockResolvedValueOnce({
      data: {
        success: true,
        message: '',
        data: { id: 1, username: 'testuser', role: 1 },
      },
    } as any);

    renderOidcOAuthPage();

    await waitFor(() => {
      expect(screen.getByText('home')).toBeInTheDocument();
    });

    expect(mockApiGet).toHaveBeenCalledWith('/api/oauth/oidc?code=abc&state=xyz');
    expect(mockLogin).toHaveBeenCalledWith({ id: 1, username: 'testuser', role: 1 }, '');
  });

  it('navigates to /settings when message is "bind"', async () => {
    mockApiGet.mockResolvedValueOnce({
      data: {
        success: true,
        message: 'bind',
        data: null,
      },
    } as any);

    renderOidcOAuthPage();

    await waitFor(() => {
      expect(screen.getByText('settings')).toBeInTheDocument();
    });

    expect(mockLogin).not.toHaveBeenCalled();
  });

  it('surfaces an error state when the OAuth call fails', async () => {
    vi.useFakeTimers();
    mockApiGet.mockResolvedValue({
      data: {
        success: false,
        message: 'oauth failed',
        data: null,
      },
    } as any);

    renderOidcOAuthPage();

    // Wait for the first failure to be processed; the prompt should switch
    // away from the initial "processing" state to a retry/error message.
    await vi.waitFor(() => {
      expect(screen.queryByText(/Processing OIDC authentication/i)).not.toBeInTheDocument();
    });

    // After the initial attempt fails, the page is in retry mode and shows
    // the retry prompt. The mock t() does not interpolate {{retry}}, so we
    // match the static prefix of the translated string. This proves an error
    // state has been surfaced before any /login navigation occurs.
    expect(screen.getByText(/Authentication error, retrying/i)).toBeInTheDocument();

    // Advance through all retry backoffs (3 retries: 2s, 4s, 6s) plus the
    // post-failure 2s navigate timeout, so the page eventually redirects to /login.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
      await vi.advanceTimersByTimeAsync(4000);
      await vi.advanceTimersByTimeAsync(6000);
      await vi.advanceTimersByTimeAsync(2000);
    });

    await vi.waitFor(() => {
      expect(screen.getByText('login')).toBeInTheDocument();
    });

    expect(mockLogin).not.toHaveBeenCalled();
  });
});
