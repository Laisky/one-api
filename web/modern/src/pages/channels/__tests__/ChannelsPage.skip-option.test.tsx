import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';
import { CHANNEL_TESTING_MODEL_SKIP } from '../channels-page-columns';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), delete: vi.fn(), put: vi.fn() },
}));

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return { ...actual, useNavigate: () => vi.fn() };
});

const mockApiGet = vi.mocked(api.get);
const mockApiPut = vi.mocked(api.put);

const channelRow = {
  id: 11,
  name: 'Channel 11',
  type: 1,
  status: 1,
  created_time: 1700000000,
  priority: 0,
  weight: 0,
  models: 'gpt-4o-mini',
  test_models: ['gpt-4o-mini'],
  testing_model: null as string | null,
  group: 'default',
};

const renderPage = () =>
  render(
    <BrowserRouter>
      <ChannelsPage />
    </BrowserRouter>
  );

describe('ChannelsPage testing-model SKIP option', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({ data: { success: true, data: [channelRow], total: 1 } });
      }
      return Promise.resolve({ data: { success: true } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true } });
  });

  it('offers SKIP alongside CHEAPEST and the channel models', async () => {
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;
    const values = Array.from(select.options).map((option) => option.value);
    expect(values).toEqual(['', CHANNEL_TESTING_MODEL_SKIP, 'gpt-4o-mini']);
  });

  it('persists SKIP as the channel testing model', async () => {
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;

    const user = userEvent.setup();
    await user.selectOptions(select, CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockApiPut).toHaveBeenCalled());
    const [, payload] = mockApiPut.mock.calls[0] as [string, any];
    expect(payload.testing_model).toBe(CHANNEL_TESTING_MODEL_SKIP);
  });

  it('shows SKIP as the current value when the channel is opted out', async () => {
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({
          data: {
            success: true,
            data: [{ ...channelRow, testing_model: CHANNEL_TESTING_MODEL_SKIP }],
            total: 1,
          },
        });
      }
      return Promise.resolve({ data: { success: true } });
    });
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;
    // Without explicit handling the sentinel is not among the model names, so the
    // control would silently fall back to CHEAPEST and misreport the setting.
    expect(select.value).toBe(CHANNEL_TESTING_MODEL_SKIP);
  });

  it('does not re-filter server-provided test_models', async () => {
    // The server classifies by API format; a name-based client filter would hide
    // chat models it accepts, such as a video-understanding chat model.
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({
          data: {
            success: true,
            data: [{ ...channelRow, models: 'hunyuan-turbos-vision-video', test_models: ['hunyuan-turbos-vision-video'] }],
            total: 1,
          },
        });
      }
      return Promise.resolve({ data: { success: true } });
    });
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;
    const values = Array.from(select.options).map((option) => option.value);
    expect(values).toContain('hunyuan-turbos-vision-video');
  });
});
