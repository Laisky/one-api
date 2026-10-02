import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import { UsersPage } from '../UsersPage';

vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: true, isTablet: false }) }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn() }) }));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));

const mockGet = vi.mocked(api.get);
const response = {
  data: {
    success: true,
    total: 1,
    data: [{ uuid: 'user-1', username: 'alpha', display_name: 'Alpha', role: 1, status: 1, quota: 100, used_quota: 1, group: 'default' }],
  },
};

/** renderUsers mounts the actual page and waits until its initial server request has completed. */
async function renderUsers() {
  render(<MemoryRouter><UsersPage /></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Select alpha' })).toBeEnabled());
}

describe('UsersPage mobile sorting', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGet.mockResolvedValue(response);
  });

  it('has one accessible sort control and retains ID sorting without exposing unsupported display fields', async () => {
    await renderUsers();
    const controls = screen.getAllByRole('combobox', { name: 'Sort by' });
    expect(controls).toHaveLength(1);
    // Include unnamed native controls so the original duplicate cannot hide
    // behind a missing accessible name; ignore Radix's hidden form controls.
    expect(screen.getAllByRole('combobox').filter((element) => element.tagName === 'SELECT')).toHaveLength(1);
    expect(within(controls[0]).getByRole('option', { name: 'ID' })).toHaveValue('id');
    expect(within(controls[0]).getByRole('option', { name: 'Remaining Quota' })).toHaveValue('quota');
    expect(within(controls[0]).queryByRole('option', { name: 'Display Name' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'DESC' })).not.toBeInTheDocument();
  });

  it('preserves descending-first server sorting and makes exactly one request per change', async () => {
    await renderUsers();
    mockGet.mockClear();
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Sort by' }), 'id');
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));
    expect(mockGet).toHaveBeenLastCalledWith(expect.stringContaining('&sort=id&order=desc'));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sort ascending' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Sort ascending' }));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(2));
    expect(mockGet).toHaveBeenLastCalledWith(expect.stringContaining('&sort=id&order=asc'));
  });
});
