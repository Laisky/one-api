import { render, screen } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';

import { PersonalSecurityCard } from './PersonalSecurityCard';

/** translate returns the requested key so the security-card structure remains observable in this behavior test. */
const translate = ((key: string) => key) as TFunction;

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
