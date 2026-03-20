import { render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter, MemoryRouter, Routes, Route } from 'react-router-dom';
import { vi } from 'vitest';
import { ProtectedRoute } from '../ProtectedRoute';
import { useAuthStore } from '@/lib/stores/auth';

// Mock the auth store
vi.mock('@/lib/stores/auth');

const mockUseAuthStore = useAuthStore as any;
const mockValidateSession = vi.fn();

// Mock Navigate component to track navigation calls
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    Navigate: ({ to }: { to: string }) => {
      mockNavigate(to);
      return <div data-testid="navigate-to">{to}</div>;
    },
  };
});

const renderProtectedRoute = (initialEntries = ['/dashboard']) => {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <Routes>
        <Route path="/" element={<ProtectedRoute />}>
          <Route index element={<div>Protected Content</div>} />
          <Route path="dashboard" element={<div>Protected Content</div>} />
          <Route path="channels/*" element={<div>Protected Content</div>} />
        </Route>
        <Route path="/login" element={<div>Login Page</div>} />
      </Routes>
    </MemoryRouter>
  );
};

describe('ProtectedRoute', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockNavigate.mockClear();
    mockValidateSession.mockClear();
  });

  it('shows loading spinner initially', () => {
    mockUseAuthStore.mockReturnValue({
      user: null,
      isValidating: false,
      validateSession: mockValidateSession,
    });

    renderProtectedRoute();

    // Should show loading spinner
    expect(document.querySelector('.animate-spin')).not.toBeNull();
  });

  it('redirects to login with redirect_to parameter when user is not authenticated', async () => {
    mockUseAuthStore.mockReturnValue({
      user: null,
      isValidating: false,
      validateSession: mockValidateSession,
    });

    renderProtectedRoute(['/dashboard?tab=tokens']);

    // Wait for initialization and navigation
    await waitFor(
      () => {
        expect(mockNavigate).toHaveBeenCalledWith('/login?redirect_to=%2Fdashboard%3Ftab%3Dtokens');
      },
      { timeout: 300 }
    );
  });

  it('validates session and renders outlet when user exists', async () => {
    const mockUser = {
      id: 1,
      username: 'testuser',
      role: 1,
      status: 1,
      quota: 1000,
      used_quota: 100,
      group: 'default',
    };

    mockValidateSession.mockResolvedValue(true);
    mockUseAuthStore.mockReturnValue({
      user: mockUser,
      isValidating: false,
      validateSession: mockValidateSession,
    });

    renderProtectedRoute();

    // Wait for validation and rendering
    await waitFor(
      () => {
        expect(mockValidateSession).toHaveBeenCalled();
        expect(screen.getByText('Protected Content')).toBeInTheDocument();
      },
      { timeout: 500 }
    );
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('redirects to login when session validation fails', async () => {
    const mockUser = {
      id: 1,
      username: 'testuser',
      role: 1,
      status: 1,
      quota: 1000,
      used_quota: 100,
      group: 'default',
    };

    mockValidateSession.mockResolvedValue(false);
    mockUseAuthStore.mockReturnValue({
      user: mockUser,
      isValidating: false,
      validateSession: mockValidateSession,
    });

    renderProtectedRoute(['/channels/edit/123?tab=config&mode=advanced']);

    await waitFor(
      () => {
        expect(mockValidateSession).toHaveBeenCalled();
      },
      { timeout: 500 }
    );
    await waitFor(
      () => {
        expect(mockNavigate).toHaveBeenCalledWith('/login?redirect_to=%2Fchannels%2Fedit%2F123%3Ftab%3Dconfig%26mode%3Dadvanced');
      },
      { timeout: 500 }
    );
  });
});
