import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { TFunction } from 'i18next';

export type PersonalPasswordSectionProps = {
  newPassword: string;
  confirmPassword: string;
  passwordError: string;
  passwordLoading: boolean;
  onNewPasswordChange: (value: string) => void;
  onConfirmPasswordChange: (value: string) => void;
  onUpdatePassword: () => void;
};

/**
 * PersonalPasswordSection renders password-change inputs and their submission control.
 * It receives password state and callbacks from PersonalSettings and returns the password settings section.
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
  return (
    <div className="space-y-4">
      <h3 className="text-base font-semibold">{t('personal_settings.security.password.title')}</h3>
      <p className="text-sm text-muted-foreground">{t('personal_settings.security.password.description')}</p>
      {passwordError && <div className="text-sm text-destructive font-medium">{passwordError}</div>}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="space-y-1">
          <FormLabel>{t('personal_settings.security.password.new_password')}</FormLabel>
          <Input
            type="password"
            placeholder={t('personal_settings.security.password.new_password_placeholder')}
            value={newPassword}
            onChange={(event) => onNewPasswordChange(event.target.value)}
          />
        </div>
        <div className="space-y-1">
          <FormLabel>{t('personal_settings.security.password.confirm_password')}</FormLabel>
          <Input
            type="password"
            placeholder={t('personal_settings.security.password.confirm_password_placeholder')}
            value={confirmPassword}
            onChange={(event) => onConfirmPasswordChange(event.target.value)}
          />
        </div>
      </div>
      <Button onClick={onUpdatePassword} disabled={passwordLoading || !newPassword} className="w-full md:w-auto">
        {passwordLoading ? t('personal_settings.security.password.updating') : t('personal_settings.security.password.update_button')}
      </Button>
    </div>
  );
}
