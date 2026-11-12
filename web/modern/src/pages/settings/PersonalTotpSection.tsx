import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { TFunction } from 'i18next';
import { type FormEvent, useId } from 'react';

export type PersonalTotpSectionProps = {
  totpEnabled: boolean;
  totpError: string;
  setupTotpError: string;
  disableTotpError: string;
  totpCode: string;
  onTotpCodeChange: (value: string) => void;
  totpLoading: boolean;
  onSetupTotp: () => void;
  onDisableTotp: () => void;
  showTotpSetup: boolean;
  onShowTotpSetupChange: (open: boolean) => void;
  totpQRCode: string;
  totpSecret: string;
  confirmTotpError: string;
  onConfirmTotp: () => void;
  isMobile: boolean;
};

/**
 * PersonalTotpSection renders TOTP enablement, disablement, and setup-dialog controls.
 * It receives TOTP state and callbacks from PersonalSettings and returns the TOTP settings section and dialog.
 */
export function PersonalTotpSection({
  t,
  totpEnabled,
  totpError,
  setupTotpError,
  disableTotpError,
  totpCode,
  onTotpCodeChange,
  totpLoading,
  onSetupTotp,
  onDisableTotp,
  showTotpSetup,
  onShowTotpSetupChange,
  totpQRCode,
  totpSecret,
  confirmTotpError,
  onConfirmTotp,
  isMobile,
}: PersonalTotpSectionProps & { t: TFunction }) {
  const fieldId = useId();
  const disableCodeId = `${fieldId}-disable-code`;
  const confirmCodeId = `${fieldId}-confirm-code`;
  const secretId = `${fieldId}-secret`;

  /**
   * submitWith returns a form submit handler that suppresses native navigation and runs the supplied action.
   * It takes the TOTP action to run and returns the handler for the form's onSubmit.
   */
  const submitWith = (action: () => void) => (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    action();
  };

  return (
    <>
      <div className="space-y-4">
        <h3 className="text-base font-semibold">{t('personal_settings.totp.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('personal_settings.totp.description')}</p>
        {totpError && (
          <div role="alert" className="text-sm text-destructive font-medium">
            {totpError}
          </div>
        )}
        {totpEnabled ? (
          // A static status box, not a live region: the disable error inside it is the only alert to announce.
          <Alert role="group" aria-labelledby={`${fieldId}-enabled-title`} className="bg-success-muted border-success-border">
            <div className="flex flex-col space-y-3">
              <div>
                <AlertTitle id={`${fieldId}-enabled-title`} className="text-success-foreground">
                  {t('personal_settings.totp.enabled_title')}
                </AlertTitle>
                <AlertDescription>{t('personal_settings.totp.enabled_desc')}</AlertDescription>
              </div>
              {/* noValidate: the pattern/length attributes drive autofill and the numeric keyboard, while the
                  localized handler messages own validation feedback. */}
              <form className="flex flex-col space-y-2" onSubmit={submitWith(onDisableTotp)} noValidate>
                <FormLabel htmlFor={disableCodeId}>{t('personal_settings.totp.verify_code')}</FormLabel>
                <Input
                  id={disableCodeId}
                  name="totp_code"
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  enterKeyHint="done"
                  placeholder={t('personal_settings.totp.disable_placeholder')}
                  value={totpCode}
                  onChange={(event) => onTotpCodeChange(event.target.value)}
                  aria-invalid={disableTotpError ? true : undefined}
                  aria-describedby={disableTotpError ? `${disableCodeId}-error` : undefined}
                />
                {disableTotpError && (
                  <div id={`${disableCodeId}-error`} role="alert" className="text-sm text-destructive font-medium">
                    {disableTotpError}
                  </div>
                )}
                <Button type="submit" variant="destructive" disabled={totpLoading} className="w-full md:w-auto">
                  {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.disable_button')}
                </Button>
              </form>
            </div>
          </Alert>
        ) : (
          <div className="space-y-2">
            {setupTotpError && (
              <div role="alert" className="text-sm text-destructive font-medium">
                {setupTotpError}
              </div>
            )}
            <Button variant="default" onClick={onSetupTotp} disabled={totpLoading} className="w-full md:w-auto">
              {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.enable_button')}
            </Button>
          </div>
        )}
      </div>
      <Dialog open={showTotpSetup} onOpenChange={onShowTotpSetupChange}>
        <DialogContent className={isMobile ? 'max-w-[95vw] p-4 max-h-[90vh] overflow-y-auto' : 'max-w-[500px]'}>
          <DialogHeader>
            <DialogTitle className={isMobile ? 'text-base' : ''}>{t('personal_settings.totp.setup_title')}</DialogTitle>
            <DialogDescription className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.setup_desc')}</DialogDescription>
          </DialogHeader>
          <form id={`${fieldId}-confirm-form`} className={isMobile ? 'space-y-3' : 'space-y-4'} onSubmit={submitWith(onConfirmTotp)} noValidate>
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
              <FormLabel htmlFor={secretId} className={isMobile ? 'text-xs' : ''}>
                {t('personal_settings.totp.secret_key')}
              </FormLabel>
              <Input
                id={secretId}
                value={totpSecret}
                readOnly
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                className={isMobile ? 'font-mono text-xs h-9' : 'font-mono'}
              />
            </div>
            <div className="space-y-2">
              <FormLabel htmlFor={confirmCodeId} className={isMobile ? 'text-xs' : ''}>
                {t('personal_settings.totp.verify_code')}
              </FormLabel>
              <Input
                id={confirmCodeId}
                name="totp_code"
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9]{6}"
                enterKeyHint="done"
                aria-invalid={confirmTotpError ? true : undefined}
                aria-describedby={confirmTotpError ? `${confirmCodeId}-error` : undefined}
                placeholder={
                  isMobile ? t('personal_settings.totp.verify_placeholder_mobile') : t('personal_settings.totp.verify_placeholder')
                }
                value={totpCode}
                onChange={(event) => onTotpCodeChange(event.target.value)}
                maxLength={6}
                className={isMobile ? 'text-base h-10' : ''}
              />
              {confirmTotpError && (
                <div
                  id={`${confirmCodeId}-error`}
                  role="alert"
                  className={`${isMobile ? 'text-xs' : 'text-sm'} text-destructive font-medium mt-1`}
                >
                  {confirmTotpError}
                </div>
              )}
            </div>
          </form>
          <DialogFooter className={isMobile ? 'flex-col space-y-2 sm:space-y-0' : ''}>
            <Button
              type="button"
              variant="outline"
              onClick={() => onShowTotpSetupChange(false)}
              disabled={totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {t('personal_settings.totp.cancel')}
            </Button>
            <Button
              type="submit"
              form={`${fieldId}-confirm-form`}
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
