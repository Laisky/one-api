import { api } from '@/lib/api';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterAll, beforeAll, beforeEach, describe, expect, test, vi } from 'vitest';
import { RegisterPage } from '../RegisterPage';

// Render a trivial stand-in for the Cloudflare widget so we can drive onVerify/onExpire
// deterministically instead of loading the real challenge iframe.
vi.mock('@/components/Turnstile', () => ({
  __esModule: true,
  default: ({ onVerify, className }: { onVerify?: (token: string) => void; className?: string }) => (
    <div data-testid="turnstile-mock" className={className} onClick={() => onVerify?.('mock-token')}>
      TurnstileMock
    </div>
  ),
}));

const originalLocalStorage = window.localStorage;
const storage: Record<string, string> = {};
const storageMock = {
  getItem: (key: string) => (key in storage ? storage[key] : null),
  setItem: (key: string, value: string) => {
    storage[key] = value;
  },
  removeItem: (key: string) => {
    delete storage[key];
  },
  clear: () => {
    for (const key of Object.keys(storage)) delete storage[key];
  },
};

const status = { turnstile_check: true, turnstile_site_key: 'site-key', email_verification: true, github_oauth: false };
const statusResponse = { data: { success: true, data: status } };

// Renders the page and drives it up to the point where a Turnstile token is held and a
// correctly formatted verification code has been entered.
async function setup() {
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/register']}>
      <RegisterPage />
    </MemoryRouter>
  );
  const widget = await screen.findByTestId('turnstile-mock');
  return { user, widget };
}

const submitButton = () => screen.getByRole('button', { name: /create account/i });

describe('RegisterPage Turnstile integration', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'localStorage', { value: storageMock, configurable: true });
  });

  afterAll(() => {
    Object.defineProperty(window, 'localStorage', { value: originalLocalStorage, configurable: true });
  });

  beforeEach(() => {
    storage['status'] = JSON.stringify(status);
    vi.restoreAllMocks();
    vi.spyOn(api, 'get').mockImplementation((url: string) => {
      if (url.startsWith('/api/status')) return Promise.resolve(statusResponse as any);
      // /api/verification (Send Code)
      return Promise.resolve({ data: { success: true, message: '' } } as any);
    });
  });

  test('renders the Turnstile widget above the Create Account button', async () => {
    const { widget } = await setup();
    // Node.DOCUMENT_POSITION_FOLLOWING (4) means the button comes after the widget in the DOM.
    expect(widget.compareDocumentPosition(submitButton()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test('enables Create Account only with both a valid code and a Turnstile token', async () => {
    const { user, widget } = await setup();

    // Nothing entered yet -> disabled.
    expect(submitButton()).toBeDisabled();

    // Valid code but no token -> still disabled (Turnstile is configured).
    await user.type(screen.getByPlaceholderText(/enter verification code/i), 'abcdef');
    expect(submitButton()).toBeDisabled();

    // Complete Turnstile -> now both conditions met -> enabled.
    await user.click(widget);
    await waitFor(() => expect(submitButton()).not.toBeDisabled());
  });

  test('rejects a malformed verification code', async () => {
    const { user, widget } = await setup();
    await user.click(widget); // hold a token
    await user.type(screen.getByPlaceholderText(/enter verification code/i), 'abc'); // too short
    expect(submitButton()).toBeDisabled();
  });

  test('keeps the token after Send Code so the button is never stranded', async () => {
    // Regression: the old code wiped the token after "Send Code" without resetting the
    // single-use widget, leaving "Create Account" permanently grey.
    const { user, widget } = await setup();
    await user.click(widget); // token obtained

    await user.type(screen.getByPlaceholderText(/enter email/i), 'user@example.com');
    const sendCode = screen.getByRole('button', { name: /send code/i });
    await waitFor(() => expect(sendCode).not.toBeDisabled());
    await user.click(sendCode);

    // With a valid code entered, the button must stay enabled after sending the code.
    await user.type(screen.getByPlaceholderText(/enter verification code/i), 'abcdef');
    await waitFor(() => expect(submitButton()).not.toBeDisabled());
  });

  test('submits registration with the held token once the form is complete', async () => {
    const postSpy = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } } as any);
    const { user, widget } = await setup();
    await user.click(widget);

    await user.type(screen.getByPlaceholderText(/enter username/i), 'newuser');
    await user.type(screen.getByPlaceholderText(/enter password/i), 'password123');
    await user.type(screen.getByPlaceholderText(/confirm password/i), 'password123');
    await user.type(screen.getByPlaceholderText(/enter email/i), 'user@example.com');
    await user.type(screen.getByPlaceholderText(/enter verification code/i), 'abcdef');

    await waitFor(() => expect(submitButton()).not.toBeDisabled());
    await user.click(submitButton());

    await waitFor(() => expect(postSpy).toHaveBeenCalled());
    const [path] = postSpy.mock.calls[0];
    expect(path).toBe('/api/user/register?turnstile=mock-token');
  });
});
