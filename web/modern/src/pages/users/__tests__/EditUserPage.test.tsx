import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EditUserPage } from '../EditUserPage';

const notify = vi.fn();
vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/lib/api', () => {
  const get = vi.fn();
  const post = vi.fn();
  const put = vi.fn();
  const del = vi.fn();
  return {
    api: {
      get,
      post,
      put,
      delete: del,
      defaults: { withCredentials: true },
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
    },
  };
});

const renderEditPage = (id: string) =>
  render(
    <MemoryRouter initialEntries={[`/users/edit/${id}`]}>
      <Routes>
        <Route path="/users/edit/:id" element={<EditUserPage />} />
      </Routes>
    </MemoryRouter>
  );

const setAdmin = () => {
  useAuthStore.setState({
    user: {
      id: 1,
      username: 'admin',
      role: 100,
      status: 1,
      quota: 0,
      used_quota: 0,
      group: 'default',
    } as any,
    token: 'token',
    isAuthenticated: true,
    login: vi.fn() as any,
    logout: vi.fn() as any,
    updateUser: vi.fn() as any,
  });
};

const seedTargetUser = () => {
  (api.get as any).mockReset();
  (api.get as any).mockImplementation((url: string) => {
    if (url.startsWith('/api/user/') && !url.startsWith('/api/user/search')) {
      return Promise.resolve({
        data: {
          success: true,
          data: {
            id: 2,
            username: 'alice',
            display_name: 'Alice',
            email: 'alice@example.com',
            quota: 100,
            group: 'default',
            mcp_tool_blacklist: [],
          },
        },
      });
    }
    if (url === '/api/group/') {
      return Promise.resolve({ data: { success: true, data: ['default'] } });
    }
    return Promise.resolve({ data: { success: true } });
  });
};

describe('EditUserPage 2FA disable button', () => {
  beforeEach(() => {
    notify.mockReset();
    (api.post as any).mockReset();
    (api.put as any).mockReset();
    (api.delete as any).mockReset();
    localStorage.clear();
  });

  it('renders disable 2FA button when admin views another user and triggers API call on confirm', async () => {
    setAdmin();
    seedTargetUser();
    (api.post as any).mockResolvedValue({ data: { success: true, message: 'ok' } });

    renderEditPage('2');
    const user = userEvent.setup();

    const button = await screen.findByRole('button', { name: /disable user 2fa/i });
    await user.click(button);

    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Disable 2FA' }));

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/totp/disable/2');
    });
    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    });
  });

  it('does not render disable 2FA button when admin edits their own profile', async () => {
    useAuthStore.setState({
      user: {
        id: 2,
        username: 'alice',
        role: 100,
        status: 1,
        quota: 0,
        used_quota: 0,
        group: 'default',
      } as any,
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: vi.fn() as any,
    });
    seedTargetUser();

    renderEditPage('2');

    await screen.findByDisplayValue('alice');
    expect(screen.queryByRole('button', { name: /disable user 2fa/i })).not.toBeInTheDocument();
  });
});
