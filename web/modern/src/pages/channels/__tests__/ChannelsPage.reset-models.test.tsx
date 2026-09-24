import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { BrowserRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { channelResetTranslations } from '@/i18n/locales/channel-reset';
import { ChannelsPage } from '../ChannelsPage';

const { notify, responsive } = vi.hoisted(() => ({
  notify: vi.fn(),
  responsive: { isMobile: false, isTablet: false },
}));

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));
vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));
vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => responsive,
}));

const mockGet = vi.mocked(api.get);
const mockPost = vi.mocked(api.post);
const channels = [
  {
    uuid: '018fcf6d-c484-7000-8000-000000000101',
    name: 'Provider A',
    type: 1,
    status: 1,
    created_time: 1,
    priority: 0,
    weight: 0,
    models: 'old-model',
    group: 'default',
  },
  {
    uuid: '018fcf6d-c484-7000-8000-000000000102',
    name: 'Private B',
    type: 50,
    status: 2,
    created_time: 1,
    priority: 0,
    weight: 0,
    models: 'private-model',
    group: 'default',
  },
];

/** renderPage mounts the real list and confirmation dialogs, mocking only the API. */
function renderPage() {
  return render(
    <BrowserRouter>
      <ChannelsPage />
    </BrowserRouter>
  );
}

/** rowResetButton locates a visible row reset action after the list has loaded. */
async function rowResetButton(name = 'Provider A') {
  const buttons = await screen.findAllByRole('button', { name: `Reset ${name} to default models` });
  await waitFor(() => expect(buttons[0]).toBeEnabled());
  return buttons[0];
}

/** confirmReset accepts the actual destructive confirmation rather than bypassing it. */
async function confirmReset(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await screen.findByRole('dialog');
  await user.click(within(dialog).getByRole('button', { name: 'Reset models' }));
}

