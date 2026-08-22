import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { TFunction } from 'i18next';
import type { PasskeyInfo } from './personal-settings-types';

export type PersonalPasskeySectionProps = {
  passkeys: PasskeyInfo[];
  passkeySupported: boolean;
  passkeyError: string;
  passkeyLoading: boolean;
  showPasskeyName: boolean;
  passkeyName: string;
  onPasskeyNameChange: (value: string) => void;
  onOpenPasskeyName: () => void;
  onCancelPasskeyName: () => void;
  onRegisterPasskey: () => void;
  onDeletePasskey: (passkey: PasskeyInfo) => void;
};

/**
 * PersonalPasskeySection renders passkey status, registration, and deletion controls.
 * It receives passkey state and callbacks from PersonalSettings and returns the passkey settings section.
 */
export function PersonalPasskeySection({
  t,
  passkeys,
  passkeySupported,
  passkeyError,
  passkeyLoading,
  showPasskeyName,
  passkeyName,
  onPasskeyNameChange,
  onOpenPasskeyName,
  onCancelPasskeyName,
  onRegisterPasskey,
  onDeletePasskey,
}: PersonalPasskeySectionProps & { t: TFunction }) {
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-base font-semibold">{t('personal_settings.passkey.title')}</h3>
        <Badge className="bg-primary/10 text-primary border-primary/20 text-xs">{t('personal_settings.passkey.recommended')}</Badge>
      </div>
      <p className="text-sm text-muted-foreground">{t('personal_settings.passkey.description')}</p>
      {passkeyError && <div className="text-sm text-destructive font-medium">{passkeyError}</div>}
      {!passkeySupported ? (
        <Alert>
          <AlertTitle>{t('personal_settings.passkey.errors.not_supported')}</AlertTitle>
          <AlertDescription>{t('personal_settings.passkey.not_supported_desc')}</AlertDescription>
        </Alert>
      ) : (
        <>
          {passkeys.length > 0 ? (
            <div className="space-y-2">
              {passkeys.map((passkey) => (
                <div key={passkey.uuid || passkey.id} className="flex items-center justify-between p-3 border rounded-lg bg-background">
                  <div className="flex-1 min-w-0">
                    <div className="font-medium truncate">{passkey.credential_name}</div>
                    <div className="text-xs text-muted-foreground">
                      {t('personal_settings.passkey.registered')}: {new Date(passkey.created_at).toLocaleDateString()}
                      {' · '}
                      {t('personal_settings.passkey.sign_count')}: {passkey.sign_count}
                    </div>
                  </div>
                  <Button variant="destructive" size="sm" onClick={() => onDeletePasskey(passkey)} disabled={passkeyLoading}>
                    {t('personal_settings.passkey.delete_button')}
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <Alert className="bg-info-muted border-info-border">
              <AlertTitle className="text-info-foreground">{t('personal_settings.passkey.no_passkeys')}</AlertTitle>
              <AlertDescription>{t('personal_settings.passkey.no_passkeys_desc')}</AlertDescription>
            </Alert>
          )}
          {showPasskeyName ? (
            <div className="flex flex-col space-y-2">
              <FormLabel>{t('personal_settings.passkey.name_label')}</FormLabel>
              <Input
                placeholder={t('personal_settings.passkey.name_placeholder')}
                value={passkeyName}
                onChange={(event) => onPasskeyNameChange(event.target.value)}
                maxLength={128}
              />
              <div className="flex gap-2">
                <Button onClick={onRegisterPasskey} disabled={passkeyLoading}>
                  {passkeyLoading ? t('personal_settings.passkey.processing') : t('personal_settings.passkey.register_button')}
                </Button>
                <Button variant="outline" onClick={onCancelPasskeyName} disabled={passkeyLoading}>
                  {t('personal_settings.totp.cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <Button onClick={onOpenPasskeyName} disabled={passkeyLoading} className="w-full md:w-auto">
              {t('personal_settings.passkey.register_button')}
            </Button>
          )}
        </>
      )}
    </div>
  );
}
