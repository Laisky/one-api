import { Button } from '@/components/ui/button';
import { FormControl, FormField, FormItem, FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { getRegistrationPasswordChecks } from '@/lib/registration-password';
import { Check, Circle, Eye, EyeOff } from 'lucide-react';
import { useId, useState } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

/** RegistrationPasswords describes the two password values owned by the registration form. */
interface RegistrationPasswords {
  password: string;
  password2: string;
}

/** RegistrationPasswordFields renders live, accessible feedback for the parent form's password fields without changing their values. */
export function RegistrationPasswordFields() {
  const { t } = useTranslation();
  const { control, formState } = useFormContext<RegistrationPasswords>();
  const [password = '', confirmation = ''] = useWatch({ control, name: ['password', 'password2'] });
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmation, setShowConfirmation] = useState(false);
  const id = useId();
  const passwordId = `${id}-password`;
  const confirmationId = `${id}-confirmation`;
  const checks = getRegistrationPasswordChecks(password);
  const met = Object.values(checks).filter(Boolean).length;
  const total = Object.keys(checks).length;
  const valid = met === total;
  const hasPassword = password.length > 0;
  const matches = confirmation.length > 0 && password === confirmation;

  return (
    <>
      <FormField
        control={control}
        name="password"
        render={({ field, fieldState }) => (
          <FormItem>
            <FormLabel htmlFor={passwordId}>{t('common.password')}</FormLabel>
            <FormControl>
              <div className="relative">
                <Input
                  {...field}
                  id={passwordId}
                  type={showPassword ? 'text' : 'password'}
                  placeholder={t('auth.register.enter_password')}
                  autoComplete="new-password"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  className="pr-12"
                  aria-invalid={(fieldState.isTouched || formState.isSubmitted) && !valid}
                  aria-describedby={`${passwordId}-requirements ${passwordId}-hint ${passwordId}-status`}
                />
                <Button
                  type="button"
                  variant="ghost"
                  className="absolute inset-y-0 right-0 h-full w-11 px-0"
                  aria-label={t(showPassword ? 'registration_password.hide_password' : 'registration_password.show_password')}
                  aria-controls={passwordId}
                  onClick={() => setShowPassword((visible) => !visible)}
                >
                  {showPassword ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
                </Button>
              </div>
            </FormControl>
            <div className="rounded-lg border bg-muted/30 p-3 space-y-2">
              <p id={`${passwordId}-heading`} className="text-xs font-medium">{t('registration_password.requirements')}</p>
              <ul id={`${passwordId}-requirements`} aria-labelledby={`${passwordId}-heading`} className="space-y-1 text-xs">
                {Object.entries(checks).map(([rule, satisfied]) => (
                  <li key={rule} className={`flex items-start gap-2 ${satisfied && hasPassword ? 'text-emerald-700 dark:text-emerald-400' : 'text-muted-foreground'}`}>
                    {satisfied && hasPassword ? <Check className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" /> : <Circle className="mt-0.5 h-3 w-3 shrink-0" aria-hidden="true" />}
                    <span>
                      <span className="sr-only">{t(satisfied && hasPassword ? 'registration_password.met' : 'registration_password.pending')}: </span>
                      {t(`registration_password.${rule}`)}
                    </span>
                  </li>
                ))}
              </ul>
              <p id={`${passwordId}-hint`} className="text-xs text-muted-foreground">{t('registration_password.ascii_hint')}</p>
            </div>
            <p id={`${passwordId}-status`} role="status" aria-live="polite" aria-atomic="true" className={`text-xs ${valid ? 'text-emerald-700 dark:text-emerald-400' : 'text-muted-foreground'}`}>
              {valid ? t('registration_password.valid') : t('registration_password.progress', { met, total })}
            </p>
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name="password2"
        render={({ field, fieldState }) => {
          const missing = !confirmation && (fieldState.isTouched || formState.isSubmitted);
          const mismatch = confirmation.length > 0 && !matches;
          // Derive feedback from both values, not the resolver's last field error:
          // editing the original password must immediately update confirmation.
          const message = matches ? 'match' : mismatch ? 'mismatch' : missing ? 'confirmation_required' : 'confirmation_hint';
          return (
            <FormItem>
              <FormLabel htmlFor={confirmationId}>{t('auth.register.confirm_password')}</FormLabel>
              <FormControl>
                <div className="relative">
                  <Input
                    {...field}
                    id={confirmationId}
                    type={showConfirmation ? 'text' : 'password'}
                    placeholder={t('auth.register.confirm_password')}
                    autoComplete="new-password"
                    autoCapitalize="none"
                    autoCorrect="off"
                    spellCheck={false}
                    className="pr-12"
                    aria-invalid={missing || mismatch}
                    aria-describedby={`${confirmationId}-status`}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    className="absolute inset-y-0 right-0 h-full w-11 px-0"
                    aria-label={t(showConfirmation ? 'registration_password.hide_confirmation' : 'registration_password.show_confirmation')}
                    aria-controls={confirmationId}
                    onClick={() => setShowConfirmation((visible) => !visible)}
                  >
                    {showConfirmation ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
                  </Button>
                </div>
              </FormControl>
              <p id={`${confirmationId}-status`} role="status" aria-live="polite" aria-atomic="true" className={`text-xs ${matches ? 'text-emerald-700 dark:text-emerald-400' : missing || mismatch ? 'text-destructive' : 'text-muted-foreground'}`}>
                {t(`registration_password.${message}`)}
              </p>
            </FormItem>
          );
        }}
      />
    </>
  );
}
