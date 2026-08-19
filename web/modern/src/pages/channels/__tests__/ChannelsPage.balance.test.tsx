import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    delete: vi.fn(),
    put: vi.fn(),
  },
}));

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => vi.fn(),
  };
});

const mockApiGet = vi.mocked(api.get);

const channelRow = {
  id: 3,
  name: 'Channel 3',
  type: 1,
  status: 1,
  created_time: 1700000000,
  priority: 0,
  weight: 0,
  models: '',
  group: 'default',
  balance: 12.5,
  balance_updated_time: 1700000000,
};

describe('ChannelsPage balance refresh', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    // Default behavior: list-load returns single channel.
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({ data: { success: true, data: [channelRow], total: 1 } });
      }
      if (url.startsWith('/api/channel/update_balance/')) {
        return Promise.resolve({
          data: { success: true, balance: 42.0, balance_updated_time: 1700001234 },
        });
      }
      if (url === '/api/channel/update_balance') {
        return Promise.resolve({ data: { success: true } });
      }
      return Promise.resolve({ data: { success: true } });
    });
  });

  const renderPage = () =>
    render(
      <BrowserRouter>
        <ChannelsPage />
      </BrowserRouter>
    );

  it('refreshes a single row balance via per-row icon', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const user = userEvent.setup();
    const refreshBtn = await screen.findByLabelText('Refresh balance for Channel 3');
    await user.click(refreshBtn);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/update_balance/3');
    });
  });

  it('refreshes all balances from header bulk action', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const user = userEvent.setup();
    const bulkBtn = await screen.findByRole('button', { name: /refresh all balances/i });
    await user.click(bulkBtn);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/update_balance');
    });
  });
});
