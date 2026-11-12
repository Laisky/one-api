import { fireEvent, render, screen, within } from '@testing-library/react';
import type { TFunction } from 'i18next';
import type { ComponentProps } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { PersonalSecurityCard } from './PersonalSecurityCard';

/** translate returns the requested key so the security-card structure remains observable in this behavior test. */
const translate = ((key: string) => key) as TFunction;

type SecurityCardProps = ComponentProps<typeof PersonalSecurityCard>;

/**
 * buildProps returns PersonalSecurityCard props with inert callbacks, merging the supplied section overrides.
 * It takes partial passkey, TOTP and password overrides and returns the complete props object.
 */
const buildProps = (overrides: {
  passkey?: Partial<SecurityCardProps['passkey']>;
  totp?: Partial<SecurityCardProps['totp']>;
  password?: Partial<SecurityCardProps['password']>;
} = {}): SecurityCardProps => ({
  t: translate,
  passkey: {
    passkeys: [],
    passkeySupported: true,
    passkeyError: '',
    passkeyLoading: false,
    showPasskeyName: false,
    passkeyName: '',
    onPasskeyNameChange: vi.fn(),
    onOpenPasskeyName: vi.fn(),
    onCancelPasskeyName: vi.fn(),
    onRegisterPasskey: vi.fn(),
    onDeletePasskey: vi.fn(),
    ...overrides.passkey,
  },
  totp: {
    totpEnabled: false,
    totpError: '',
    setupTotpError: '',
    disableTotpError: '',
    totpCode: '',
    onTotpCodeChange: vi.fn(),
    totpLoading: false,
    onSetupTotp: vi.fn(),
    onDisableTotp: vi.fn(),
    showTotpSetup: false,
    onShowTotpSetupChange: vi.fn(),
    totpQRCode: '',
    totpSecret: '',
    confirmTotpError: '',
    onConfirmTotp: vi.fn(),
    isMobile: false,
    ...overrides.totp,
  },
  password: {
    newPassword: '',
    confirmPassword: '',
    passwordError: '',
    passwordLoading: false,
    onNewPasswordChange: vi.fn(),
    onConfirmPasswordChange: vi.fn(),
    onUpdatePassword: vi.fn(),
    ...overrides.password,
  },
});

