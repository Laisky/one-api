import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
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
  };
});

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
      expect(api.get).toHaveBeenCalledWith('/api/oauth/email/bind?email=new%40example.com&code=123456');
    });

    await waitFor(() => {
      expect((screen.getByPlaceholderText('personal_settings.profile_info.email_placeholder') as HTMLInputElement).value).toBe(
        'new@example.com'
      );
    });

    expect(useAuthStore.getState().user?.email).toBe('new@example.com');
  });
});
