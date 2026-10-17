import { api } from '@/lib/api';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RegisterPage } from '../RegisterPage';

vi.mock('@/hooks/useSystemStatus', () => ({
  useSystemStatus: () => ({ systemStatus: { turnstile_check: false, github_oauth: false } }),
}));
vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }));

/** setup renders the real registration form and returns a user-event driver and its labeled password controls. */
function setup() {
  const user = userEvent.setup();
  render(<MemoryRouter initialEntries={['/register?aff=KEEP123']}><RegisterPage /></MemoryRouter>);
  return {
    user,
    password: screen.getByLabelText(/^password$/i),
    confirmation: screen.getByLabelText(/^confirm password$/i),
  };
}

/** completeOtherFields fills the unrelated required fields using the supplied driver, preserving the invitation URL. */
async function completeOtherFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/^username$/i), 'newuser');
  await user.type(screen.getByLabelText(/^email$/i), 'user@example.com');
  await user.type(screen.getByPlaceholderText(/enter verification code/i), 'Ab1234');
}

describe('RegisterPage live password feedback', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as Awaited<ReturnType<typeof api.post>>);
  });
  afterEach(cleanup);

  it('shows neutral requirements before interaction and updates individual checks while typing', async () => {
    const { user, password, confirmation } = setup();
    const checklist = screen.getByRole('list', { name: 'Password requirements' });
    expect(within(checklist).getAllByRole('listitem')).toHaveLength(4);
    expect(screen.getByText('0 of 4 requirements met')).toBeInTheDocument();
    expect(password).toHaveAttribute('aria-invalid', 'false');
    expect(confirmation).toHaveAttribute('aria-invalid', 'false');
    expect(password).toHaveAccessibleDescription(/10–30 characters/);

    await user.type(password, 'Abcdefg12');
    expect(screen.getByText('3 of 4 requirements met')).toBeInTheDocument();
    await user.type(password, '3');
    expect(screen.getByText('Password meets all requirements.')).toBeInTheDocument();
    expect(within(checklist).getAllByText('Met:')).toHaveLength(4);
    await user.clear(password);
    expect(screen.getByText('0 of 4 requirements met')).toBeInTheDocument();
    expect(screen.queryByText('Password meets all requirements.')).not.toBeInTheDocument();
  });

  it('recomputes confirmation when either field changes, including after clearing the original', async () => {
    const { user, password, confirmation } = setup();
    await user.type(password, 'Password12');
    await user.type(confirmation, 'Password1');
    expect(screen.getByText('Passwords do not match.')).toBeInTheDocument();
    expect(confirmation).toHaveAttribute('aria-invalid', 'true');
    await user.type(confirmation, '2');
    expect(screen.getByText('Passwords match.')).toBeInTheDocument();
    await user.type(password, '3');
    expect(screen.getByText('Passwords do not match.')).toBeInTheDocument();
    await user.keyboard('{Backspace}');
    expect(screen.getByText('Passwords match.')).toBeInTheDocument();
    await user.clear(password);
    expect(screen.getByText('Passwords do not match.')).toBeInTheDocument();
    await user.clear(confirmation);
    await user.tab();
    expect(screen.getByText('Please confirm your password.')).toBeInTheDocument();
  });

  it('provides independent keyboard-operable visibility toggles without submitting or changing values', async () => {
    const { user, password, confirmation } = setup();
    await completeOtherFields(user);
    await user.type(password, 'Password12');
    await user.type(confirmation, 'Password12');
    const show = screen.getByRole('button', { name: 'Show password' });
    expect(show).toHaveAttribute('type', 'button');
    expect(show).toHaveAttribute('aria-controls', password.id);
    show.focus();
    await user.keyboard('{Enter}');
    expect(password).toHaveAttribute('type', 'text');
    expect(confirmation).toHaveAttribute('type', 'password');
    await user.click(screen.getByRole('button', { name: 'Show confirmation password' }));
    expect(confirmation).toHaveAttribute('type', 'text');
    await user.click(screen.getByRole('button', { name: 'Hide password' }));
    await user.click(screen.getByRole('button', { name: 'Hide confirmation password' }));
    for (const input of [password, confirmation]) {
      expect(input).toHaveValue('Password12');
      expect(input).toHaveAttribute('type', 'password');
      expect(input).toHaveAttribute('autocomplete', 'new-password');
      expect(input).toHaveAttribute('spellcheck', 'false');
    }
    expect(api.post).not.toHaveBeenCalled();
  });

  it.each(['Abcdefg12', 'abcdefghij', '1234567890', 'Abcdefg12é', 'Abcdefg12😀', 'A1' + 'x'.repeat(29), ' Abcdefgh12', 'Abcde fgh12', 'Abcdefgh12 ', 'Abcdefgh12\t', 'Abcdefgh12\u00a0', 'Abcdefgh12\u200b'])(
    'blocks an invalid password %j before the API call without silently truncating it', async (value) => {
      const { user, password, confirmation } = setup();
      await completeOtherFields(user);
      fireEvent.change(password, { target: { value } });
      fireEvent.change(confirmation, { target: { value } });
      expect(password).toHaveValue(value);
      expect(screen.queryByText('Password meets all requirements.')).not.toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: /create account/i }));
      await waitFor(() => expect(password).toHaveAttribute('aria-invalid', 'true'));
      expect(api.post).not.toHaveBeenCalled();
    }
  );

  it.each(['', 'Different1'])('blocks missing or mismatched confirmation %j and focuses that field', async (value) => {
    const { user, password, confirmation } = setup();
    await completeOtherFields(user);
    await user.type(password, 'Password12');
    fireEvent.change(confirmation, { target: { value } });
    await user.click(screen.getByRole('button', { name: /create account/i }));
    await waitFor(() => expect(confirmation).toHaveFocus());
    expect(confirmation).toHaveAttribute('aria-invalid', 'true');
    expect(api.post).not.toHaveBeenCalled();
  });

  it('recovers from a submitted mismatch by editing the original password only', async () => {
    const { user, password, confirmation } = setup();
    await completeOtherFields(user);
    await user.type(password, 'Password12');
    await user.type(confirmation, 'Password123');
    await user.click(screen.getByRole('button', { name: /create account/i }));
    await waitFor(() => expect(confirmation).toHaveFocus());
    await user.type(password, '3');
    expect(screen.getByText('Passwords match.')).toBeInTheDocument();
    expect(confirmation).toHaveAttribute('aria-invalid', 'false');
    await user.click(screen.getByRole('button', { name: /create account/i }));
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
  });

  it.each(['Abcdefgh12', 'A1' + 'x'.repeat(18), 'A1' + 'x'.repeat(19), 'A1' + 'x'.repeat(27), 'A1' + 'x'.repeat(28), 'Abcdefg12!@#_-', `Abcdefg12'\"\\/<>`])(
    'accepts pasted passwords %j unchanged and sends neither confirmation nor new payload fields', async (value) => {
      const { user, password, confirmation } = setup();
      await completeOtherFields(user);
      await user.click(password);
      await user.paste(value);
      await user.click(confirmation);
      await user.paste(value);
      expect(screen.getByText('Password meets all requirements.')).toBeInTheDocument();
      expect(screen.getByText('Passwords match.')).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: /create account/i }));
      await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
      expect(api.post).toHaveBeenCalledWith('/api/user/register', {
        username: 'newuser', password: value, email: 'user@example.com', verification_code: 'Ab1234', aff_code: 'KEEP123',
      });
    }
  );

  it('rejects pasted spaces without trimming and recovers after an explicit correction', async () => {
    const { user, password, confirmation } = setup();
    await completeOtherFields(user);
    const original = ' Abcdefgh12 ';
    await user.click(password);
    await user.paste(original);
    await user.click(confirmation);
    await user.paste(original);
    expect(password).toHaveValue(original);
    expect(confirmation).toHaveValue(original);
    expect(password).toHaveAccessibleDescription(/no spaces/i);
    await user.click(screen.getByRole('button', { name: /create account/i }));
    await waitFor(() => expect(password).toHaveFocus());
    expect(api.post).not.toHaveBeenCalled();
    await user.clear(password);
    await user.paste('Abcdefgh12!');
    await user.clear(confirmation);
    await user.paste('Abcdefgh12!');
    expect(screen.getByText('Password meets all requirements.')).toBeInTheDocument();
    expect(screen.getByText('Passwords match.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /create account/i }));
    await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1));
    expect(api.post).toHaveBeenCalledWith('/api/user/register', expect.objectContaining({ password: 'Abcdefgh12!' }));
  });
});
