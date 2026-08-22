import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import type { TFunction } from 'i18next';
import { PersonalPasskeySection, type PersonalPasskeySectionProps } from './PersonalPasskeySection';
import { PersonalPasswordSection, type PersonalPasswordSectionProps } from './PersonalPasswordSection';
import { PersonalTotpSection, type PersonalTotpSectionProps } from './PersonalTotpSection';

type PersonalSecurityCardProps = {
  t: TFunction;
  passkey: PersonalPasskeySectionProps;
  totp: PersonalTotpSectionProps;
  password: PersonalPasswordSectionProps;
};

/**
 * PersonalSecurityCard composes the independently managed passkey, TOTP, and password sections.
 * It receives grouped security props from PersonalSettings and returns the account-security card and TOTP dialog.
 */
export function PersonalSecurityCard({ t, passkey, totp, password }: PersonalSecurityCardProps) {
  const hasPasskeys = passkey.passkeys.length > 0;
  const securityScore = (hasPasskeys ? 1 : 0) + (totp.totpEnabled ? 1 : 0) + 1;

  return (
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
                totp.totpEnabled
                  ? 'bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800'
                  : 'text-muted-foreground border-dashed'
              }
              variant={totp.totpEnabled ? 'default' : 'outline'}
            >
              {totp.totpEnabled ? t('personal_settings.security.status.totp_on') : t('personal_settings.security.status.totp_off')}
            </Badge>
            <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
              {t('personal_settings.security.status.password_on')}
            </Badge>
          </div>
          {securityScore === 1 && (
            <p className="text-xs text-muted-foreground mt-2">{t('personal_settings.passkey.no_passkeys_desc')}</p>
          )}
        </div>
        <Separator />
        <PersonalPasskeySection t={t} {...passkey} />
        <Separator />
        <PersonalTotpSection t={t} {...totp} />
        <Separator />
        <PersonalPasswordSection t={t} {...password} />
      </CardContent>
    </Card>
  );
}
