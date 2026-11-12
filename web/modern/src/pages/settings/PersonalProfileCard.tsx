import Turnstile from '@/components/Turnstile';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { TFunction } from 'i18next';
import { useId } from 'react';
import type { UseFormReturn } from 'react-hook-form';
import type { PersonalForm } from './personal-settings-types';

export type PersonalProfileCardProps = {
  t: TFunction;
  form: UseFormReturn<PersonalForm>;
  loading: boolean;
  onSubmit: (data: PersonalForm) => Promise<void>;
  emailVerificationCode: string;
  onEmailVerificationCodeChange: (value: string) => void;
  emailVerificationError: string;
  emailAction: 'send' | 'bind' | null;
  onSendEmailVerificationCode: () => void;
  onBindEmail: () => void;
  turnstileEnabled: boolean;
  turnstileToken: string;
  turnstileSiteKey?: string;
  turnstileRenderable: boolean;
  onTurnstileVerify: (token: string) => void;
  onTurnstileExpire: () => void;
};

/**
 * PersonalProfileCard renders the profile form: the read-only username, the display name, and the email address with
 * its verification-code binding flow. It receives the form instance, email-binding state and callbacks from
 * PersonalSettings and returns the profile information card.
 */
export function PersonalProfileCard({
  t,
  form,
  loading,
  onSubmit,
  emailVerificationCode,
  onEmailVerificationCodeChange,
  emailVerificationError,
  emailAction,
  onSendEmailVerificationCode,
  onBindEmail,
  turnstileEnabled,
  turnstileToken,
  turnstileSiteKey,
  turnstileRenderable,
  onTurnstileVerify,
  onTurnstileExpire,
}: PersonalProfileCardProps) {
  const emailCodeId = useId();
  const emailCodeErrorId = `${emailCodeId}-error`;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('personal_settings.profile_info.title')}</CardTitle>
        <CardDescription>{t('personal_settings.profile_info.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('personal_settings.profile_info.username')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="display_name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('personal_settings.profile_info.display_name')}</FormLabel>
                    <FormControl>
                      <Input placeholder={t('personal_settings.profile_info.display_name_placeholder')} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <FormItem className="space-y-3">
                  <FormLabel>{t('personal_settings.profile_info.email')}</FormLabel>
                  <FormControl>
                    <Input
                      type="email"
                      autoComplete="email"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      placeholder={t('personal_settings.profile_info.email_placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <div className="flex flex-col gap-3">
                    <p className="text-sm text-muted-foreground">{t('personal_settings.profile_info.email_help')}</p>
                    <div className="flex flex-col gap-3 md:flex-row items-end">
                      <div className="flex-1 w-full space-y-1">
                        <FormLabel htmlFor={emailCodeId}>{t('personal_settings.profile_info.email_verification_code')}</FormLabel>
                        <Input
                          id={emailCodeId}
                          autoComplete="one-time-code"
                          autoCapitalize="none"
                          autoCorrect="off"
                          spellCheck={false}
                          value={emailVerificationCode}
                          onChange={(e) => onEmailVerificationCodeChange(e.target.value)}
                          placeholder={t('personal_settings.profile_info.email_verification_code_placeholder')}
                          aria-invalid={emailVerificationError ? true : undefined}
                          aria-describedby={emailVerificationError ? emailCodeErrorId : undefined}
                        />
                      </div>
                      <div className="flex flex-col gap-2 sm:flex-row flex-shrink-0">
                        <Button
                          type="button"
                          variant="outline"
                          onClick={onSendEmailVerificationCode}
                          disabled={emailAction !== null || (turnstileEnabled && !turnstileToken)}
                        >
                          {emailAction === 'send'
                            ? t('personal_settings.profile_info.sending_code')
                            : t('personal_settings.profile_info.send_code')}
                        </Button>
                        <Button type="button" onClick={onBindEmail} disabled={emailAction !== null}>
                          {emailAction === 'bind'
                            ? t('personal_settings.profile_info.binding_email')
                            : t('personal_settings.profile_info.bind_email')}
                        </Button>
                      </div>
                    </div>
                    {turnstileRenderable && turnstileSiteKey && (
                      <Turnstile siteKey={turnstileSiteKey} onVerify={onTurnstileVerify} onExpire={onTurnstileExpire} />
                    )}
                    {emailVerificationError && (
                      <div id={emailCodeErrorId} role="alert" className="text-sm text-destructive">
                        {emailVerificationError}
                      </div>
                    )}
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />

            {form.formState.errors.root && (
              <div role="alert" className="text-sm text-destructive">
                {form.formState.errors.root.message}
              </div>
            )}

            <Button type="submit" disabled={loading}>
              {loading ? t('personal_settings.profile_info.updating') : t('personal_settings.profile_info.update_button')}
            </Button>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}
