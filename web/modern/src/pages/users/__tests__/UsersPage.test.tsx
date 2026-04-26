import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { UsersPage } from '../UsersPage';

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

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}));

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<any>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const baseUser = {
  id: 2,
  username: 'alice',
  display_name: 'Alice',
  role: 1,
  status: 1,
  email: 'alice@example.com',
  quota: 100,
  used_quota: 5,
  group: 'default',
};

const adminUser = {
  ...baseUser,
  id: 3,
  username: 'bob',
  display_name: 'Bob',
  role: 10,
};

const setSuperAdmin = () => {
  useAuthStore.setState({
    user: {
      id: 1,
      username: 'root',
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

const setRegularUser = () => {
  useAuthStore.setState({
    user: {
      id: 1,
      username: 'normal',
      role: 1,
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

const renderPage = () =>
  render(
    <BrowserRouter>
      <UsersPage />
    </BrowserRouter>
  );

const seedListResponse = (rows: any[]) => {
  (api.get as any).mockReset();
  (api.get as any).mockResolvedValue({
    data: { success: true, data: rows, total: rows.length },
  });
};

describe('UsersPage promote/demote/disable_2fa actions', () => {
  beforeEach(() => {
    notify.mockReset();
    (api.post as any).mockReset();
    (api.put as any).mockReset();
    (api.delete as any).mockReset();
    mockNavigate.mockReset();
    localStorage.clear();
  });

  it('hides Promote action when current user is not super admin', async () => {
    setRegularUser();
    seedListResponse([baseUser]);
    renderPage();

    await waitFor(() => expect(api.get).toHaveBeenCalled());
    await screen.findByText('alice');

    expect(screen.queryByRole('button', { name: 'Promote' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Demote' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Disable 2FA' })).not.toBeInTheDocument();
  });

  it('shows Promote when super admin views a regular user', async () => {
    setSuperAdmin();
    seedListResponse([baseUser]);
    renderPage();

    await waitFor(() => expect(api.get).toHaveBeenCalled());
    await screen.findByText('alice');

    expect(screen.getByRole('button', { name: 'Promote' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Demote' })).not.toBeInTheDocument();
  });

  it('promotes a user, updates the row role, and notifies success', async () => {
    setSuperAdmin();
    seedListResponse([baseUser]);
    (api.post as any).mockResolvedValue({
      data: { success: true, data: { ...baseUser, role: 10 } },
    });

    renderPage();
    const user = userEvent.setup();

    await screen.findByText('alice');

    // Verify initial role label
    expect(screen.getByText('User')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Promote' }));

    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/manage', {
        username: 'alice',
        action: 'promote',
      });
    });

    await waitFor(() => {
      expect(screen.getByText('Admin')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    });
  });

  it('shows error notification when promote fails', async () => {
    setSuperAdmin();
    seedListResponse([baseUser]);
    (api.post as any).mockRejectedValue({
      response: { data: { success: false, message: 'backend says no' } },
    });

    renderPage();
    const user = userEvent.setup();
    await screen.findByText('alice');

    await user.click(screen.getByRole('button', { name: 'Promote' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'backend says no' }));
    });
  });

  it('demotes an admin and updates the row role', async () => {
    setSuperAdmin();
    seedListResponse([adminUser]);
    (api.post as any).mockResolvedValue({
      data: { success: true, data: { ...adminUser, role: 1 } },
    });

    renderPage();
    const user = userEvent.setup();
    await screen.findByText('bob');
    expect(screen.getByText('Admin')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Demote' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/manage', {
        username: 'bob',
        action: 'demote',
      });
    });

    await waitFor(() => {
      expect(screen.getByText('User')).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    });
  });

  it('shows error notification when demote fails', async () => {
    setSuperAdmin();
    seedListResponse([adminUser]);
    (api.post as any).mockRejectedValue({
      response: { data: { success: false, message: 'cannot demote' } },
    });

    renderPage();
    const user = userEvent.setup();
    await screen.findByText('bob');

    await user.click(screen.getByRole('button', { name: 'Demote' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'cannot demote' }));
    });
  });

  it('disables 2FA after confirmation and notifies success', async () => {
    setSuperAdmin();
    seedListResponse([baseUser]);
    (api.post as any).mockResolvedValue({
      data: { success: true, message: 'ok' },
    });

    renderPage();
    const user = userEvent.setup();
    await screen.findByText('alice');

    await user.click(screen.getByRole('button', { name: 'Disable 2FA' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith('/api/user/totp/disable/2');
    });
    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    });
  });

  it('shows error notification when disable 2FA fails', async () => {
    setSuperAdmin();
    seedListResponse([baseUser]);
    (api.post as any).mockResolvedValue({
      data: { success: false, message: 'totp not configured' },
    });

    renderPage();
    const user = userEvent.setup();
    await screen.findByText('alice');

    await user.click(screen.getByRole('button', { name: 'Disable 2FA' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'totp not configured' }));
    });
  });
});
