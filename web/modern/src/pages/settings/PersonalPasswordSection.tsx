import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { TFunction } from 'i18next';
import { type FormEvent, useId } from 'react';

export type PersonalPasswordSectionProps = {
  newPassword: string;
  confirmPassword: string;
  passwordError: string;
  passwordLoading: boolean;
  onNewPasswordChange: (value: string) => void;
  onConfirmPasswordChange: (value: string) => void;
  onUpdatePassword: () => void;
};

// SELF_PASSWORD_MIN_LENGTH mirrors the 8-character minimum dto.UserSelfUpdateRequest enforces for a new password.
const SELF_PASSWORD_MIN_LENGTH = 8;

/**
 * PersonalPasswordSection renders the password-change form and its submission control.
 * It receives password state and callbacks from PersonalSettings and returns the password settings section; submitting
 * the form (button or Enter) calls onUpdatePassword without a page navigation.
 */
export function PersonalPasswordSection({
  t,
  newPassword,
  confirmPassword,
  passwordError,
  passwordLoading,
  onNewPasswordChange,
  onConfirmPasswordChange,
  onUpdatePassword,
}: PersonalPasswordSectionProps & { t: TFunction }) {
  const fieldId = useId();
  const newPasswordId = `${fieldId}-new`;
  const confirmPasswordId = `${fieldId}-confirm`;

  /** handleSubmit suppresses the native form navigation for the given submit event and runs the password update. */
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onUpdatePassword();
  };

  return (
    // noValidate: minLength hints browser password generators; the server and handler messages own validation.
    <form className="space-y-4" onSubmit={handleSubmit} noValidate aria-labelledby={`${fieldId}-title`}>
      <h3 id={`${fieldId}-title`} className="text-base font-semibold">
        {t('personal_settings.security.password.title')}
      </h3>
      <p className="text-sm text-muted-foreground">{t('personal_settings.security.password.description')}</p>
      {passwordError && (
        <div role="alert" className="text-sm text-destructive font-medium">
          {passwordError}
        </div>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-1">
          <FormLabel htmlFor={newPasswordId}>{t('personal_settings.security.password.new_password')}</FormLabel>
          <Input
            id={newPasswordId}
            name="new-password"
            type="password"
            autoComplete="new-password"
            minLength={SELF_PASSWORD_MIN_LENGTH}
            placeholder={t('personal_settings.security.password.new_password_placeholder')}
            value={newPassword}
            onChange={(event) => onNewPasswordChange(event.target.value)}
          />
        </div>
        <div className="space-y-1">
          <FormLabel htmlFor={confirmPasswordId}>{t('personal_settings.security.password.confirm_password')}</FormLabel>
          <Input
            id={confirmPasswordId}
            name="confirm-password"
            type="password"
            autoComplete="new-password"
            minLength={SELF_PASSWORD_MIN_LENGTH}
            placeholder={t('personal_settings.security.password.confirm_password_placeholder')}
            value={confirmPassword}
            onChange={(event) => onConfirmPasswordChange(event.target.value)}
          />
        </div>
      </div>
      <Button type="submit" disabled={passwordLoading || !newPassword} className="w-full md:w-auto">
        {passwordLoading ? t('personal_settings.security.password.updating') : t('personal_settings.security.password.update_button')}
      </Button>
    </form>
  );
}
