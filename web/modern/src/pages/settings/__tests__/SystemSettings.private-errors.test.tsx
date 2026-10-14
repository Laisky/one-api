import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as privacy from '../../../../../legacy/browser-sentry-privacy.mjs';
import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

const notify = vi.fn();
vi.mock('@/components/ui/notifications', () => ({ useNotifications: () => ({ notify }) }));
vi.mock('../../../../../legacy/browser-sentry-privacy.mjs', { spy: true });

beforeEach(() => {
  vi.clearAllMocks();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

/** mountSettings initializes actual settings UI with an offline request failure fixture. */
async function mountSettings(error: Error, action: 'save' | 'clear') {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [{ key: 'SMTPServer', value: 'smtp.example.test' }] },
  });
  const put = vi.spyOn(api, 'put').mockRejectedValue(error);
  vi.spyOn(console, 'error').mockImplementation(() => {});
  const user = userEvent.setup();
  render(<SystemSettings />);
  if (action === 'clear') {
    await user.click(await screen.findByLabelText('Clear SMTPToken'));
  } else {
    const input = await screen.findByLabelText('SMTPToken value');
    await user.type(input, 'synthetic-private-submitted-secret');
    await user.click(input.parentElement!.querySelector('button')!);
  }
  await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: error.message })));
  return put;
}

describe.each(['save', 'clear'] as const)('private settings %s origin', (action) => {
  it('marks a raw Axios response failure without mutating identity, status, config or UI text', async () => {
    const error = Object.assign(new Error('synthetic-private-option-response'), {
      name: 'AxiosError',
      isAxiosError: true,
      response: { status: 400, data: { message: 'synthetic-private-option-response' } },
      config: { data: '{"value":"synthetic-private-submitted-secret"}' },
    });
    const stack = error.stack;
    const put = await mountSettings(error, action);
    expect(privacy.markPrivateError).toHaveBeenCalledWith(
      error,
      action === 'save' ? 'System settings save failed' : 'System settings clear failed',
      true
    );
    expect(error.message).toBe('synthetic-private-option-response');
    expect(error.stack).toBe(stack);
    expect(error.response.status).toBe(400);
    expect(error.config.data).toContain('synthetic-private-submitted-secret');
    expect(put).toHaveBeenCalledWith(
      '/api/option/',
      action === 'clear' ? { key: 'SMTPToken', value: '', clear: true } : { key: 'SMTPToken', value: 'synthetic-private-submitted-secret' }
    );
  });

  it('keeps an ordinary programmer TypeError unmarked and preserves its UI message', async () => {
    const error = new TypeError('ordinary settings implementation failure');
    await mountSettings(error, action);
    expect(privacy.markPrivateError).not.toHaveBeenCalled();
    expect(privacy.privateResponseError).not.toHaveBeenCalled();
    expect(error.message).toBe('ordinary settings implementation failure');
  });
});
