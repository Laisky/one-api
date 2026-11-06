import { api } from '@/lib/api';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EditChannelPage } from '../EditChannelPage';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}));

const mockApiGet = vi.mocked(api.get);

/** GroupResponse is what the mocked `/api/group/` endpoint does on one call. */
type GroupResponse = { reject: true } | { body: unknown };

/**
 * mockChannelEditApi routes every GET the channel editor makes. The channel
 * under edit is assigned only to `channelGroups`, and each `/api/group/` call
 * consumes the next entry of `groupResponses` (the last one repeats).
 */
function mockChannelEditApi(channelGroups: string, groupResponses: GroupResponse[]) {
  let groupCall = 0;
  mockApiGet.mockImplementation((url: string) => {
    if (url.startsWith('/api/group/')) {
      const response = groupResponses[Math.min(groupCall, groupResponses.length - 1)];
      groupCall += 1;
      if ('reject' in response) {
        return Promise.reject(new Error('network down'));
      }
      return Promise.resolve({ data: response.body });
    }
    if (url.startsWith('/api/channel/default-pricing')) {
      return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
    }
    if (url.startsWith('/api/channel/metadata')) {
      return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.openai.com' } } });
    }
    if (url.startsWith('/api/channel/1')) {
      return Promise.resolve({
        data: {
          success: true,
          data: { id: 1, type: 1, name: 'Test Channel', models: 'gpt-4o', group: channelGroups, config: '{}', tooling: '{}' },
        },
      });
    }
    if (url.startsWith('/api/models')) {
      return Promise.resolve({ data: { success: true, data: { 1: ['gpt-4o'] } } });
    }
    return Promise.resolve({ data: { success: false } });
  });
}

/** renderEditChannel renders the channel editor for channel 1 inside a router. */
function renderEditChannel() {
  return render(
    <MemoryRouter initialEntries={['/channels/edit/1']}>
      <Routes>
        <Route path="/channels/edit/:id" element={<EditChannelPage />} />
      </Routes>
    </MemoryRouter>
  );
}

/** openGroupOptions focuses the groups combobox and returns the offered option labels. */
async function openGroupOptions(user: ReturnType<typeof userEvent.setup>): Promise<string[]> {
  await user.click(await screen.findByRole('combobox', { name: 'Groups *' }));
  const listbox = await screen.findByRole('listbox');
  return within(listbox)
    .getAllByRole('option')
    .map((option) => option.textContent ?? '');
}

describe('EditChannelPage groups catalog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows a retryable error instead of silently offering "default" when the group catalog request fails', async () => {
    mockChannelEditApi('vip', [{ reject: true }, { body: { success: true, data: ['vip', 'default', 'svip'] } }]);
    const user = userEvent.setup();
    renderEditChannel();

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('The configured groups could not be loaded');
    expect(alert).toHaveTextContent('groups already selected are kept');

    // The stored group stays visible and removable; nothing else is invented.
    expect(screen.getByRole('button', { name: 'Remove group vip' })).toBeInTheDocument();
    expect(await openGroupOptions(user)).toEqual(['vip']);

    await user.click(within(alert).getByRole('button', { name: 'Retry' }));

    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    expect(await openGroupOptions(user)).toEqual(['default', 'svip', 'vip']);
  });

  it('treats a success:false catalog response as a load failure', async () => {
    mockChannelEditApi('vip', [{ body: { success: false, message: 'forbidden' } }]);
    const user = userEvent.setup();
    renderEditChannel();

    expect(await screen.findByRole('alert')).toHaveTextContent('The configured groups could not be loaded');
    expect(await openGroupOptions(user)).toEqual(['vip']);
  });

  it('does not offer catalog keys that break the group-name rule', async () => {
    mockChannelEditApi('default', [{ body: { success: true, data: ['default', ' vip', 'my group', 'gold'] } }]);
    const user = userEvent.setup();
    renderEditChannel();

    await screen.findByRole('button', { name: 'Remove group default' });
    await waitFor(async () => {
      expect(await openGroupOptions(user)).toEqual(['default', 'gold']);
    });
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
