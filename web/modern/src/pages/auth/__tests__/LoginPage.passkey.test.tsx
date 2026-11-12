import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { startAuthentication } from '@simplewebauthn/browser';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { LoginPage } from '../LoginPage.impl';

vi.mock('@/lib/stores/auth');
vi.mock('@/lib/api');
vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: () => true,
  startAuthentication: vi.fn(),
}));

const mockLogin = vi.fn();
const mockApiGet = vi.mocked(api.get);
const mockApiPost = vi.mocked(api.post);
const mockStartAuthentication = vi.mocked(startAuthentication);

/**
 * webAuthnError builds the error shape @simplewebauthn/browser 14 throws: a WebAuthnError whose name is copied from
 * the browser DOMException and whose message is the browser's own text. It takes both values and returns the error.
 */
const webAuthnError = (name: string, message: string) => Object.assign(new Error(message), { name, code: 'ERROR_PASSTHROUGH_SEE_CAUSE_PROPERTY' });

/** renderLoginPage mounts the login page with a dashboard route so successful logins can be observed. */
const renderLoginPage = () =>
  render(
    <MemoryRouter initialEntries={['/login']}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/dashboard" element={<div>dashboard-page</div>} />
      </Routes>
    </MemoryRouter>
  );

describe('LoginPage passkey sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useAuthStore).mockReturnValue({ login: mockLogin } as any);
    mockApiGet.mockResolvedValue({ data: { success: true, data: { system_name: 'Test API' } } } as any);
    mockApiPost.mockImplementation(async (url: string) => {
      if (url === '/api/user/passkey/login/begin') {
        return { data: { success: true, data: { publicKey: { challenge: 'abc', rpId: 'localhost' } } } } as any;
      }
      if (url === '/api/user/passkey/login/finish') {
        return { data: { success: true, data: { id: 3, username: 'passkey-user' } } } as any;
      }
      throw new Error(`unexpected POST ${url}`);
    });
  });

  it('treats a dismissed passkey prompt (NotAllowedError with the browser message) as a silent cancel', async () => {
    const browserMessage = 'The operation either timed out or was not allowed. See: https://www.w3.org/TR/webauthn-2/#sctn-privacy-considerations-client.';
    mockStartAuthentication.mockRejectedValueOnce(webAuthnError('NotAllowedError', browserMessage));

    renderLoginPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Passkey' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Sign in with Passkey' })).not.toBeDisabled());
    expect(mockStartAuthentication).toHaveBeenCalledWith({ optionsJSON: { challenge: 'abc', rpId: 'localhost' } });
    expect(screen.queryByText(browserMessage)).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(mockApiPost).not.toHaveBeenCalledWith('/api/user/passkey/login/finish', expect.anything());
  });

  it('treats an aborted passkey ceremony as a silent cancel', async () => {
    mockStartAuthentication.mockRejectedValueOnce(webAuthnError('AbortError', 'Authentication ceremony was sent an abort signal'));

    renderLoginPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Passkey' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Sign in with Passkey' })).not.toBeDisabled());
    expect(screen.queryByText('Authentication ceremony was sent an abort signal')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('announces genuine passkey failures', async () => {
    mockStartAuthentication.mockRejectedValueOnce(webAuthnError('SecurityError', 'The RP ID "example.com" is invalid for this domain'));

    renderLoginPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Passkey' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('The RP ID "example.com" is invalid for this domain');
  });

  it('completes the login after a successful passkey assertion', async () => {
    mockStartAuthentication.mockResolvedValueOnce({ id: 'cred-id' } as any);

    renderLoginPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Sign in with Passkey' }));

    expect(await screen.findByText('dashboard-page')).toBeInTheDocument();
    expect(mockApiPost).toHaveBeenCalledWith('/api/user/passkey/login/finish', { id: 'cred-id' });
    expect(mockLogin).toHaveBeenCalledWith({ id: 3, username: 'passkey-user' }, '');
  });
});
