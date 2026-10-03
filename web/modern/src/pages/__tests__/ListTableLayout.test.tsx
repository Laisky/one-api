import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ChannelsPage } from '../channels/ChannelsPage';
import { MCPServersPage } from '../mcp/MCPServersPage';
import { TokensPage } from '../tokens/TokensPage.impl';

const { get, responsive } = vi.hoisted(() => ({
  get: vi.fn(),
  responsive: { isMobile: false, isTablet: false },
}));
vi.mock('@/lib/api', () => ({ api: { get } }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => responsive }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn() }) }));
vi.mock('@/components/ui/confirm-dialog', () => ({ useConfirmDialog: () => [vi.fn(), () => null] }));
vi.mock('@/pages/channels/useChannelModelReset', () => ({
  useChannelModelReset: () => ({ busy: false, report: null, confirmation: null }),
  ChannelModelResetButton: () => null,
}));
vi.mock('@/pages/channels/useSelectedChannelActions', () => ({
  useSelectedChannelActions: () => ({ busy: false, report: null, confirmation: null }),
}));
vi.mock('@/components/ui/enhanced-data-table', () => ({
  // EnhancedDataTable isolates the page-owned shell from table internals and CSS execution in jsdom.
  EnhancedDataTable: ({ loading, data }: { loading: boolean; data: unknown[] }) => (
    <div data-testid="management-table" aria-busy={loading} data-row-count={data.length} />
  ),
}));

/** deferred keeps loading visible until the test supplies a server response. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

/** expectTableSpacing checks the real Card and CardContent used by the page, not a mocked wrapper. */
function expectTableSpacing(isMobile: boolean) {
  const table = screen.getByTestId('management-table');
  const content = table.parentElement;
  const card = content?.parentElement;
  expect(content).toHaveClass(isMobile ? 'p-2' : 'p-6');
  expect(content).not.toHaveClass(isMobile ? 'p-6' : 'p-2');
  // CardContent's default pt-0 must not remove the space above the toolbar.
  expect(content).not.toHaveClass('pt-0');
  expect(card).toHaveClass('rounded-lg', 'bg-card', 'border-0', 'md:border', 'shadow-none', 'md:shadow-sm');
  return table;
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  responsive.isMobile = false;
  responsive.isTablet = false;
});
afterEach(cleanup);

const pages = [
  { name: 'tokens', Page: TokensPage, route: '/tokens' },
  { name: 'channels', Page: ChannelsPage, route: '/channels' },
  { name: 'MCP servers', Page: MCPServersPage, route: '/mcps' },
] as const;

const viewports = [
  { name: 'desktop', isMobile: false, isTablet: false },
  { name: 'tablet', isMobile: false, isTablet: true },
  { name: 'mobile', isMobile: true, isTablet: false },
] as const;

describe.each(pages)('$name management table spacing', ({ Page, route }) => {
  it.each(viewports)('preserves shared padding during loading and after an empty response on $name', async (viewport) => {
    Object.assign(responsive, { isMobile: viewport.isMobile, isTablet: viewport.isTablet });
    const request = deferred<{ data: { success: boolean; data: []; total: number } }>();
    get.mockReturnValue(request.promise);
    render(<MemoryRouter initialEntries={[route]}><Page /></MemoryRouter>);
    expect(expectTableSpacing(viewport.isMobile)).toHaveAttribute('aria-busy', 'true');
    await act(async () => request.resolve({ data: { success: true, data: [], total: 0 } }));
    expect(expectTableSpacing(viewport.isMobile)).toHaveAttribute('aria-busy', 'false');
    expect(screen.getByTestId('management-table')).toHaveAttribute('data-row-count', '0');
  });

  it('updates the existing shell when the viewport switches between desktop and mobile', async () => {
    get.mockResolvedValue({ data: { success: true, data: [], total: 0 } });
    const page = <MemoryRouter initialEntries={[route]}><Page /></MemoryRouter>;
    const { rerender } = render(page);
    await waitFor(() => expect(screen.getByTestId('management-table')).toHaveAttribute('aria-busy', 'false'));
    expectTableSpacing(false);
    responsive.isMobile = true;
    // A new element models the rerender caused by the real responsive hook.
    rerender(<MemoryRouter initialEntries={[route]}><Page /></MemoryRouter>);
    expectTableSpacing(true);
    responsive.isMobile = false;
    rerender(<MemoryRouter initialEntries={[route]}><Page /></MemoryRouter>);
    expectTableSpacing(false);
  });
});
