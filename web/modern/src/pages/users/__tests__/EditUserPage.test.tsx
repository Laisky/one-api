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

/**
 * seedUserWithCatalog serves user 2 in `group` and answers each `/api/group/`
 * call with the next entry of `catalog` (the last one repeats); `'reject'`
 * simulates a network error and `'pending'` a request that never settles.
 */
const seedUserWithCatalog = (group: string, catalog: Array<'reject' | 'pending' | unknown>) => {
  let groupCall = 0;
  (api.get as any).mockReset();
  (api.get as any).mockImplementation((url: string) => {
    if (url === '/api/group/') {
      const next = catalog[Math.min(groupCall, catalog.length - 1)];
      groupCall += 1;
      if (next === 'reject') return Promise.reject(new Error('network down'));
      if (next === 'pending') return new Promise(() => {});
      return Promise.resolve({ data: next });
    }
    if (url.startsWith('/api/user/')) {
      return Promise.resolve({
        data: {
          success: true,
          data: { id: 2, username: 'alice', display_name: 'Alice', email: 'alice@example.com', quota: 100, group, mcp_tool_blacklist: [] },
        },
      });
    }
    return Promise.resolve({ data: { success: true } });
  });
};

describe('EditUserPage group catalog', () => {
  beforeEach(() => {
    notify.mockReset();
    (api.put as any).mockReset();
    setAdmin();
  });

  it('shows a retryable error and keeps displaying the current group when the catalog fails', async () => {
    seedUserWithCatalog('legacy group', ['reject', { success: true, data: ['default', 'vip'] }]);
    const user = userEvent.setup();
    renderEditPage('2');

    await screen.findByDisplayValue('alice');
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('The configured groups could not be loaded');
    expect(screen.getByRole('combobox')).toHaveTextContent('legacy group');

    await user.click(within(alert).getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());

    // The stored group stays selected and listed next to the configured groups.
    const trigger = screen.getByRole('combobox');
    expect(trigger).toHaveTextContent('legacy group');
    await user.click(trigger);
    expect((await screen.findAllByRole('option')).map((option) => option.textContent)).toEqual(['default', 'vip', 'legacy group']);
  });

  it('treats a success:false catalog response as a load failure', async () => {
    seedUserWithCatalog('default', [{ success: false, message: 'forbidden' }]);
    renderEditPage('2');

    expect(await screen.findByRole('alert')).toHaveTextContent('The configured groups could not be loaded');
    expect(screen.getByRole('combobox')).toHaveTextContent('default');
  });

  it('keeps showing an unconfigured current group while the catalog is loading and does not resend it', async () => {
    seedUserWithCatalog('partners', ['pending']);
    (api.put as any).mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    renderEditPage('2');

    const displayName = await screen.findByDisplayValue('Alice');
    expect(screen.getByRole('combobox')).toHaveTextContent('partners');

    await user.clear(displayName);
    await user.type(displayName, 'Alice B');
    await user.click(screen.getByRole('button', { name: 'Update User' }));

    await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1));
    const payload = (api.put as any).mock.calls[0][1];
    expect(payload).toEqual(expect.objectContaining({ display_name: 'Alice B' }));
    expect(payload).not.toHaveProperty('group');
  });
});

describe('EditUserPage password policy', () => {
  beforeEach(() => {
    notify.mockReset();
    (api.post as any).mockReset();
    (api.put as any).mockReset();
    localStorage.clear();
  });

  it('describes the new-password field for autofill and links the label', async () => {
    setAdmin();
    seedTargetUser();
    renderEditPage('2');

    await screen.findByDisplayValue('alice');
    const password = screen.getByLabelText('New Password (leave empty to keep current)');
    expect(password).toHaveAttribute('autocomplete', 'new-password');
    expect(password).toHaveAttribute('minlength', '8');
    const username = screen.getByLabelText('Username *');
    expect(username).toHaveAttribute('autocomplete', 'off');
    expect(username).toHaveAttribute('spellcheck', 'false');
    expect(username).toHaveAttribute('maxlength', '30');
  });

  it('rejects edit passwords outside the 8-20 characters accepted by the admin update endpoint', async () => {
    setAdmin();
    seedTargetUser();
    renderEditPage('2');
    const user = userEvent.setup();

    await screen.findByDisplayValue('alice');
    const password = screen.getByLabelText('New Password (leave empty to keep current)');

    await user.type(password, 'abc1234');
    await user.click(screen.getByRole('button', { name: 'Update User' }));
    expect(await screen.findByText('Password must be at least 8 characters')).toBeInTheDocument();

    await user.clear(password);
    await user.type(password, 'a1'.repeat(11));
    await user.click(screen.getByRole('button', { name: 'Update User' }));
    expect(await screen.findByText('Password must be at most 20 characters')).toBeInTheDocument();
    expect(api.put).not.toHaveBeenCalled();
  });
});
