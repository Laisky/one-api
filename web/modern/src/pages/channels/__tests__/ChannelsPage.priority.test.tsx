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

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const mockApiGet = vi.mocked(api.get);
const mockApiPut = vi.mocked(api.put);

const channelRow = {
  id: 7,
  name: 'Channel 7',
  type: 1,
  status: 1,
  created_time: 1700000000,
  priority: 5,
  weight: 0,
  models: 'gpt-4',
  group: 'default',
  used_quota: 0,
};

describe('ChannelsPage priority editor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockApiGet.mockResolvedValue({
      data: { success: true, data: [channelRow], total: 1 },
    });
    mockApiPut.mockResolvedValue({ data: { success: true } });
  });

  const renderPage = () =>
    render(
      <BrowserRouter>
        <ChannelsPage />
      </BrowserRouter>
    );

  it('saves a changed priority on blur', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const input = await screen.findByLabelText('Priority for Channel 7');
    expect(input).toHaveValue(5);

    const user = userEvent.setup();
    await user.clear(input);
    await user.type(input, '12');
    await user.tab();

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith('/api/channel/', {
        id: 7,
        name: 'Channel 7',
        priority: 12,
      });
    });
  });

  it('does not call api.put when value is unchanged', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const input = await screen.findByLabelText('Priority for Channel 7');
    const user = userEvent.setup();
    // Focus + blur with no change
    await user.click(input);
    await user.tab();

    expect(mockApiPut).not.toHaveBeenCalled();
  });
});
