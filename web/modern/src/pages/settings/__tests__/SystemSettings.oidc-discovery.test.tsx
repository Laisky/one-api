import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

describe('SystemSettings: OIDC well-known auto-discovery', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('fetches well-known endpoints and saves each via api.put', async () => {
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

    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

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

    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1));

    const fetchBtn = await screen.findByRole('button', { name: /fetch endpoints/i });
    await user.click(fetchBtn);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    expect(fetchMock).toHaveBeenCalledWith('https://issuer.example.com/.well-known/openid-configuration');

    await waitFor(() => {
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'OidcAuthorizationEndpoint',
        value: 'https://issuer.example.com/authorize',
      });
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'OidcTokenEndpoint',
        value: 'https://issuer.example.com/token',
      });
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'OidcUserinfoEndpoint',
        value: 'https://issuer.example.com/userinfo',
      });
    });

    expect(putMock.mock.calls.filter(([url]) => url === '/api/option/').length).toBeGreaterThanOrEqual(3);
  });
});
