import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';

import { Header } from './Header';
import { useAuthStore } from '@/lib/stores/auth';
import { api } from '@/lib/api';

type BreakpointState = {
  isMobile: boolean;
  isTablet: boolean;
  isDesktop: boolean;
  isLarge: boolean;
  currentBreakpoint: 'mobile' | 'tablet' | 'desktop' | 'large';
  width: number;
  height: number;
};

const mockUseResponsive = vi.fn();
let responsiveState: BreakpointState;

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => mockUseResponsive(),
}));

vi.mock('@/lib/api', () => {
  const get = vi.fn();
  return {
    api: {
      get,
      defaults: { withCredentials: true },
      interceptors: {
        request: { use: vi.fn() },
        response: { use: vi.fn() },
      },
    },
  };
});

const renderHeader = () =>
  render(
    <MemoryRouter>
      <Header />
    </MemoryRouter>
  );

describe('Header logout UX', () => {
  let logoutMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    mockUseResponsive.mockReset();
    responsiveState = {
      isMobile: false,
      isTablet: false,
      isDesktop: true,
      isLarge: false,
      currentBreakpoint: 'desktop',
      width: 1280,
      height: 800,
    };
    mockUseResponsive.mockImplementation(() => responsiveState);

    logoutMock = vi.fn();

    useAuthStore.setState({
      user: {
        id: 1,
        username: 'demo-user',
        role: 10,
      } as any,
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: logoutMock as any,
      updateUser: vi.fn() as any,
    });

    localStorage.clear();
    localStorage.setItem('system_name', 'OneAPI Test');
    (api.get as any).mockReset();
    (api.get as any).mockResolvedValue({ data: { success: true } });
  });

  it('hides the logout action by default', () => {
    renderHeader();

    expect(screen.queryByRole('button', { name: /logout/i })).toBeNull();
  });

  it('confirms logout through the desktop hamburger menu', async () => {
    const user = userEvent.setup();
    renderHeader();

    const accountMenuButton = screen.getByLabelText(/open account menu/i);
    await user.click(accountMenuButton);

    const logoutMenuItem = await screen.findByText('Logout');
    await user.click(logoutMenuItem);

    await screen.findByText(/confirm logout/i);

    const confirmButton = screen.getByRole('button', { name: /log out/i });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith('/api/user/logout');
    });
    expect(logoutMock).toHaveBeenCalled();
  });

  it('offers logout inside the mobile navigation drawer', async () => {
    responsiveState = {
      isMobile: true,
      isTablet: false,
      isDesktop: false,
      isLarge: false,
      currentBreakpoint: 'mobile',
      width: 375,
      height: 812,
    };

    const user = userEvent.setup();
    renderHeader();

    expect(screen.queryByRole('button', { name: /logout/i })).toBeNull();

    const mobileMenuButton = screen.getByLabelText(/open navigation menu/i);
    await user.click(mobileMenuButton);

    const drawerLogoutButton = await screen.findByRole('button', { name: /logout/i });
    await user.click(drawerLogoutButton);

    await screen.findByText(/confirm logout/i);

    const confirmButton = screen.getByRole('button', { name: /log out/i });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith('/api/user/logout');
    });
    expect(logoutMock).toHaveBeenCalled();
  });
});
