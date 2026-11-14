import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { invalidateSystemStatusCache } from '@/lib/utils';
import OtherSettings from '../OtherSettings';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

vi.mock('@/lib/utils', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/utils')>()),
  invalidateSystemStatusCache: vi.fn(),
}));

describe('OtherSettings action feedback', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    notify.mockReset();
    vi.mocked(invalidateSystemStatusCache).mockClear();
  });

  it('shows an error when a save button returns success false', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'SystemName', value: 'One API' }],
      },
    });
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'branding save rejected' } });

    const user = userEvent.setup();
    render(<OtherSettings />);

    await screen.findByDisplayValue('One API');
    await user.click(screen.getAllByRole('button', { name: /save/i })[0]);

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'branding save rejected' }));
    });
    expect(invalidateSystemStatusCache).not.toHaveBeenCalled();
  });

  it('drops the cached /api/status after a successful save so the change shows immediately', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'SystemName', value: 'One API' }],
      },
    });
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

    const user = userEvent.setup();
    render(<OtherSettings />);

    await screen.findByDisplayValue('One API');
    await user.click(screen.getAllByRole('button', { name: /save/i })[0]);

    await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' })));
    expect(invalidateSystemStatusCache).toHaveBeenCalled();
  });
});
