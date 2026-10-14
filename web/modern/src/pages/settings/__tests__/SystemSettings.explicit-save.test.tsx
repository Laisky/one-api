import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

const notify = vi.fn();
const savedConfig = '{"dsn":"https://11111111111111111111111111111111@o0.ingest.sentry.io/1","environment":"staging"}';
const changedConfig = '{"dsn":"https://22222222222222222222222222222222@o0.ingest.sentry.io/2","environment":"production"}';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

/** mockOptions returns synthetic persisted settings without requiring an authenticated API. */
function mockOptions(extraOptions: Array<{ key: string; value: string }> = []) {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        { key: 'BrowserSentryConfig', value: savedConfig },
        { key: 'SystemName', value: 'Original name' },
        { key: 'PasswordLoginEnabled', value: 'true' },
        { key: 'EmailProvider', value: 'smtp' },
        ...extraOptions,
      ],
    },
  });
}

/** findField returns the accessible setting control and its own Save action. */
async function findField(key = 'BrowserSentryConfig') {
  const input = await screen.findByLabelText(`${key} value`);
  const container = input.closest('.border.rounded-lg');
  expect(container).toBeInstanceOf(HTMLElement);
  const field = within(container as HTMLElement);
  return { input, field, save: field.getByRole('button', { name: /^save$/i }) };
}

