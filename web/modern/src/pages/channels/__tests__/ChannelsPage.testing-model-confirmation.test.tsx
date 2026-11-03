import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';
import { CHANNEL_TESTING_MODEL_SKIP } from '../channels-page-columns';

const { mockNotify } = vi.hoisted(() => ({ mockNotify: vi.fn() }));

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: mockNotify }),
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
  uuid: '018f0000-0000-7000-8000-000000000011',
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

const singleChannelUrl = `/api/channel/${channelRow.uuid}`;

/** mockChannelReads answers the list request and, optionally, the single-channel read used for confirmation. */
const mockChannelReads = (single: () => Promise<unknown>) => {
  mockApiGet.mockImplementation((url: string) => {
    if (url.startsWith('/api/channel/?')) {
      return Promise.resolve({ data: { success: true, data: [channelRow], total: 1 } });
    }
    if (url === singleChannelUrl) {
      return single();
    }
    return Promise.resolve({ data: { success: true } });
  });
};

const renderPage = () =>
  render(
    <BrowserRouter>
      <ChannelsPage />
    </BrowserRouter>
  );

const currentSelect = () => screen.getByLabelText('Testing Model') as HTMLSelectElement;

describe('ChannelsPage testing-model confirmation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('re-reads the channel when the save response omits the row', async () => {
    // An older server answers success without the saved row and silently stores NULL.
    mockApiPut.mockResolvedValue({ data: { success: true } });
    mockChannelReads(() => Promise.resolve({ data: { success: true, data: { ...channelRow, testing_model: null } } }));
    renderPage();

    await userEvent.setup().selectOptions(await screen.findByLabelText('Testing Model'), CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith(singleChannelUrl));
    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' })));
    expect(mockNotify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    expect(currentSelect().value).toBe('');
  });

  it('confirms success from the re-read when the server did keep the selection', async () => {
    mockApiPut.mockResolvedValue({ data: { success: true } });
    mockChannelReads(() =>
      Promise.resolve({ data: { success: true, data: { ...channelRow, testing_model: CHANNEL_TESTING_MODEL_SKIP } } })
    );
    renderPage();

    await userEvent.setup().selectOptions(await screen.findByLabelText('Testing Model'), CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' })));
    await waitFor(() => expect(currentSelect().value).toBe(CHANNEL_TESTING_MODEL_SKIP));
  });

  it('reports an unverified save instead of success when the re-read fails', async () => {
    mockApiPut.mockResolvedValue({ data: { success: true } });
    mockChannelReads(() => Promise.reject(new Error('network down')));
    renderPage();

    await userEvent.setup().selectOptions(await screen.findByLabelText('Testing Model'), CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'info' })));
    expect(mockNotify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('locks the selector while a save is in flight so saves cannot overlap', async () => {
    let resolvePut: (value: unknown) => void = () => {};
    mockApiPut.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvePut = resolve;
        })
    );
    mockChannelReads(() => Promise.resolve({ data: { success: true } }));
    renderPage();
    const user = userEvent.setup();

    await user.selectOptions(await screen.findByLabelText('Testing Model'), CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(currentSelect().disabled).toBe(true));
    expect(mockApiPut).toHaveBeenCalledTimes(1);

    resolvePut({ data: { success: true, data: { ...channelRow, testing_model: CHANNEL_TESTING_MODEL_SKIP } } });

    await waitFor(() => expect(currentSelect().disabled).toBe(false));
    expect(currentSelect().value).toBe(CHANNEL_TESTING_MODEL_SKIP);
    expect(mockApiPut).toHaveBeenCalledTimes(1);
  });
});
