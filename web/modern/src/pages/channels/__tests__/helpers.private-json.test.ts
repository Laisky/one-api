import { afterEach, describe, expect, it, vi } from 'vitest';
import * as privacy from '../../../../../legacy/browser-sentry-privacy.mjs';
import { validateModelConfigs, validateToolingConfig } from '../helpers';

vi.mock('../../../../../legacy/browser-sentry-privacy.mjs', { spy: true });
afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe.each([validateModelConfigs, validateToolingConfig])('%s private pasted JSON', (validate) => {
  it('marks the original parsing error while retaining the validation UI diagnostic', () => {
    const error = new SyntaxError('synthetic-private-pasted-credential');
    const stack = error.stack;
    vi.spyOn(JSON, 'parse').mockImplementationOnce(() => {
      throw error;
    });
    expect(validate('synthetic-private-pasted-json')).toEqual({
      valid: false,
      error: 'Invalid JSON format: ' + error.message,
    });
    expect(privacy.markPrivateError).toHaveBeenCalledWith(error, 'Channel configuration decoding failed', true);
    expect(error.stack).toBe(stack);
    expect(error.message).toBe('synthetic-private-pasted-credential');
  });

  it('retains an ordinary programming failure without a private annotation', () => {
    const error = new TypeError('ordinary parser implementation failure');
    vi.spyOn(JSON, 'parse').mockImplementationOnce(() => {
      throw error;
    });
    expect(validate('{}')).toEqual({ valid: false, error: 'Invalid JSON format: ' + error.message });
    expect(privacy.markPrivateError).not.toHaveBeenCalled();
    expect(error.message).toBe('ordinary parser implementation failure');
  });
});
