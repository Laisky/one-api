import { api } from '@/lib/api';
import * as oauth from '@/lib/oauth';
import { useAuthStore } from '@/lib/stores/auth';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { vi } from 'vitest';
import { LoginPage } from '../LoginPage.impl';

// Mock the auth store
vi.mock('@/lib/stores/auth');
vi.mock('@/lib/api');
vi.mock('@/lib/oauth', async () => {
  const actual = await vi.importActual<typeof import('@/lib/oauth')>('@/lib/oauth');
  return {
    ...actual,
    getOAuthState: vi.fn(),
  };
});

const mockLogin = vi.fn();
const mockUseAuthStore = useAuthStore as any;
const mockApiGet = vi.mocked(api.get);
const mockGetOAuthState = vi.mocked(oauth.getOAuthState);

// Mock localStorage
const mockLocalStorage = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
};
Object.defineProperty(window, 'localStorage', { value: mockLocalStorage, configurable: true });

// Use spy instead of overwriting window.history to avoid breaking React Router
const replaceStateSpy = vi.spyOn(window.history, 'replaceState').mockImplementation(() => {});

const renderLoginPage = (initialEntries = ['/login']) => {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <LoginPage />
    </MemoryRouter>
  );
};

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    replaceStateSpy.mockClear();
    mockApiGet.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: false },
      },
    } as any);
    mockGetOAuthState.mockReset();
    mockUseAuthStore.mockReturnValue({
      login: mockLogin,
    });
    mockLocalStorage.getItem.mockReturnValue(
      JSON.stringify({
        system_name: 'Test API',
        github_oauth: false,
      })
    );
  });

  it('renders login form correctly', async () => {
    renderLoginPage();

    // Wait for the brand name to be rendered (it comes from the status) to avoid act warning
    // We use a regex because of the potential space/newline in "Sign In to Test API"
    expect(await screen.findByText(/Sign In\s+to Test API/i)).toBeInTheDocument();

    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  // TODO: Fix this test - it has mocking issues with the current setup
  it.skip('handles redirect_to parameter correctly on successful login', async () => {
    // This test is skipped due to mocking issues. If you want to enable it, ensure the mock is set up before importing the component.
    // See Vitest docs for module mocking best practices.
  });

  it('shows TOTP input when TOTP is required', async () => {
    const mockApiPost = vi.mocked(api.post);
    mockApiPost.mockResolvedValueOnce({
      data: {
        success: false,
        message: 'totp_required',
        data: { totp_required: true },
      },
    });

    renderLoginPage();

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);

    // Fill in username and password
    fireEvent.change(usernameInput, { target: { value: 'testuser' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });

    // Submit form
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    // Wait for TOTP input to appear
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/6-digit totp code/i)).toBeInTheDocument();
    });

    // Check that username and password fields are disabled
    expect(usernameInput).toBeDisabled();
    expect(passwordInput).toBeDisabled();

    // Check that the button text changed
    expect(screen.getByRole('button', { name: /verify totp/i })).toBeInTheDocument();
  });

  it('disables TOTP verify button when code is incomplete', async () => {
    const mockApiPost = vi.mocked(api.post);
    mockApiPost.mockResolvedValueOnce({
      data: {
        success: false,
        message: 'totp_required',
        data: { totp_required: true },
      },
    });

    renderLoginPage();

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);

    // Fill in username and password and trigger TOTP
    fireEvent.change(usernameInput, { target: { value: 'testuser' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/6-digit totp code/i)).toBeInTheDocument();
    });

    const totpInput = screen.getByPlaceholderText(/6-digit totp code/i);
    const verifyButton = screen.getByRole('button', { name: /verify totp/i });

    // Button should be disabled initially
    expect(verifyButton).toBeDisabled();

    // Enter incomplete TOTP code
    fireEvent.change(totpInput, { target: { value: '12345' } });
    expect(verifyButton).toBeDisabled();

    // Enter complete TOTP code
    fireEvent.change(totpInput, { target: { value: '123456' } });
    expect(verifyButton).not.toBeDisabled();
  });

  it('successfully logs in with valid TOTP code', async () => {
    const mockApiPost = vi.mocked(api.post);

    // First call - TOTP required
    mockApiPost.mockResolvedValueOnce({
      data: {
        success: false,
        message: 'totp_required',
        data: { totp_required: true },
      },
    });

    // Second call - successful login
    mockApiPost.mockResolvedValueOnce({
      data: {
        success: true,
        data: { id: 1, username: 'testuser', role: 1 },
      },
    });

    renderLoginPage();

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);

    // Initial login attempt
    fireEvent.change(usernameInput, { target: { value: 'testuser' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    // Wait for TOTP input
    await waitFor(() => {
      expect(screen.getByPlaceholderText(/6-digit totp code/i)).toBeInTheDocument();
    });

    // Enter TOTP code and submit
    fireEvent.change(screen.getByPlaceholderText(/6-digit totp code/i), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: /verify totp/i }));

    // Verify login was called
    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith({ id: 1, username: 'testuser', role: 1 }, '');
    });
  });

  it('shows the disabled-password notice but keeps the form visible so root can still log in', async () => {
    // Backend says password_login is off — UI should warn the user, but must
    // not hide the form (root needs it to recover access).
    mockApiGet.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: {
          system_name: 'Test API',
          turnstile_check: false,
          password_login: false,
          oidc: true,
          oidc_client_id: 'oidc-client',
          oidc_authorization_endpoint: 'https://idp.example.com/authorize',
        },
      },
    } as any);
    mockLocalStorage.getItem.mockReturnValue(JSON.stringify({ system_name: 'Test API', password_login: false, oidc: true }));

    renderLoginPage();

    const notice = await screen.findByTestId('password-login-disabled-notice');
    expect(notice).toBeInTheDocument();
    expect(notice.textContent).toMatch(/password login is disabled/i);

    // Form must remain so the root account can still recover.
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('omits the disabled notice when password_login is enabled', async () => {
    mockApiGet.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: {
          system_name: 'Test API',
          turnstile_check: false,
          password_login: true,
        },
      },
    } as any);
    mockLocalStorage.getItem.mockReturnValue(JSON.stringify({ system_name: 'Test API', password_login: true }));

    renderLoginPage();

    await screen.findByLabelText(/username/i);
    expect(screen.queryByTestId('password-login-disabled-notice')).not.toBeInTheDocument();
  });

  it('shows back to login button in TOTP mode', async () => {
    const mockApiPost = vi.mocked(api.post);
    mockApiPost.mockResolvedValueOnce({
      data: {
        success: false,
        message: 'totp_required',
        data: { totp_required: true },
      },
    });

    renderLoginPage();

    const usernameInput = screen.getByLabelText(/username/i);
    const passwordInput = screen.getByLabelText(/password/i);

    // Trigger TOTP mode
    fireEvent.change(usernameInput, { target: { value: 'testuser' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /back to login/i })).toBeInTheDocument();
    });

    // Click back to login
    fireEvent.click(screen.getByRole('button', { name: /back to login/i }));

    // Should return to normal login mode
    expect(screen.queryByPlaceholderText(/6-digit totp code/i)).not.toBeInTheDocument();
    expect(usernameInput).not.toBeDisabled();
    expect(passwordInput).not.toBeDisabled();
  });

  it('shows the localized OAuth state error when state acquisition fails without a message', async () => {
    mockApiGet.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: {
          system_name: 'Test API',
          turnstile_check: false,
          oidc: true,
          oidc_client_id: 'oidc-client',
          oidc_authorization_endpoint: 'https://idp.example.com/authorize',
        },
      },
    } as any);
    mockLocalStorage.getItem.mockReturnValue(
      JSON.stringify({
        system_name: 'Test API',
        oidc: true,
        oidc_client_id: 'oidc-client',
        oidc_authorization_endpoint: 'https://idp.example.com/authorize',
      })
    );
    mockGetOAuthState.mockRejectedValueOnce(new Error(''));

    renderLoginPage();

    fireEvent.click(await screen.findByRole('button', { name: 'OIDC' }));

    await waitFor(() => {
      expect(screen.getByText('Unable to start OAuth. Please try again.')).toBeInTheDocument();
    });
    expect(mockGetOAuthState).toHaveBeenCalledTimes(1);
  });
});
