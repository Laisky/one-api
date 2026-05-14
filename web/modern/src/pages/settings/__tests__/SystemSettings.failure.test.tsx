import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

const notify = vi.fn();

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

describe('SystemSettings failure handling', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    notify.mockReset();
  });

  it('restores the previous value when save returns success false', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ key: 'SystemName', value: 'Original Name' }],
      },
    });

    const putMock = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'save rejected' },
    });

    const user = userEvent.setup();

    render(<SystemSettings />);

    const input = (await screen.findByLabelText('SystemName value')) as HTMLInputElement;
    await user.clear(input);
    await user.type(input, 'Updated Name');
    await user.click(screen.getAllByRole('button', { name: 'Save' })[0]);

    await waitFor(() => {
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'SystemName',
        value: 'Updated Name',
      });
    });

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'save rejected',
        })
      );
    });

    expect(input.value).toBe('Original Name');
  });

  it('shows an error when oidc discovery save returns success false', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [
          { key: 'OidcEnabled', value: 'true' },
          { key: 'OidcWellKnown', value: 'https://issuer.example.com/.well-known/openid-configuration' },
          { key: 'OidcAuthorizationEndpoint', value: '' },
          { key: 'OidcTokenEndpoint', value: '' },
          { key: 'OidcUserinfoEndpoint', value: '' },
        ],
      },
    });

    vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'endpoint save failed' },
    });

    const fetchMock = vi.fn().mockResolvedValue({
      json: () =>
        Promise.resolve({
          authorization_endpoint: 'https://issuer.example.com/authorize',
          token_endpoint: 'https://issuer.example.com/token',
          userinfo_endpoint: 'https://issuer.example.com/userinfo',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const user = userEvent.setup();

    render(<SystemSettings />);

    const button = await screen.findByRole('button', { name: /fetch endpoints/i });
    await user.click(button);

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          message: 'endpoint save failed',
        })
      );
    });
  });
});