describe('SystemSettings explicit text saves and JSON validation', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    notify.mockReset();
    mockOptions();
  });

  it('does not mutate a changed JSON setting when keyboard focus leaves it', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, save } = await findField();

    await user.click(input);
    fireEvent.change(input, { target: { value: changedConfig } });
    await user.tab();

    expect(input).not.toHaveFocus();
    expect(input).toHaveValue(changedConfig);
    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();

    await user.click(save);
    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: changedConfig,
      })
    );
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
  });

  it('does not save when clicking another text field or pressing Enter inside the setting', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input } = await findField();

    await user.click(input);
    fireEvent.change(input, { target: { value: changedConfig } });
    await user.keyboard('{Enter}');
    await user.click(screen.getByLabelText('SystemName value'));

    expect(input).not.toHaveFocus();
    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();
  });

  it('does not send a redundant request for an unchanged loaded value', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, save } = await findField();
    expect(input).toHaveValue(savedConfig);

    await user.click(input);
    await user.tab();
    await user.click(save);

    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();
  });

  it.each([
    ['incomplete object', '{"environment":'],
    ['trailing comma', '{"environment":"staging",}'],
    ['single quoted property', "{'environment':'staging'}"],
    ['JSON followed by text', '{"environment":"staging"} extra'],
    ['JSONC comment', '{"environment":/* comment */"staging"}'],
  ])('shows a field-local error and blocks malformed JSON: %s', async (_name, invalid) => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();

    await user.click(input);
    fireEvent.change(input, { target: { value: invalid } });
    await user.tab();
    await user.click(save);

    expect(input).toHaveValue(invalid);
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(field.getByRole('alert')).toHaveTextContent(/JSON/i);
    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();
  });

  it('clears the syntax error after correction and saves the complete JSON exactly once', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();
    fireEvent.change(input, { target: { value: '{"dsn":' } });
    await user.click(save);
    expect(field.getByRole('alert')).toHaveTextContent(/JSON/i);
    expect(put).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { value: changedConfig } });
    expect(field.queryByRole('alert')).not.toBeInTheDocument();
    expect(input).not.toHaveAttribute('aria-invalid', 'true');
    expect(put).not.toHaveBeenCalled();
    await user.click(save);

    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: changedConfig,
      })
    );
    await waitFor(() => expect(notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'success' })));
    expect(input).toHaveValue(changedConfig);
    expect(input).not.toBeDisabled();
  });

  it('keeps clearing the Sentry setting valid and requires Save to disable it', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();

    await user.clear(input);
    await user.tab();
    expect(input).toHaveValue('');
    expect(field.queryByRole('alert')).not.toBeInTheDocument();
    expect(put).not.toHaveBeenCalled();
    await user.click(save);

    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: '',
      })
    );
    expect(input).toHaveValue('');
    expect(input).not.toBeDisabled();
  });

  it.each(['server', 'network'] as const)('restores the saved JSON after %s failure without exposing submitted data', async (failure) => {
    const put = vi.spyOn(api, 'put');
    if (failure === 'server') {
      put.mockResolvedValue({ data: { success: false, message: `synthetic-private-detail ${changedConfig}` } });
    } else {
      put.mockRejectedValue({ message: 'synthetic-private-detail', config: { data: changedConfig } });
    }
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, save } = await findField();
    fireEvent.change(input, { target: { value: changedConfig } });
    await user.click(save);

    await waitFor(() =>
      expect(notify).toHaveBeenCalledExactlyOnceWith({
        type: 'error',
        title: 'Save failed',
        message: 'Save failed',
      })
    );
    expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
      key: 'BrowserSentryConfig',
      value: changedConfig,
    });
    expect(input).toHaveValue(savedConfig);
    expect(input).not.toBeDisabled();
    expect(JSON.stringify(notify.mock.calls)).not.toContain('synthetic-private-detail');
    expect(JSON.stringify(consoleError.mock.calls)).not.toContain('ingest.sentry.io');
    expect(JSON.stringify(consoleError.mock.calls)).not.toContain('synthetic-private-detail');
  });

  it('validates uncategorized GroupRatio JSON and saves only the corrected value explicitly', async () => {
    mockOptions([{ key: 'GroupRatio', value: '{"default":1}' }]);
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField('GroupRatio');
    expect(input).toHaveValue('{"default":1}');

    await user.click(input);
    fireEvent.change(input, { target: { value: '{"default":' } });
    await user.tab();
    await user.click(save);
    expect(field.getByRole('alert')).toHaveTextContent(/JSON/i);
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(put).not.toHaveBeenCalled();

    const corrected = '{"default":1,"synthetic-group":0.5}';
    await user.click(input);
    fireEvent.change(input, { target: { value: corrected } });
    await user.tab();
    expect(field.queryByRole('alert')).not.toBeInTheDocument();
    expect(input).not.toHaveAttribute('aria-invalid', 'true');
    expect(put).not.toHaveBeenCalled();
    await user.click(save);
    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'GroupRatio',
        value: corrected,
      })
    );
    await waitFor(() => expect(notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'success' })));
    expect(input).toHaveValue(corrected);
  });

  it.each(['', ' \n\t '])('rejects blank GroupRatio JSON without changing the persisted ratios: %j', async (blank) => {
    mockOptions([{ key: 'GroupRatio', value: '{"default":1}' }]);
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField('GroupRatio');
    expect(input).toHaveValue('{"default":1}');

    await user.click(input);
    fireEvent.change(input, { target: { value: blank } });
    await user.tab();
    expect(put).not.toHaveBeenCalled();
    await user.click(save);

    expect(input).toHaveValue(blank);
    expect(put).not.toHaveBeenCalled();
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(field.getByRole('alert')).toHaveTextContent(/JSON/i);
    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();
  });

  it('reports malformed JSON without displaying or logging its private input or parser details', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();
    const invalid = '{"dsn":"synthetic-private-json-marker","environment":';
    await user.click(input);
    fireEvent.change(input, { target: { value: invalid } });
    await user.tab();
    await user.click(save);

    const alert = field.getByRole('alert');
    expect(alert).toHaveTextContent(/JSON/i);
    expect(alert).not.toHaveTextContent('synthetic-private-json-marker');
    expect(alert.textContent).not.toContain(invalid);
    expect(alert).not.toHaveTextContent(/Unexpected|SyntaxError|position|property name/i);
    expect(input).toHaveValue(invalid);
    expect(put).not.toHaveBeenCalled();
    expect(notify).not.toHaveBeenCalled();
    expect(JSON.stringify([...consoleError.mock.calls, ...consoleWarn.mock.calls])).not.toContain('synthetic-private-json-marker');
  });

  it('accepts a whitespace-only Sentry value as an explicit clear without JSON errors', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();
    const whitespace = ' \n\t ';
    await user.click(input);
    fireEvent.change(input, { target: { value: whitespace } });
    await user.tab();
    expect(input).toHaveValue(whitespace);
    expect(field.queryByRole('alert')).not.toBeInTheDocument();
    expect(input).not.toHaveAttribute('aria-invalid', 'true');
    expect(put).not.toHaveBeenCalled();
    await user.click(save);

    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: whitespace,
      })
    );
    await waitFor(() => expect(notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'success' })));
    expect(input).not.toBeDisabled();
  });

  it('does not submit another request while an explicit save is in flight', async () => {
    let resolveSave!: (value: { data: { success: boolean } }) => void;
    const put = vi.spyOn(api, 'put').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve;
        })
    );
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField();
    fireEvent.change(input, { target: { value: changedConfig } });
    await user.click(save);
    expect(input).toBeDisabled();
    const saving = field.getByRole('button', { name: /saving/i });
    expect(saving).toBeDisabled();
    await user.click(saving);
    fireEvent.blur(input);
    expect(put).toHaveBeenCalledTimes(1);

    resolveSave({ data: { success: true } });
    await waitFor(() => expect(input).not.toBeDisabled());
    expect(input).toHaveValue(changedConfig);
    expect(put).toHaveBeenCalledTimes(1);
  });

  it('requires Save for ordinary text and does not treat arbitrary text as JSON', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input, field, save } = await findField('SystemName');
    await user.click(input);
    fireEvent.change(input, { target: { value: '{not JSON}' } });
    await user.tab();
    expect(put).not.toHaveBeenCalled();
    expect(field.queryByRole('alert')).not.toBeInTheDocument();

    await user.click(save);
    await waitFor(() =>
      expect(put).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'SystemName',
        value: '{not JSON}',
      })
    );
  });

  it('retains immediate saves for boolean and enumerated settings', async () => {
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);
    const { input: boolean } = await findField('PasswordLoginEnabled');
    await user.click(boolean);
    await user.click(screen.getByRole('option', { name: /^disabled$/i }));
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'PasswordLoginEnabled',
        value: 'false',
      })
    );

    const { input: provider } = await findField('EmailProvider');
    await user.click(provider);
    await user.click(screen.getByRole('option', { name: 'Resend (HTTP API)' }));
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/', {
        key: 'EmailProvider',
        value: 'resend',
      })
    );
    expect(put).toHaveBeenCalledTimes(2);
  });
});
