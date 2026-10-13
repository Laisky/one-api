import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest';

import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

const notify = vi.fn();

// These public-format fixtures are synthetic and are never sent to a Sentry server.
const savedConfig = JSON.stringify({
  dsn: 'https://11111111111111111111111111111111@o0.ingest.sentry.io/1',
  environment: 'staging',
});
const updatedConfig = JSON.stringify({
  dsn: 'https://22222222222222222222222222222222@o0.ingest.sentry.io/2',
  environment: 'production',
});

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify }),
}));

// mockSavedOptions makes the settings read return one public configuration value.
function mockSavedOptions(value = savedConfig) {
  return vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [{ key: 'BrowserSentryConfig', value }],
    },
  });
}

// findConfigField returns the loaded configuration input and its own field container.
async function findConfigField() {
  const input = (await screen.findByLabelText('BrowserSentryConfig value')) as HTMLInputElement;
  const container = input.closest('.border.rounded-lg');
  expect(container).toBeInstanceOf(HTMLElement);
  return { input, field: within(container as HTMLElement) };
}

// expectSafeDiagnostics verifies notifications and console calls omit public configuration and error details.
function expectSafeDiagnostics(consoleError: MockInstance<typeof console.error>, consoleWarn: MockInstance<typeof console.warn>) {
  const notifications = JSON.stringify(notify.mock.calls);
  expect(notifications).not.toContain(savedConfig);
  expect(notifications).not.toContain(updatedConfig);
  expect(notifications).not.toContain('ingest.sentry.io');
  expect(notifications).not.toContain('synthetic-error-detail');
  expect(notifications).not.toContain('synthetic-response-detail');
  for (const call of [...consoleError.mock.calls, ...consoleWarn.mock.calls]) {
    for (const argument of call) {
      expect(typeof argument).toBe('string');
      expect(argument).not.toContain('ingest.sentry.io');
      expect(argument).not.toContain('synthetic-error-detail');
      expect(argument).not.toContain('synthetic-response-detail');
    }
  }
}

describe('SystemSettings browser Sentry configuration', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    notify.mockReset();
  });

  it('loads the saved JSON as one field without a separate activation control', async () => {
    const getMock = mockSavedOptions();
    const putMock = vi.spyOn(api, 'put');
    render(<SystemSettings />);

    const { input, field } = await findConfigField();
    expect(input).toHaveValue(savedConfig);
    expect(input.type).toBe('text');
    expect(getMock).toHaveBeenCalledExactlyOnceWith('/api/option/');
    expect(putMock).not.toHaveBeenCalled();
    expect(screen.getAllByLabelText(/^BrowserSentry.* value$/)).toHaveLength(1);
    expect(field.queryByRole('combobox')).not.toBeInTheDocument();
    expect(field.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument();
    expect(notify).not.toHaveBeenCalled();
  });

  it('saves the complete public JSON with exactly one atomic option request', async () => {
    mockSavedOptions();
    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);

    const { input, field } = await findConfigField();
    fireEvent.change(input, { target: { value: updatedConfig } });
    await user.click(field.getByRole('button', { name: /^save$/i }));

    await waitFor(() => {
      expect(putMock).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: updatedConfig,
      });
      expect(notify).toHaveBeenCalledExactlyOnceWith({
        type: 'success',
        title: 'Setting saved',
        message: 'BrowserSentryConfig updated successfully.',
      });
    });
    expect(input).toHaveValue(updatedConfig);
    expect(input).not.toBeDisabled();
  });

  it('clears the setting by saving an empty value without an activation or clear flag', async () => {
    mockSavedOptions();
    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });
    const user = userEvent.setup();
    render(<SystemSettings />);

    const { input, field } = await findConfigField();
    expect(field.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument();
    await user.clear(input);
    await user.tab();

    await waitFor(() => {
      expect(putMock).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'BrowserSentryConfig',
        value: '',
      });
      expect(notify).toHaveBeenCalledExactlyOnceWith({
        type: 'success',
        title: 'Setting saved',
        message: 'BrowserSentryConfig updated successfully.',
      });
    });
    expect(input).toHaveValue('');
    expect(input).not.toBeDisabled();
  });

  it.each(['server rejection', 'network rejection'] as const)(
    'restores the saved configuration and reports a generic error after %s',
    async (failure) => {
      mockSavedOptions();
      const putMock = vi.spyOn(api, 'put');
      if (failure === 'server rejection') {
        putMock.mockResolvedValue({
          data: { success: false, message: 'synthetic-response-detail ' + updatedConfig },
        });
      } else {
        putMock.mockRejectedValue({
          message: 'synthetic-error-detail',
          config: { data: updatedConfig },
          response: { data: { message: 'synthetic-response-detail' } },
        });
      }
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined);
      const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      const user = userEvent.setup();
      render(<SystemSettings />);

      const { input, field } = await findConfigField();
      fireEvent.change(input, { target: { value: updatedConfig } });
      await user.click(field.getByRole('button', { name: /^save$/i }));

      await waitFor(() => {
        expect(putMock).toHaveBeenCalledExactlyOnceWith('/api/option/', {
          key: 'BrowserSentryConfig',
          value: updatedConfig,
        });
        expect(notify).toHaveBeenCalledExactlyOnceWith({
          type: 'error',
          title: 'Save failed',
          message: 'Save failed',
        });
        expect(input).toHaveValue(savedConfig);
        expect(input).not.toBeDisabled();
      });
      expectSafeDiagnostics(consoleError, consoleWarn);
    }
  );
});
