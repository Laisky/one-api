import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import { MCPServersPage } from '../MCPServersPage';

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn() }) }));
const get = vi.mocked(api.get);

/** response returns a list response labelled with its applied query and page. */
function response(keyword: string, page: number) {
  return {
    data: {
      success: true,
      total: 25,
      data: [
        {
          server: {
            uuid: `018fcf6d-c484-7000-8000-${String(page + 1).padStart(12, '0')}`,
            name: `${keyword || 'Unfiltered'} page ${page + 1}`,
            status: 1,
            priority: 0,
            base_url: 'https://mcp.example.com',
            protocol: 'streamable_http',
            auth_type: 'none',
          },
          tool_count: 1,
        },
      ],
    },
  };
}

/** LocationProbe exposes the actual router query so search pagination can be asserted. */
function LocationProbe() {
  return <output aria-label="Current location">{useLocation().search}</output>;
}

/** renderPage mounts the real search, selection and pagination controls at a requested page. */
function renderPage(page = 1) {
  render(
    <MemoryRouter initialEntries={[`/mcps?p=${page}`]}>
      <MCPServersPage />
      <LocationProbe />
    </MemoryRouter>
  );
}

/** editKeyword edits free text through the real dropdown without yet submitting the list query. */
async function editKeyword(keyword: string) {
  const user = userEvent.setup();
  await user.click(screen.getAllByRole('combobox')[0]);
  await user.type(screen.getByPlaceholderText('Search MCP servers by name, URL, or UUID...'), keyword);
  await user.click(await screen.findByRole('option', { name: /Search for:/i }));
}

/** submitKeyword submits edited search text through the real toolbar button. */
async function submitKeyword(keyword: string) {
  await editKeyword(keyword);
  await userEvent.click(screen.getByRole('button', { name: 'Search' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  get.mockImplementation(async (url) => {
    const params = new URL(url, 'https://example.com').searchParams;
    return response(params.get('keyword') || '', Number(params.get('p')));
  });
});

describe('MCP server search pagination', () => {
  it.each(['', 'Previous'])('applies a new keyword from page two, not the old scope %j', async (previous) => {
    renderPage();
    await screen.findByText('Unfiltered page 1');
    if (previous) {
      await submitKeyword(previous);
      await screen.findByText(`${previous} page 1`);
    }
    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }));
    await screen.findByText(`${previous || 'Unfiltered'} page 2`);
    get.mockClear();
    await submitKeyword('  Fresh  ');
    await screen.findByText('Fresh page 1');
    expect(get).toHaveBeenCalledTimes(1);
    expect(new URL(get.mock.calls[0][0], 'https://example.com').searchParams.get('keyword')).toBe('Fresh');
    expect(screen.getByRole('status', { name: 'Current location' })).toHaveTextContent('p=1');
    expect(screen.getByRole('checkbox', { name: 'Select Fresh page 1' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Row selection' })).toBeEnabled();
    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }));
    await screen.findByText('Fresh page 2');
    expect(new URL(get.mock.calls.at(-1)![0], 'https://example.com').searchParams.get('keyword')).toBe('Fresh');
  });

  it('replaces a first-page keyword without a duplicate page-effect request', async () => {
    renderPage();
    await screen.findByText('Unfiltered page 1');
    await submitKeyword('Previous');
    await screen.findByText('Previous page 1');
    get.mockClear();
    await submitKeyword('Fresh');
    await screen.findByText('Fresh page 1');
    expect(get).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('checkbox', { name: 'Select Fresh page 1' })).toBeEnabled();
  });

  it('retains the submitted scope when sorting after a delayed search response', async () => {
    renderPage(2);
    await screen.findByText('Unfiltered page 2');
    const pending: Array<{ url: string; resolve: (value: ReturnType<typeof response>) => void }> = [];
    get.mockImplementation((url) => new Promise((resolve) => pending.push({ url, resolve })));
    await submitKeyword('Fresh');
    await waitFor(() => expect(pending).toHaveLength(1));
    // Loading prevents another sort until the current request has settled.
    expect(screen.getByRole('checkbox', { name: 'Select Unfiltered page 2' })).toBeDisabled();
    await act(async () => pending[0].resolve(response('Fresh', 0)));
    await screen.findByText('Fresh page 1');
    await userEvent.click(within(screen.getByRole('columnheader', { name: 'Priority' })).getByRole('button'));
    await waitFor(() => expect(pending).toHaveLength(2));
    expect(new URL(pending[1].url, 'https://example.com').searchParams.get('keyword')).toBe('Fresh');
    await act(async () => pending[1].resolve(response('Fresh', 0)));
    expect(screen.getByRole('checkbox', { name: 'Select Fresh page 1' })).toBeEnabled();
  });

  it('does not apply an unsubmitted edit when refreshing the current result set', async () => {
    renderPage();
    await screen.findByText('Unfiltered page 1');
    await submitKeyword('Previous');
    await screen.findByText('Previous page 1');
    await editKeyword('Fresh');
    get.mockClear();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
    expect(new URL(get.mock.calls[0][0], 'https://example.com').searchParams.get('keyword')).toBe('Previous');
    expect(screen.getByRole('checkbox', { name: 'Select Previous page 1' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Search' }));
    await screen.findByText('Fresh page 1');
    expect(screen.getByRole('checkbox', { name: 'Select Fresh page 1' })).toBeEnabled();
  });

  it('returns to the unfiltered first page when a blank search is submitted from page two', async () => {
    renderPage();
    await screen.findByText('Unfiltered page 1');
    await submitKeyword('Previous');
    await screen.findByText('Previous page 1');
    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }));
    await screen.findByText('Previous page 2');
    get.mockClear();
    await submitKeyword('   ');
    await screen.findByText('Unfiltered page 1');
    expect(get).toHaveBeenCalledTimes(1);
    expect(new URL(get.mock.calls[0][0], 'https://example.com').searchParams.has('keyword')).toBe(false);
    expect(screen.getByRole('checkbox', { name: 'Select Unfiltered page 1' })).toBeEnabled();
  });
});