describe('ChannelsPage default model reset', () => {
  beforeEach(() => {
    // Preserve the shared DOM/observer implementations installed by test setup.
    vi.clearAllMocks();
    mockGet.mockReset();
    mockPost.mockReset();
    responsive.isMobile = false;
    window.history.replaceState({}, '', '/channels');
    mockGet.mockResolvedValue({ data: { success: true, data: channels, total: 25 } });
    mockPost.mockResolvedValue({
      data: { success: true, data: { uuid: channels[0].uuid, name: channels[0].name, success: true, model_count: 2 } },
    });
  });

  it('replaces the two old toolbar controls and exposes a reset for every row', async () => {
    renderPage();
    await rowResetButton();
    await rowResetButton('Private B');
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /refresh all balances/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /bulk actions/i })).not.toBeInTheDocument();
    expect(mockPost).not.toHaveBeenCalled();
  });

  it('does not send a mutation when the single-row confirmation is canceled', async () => {
    renderPage();
    const row = await rowResetButton();
    const user = userEvent.setup();
    await user.click(row);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText(/Channel status and compatible mappings\/pricing remain unchanged/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: /cancel/i }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(mockPost).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
  });

  it('posts the UUID with no client model list and refreshes the current page', async () => {
    window.history.replaceState({}, '', '/channels?p=2');
    renderPage();
    const button = await rowResetButton();
    mockGet.mockClear();
    const user = userEvent.setup();
    await user.click(button);
    expect(mockPost).not.toHaveBeenCalled();
    await confirmReset(user);
    await waitFor(() =>
      expect(mockPost).toHaveBeenCalledWith(`/api/channel/${channels[0].uuid}/reset_models`, undefined, { timeout: 120_000 })
    );
    expect(mockPost).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(mockGet).toHaveBeenCalledWith(expect.stringContaining('/api/channel/?p=1&')));
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success', message: 'Provider A now uses 2 default models.' }));
    expect(api.put).not.toHaveBeenCalled();
  });

  it.each([
    ['mapping_conflict', 'model_mapping', 'Model mappings reference models outside the default list.'],
    ['pricing_conflict', 'model_configs', 'Custom pricing references models outside the default list.'],
    ['unsupported_channel', 'type', 'Custom compatible channels cannot be reset to provider defaults.'],
  ])('shows a persistent, actionable %s rejection without a success message', async (code, field, reason) => {
    mockPost.mockRejectedValue({
      response: { status: 409, data: { success: false, message: 'Rejected', conflict: { code, field, models: ['Alias'] } } },
    });
    renderPage();
    const user = userEvent.setup();
    await user.click(await rowResetButton());
    await confirmReset(user);
    const report = await screen.findByRole('region', { name: 'Model reset results' });
    expect(within(report).getByText((text) => text.startsWith(reason))).toBeInTheDocument();
    expect(report).toHaveTextContent(channels[0].uuid);
    expect(report).toHaveTextContent(`Configuration field: ${field}.`);
    expect(report).toHaveTextContent('Conflicting models: Alias.');
    expect(report).toHaveTextContent('Reset 0 of 1 channels; 1 rejected; 0 failed.');
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
    await waitFor(() => expect(mockGet.mock.calls.length).toBeGreaterThan(1));
  });

  it('disables both row and selected-channel resets while a request is pending', async () => {
    let finish!: () => void;
    mockPost.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = () => resolve({ data: { success: true, data: { success: true, model_count: 2 } } });
        })
    );
    renderPage();
    const button = await rowResetButton();
    const user = userEvent.setup();
    await user.click(screen.getByRole('checkbox', { name: 'Select Provider A' }));
    await user.click(await rowResetButton());
    await confirmReset(user);
    await waitFor(() => expect(mockPost).toHaveBeenCalledTimes(1));
    const allButton = screen.getByRole('button', { name: 'Actions' });
    expect(allButton).toBeDisabled();
    const currentButton = screen.getAllByRole('button', { name: 'Reset Provider A to default models' })[0];
    expect(currentButton).toBeDisabled();
    // Disabled controls must also ignore programmatically dispatched clicks.
    fireEvent.click(allButton);
    fireEvent.click(currentButton);
    expect(mockPost).toHaveBeenCalledTimes(1);
    finish();
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'Reset Provider A to default models' })[0]).toBeEnabled());
  });

  it('reloads after an ambiguous transport failure and does not claim rollback', async () => {
    mockPost.mockRejectedValue(new Error('connection interrupted'));
    renderPage();
    const user = userEvent.setup();
    await user.click(await rowResetButton());
    await confirmReset(user);
    await waitFor(() =>
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: expect.stringContaining('a connection can fail after changes are saved'),
        })
      )
    );
    await waitFor(() => expect(mockGet.mock.calls.length).toBeGreaterThan(1));
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('does not treat an HTTP 200 failure envelope as a successful reset', async () => {
    mockPost.mockResolvedValue({ data: { success: false, message: 'Reset was refused.' } });
    renderPage();
    const user = userEvent.setup();
    await user.click(await rowResetButton());
    await confirmReset(user);
    await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'Reset was refused.' })));
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('also exposes the reset action in the mobile channel layout', async () => {
    responsive.isMobile = true;
    renderPage();
    expect(await rowResetButton()).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Select Provider A' }));
    await userEvent.click(screen.getByRole('button', { name: 'Actions' }));
    expect(screen.getByRole('menuitem', { name: 'Reset selected models' })).toBeInTheDocument();
  });

  it('provides the complete reset vocabulary in every supported locale', () => {
    for (const locale of Object.values(channelResetTranslations)) {
      expect(Object.keys(locale).sort()).toEqual(Object.keys(channelResetTranslations.en).sort());
      expect(Object.keys(locale.reasons).sort()).toEqual(Object.keys(channelResetTranslations.en.reasons).sort());
      expect(Object.values(locale.reasons).every((value) => value.trim().length > 0)).toBe(true);
    }
    expect(channelResetTranslations.zh.action).toBe('重置为默认模型');
    expect(channelResetTranslations.zh.all).toBe('全部重置');
  });
});
