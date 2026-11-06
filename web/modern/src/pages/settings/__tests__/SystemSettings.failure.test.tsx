import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { AxiosError, AxiosHeaders } from 'axios';
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
    const field = within(input.closest('.border.rounded-lg') as HTMLElement);
    await user.click(field.getByRole('button', { name: 'Save' }));

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
  it('surfaces the backend reason when a GroupRatio save is rejected with HTTP 400', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: [{ key: 'GroupRatio', value: '{"default":1}' }] },
    });
    const reason = "group name \"my group\" may only contain letters, digits, '.', '_' and '-'";
    const rejection = new AxiosError('Request failed with status code 400', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      statusText: 'Bad Request',
      headers: {},
      config: { headers: new AxiosHeaders() },
      data: { success: false, message: reason },
    });
    vi.spyOn(api, 'put').mockRejectedValue(rejection);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const user = userEvent.setup();

    render(<SystemSettings />);

    const editor = (await screen.findByLabelText('GroupRatio value')) as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: '{"default":1,"my group":2}' } });
    const field = within(editor.closest('.border.rounded-lg') as HTMLElement);
    await user.click(field.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: reason }));
    });
  });
});
