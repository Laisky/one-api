import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';
import { CHANNEL_TESTING_MODEL_SKIP } from '../channels-page-columns';
import { confirmPersistedTestingModel, readPersistedTestingModel } from '../helpers';

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

  it('keeps SKIP selected after the server confirms it was persisted', async () => {
    mockApiPut.mockResolvedValue({
      data: { success: true, data: { ...channelRow, testing_model: CHANNEL_TESTING_MODEL_SKIP } },
    });
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;

    await userEvent.setup().selectOptions(select, CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' })));
    // Re-query: the table may remount the cell, so the original node can be stale.
    await waitFor(() => expect((screen.getByLabelText('Testing Model') as HTMLSelectElement).value).toBe(CHANNEL_TESTING_MODEL_SKIP));
  });

  it('shows the persisted value and warns when the server did not keep the selection', async () => {
    // Issue #609: the server answered success but stored NULL; the row must not keep
    // showing SKIP until a refresh reveals auto-select.
    mockApiPut.mockResolvedValue({ data: { success: true, data: { ...channelRow, testing_model: null } } });
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;

    await userEvent.setup().selectOptions(select, CHANNEL_TESTING_MODEL_SKIP);

    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' })));
    expect(mockNotify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    expect((screen.getByLabelText('Testing Model') as HTMLSelectElement).value).toBe('');
  });

  it('switches back to auto-select by sending an explicit null', async () => {
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({
          data: { success: true, data: [{ ...channelRow, testing_model: CHANNEL_TESTING_MODEL_SKIP }], total: 1 },
        });
      }
      return Promise.resolve({ data: { success: true } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true, data: { ...channelRow, testing_model: null } } });
    renderPage();
    const select = (await screen.findByLabelText('Testing Model')) as HTMLSelectElement;
    expect(select.value).toBe(CHANNEL_TESTING_MODEL_SKIP);

    await userEvent.setup().selectOptions(select, '');

    await waitFor(() => expect(mockApiPut).toHaveBeenCalled());
    const [, payload] = mockApiPut.mock.calls[0] as [string, Record<string, unknown>];
    // The backend treats an explicit null as "clear to auto-select" and an omitted
    // field as "keep", so the key must be present.
    expect(payload).toHaveProperty('testing_model', null);
    await waitFor(() => expect(mockNotify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' })));
    expect((screen.getByLabelText('Testing Model') as HTMLSelectElement).value).toBe('');
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

describe('readPersistedTestingModel', () => {
  it('reads the testing model echoed by the server', () => {
    expect(readPersistedTestingModel({ testing_model: CHANNEL_TESTING_MODEL_SKIP })).toBe(CHANNEL_TESTING_MODEL_SKIP);
    expect(readPersistedTestingModel({ testing_model: 'gpt-4o-mini' })).toBe('gpt-4o-mini');
  });

  it('maps a null or blank persisted value to auto-select', () => {
    expect(readPersistedTestingModel({ testing_model: null })).toBeNull();
    expect(readPersistedTestingModel({ testing_model: '' })).toBeNull();
  });

  it('reports a row without the field as unknown', () => {
    expect(readPersistedTestingModel(undefined)).toBeUndefined();
    expect(readPersistedTestingModel(null)).toBeUndefined();
    expect(readPersistedTestingModel({ name: 'Channel 11' })).toBeUndefined();
  });
});

describe('confirmPersistedTestingModel', () => {
  it('trusts the saved row in the response without re-reading', async () => {
    const readChannel = vi.fn();
    await expect(confirmPersistedTestingModel({ testing_model: null }, CHANNEL_TESTING_MODEL_SKIP, readChannel)).resolves.toEqual({
      value: null,
      verified: true,
    });
    expect(readChannel).not.toHaveBeenCalled();
  });

  it('re-reads the channel when the response omits the row', async () => {
    const readChannel = vi.fn().mockResolvedValue({ testing_model: CHANNEL_TESTING_MODEL_SKIP });
    await expect(confirmPersistedTestingModel(undefined, CHANNEL_TESTING_MODEL_SKIP, readChannel)).resolves.toEqual({
      value: CHANNEL_TESTING_MODEL_SKIP,
      verified: true,
    });
  });

  it('reports the requested value as unverified when the re-read fails or lacks the field', async () => {
    await expect(confirmPersistedTestingModel(undefined, 'gpt-4o-mini', () => Promise.reject(new Error('offline')))).resolves.toEqual({
      value: 'gpt-4o-mini',
      verified: false,
    });
    await expect(confirmPersistedTestingModel({}, null, () => Promise.resolve(undefined))).resolves.toEqual({
      value: null,
      verified: false,
    });
  });
});
