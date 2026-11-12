import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { copyToClipboard } from '@/lib/utils';
import { startRegistration } from '@simplewebauthn/browser';
import QRCode from 'qrcode';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PersonalSettings } from './PersonalSettings';

const notify = vi.fn();

vi.mock('@/lib/api', () => {
  const get = vi.fn();
  const put = vi.fn();
  const post = vi.fn();
  return {
    api: {
      get,
      put,
      post,
      defaults: { withCredentials: true },
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
    },
  };
});

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return {
    ...actual,
    loadSystemStatus: vi.fn().mockResolvedValue({}),
    copyToClipboard: vi.fn().mockResolvedValue(undefined),
  };
});

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: () => true,
  startRegistration: vi.fn(),
}));

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false }),
}));

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/components/Turnstile', () => ({
  default: () => null,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('qrcode', () => ({
  default: {
    toDataURL: vi.fn().mockResolvedValue('data:image/png;base64,qr'),
  },
}));

describe('PersonalSettings', () => {
  let currentProfile: {
    username: string;
    display_name: string;
    email: string;
  };

  beforeEach(() => {
    notify.mockReset();
    vi.mocked(copyToClipboard).mockReset();
    vi.mocked(copyToClipboard).mockResolvedValue(undefined);
    vi.mocked(startRegistration).mockReset();
    localStorage.clear();

    currentProfile = {
      username: 'testuser',
      display_name: 'Stored Name',
      email: 'fresh@example.com',
    };

    useAuthStore.setState({
      user: {
        id: 1,
        username: 'testuser',
        display_name: 'Stored Name',
        email: '',
        role: 1,
        status: 1,
        quota: 0,
        used_quota: 0,
        group: 'default',
      },
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: useAuthStore.getState().updateUser,
    });

    (api.get as any).mockReset();
    (api.put as any).mockReset();
    (api.post as any).mockReset();
    (api.post as any).mockImplementation((url: string) => api.get(url));

    (api.get as any).mockImplementation((url: string) => {
      if (url === '/api/user/self') {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              ...currentProfile,
            },
          },
        });
      }

      if (url === '/api/user/totp/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { totp_enabled: false },
          },
        });
      }

      if (url.startsWith('/api/verification?email=')) {
        return Promise.resolve({
          data: {
            success: true,
            message: 'verification-sent',
          },
        });
      }

      if (url.startsWith('/api/oauth/email/bind?')) {
        currentProfile = {
          ...currentProfile,
          email: 'new@example.com',
        };
        return Promise.resolve({
          data: {
            success: true,
          },
        });
      }

      return Promise.resolve({ data: { success: true } });
    });

    (api.put as any).mockImplementation((_url: string, payload: Record<string, string>) => {
      currentProfile = {
        ...currentProfile,
        display_name: payload.display_name || currentProfile.display_name,
      };
      return Promise.resolve({ data: { success: true, message: '' } });
    });
  });

  it('hydrates the email from self and shows success after profile update', async () => {
    render(<PersonalSettings />);

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith('/api/user/self');
    });

    const emailInput = (await screen.findByPlaceholderText('personal_settings.profile_info.email_placeholder')) as HTMLInputElement;
    expect(emailInput.value).toBe('fresh@example.com');

    const displayNameInput = screen.getByPlaceholderText('personal_settings.profile_info.display_name_placeholder');
    fireEvent.change(displayNameInput, { target: { value: 'Updated Name' } });

    fireEvent.click(screen.getByRole('button', { name: 'personal_settings.profile_info.update_button' }));

    await waitFor(() => {
      expect(api.put).toHaveBeenCalledWith('/api/user/self', {
        username: 'testuser',
        display_name: 'Updated Name',
      });
    });

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'success',
          message: 'personal_settings.profile_info.success',
        })
      );
    });

    expect(useAuthStore.getState().user?.display_name).toBe('Updated Name');
    expect(useAuthStore.getState().user?.email).toBe('fresh@example.com');
  });

  it('sends a verification code and binds the new email address', async () => {
    render(<PersonalSettings />);

    const emailInput = (await screen.findByPlaceholderText('personal_settings.profile_info.email_placeholder')) as HTMLInputElement;
    fireEvent.change(emailInput, { target: { value: 'new@example.com' } });

    fireEvent.click(screen.getByRole('button', { name: 'personal_settings.profile_info.send_code' }));

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith('/api/verification?email=new%40example.com');
    });

    const codeInput = screen.getByPlaceholderText('personal_settings.profile_info.email_verification_code_placeholder');
    fireEvent.change(codeInput, { target: { value: '123456' } });

    fireEvent.click(screen.getByRole('button', { name: 'personal_settings.profile_info.bind_email' }));

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/oauth/email/bind?email=new%40example.com&code=123456');
    });

    await waitFor(() => {
      expect((screen.getByPlaceholderText('personal_settings.profile_info.email_placeholder') as HTMLInputElement).value).toBe(
        'new@example.com'
      );
    });

    expect(useAuthStore.getState().user?.email).toBe('new@example.com');
  });

  it('shows an error notification when generate access token returns success false', async () => {
    // /api/user/token is a state-changing POST (CSRF hardening); serve it only from the
    // POST mock so a regression back to GET cannot reach the rejection response.
    (api.post as any).mockImplementation((url: string) => {
      if (url === '/api/user/token') {
        return Promise.resolve({ data: { success: false, message: 'token rejected' } });
      }
      return Promise.resolve({ data: { success: true } });
    });

    render(<PersonalSettings />);

    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.access_token.generate_token' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'token rejected',
        })
      );
    });

    expect(api.post).toHaveBeenCalledWith('/api/user/token');
    expect(api.get).not.toHaveBeenCalledWith('/api/user/token');
  });

  it('shows an error notification when getting the invite link returns success false', async () => {
    // /api/user/aff is a state-changing POST (CSRF hardening); serve it only from the
    // POST mock so a regression back to GET cannot reach the rejection response.
    (api.post as any).mockImplementation((url: string) => {
      if (url === '/api/user/aff') {
        return Promise.resolve({ data: { success: false, message: 'invite rejected' } });
      }
      return Promise.resolve({ data: { success: true } });
    });

    render(<PersonalSettings />);

    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.access_token.get_invite_link' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'invite rejected',
        })
      );
    });

    expect(api.post).toHaveBeenCalledWith('/api/user/aff');
    expect(api.get).not.toHaveBeenCalledWith('/api/user/aff');
  });

  it('copies a generated access token without reporting an error', async () => {
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(url === '/api/user/token' ? { data: { success: true, data: 'generated-token' } } : { data: { success: true } })
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.access_token.generate_token' }));

    expect(await screen.findByText('generated-token')).toBeInTheDocument();
    expect(copyToClipboard).toHaveBeenCalledWith('generated-token');
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
  });

  it('reports a clipboard failure for a generated access token instead of a generation failure', async () => {
    vi.mocked(copyToClipboard).mockRejectedValueOnce(new Error('Copy to clipboard failed'));
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(url === '/api/user/token' ? { data: { success: true, data: 'generated-token' } } : { data: { success: true } })
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.access_token.generate_token' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'personal_settings.access_token.copy_failed' }));
    });
    // The token stays visible so it can be copied manually.
    expect(screen.getByText('generated-token')).toBeInTheDocument();
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ message: 'personal_settings.access_token.generate_failed' }));
  });

  it('reports a clipboard failure for the invitation link', async () => {
    vi.mocked(copyToClipboard).mockRejectedValueOnce(new Error('Copy to clipboard failed'));
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(url === '/api/user/aff' ? { data: { success: true, data: 'AFF1' } } : { data: { success: true } })
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.access_token.get_invite_link' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'personal_settings.access_token.copy_failed' }));
    });
    expect(copyToClipboard).toHaveBeenCalledWith(`${window.location.origin}/register?aff=AFF1`);
  });

  it('labels the email and verification-code inputs with identifier autofill hints', async () => {
    render(<PersonalSettings />);

    const email = await screen.findByLabelText('personal_settings.profile_info.email');
    expect(email).toHaveAttribute('type', 'email');
    expect(email).toHaveAttribute('autocomplete', 'email');
    expect(email).toHaveAttribute('spellcheck', 'false');

    const code = screen.getByLabelText('personal_settings.profile_info.email_verification_code');
    expect(code).toHaveAttribute('autocomplete', 'one-time-code');
    expect(code).toHaveAttribute('autocapitalize', 'none');
    expect(code).toHaveAttribute('spellcheck', 'false');
  });

  it('treats a dismissed passkey registration prompt as a silent cancel', async () => {
    const browserMessage = 'The operation either timed out or was not allowed.';
    vi.mocked(startRegistration).mockRejectedValueOnce(Object.assign(new Error(browserMessage), { name: 'NotAllowedError' }));
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(url === '/api/user/passkey/register/begin' ? { data: { success: true, data: { publicKey: { challenge: 'c' } } } } : { data: { success: true } })
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.passkey.register_button' }));
    const buttons = await screen.findAllByRole('button', { name: 'personal_settings.passkey.register_button' });
    fireEvent.click(buttons[buttons.length - 1]);

    await waitFor(() => expect(startRegistration).toHaveBeenCalled());
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'personal_settings.passkey.register_button' }).at(-1)).not.toBeDisabled());
    expect(screen.queryByText(browserMessage)).not.toBeInTheDocument();
  });

  it('loads the QR encoder on demand when TOTP setup starts', async () => {
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(
        url === '/api/user/totp/setup'
          ? { data: { success: true, data: { secret: 'SECRET', qr_code: 'otpauth://totp/test' } } }
          : { data: { success: true } }
      )
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.totp.enable_button' }));

    await waitFor(() => expect(QRCode.toDataURL).toHaveBeenCalledWith('otpauth://totp/test', { width: 256, margin: 2 }));
  });

  it('dismisses the passkey setup prompt once a passkey is registered', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem');
    vi.mocked(startRegistration).mockResolvedValueOnce({ id: 'cred' } as any);
    (api.post as any).mockImplementation((url: string) =>
      Promise.resolve(
        url === '/api/user/passkey/register/begin' ? { data: { success: true, data: { publicKey: { challenge: 'c' } } } } : { data: { success: true } }
      )
    );

    render(<PersonalSettings />);
    fireEvent.click(await screen.findByRole('button', { name: 'personal_settings.passkey.register_button' }));
    const buttons = await screen.findAllByRole('button', { name: 'personal_settings.passkey.register_button' });
    fireEvent.click(buttons[buttons.length - 1]);

    await waitFor(() => expect(api.post).toHaveBeenCalledWith('/api/user/passkey/register/finish?name=Passkey', { id: 'cred' }));
    await waitFor(() => expect(setItem).toHaveBeenCalledWith('passkey_prompt_dismissed', '1'));
    setItem.mockRestore();
  });
});
