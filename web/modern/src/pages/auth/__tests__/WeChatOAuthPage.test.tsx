import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { vi } from 'vitest';
import { WeChatOAuthPage } from '../WeChatOAuthPage';

vi.mock('@/lib/stores/auth');
vi.mock('@/lib/api');

const mockLogin = vi.fn();
const mockUseAuthStore = vi.mocked(useAuthStore);
const mockApiGet = vi.mocked(api.get);

const renderWeChatOAuthPage = (search = '?code=abc&state=xyz') => {
  return render(
    <MemoryRouter initialEntries={[`/oauth/wechat${search}`]}>
      <Routes>
        <Route path="/oauth/wechat" element={<WeChatOAuthPage />} />
        <Route path="/" element={<div>home</div>} />
        <Route path="/settings" element={<div>settings</div>} />
        <Route path="/login" element={<div>login</div>} />
      </Routes>
    </MemoryRouter>
  );
};

describe('WeChatOAuthPage', () => {
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

    renderWeChatOAuthPage();

    await waitFor(() => {
      expect(screen.getByText('home')).toBeInTheDocument();
    });

    expect(mockApiGet).toHaveBeenCalledWith('/api/oauth/wechat?code=abc&state=xyz');
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

    renderWeChatOAuthPage();

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

    renderWeChatOAuthPage();

    // Wait for the first failure to be processed; the prompt should switch
    // away from the initial "processing" state to a retry/error message.
    await vi.waitFor(() => {
      expect(screen.queryByText(/Processing WeChat authentication/i)).not.toBeInTheDocument();
    });

    // After the initial attempt fails, the page is in retry mode. The mock
    // t() does not interpolate {{retry}}, so match the static prefix.
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
