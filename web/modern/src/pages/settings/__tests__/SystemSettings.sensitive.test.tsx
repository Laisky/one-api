import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

// These tests cover the sensitive-secret hardening: the plaintext secret must
// never be rendered visibly nor round-tripped back into the input after a save,
// and a stored secret must be removable through an explicit clear request.
describe('SystemSettings sensitive secret handling', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders a sensitive field as a password input and never repopulates the secret', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'SMTPServer', value: 'smtp.example.com' }],
      },
    });
    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

    const user = userEvent.setup();
    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    const input = (await screen.findByLabelText('SMTPToken value')) as HTMLInputElement;
    expect(input.type).toBe('password');

    await user.type(input, 'super-secret-token');
    const saveButton = input.parentElement?.querySelector('button') as HTMLButtonElement;
    await user.click(saveButton);

    // The real secret is sent to the backend, without a clear flag.
    await waitFor(() =>
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'SMTPToken',
        value: 'super-secret-token',
      })
    );
    expect(putMock).not.toHaveBeenCalledWith('/api/option/', expect.objectContaining({ clear: true }));

    // The parent → child prop sync must not push the plaintext secret back in.
    expect(input.value).toBe('');
  });

  it('removes a stored secret through an explicit clear request', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'SMTPServer', value: 'smtp.example.com' }],
      },
    });
    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

    const user = userEvent.setup();
    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    const clearButton = await screen.findByLabelText('Clear SMTPToken');
    await user.click(clearButton);

    await waitFor(() =>
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'SMTPToken',
        value: '',
        clear: true,
      })
    );
  });

  it('maps an empty EmailProvider to the Auto option', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'EmailProvider', value: '' }],
      },
    });
    vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    // The select trigger reflects the empty (auto) backend value as "Auto ...".
    expect(await screen.findByText(/Auto \(Resend if key set/)).toBeInTheDocument();
  });
});
