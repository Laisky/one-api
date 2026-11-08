import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import { MCPServersPage } from '../MCPServersPage';

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } }));
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify: vi.fn() }) }));
const get = vi.mocked(api.get);

/** serverList returns a one-row MCP server list whose server has the supplied status. */
function serverList(status: number) {
  return {
    data: {
      success: true,
      total: 1,
      data: [
        {
          server: {
            uuid: '018fcf6d-c484-7000-8000-000000000001',
            name: 'Docs server',
            status,
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

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('MCP server status with browser-translated text', () => {
  it.each([
    [1, 'Enabled', 0, 'Disabled'],
    [0, 'Disabled', 1, 'Enabled'],
  ])('shows the refreshed status after status %i is translated', async (before, beforeLabel, after, afterLabel) => {
    let status = before;
    get.mockImplementation(async () => serverList(status));
    render(
      <MemoryRouter initialEntries={['/mcps']}>
        <DomMutationBoundary>
          <MCPServersPage />
        </DomMutationBoundary>
      </MemoryRouter>
    );
    const label = await screen.findByText(beforeLabel);
    const cell = label.closest('td');
    if (!cell) throw new Error('Status cell is unavailable');
    expect(translateText(cell)).toBeGreaterThan(0);

    status = after;
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2));

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    await waitFor(() => expect(cell).toHaveTextContent(afterLabel));
    expect(cell).not.toHaveTextContent(beforeLabel);
  });
});
