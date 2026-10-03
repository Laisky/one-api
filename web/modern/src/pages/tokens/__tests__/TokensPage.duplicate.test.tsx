import type { ReactNode } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TokensPage, type Token } from '../TokensPage.impl';

const { get, post, notify } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), notify: vi.fn() }));
vi.mock('@/lib/api', () => ({ api: { get, post } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('@/components/ui/confirm-dialog', () => ({ useConfirmDialog: () => [vi.fn(), () => null] }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false, isTablet: false }) }));

vi.mock('@/components/ui/enhanced-data-table', () => ({
  // EnhancedDataTable exposes both action slots without coupling this test to table internals.
  EnhancedDataTable: ({
    data,
    columns,
    floatingRowActions,
    onSearchValueChange,
  }: {
    data: Token[];
    columns: { header: unknown; cell?: (context: { row: { original: Token } }) => ReactNode }[];
    floatingRowActions: (token: Token) => ReactNode;
    onSearchValueChange: (value: string) => void;
  }) => (
    <div>
      <input aria-label="Search draft" onChange={(event) => onSearchValueChange(event.target.value)} />
      {data.map((token) => (
        <div key={token.uuid || token.id}>
          <div role="group" aria-label="inline actions">
            {columns.find((column) => column.header === 'Actions')?.cell?.({ row: { original: token } })}
          </div>
          <div role="group" aria-label="floating actions">
            {floatingRowActions(token)}
          </div>
        </div>
      ))}
    </div>
  ),
}));

const token: Token = {
  id: 7,
  uuid: 'source-uuid',
  name: 'stale-name',
  key: 'source-secret',
  status: 1,
  remain_quota: 100,
  unlimited_quota: false,
  used_quota: 50,
  created_time: 0,
  accessed_time: 0,
  expired_time: -1,
};

describe('TokensPage duplicate action wiring', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
    post.mockResolvedValue({ data: { success: true } });
  });

  it.each([
    ['inline', true],
    ['floating', true],
    ['inline', false],
    ['floating', false],
  ] as const)('creates from the %s action with UUID availability %s', async (layout, hasUUID) => {
    const row = { ...token, uuid: hasUUID ? token.uuid : '' };
    const ref = hasUUID ? token.uuid : token.id;
    get.mockImplementation(async (url: string) => ({
      data:
        url === `/api/token/${ref}`
          ? { success: true, data: { ...row, name: 'latest-9', remain_quota: 75, models: 'model-a', subnet: '192.0.2.0/24' } }
          : { success: true, data: [row], total: 1 },
    }));
    render(
      <MemoryRouter initialEntries={['/tokens?keyword=stale']}>
        <TokensPage />
      </MemoryRouter>
    );
    await screen.findByRole('group', { name: `${layout} actions` });
    await waitFor(() => expect(get).toHaveBeenCalledWith(expect.stringContaining('/api/token/search?keyword=stale')));
    // An unsubmitted search draft must not replace the filter when creation refreshes the list.
    fireEvent.change(screen.getByRole('textbox', { name: 'Search draft' }), { target: { value: 'unsubmitted' } });
    get.mockClear();
    fireEvent.click(within(screen.getByRole('group', { name: `${layout} actions` })).getByRole('button', { name: 'Duplicate' }));
    await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
    expect(get).toHaveBeenCalledWith(`/api/token/${ref}`);
    expect(post).toHaveBeenCalledWith('/api/token/', {
      name: 'latest-10',
      remain_quota: 75,
      unlimited_quota: false,
      expired_time: -1,
      models: 'model-a',
      subnet: '192.0.2.0/24',
    });
    await waitFor(() => expect(get).toHaveBeenLastCalledWith(expect.stringContaining('/api/token/search?keyword=stale&p=0&size=')));
    expect(get.mock.calls.some(([url]) => String(url).includes('unsubmitted'))).toBe(false);
    expect(notify).toHaveBeenCalledWith({ type: 'success', message: 'Created token "latest-10".' });
  });
});