describe('PersonalSecurityCard forms', () => {
  it('submits the password change as a form with labelled new-password fields', () => {
    const onUpdatePassword = vi.fn();
    render(<PersonalSecurityCard {...buildProps({ password: { newPassword: 'abc12345', confirmPassword: 'abc12345', onUpdatePassword } })} />);

    const newPassword = screen.getByLabelText('personal_settings.security.password.new_password');
    const confirmPassword = screen.getByLabelText('personal_settings.security.password.confirm_password');
    expect(newPassword).toHaveAttribute('autocomplete', 'new-password');
    expect(confirmPassword).toHaveAttribute('autocomplete', 'new-password');
    expect(newPassword).toHaveAttribute('minlength', '8');

    const submit = screen.getByRole('button', { name: 'personal_settings.security.password.update_button' });
    expect(submit).toHaveAttribute('type', 'submit');
    fireEvent.submit(newPassword.closest('form')!);
    expect(onUpdatePassword).toHaveBeenCalledTimes(1);
  });

  it('announces password errors', () => {
    render(<PersonalSecurityCard {...buildProps({ password: { passwordError: 'Passwords do not match' } })} />);
    expect(screen.getByText('Passwords do not match')).toHaveAttribute('role', 'alert');
  });

  it('submits the TOTP disable code from a labelled one-time-code input', () => {
    const onDisableTotp = vi.fn();
    render(<PersonalSecurityCard {...buildProps({ totp: { totpEnabled: true, totpCode: '123456', onDisableTotp, disableTotpError: 'Invalid TOTP code' } })} />);

    const code = screen.getByLabelText('personal_settings.totp.verify_code');
    expect(code).toHaveAttribute('autocomplete', 'one-time-code');
    expect(code).toHaveAttribute('inputmode', 'numeric');
    expect(code).toHaveAttribute('pattern', '[0-9]{6}');
    expect(code).toHaveAttribute('maxlength', '6');
    expect(code).toHaveAccessibleDescription('Invalid TOTP code');
    expect(screen.getByText('Invalid TOTP code')).toHaveAttribute('role', 'alert');
    // The enabled-status box is a static group, so the nested error is not announced twice by an outer live region.
    expect(screen.getByRole('group', { name: 'personal_settings.totp.enabled_title' })).toContainElement(code);
    expect(code.closest('[role="alert"]')).toBeNull();

    fireEvent.submit(code.closest('form')!);
    expect(onDisableTotp).toHaveBeenCalledTimes(1);
  });

  it('confirms TOTP setup through the dialog form submit button', () => {
    const onConfirmTotp = vi.fn();
    render(
      <PersonalSecurityCard
        {...buildProps({ totp: { showTotpSetup: true, totpCode: '123456', totpSecret: 'SECRET', onConfirmTotp, confirmTotpError: 'Bad code' } })}
      />
    );

    const dialog = screen.getByRole('dialog');
    const secret = within(dialog).getByLabelText('personal_settings.totp.secret_key');
    expect(secret).toHaveAttribute('spellcheck', 'false');
    const code = within(dialog).getByLabelText('personal_settings.totp.verify_code');
    expect(code).toHaveAttribute('autocomplete', 'one-time-code');
    expect(code).toHaveAttribute('inputmode', 'numeric');
    expect(within(dialog).getByText('Bad code')).toHaveAttribute('role', 'alert');

    fireEvent.click(within(dialog).getByRole('button', { name: 'personal_settings.totp.confirm' }));
    expect(onConfirmTotp).toHaveBeenCalledTimes(1);
  });

  it('announces passkey errors', () => {
    render(<PersonalSecurityCard {...buildProps({ passkey: { passkeyError: 'Registration failed' } })} />);
    expect(screen.getByText('Registration failed')).toHaveAttribute('role', 'alert');
  });
});

describe('PersonalSecurityCard', () => {
  it('does not render an empty status hint when exactly one optional security method is enabled', () => {
    render(
      <PersonalSecurityCard
        t={translate}
        passkey={{
          passkeys: [{ id: 1, credential_name: 'Laptop', sign_count: 0, created_at: 0 }],
          passkeySupported: true,
          passkeyError: '',
          passkeyLoading: false,
          showPasskeyName: false,
          passkeyName: '',
          onPasskeyNameChange: vi.fn(),
          onOpenPasskeyName: vi.fn(),
          onCancelPasskeyName: vi.fn(),
          onRegisterPasskey: vi.fn(),
          onDeletePasskey: vi.fn(),
        }}
        totp={{
          totpEnabled: false,
          totpError: '',
          setupTotpError: '',
          disableTotpError: '',
          totpCode: '',
          onTotpCodeChange: vi.fn(),
          totpLoading: false,
          onSetupTotp: vi.fn(),
          onDisableTotp: vi.fn(),
          showTotpSetup: false,
          onShowTotpSetupChange: vi.fn(),
          totpQRCode: '',
          totpSecret: '',
          confirmTotpError: '',
          onConfirmTotp: vi.fn(),
          isMobile: false,
        }}
        password={{
          newPassword: '',
          confirmPassword: '',
          passwordError: '',
          passwordLoading: false,
          onNewPasswordChange: vi.fn(),
          onConfirmPasswordChange: vi.fn(),
          onUpdatePassword: vi.fn(),
        }}
      />
    );

    const statusBox = screen.getByText('personal_settings.security.status.title').parentElement;
    expect(statusBox?.querySelector('p')).not.toBeInTheDocument();
  });
});
