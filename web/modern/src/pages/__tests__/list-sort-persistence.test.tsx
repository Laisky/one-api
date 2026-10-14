import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { getTableSortStorageKey } from '@/lib/table-sort-preference';
import { ChannelsPage } from '../channels/ChannelsPage';
import { UsersPage } from '../users/UsersPage';
import { TokensPage } from '../tokens/TokensPage';
import { RedemptionsPage } from '../redemptions/RedemptionsPage';
import { MCPServersPage } from '../mcp/MCPServersPage';
import { LogsPage } from '../logs/LogsPage';

/** TableProbeProps contains the boundary between each list's request state and its shared renderer. */
interface TableProbeProps {
  sortBy: string;
  sortOrder: 'asc' | 'desc';
  onSortChange: (column: string, direction: 'asc' | 'desc') => void;
  loading?: boolean;
}

/** TableProbe drives the real page's sort callback without unrelated row action rendering. */
function TableProbe({ sortBy, sortOrder, onSortChange, loading }: TableProbeProps) {
  return (
    <div>
      <output data-testid="table-sort">{sortBy}:{sortOrder}</output>
      <button type="button" disabled={loading} onClick={() => onSortChange(sortBy, 'desc')}>Sort descending</button>
    </div>
  );
}

vi.mock('@/components/ui/data-table', () => ({ DataTable: (props: TableProbeProps) => <TableProbe {...props} /> }));
vi.mock('@/components/ui/enhanced-data-table', () => ({ EnhancedDataTable: (props: TableProbeProps) => <TableProbe {...props} /> }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false, isTablet: false }) }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn() }) }));
vi.mock('@/components/ui/searchable-dropdown', () => ({ SearchableDropdown: () => null }));
vi.mock('@/components/LogDetailsModal', () => ({ LogDetailsModal: () => null }));
vi.mock('../users/UserDialogs', () => ({ CreateUserDialog: () => null, TopUpDialog: () => null }));
const cursor = vi.hoisted(() => ({ supported: true, reset: vi.fn(), fetchPage: vi.fn() }));
vi.mock('../logs/useLogCursorPagination', () => ({ useLogCursorPagination: () => cursor }));

const originalUser = useAuthStore.getState().user;
const principal = JSON.stringify(['sort-test-account', 100]);

beforeEach(() => {
  localStorage.clear();
  cursor.reset.mockClear();
  cursor.fetchPage.mockClear();
  act(() => useAuthStore.setState({ user: {
    id: 'sort-test-account', uuid: 'sort-test-account', username: 'sort-test-account',
    role: 100, status: 1, quota: 0, used_quota: 0, group: 'default',
  } }));
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [], total: 0 },
  } as Awaited<ReturnType<typeof api.get>>);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  act(() => useAuthStore.setState({ user: originalUser }));
  localStorage.clear();
});

const cases = [
  { table: 'channels', Page: ChannelsPage, path: '/api/channel/', field: 'name' },
  { table: 'users', Page: UsersPage, path: '/api/user/', field: 'username' },
  { table: 'tokens', Page: TokensPage, path: '/api/token/', field: 'name' },
  { table: 'redemptions', Page: RedemptionsPage, path: '/api/redemption/', field: 'name' },
  { table: 'mcp', Page: MCPServersPage, path: '/api/mcp_servers', field: 'name' },
  { table: 'logs', Page: LogsPage, path: '/api/log/', field: 'quota' },
];

describe('list sorting request persistence', () => {
  it.each(cases)('$table restores sorting on its first requests and after a direction change/remount', async ({ table, Page, path, field }) => {
    const user = userEvent.setup();
    const key = getTableSortStorageKey(table, principal);
    localStorage.setItem(key, JSON.stringify({ sortBy: field, sortOrder: 'asc' }));
    const view = render(<MemoryRouter><Page /></MemoryRouter>);

    /** assertRequests checks every matching request, including existing duplicate initial loads. */
    const assertRequests = (direction: 'asc' | 'desc') => {
      const urls = vi.mocked(api.get).mock.calls
        .map(([url]) => new URL(String(url), 'https://example.test'))
        .filter((url) => url.pathname === path);
      expect(urls.length).toBeGreaterThan(0);
      for (const url of urls) {
        expect(url.searchParams.get('sort')).toBe(field);
        expect(url.searchParams.get('order')).toBe(direction);
      }
    };

    await waitFor(() => expect(screen.getByRole('button', { name: 'Sort descending' })).not.toBeDisabled());
    assertRequests('asc');
    expect(screen.getByTestId('table-sort')).toHaveTextContent(`${field}:asc`);

    vi.mocked(api.get).mockClear();
    await user.click(screen.getByRole('button', { name: 'Sort descending' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sort descending' })).not.toBeDisabled());
    assertRequests('desc');
    expect(JSON.parse(localStorage.getItem(key)!)).toEqual({ sortBy: field, sortOrder: 'desc' });

    view.unmount();
    vi.mocked(api.get).mockClear();
    render(<MemoryRouter><Page /></MemoryRouter>);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Sort descending' })).not.toBeDisabled());
    assertRequests('desc');
    expect(screen.getByTestId('table-sort')).toHaveTextContent(`${field}:desc`);
    if (table === 'logs') expect(cursor.fetchPage).not.toHaveBeenCalled();
  });
});
