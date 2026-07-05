import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { LogsPage } from '../LogsPage';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/components/ui/confirm-dialog', () => ({
  useConfirmDialog: () => [vi.fn().mockResolvedValue(true), () => null],
}));

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), delete: vi.fn() },
}));

describe('LogsPage action feedback', () => {
  beforeEach(() => {
    notify.mockReset();
    useAuthStore.setState({
      user: {
        id: 1,
        username: 'admin',
        role: 10,
        status: 1,
        quota: 0,
        used_quota: 0,
        group: 'default',
      } as any,
      token: 'token',
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: vi.fn() as any,
    });
    (api.get as any).mockReset();
    (api.delete as any).mockReset();
    (api.get as any).mockResolvedValue({ data: { success: true, data: [], total: 0 } });
  });

  it('shows an error when clear logs returns success false', async () => {
    (api.delete as any).mockResolvedValue({ data: { success: false, message: 'clear logs rejected' } });

    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    const clearButton = await screen.findByRole('button', { name: 'Clear' });
    await user.click(clearButton);

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'clear logs rejected' }));
    });
  });

  it('shows channel names and reveals channel UUIDs from the channel column', async () => {
    const user = userEvent.setup();
    (api.get as any).mockImplementation((url: string) => {
      if (url.startsWith('/api/log/stat')) {
        return Promise.resolve({ data: { success: true, data: { quota: 0 } } });
      }
      return Promise.resolve({
        data: {
          success: true,
          data: [
            {
              uuid: '018f0000-0000-7000-8000-000000000901',
              user_uuid: '018f0000-0000-7000-8000-000000000101',
              created_at: 1710000000,
              type: 2,
              content: 'consume log',
              username: 'admin',
              token_name: 'default',
              token_uuid: '018f0000-0000-7000-8000-000000000301',
              model_name: 'gpt-4o',
              origin_model_name: 'gpt-4o',
              quota: 100,
              prompt_tokens: 10,
              completion_tokens: 5,
              channel_uuid: '018f0000-0000-7000-8000-000000000207',
              channel_name: 'OpenAI Primary',
              request_id: 'req-1',
              trace_id: 'trace-1',
              updated_at: 1710000000000,
              elapsed_time: 120,
              is_stream: false,
              system_prompt_reset: false,
              cached_prompt_tokens: 0,
              metadata: {},
            },
          ],
          total: 1,
        },
      });
    });

    render(
      <MemoryRouter>
        <LogsPage />
      </MemoryRouter>
    );

    const channelButton = await screen.findByRole('button', { name: 'OpenAI Primary' });
    expect(channelButton).toBeInTheDocument();

    await user.click(channelButton);

    expect(screen.getAllByText('Channel: 018f0000-0000-7000-8000-000000000207').length).toBeGreaterThan(0);
  });
});
