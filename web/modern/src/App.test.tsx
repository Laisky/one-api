import { render, screen, waitFor } from '@testing-library/react';
import { Outlet } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import App from './App';

vi.mock('@/components/layout/Layout', () => ({
  Layout: () => <div><span data-testid="layout"><Outlet /></span></div>,
}));

vi.mock('@/components/theme-provider', () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@/components/ui/notifications', () => ({
  NotificationsProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn().mockResolvedValue({ data: { success: false } }) },
}));

vi.mock('@/lib/utils', () => ({
  persistSystemStatus: vi.fn(),
}));

vi.mock('@/lib/stores/auth', () => ({
  useAuthStore: () => ({ user: null }),
}));

vi.mock('@/pages/about/AboutPage', () => ({
  default: () => <div data-testid="about-page">About page</div>,
}));

vi.mock('@/pages/auth/LoginPage', () => ({
  default: () => <div data-testid="login-page">Login page</div>,
}));

vi.mock('@/components/dev/responsive-debugger', () => ({ ResponsiveDebugger: () => null }));
vi.mock('@/components/dev/responsive-validator', () => ({ ResponsiveValidator: () => null }));

describe('App public routes', () => {
  afterEach(() => {
    window.history.pushState({}, '', '/');
  });

  it('renders the About page without requiring authentication', async () => {
    window.history.pushState({}, '', '/about');

    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId('about-page')).toBeInTheDocument();
    });
    expect(window.location.pathname).toBe('/about');
    expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
  });
});
