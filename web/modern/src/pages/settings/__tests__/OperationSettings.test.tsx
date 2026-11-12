import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AxiosError, AxiosHeaders } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { invalidateSystemStatusCache } from '@/lib/utils';
import OperationSettings from '../OperationSettings';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/lib/utils', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/utils')>()),
  invalidateSystemStatusCache: vi.fn(),
}));

describe('OperationSettings failure feedback', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.mocked(invalidateSystemStatusCache).mockClear();
    notify.mockReset();
  });

  it('shows an error when grouped save returns success false', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [
          { key: 'QuotaForNewUser', value: '0' },
          { key: 'QuotaForInviter', value: '0' },
          { key: 'QuotaForInvitee', value: '0' },
          { key: 'PreConsumedQuota', value: '0' },
        ],
      },
    });
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'quota save rejected' } });

    const user = userEvent.setup();
    render(<OperationSettings />);

    await screen.findByText('Quota Settings');
    await user.click(screen.getByRole('button', { name: 'Save Quota Settings' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'quota save rejected',
        })
      );
    });
  });

  it('shows an error when clearing logs returns success false', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } });
    vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: false, message: 'clear rejected' } });

    const user = userEvent.setup();
    render(<OperationSettings />);

    await screen.findByText('Log Management');
    await user.click(screen.getByRole('button', { name: 'Clear Logs Before This Date' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'clear rejected',
        })
      );
    });
  });
});

/** mockGroupRatio serves an option list whose stored GroupRatio is `stored`. */
const mockGroupRatio = (stored: Record<string, number>) =>
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [{ key: 'GroupRatio', value: JSON.stringify(stored) }] },
  });

/** ISSUE_ONLY makes text queries skip the editor itself, whose content echoes the keys. */
const ISSUE_ONLY = { ignore: 'script, style, textarea' };

/** editGroupRatio replaces the GroupRatio editor text with `next` encoded as JSON. */
const editGroupRatio = async (next: Record<string, number>) => {
  // The label also wraps a help button, so locate the editor by its stable id.
  const textarea = await waitFor(() => {
    const element = document.getElementById('group-ratio-textarea') as HTMLTextAreaElement | null;
    expect(element?.value ?? '').toContain('default');
    return element as HTMLTextAreaElement;
  });
  // fireEvent avoids user-event's special handling of "{" in typed text.
  fireEvent.change(textarea, { target: { value: JSON.stringify(next, null, 2) } });
};

describe('OperationSettings group ratio names', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.mocked(invalidateSystemStatusCache).mockClear();
    notify.mockReset();
  });

  it('rejects a new group whose name breaks the naming rule and names the key and reason', async () => {
    mockGroupRatio({ default: 1, vip: 0.8 });
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, vip: 0.8, 'my group': 0.5 });

    const issue = await screen.findByText(/"my group"/, ISSUE_ONLY);
    expect(issue).toHaveTextContent('may only contain letters, digits');
    expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeDisabled();
  });

  it('rejects a new group with surrounding whitespace instead of trimming it', async () => {
    mockGroupRatio({ default: 1 });
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, ' vip': 1 });

    expect(await screen.findByText(/" vip"/, ISSUE_ONLY)).toHaveTextContent('must not start or end with whitespace');
    expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeDisabled();
  });

  it('keeps an unchanged legacy group editable and saves it', async () => {
    mockGroupRatio({ default: 1, 'legacy group': 1 });
    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, 'legacy group': 0.5 });

    const save = screen.getByRole('button', { name: 'Save Group Ratio' });
    await waitFor(() => expect(save).toBeEnabled());
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument();
    await user.click(save);

    await waitFor(() => {
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'GroupRatio',
        value: JSON.stringify({ default: 1, 'legacy group': 0.5 }),
      });
    });
    // /api/status is cached briefly, so a successful save must drop the cached copy.
    await waitFor(() => expect(invalidateSystemStatusCache).toHaveBeenCalled());
  });

  it('rejects a new group that differs from another only by letter case', async () => {
    mockGroupRatio({ default: 1, gold: 1 });
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, gold: 1, Gold: 0.5 });

    const issue = await screen.findByText(/differ only by letter case/, ISSUE_ONLY);
    expect(issue).toHaveTextContent('"gold"');
    expect(issue).toHaveTextContent('"Gold"');
    expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeDisabled();
  });

  it('rejects two new groups that differ only by letter case', async () => {
    mockGroupRatio({ default: 1 });
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, Gold: 1, gold: 0.5 });

    expect(await screen.findByText(/differ only by letter case/, ISSUE_ONLY)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeDisabled();
  });

  it('keeps legacy groups that already differ only by case editable', async () => {
    mockGroupRatio({ default: 1, Gold: 1, gold: 1 });
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, Gold: 0.9, gold: 1 });

    await waitFor(() => expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeEnabled());
    expect(screen.queryByText(/differ only by letter case/, ISSUE_ONLY)).not.toBeInTheDocument();
  });

  it('still rejects a negative ratio', async () => {
    mockGroupRatio({ default: 1 });
    render(<OperationSettings />);

    await editGroupRatio({ default: -1 });

    expect(await screen.findByText(/must use a number ≥ 0/, ISSUE_ONLY)).toHaveTextContent('default');
    expect(screen.getByRole('button', { name: 'Save Group Ratio' })).toBeDisabled();
  });

  it('shows the backend reason when the server rejects a GroupRatio save', async () => {
    mockGroupRatio({ default: 1 });
    const reason = 'groups "Gold" and "gold" differ only by case; group names must be unique ignoring case';
    vi.spyOn(api, 'put').mockRejectedValue(
      new AxiosError('Request failed with status code 400', 'ERR_BAD_REQUEST', undefined, undefined, {
        status: 400,
        statusText: 'Bad Request',
        headers: {},
        config: { headers: new AxiosHeaders() },
        data: { success: false, message: reason },
      })
    );
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const user = userEvent.setup();
    render(<OperationSettings />);

    await editGroupRatio({ default: 1, vip: 0.5 });
    await user.click(screen.getByRole('button', { name: 'Save Group Ratio' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: reason }));
    });
    expect(screen.getByText('Unsaved changes')).toBeInTheDocument();
  });
});
