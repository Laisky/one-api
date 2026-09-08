import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { ChannelsPage } from '../ChannelsPage';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
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
  id: 7,
  name: 'Embedding Only',
  type: 50,
  status: 1,
  created_time: 1700000000,
  priority: 0,
  weight: 0,
  models: 'text-embedding-3-small',
  test_models: [],
  group: 'default',
  response_time: 1234,
  test_time: 1700000000,
};

/**
 * A channel that serves no chat-capable endpoint is reported by the backend as
 * {success:false, skipped:true}. The UI must present that as a neutral notice
 * rather than an outage, and must not overwrite the channel's recorded latency
 * with a probe that never ran.
 */
describe('ChannelsPage skipped channel test', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockApiGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return Promise.resolve({ data: { success: true, data: [channelRow], total: 1 } });
      }
      if (url.startsWith('/api/channel/test/')) {
        return Promise.resolve({
          data: {
            success: false,
            skipped: true,
            message: 'channel exposes no chat-capable endpoint to health check',
            time: 0,
            modelName: '',
          },
        });
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

  it('reports a skipped probe as info, never as a failure', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: /^test$/i }));

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/test/7');
    });

    await waitFor(() => expect(notify).toHaveBeenCalled());
    const call = notify.mock.calls.at(-1)?.[0];
    expect(call.type).toBe('info');
    expect(call.message).toContain('no chat-capable endpoint');
    expect(
      notify.mock.calls.some(([arg]) => arg?.type === 'error')
    ).toBe(false);
  });

  it('leaves the recorded latency untouched for a skipped probe', async () => {
    renderPage();
    await waitFor(() => expect(mockApiGet).toHaveBeenCalled());

    // The row renders the stored response time before the probe.
    expect(await screen.findByText('1234ms')).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: /^test$/i }));
    await waitFor(() => expect(notify).toHaveBeenCalled());

    // A probe that never ran must not restamp the row as "just tested, 0 ms".
    expect(screen.queryByText('0ms')).not.toBeInTheDocument();
    expect(screen.getByText('1234ms')).toBeInTheDocument();
  });
});
