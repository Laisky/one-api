import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import type { TFunction } from 'i18next';
import type { PasskeyInfo } from './personal-settings-types';

type PersonalSecurityCardProps = {
  t: TFunction;
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
  totpEnabled: boolean;
  totpError: string;
  setupTotpError: string;
  disableTotpError: string;
  totpCode: string;
  onTotpCodeChange: (value: string) => void;
  totpLoading: boolean;
  onSetupTotp: () => void;
  onDisableTotp: () => void;
  newPassword: string;
  confirmPassword: string;
  passwordError: string;
  passwordLoading: boolean;
  onNewPasswordChange: (value: string) => void;
  onConfirmPasswordChange: (value: string) => void;
  onUpdatePassword: () => void;
  showTotpSetup: boolean;
  onShowTotpSetupChange: (open: boolean) => void;
  totpQRCode: string;
  totpSecret: string;
  confirmTotpError: string;
  onConfirmTotp: () => void;
  isMobile: boolean;
};

/**
 * PersonalSecurityCard renders passkey, TOTP, password, and TOTP-setup controls.
 * It receives security state and callbacks from PersonalSettings and returns the security card and dialog.
 */
export function PersonalSecurityCard({
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
  totpEnabled,
  totpError,
  setupTotpError,
  disableTotpError,
  totpCode,
  onTotpCodeChange,
  totpLoading,
  onSetupTotp,
  onDisableTotp,
  newPassword,
  confirmPassword,
  passwordError,
  passwordLoading,
  onNewPasswordChange,
  onConfirmPasswordChange,
  onUpdatePassword,
  showTotpSetup,
  onShowTotpSetupChange,
  totpQRCode,
  totpSecret,
  confirmTotpError,
  onConfirmTotp,
  isMobile,
}: PersonalSecurityCardProps) {
  const hasPasskeys = passkeys.length > 0;
  const securityScore = (hasPasskeys ? 1 : 0) + (totpEnabled ? 1 : 0) + 1;

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('personal_settings.security.title')}</CardTitle>
          <CardDescription>{t('personal_settings.security.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="rounded-lg border bg-muted/30 p-4">
            <h4 className="text-sm font-medium mb-3">{t('personal_settings.security.status.title')}</h4>
            <div className="flex flex-wrap gap-2">
              <Badge
                className={
                  hasPasskeys
                    ? 'bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800'
                    : 'text-muted-foreground border-dashed'
                }
                variant={hasPasskeys ? 'default' : 'outline'}
              >
                {hasPasskeys ? t('personal_settings.security.status.passkey_on') : t('personal_settings.security.status.passkey_off')}
              </Badge>
              <Badge
                className={
                  totpEnabled
                    ? 'bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800'
                    : 'text-muted-foreground border-dashed'
                }
                variant={totpEnabled ? 'default' : 'outline'}
              >
                {totpEnabled ? t('personal_settings.security.status.totp_on') : t('personal_settings.security.status.totp_off')}
              </Badge>
              <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                {t('personal_settings.security.status.password_on')}
              </Badge>
            </div>
            {securityScore < 3 && (
              <p className="text-xs text-muted-foreground mt-2">
                {securityScore === 1 ? t('personal_settings.passkey.no_passkeys_desc') : ''}
              </p>
            )}
          </div>

          <Separator />

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
                {hasPasskeys ? (
                  <div className="space-y-2">
                    {passkeys.map((passkey) => (
                      <div
                        key={passkey.uuid || passkey.id}
                        className="flex items-center justify-between p-3 border rounded-lg bg-background"
                      >
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

          <Separator />

          <div className="space-y-4">
            <h3 className="text-base font-semibold">{t('personal_settings.totp.title')}</h3>
            <p className="text-sm text-muted-foreground">{t('personal_settings.totp.description')}</p>
            {totpError && <div className="text-sm text-destructive font-medium">{totpError}</div>}
            {totpEnabled ? (
              <Alert className="bg-success-muted border-success-border">
                <div className="flex flex-col space-y-3">
                  <div>
                    <AlertTitle className="text-success-foreground">{t('personal_settings.totp.enabled_title')}</AlertTitle>
                    <AlertDescription>{t('personal_settings.totp.enabled_desc')}</AlertDescription>
                  </div>
                  <div className="flex flex-col space-y-2">
                    <Input
                      placeholder={t('personal_settings.totp.disable_placeholder')}
                      value={totpCode}
                      onChange={(event) => onTotpCodeChange(event.target.value)}
                    />
                    {disableTotpError && <div className="text-sm text-destructive font-medium">{disableTotpError}</div>}
                    <Button variant="destructive" onClick={onDisableTotp} disabled={totpLoading} className="w-full md:w-auto">
                      {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.disable_button')}
                    </Button>
                  </div>
                </div>
              </Alert>
            ) : (
              <div className="space-y-2">
                {setupTotpError && <div className="text-sm text-destructive font-medium">{setupTotpError}</div>}
                <Button variant="default" onClick={onSetupTotp} disabled={totpLoading} className="w-full md:w-auto">
                  {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.enable_button')}
                </Button>
              </div>
            )}
          </div>

          <Separator />

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
        </CardContent>
      </Card>

      <Dialog open={showTotpSetup} onOpenChange={onShowTotpSetupChange}>
        <DialogContent className={isMobile ? 'max-w-[95vw] p-4 max-h-[90vh] overflow-y-auto' : 'max-w-[500px]'}>
          <DialogHeader>
            <DialogTitle className={isMobile ? 'text-base' : ''}>{t('personal_settings.totp.setup_title')}</DialogTitle>
            <DialogDescription className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.setup_desc')}</DialogDescription>
          </DialogHeader>
          <div className={isMobile ? 'space-y-3' : 'space-y-4'}>
            <Alert className={isMobile ? 'text-xs' : ''}>
              <AlertTitle className={isMobile ? 'text-sm' : ''}>{t('personal_settings.totp.setup_instructions_title')}</AlertTitle>
              <AlertDescription>
                <ol className={isMobile ? 'pl-3 mt-1 space-y-0.5 text-xs' : 'pl-4 mt-2 space-y-1'}>
                  <li>{t('personal_settings.totp.setup_step1')}</li>
                  <li>{t('personal_settings.totp.setup_step2')}</li>
                  <li>{t('personal_settings.totp.setup_step3')}</li>
                  <li>{t('personal_settings.totp.setup_step4')}</li>
                </ol>
              </AlertDescription>
            </Alert>
            {totpQRCode && (
              <div className={`flex justify-center ${isMobile ? 'my-2' : 'my-4'}`}>
                <img
                  src={totpQRCode}
                  alt={t('personal_settings.totp.qr_alt')}
                  className={isMobile ? 'rounded-lg shadow-md max-w-[240px] w-full h-auto' : 'rounded-lg shadow-md max-w-full'}
                />
              </div>
            )}
            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.secret_key')}</FormLabel>
              <Input value={totpSecret} readOnly className={isMobile ? 'font-mono text-xs h-9' : 'font-mono'} />
            </div>
            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.verify_code')}</FormLabel>
              <Input
                placeholder={
                  isMobile ? t('personal_settings.totp.verify_placeholder_mobile') : t('personal_settings.totp.verify_placeholder')
                }
                value={totpCode}
                onChange={(event) => onTotpCodeChange(event.target.value)}
                maxLength={6}
                className={isMobile ? 'text-base h-10' : ''}
              />
              {confirmTotpError && (
                <div className={`${isMobile ? 'text-xs' : 'text-sm'} text-destructive font-medium mt-1`}>{confirmTotpError}</div>
              )}
            </div>
          </div>
          <DialogFooter className={isMobile ? 'flex-col space-y-2 sm:space-y-0' : ''}>
            <Button
              variant="outline"
              onClick={() => onShowTotpSetupChange(false)}
              disabled={totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {t('personal_settings.totp.cancel')}
            </Button>
            <Button
              onClick={onConfirmTotp}
              disabled={!totpCode || totpCode.length !== 6 || totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
