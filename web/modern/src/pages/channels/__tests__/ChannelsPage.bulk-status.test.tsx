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
const mockApiPut = vi.mocked(api.put);

const channels = [
  { id: 1, name: 'A', type: 1, status: 1, created_time: 1, priority: 0, weight: 0, models: '', group: 'default' },
  { id: 2, name: 'B', type: 1, status: 1, created_time: 1, priority: 0, weight: 0, models: '', group: 'default' },
  { id: 3, name: 'C', type: 1, status: 1, created_time: 1, priority: 0, weight: 0, models: '', group: 'default' },
];

describe('ChannelsPage bulk enable/disable', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockApiGet.mockResolvedValue({ data: { success: true, data: channels, total: channels.length } });
    mockApiPut.mockResolvedValue({ data: { success: true } });
  });

  const renderPage = () =>
    render(
      <BrowserRouter>
        <ChannelsPage />
      </BrowserRouter>
    );

  it('disables all visible channels via bulk action', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const user = userEvent.setup();
    const trigger = await screen.findByRole('button', { name: /bulk actions/i });
    await user.click(trigger);

    const disableItem = await screen.findByRole('menuitem', { name: /disable visible channels/i });
    await user.click(disableItem);

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledTimes(channels.length);
    });

    for (const ch of channels) {
      expect(mockApiPut).toHaveBeenCalledWith('/api/channel/?status_only=1', {
        id: ch.id,
        status: 2,
      });
    }
  });
});
