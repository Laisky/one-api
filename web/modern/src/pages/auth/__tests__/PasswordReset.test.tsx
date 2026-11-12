import { api } from '@/lib/api';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PasswordResetConfirmPage } from '../PasswordResetConfirmPage';
import { PasswordResetPage } from '../PasswordResetPage';

vi.mock('@/lib/api');
vi.mock('@/hooks/useSystemStatus', () => ({
  useSystemStatus: () => ({ systemStatus: { turnstile_check: false } }),
}));

describe('PasswordResetPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('describes the email field for autofill and announces server failures', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ data: { success: false, message: 'No account for that address' } } as any);
    render(
      <MemoryRouter>
        <PasswordResetPage />
      </MemoryRouter>
    );

    const email = screen.getByLabelText('Email');
    expect(email).toHaveAttribute('type', 'email');
    expect(email).toHaveAttribute('autocomplete', 'email');
    expect(email).toHaveAttribute('spellcheck', 'false');
    expect(email).toBeRequired();

    fireEvent.change(email, { target: { value: 'user@example.com' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send Reset Link' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('No account for that address');
    expect(api.get).toHaveBeenCalledWith('/api/reset_password?email=user%40example.com');
  });

  it('shows the localized email message instead of a native validation bubble', async () => {
    render(
      <MemoryRouter>
        <PasswordResetPage />
      </MemoryRouter>
    );

    const email = screen.getByLabelText('Email');
    fireEvent.change(email, { target: { value: 'not-an-email' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send Reset Link' }));

    await waitFor(() => expect(email).toHaveAccessibleDescription('Valid email is required'));
    expect(email).toHaveAttribute('aria-invalid', 'true');
    expect(api.get).not.toHaveBeenCalled();
  });
});

describe('PasswordResetConfirmPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('marks both fields as new passwords and announces server failures', async () => {
    vi.mocked(api.post).mockResolvedValueOnce({ data: { success: false, message: 'Reset link is illegal or expired' } } as any);
    render(
      <MemoryRouter initialEntries={['/user/reset?email=user%40example.com&token=tok']}>
        <PasswordResetConfirmPage />
      </MemoryRouter>
    );

    const password = screen.getByLabelText('New Password');
    const confirmation = screen.getByLabelText('Confirm New Password');
    for (const input of [password, confirmation]) {
      expect(input).toHaveAttribute('type', 'password');
      expect(input).toHaveAttribute('autocomplete', 'new-password');
      expect(input).toHaveAttribute('minlength', '8');
      expect(input).toBeRequired();
    }

    fireEvent.change(password, { target: { value: 'abc12345' } });
    fireEvent.change(confirmation, { target: { value: 'abc12345' } });
    fireEvent.click(screen.getByRole('button', { name: 'Update Password' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Reset link is illegal or expired');
    expect(api.post).toHaveBeenCalledWith('/api/user/reset', { email: 'user@example.com', token: 'tok', password: 'abc12345' });
  });
});
