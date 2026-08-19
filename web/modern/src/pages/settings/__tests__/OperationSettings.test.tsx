import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import OperationSettings from '../OperationSettings';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

describe('OperationSettings failure feedback', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
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
