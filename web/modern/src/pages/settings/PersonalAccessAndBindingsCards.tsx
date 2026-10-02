import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type { SystemStatus } from '@/lib/utils';
import type { TFunction } from 'i18next';
import type { OAuthBindings } from './personal-settings-types';

type PersonalAccessAndBindingsCardsProps = {
  t: TFunction;
  systemToken: string;
  affLink: string;
  onGenerateAccessToken: () => void;
  onGetAffLink: () => void;
  systemStatus: SystemStatus;
  oauthBindings: OAuthBindings;
  oauthBindingError: string;
  oauthBindingPending: 'lark' | 'oidc' | null;
  onBindLark: () => void;
  onBindOidc: () => void;
};

/**
 * PersonalAccessAndBindingsCards renders the access-token, invitation, and OAuth-binding cards.
 * It receives the current values and binding callbacks from PersonalSettings and returns the related card elements.
 */
export function PersonalAccessAndBindingsCards({
  t,
  systemToken,
  affLink,
  onGenerateAccessToken,
  onGetAffLink,
  systemStatus,
  oauthBindings,
  oauthBindingError,
  oauthBindingPending,
  onBindLark,
  onBindOidc,
}: PersonalAccessAndBindingsCardsProps) {
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('personal_settings.access_token.title')}</CardTitle>
          <CardDescription>{t('personal_settings.access_token.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Button onClick={onGenerateAccessToken} className="w-full">
                {t('personal_settings.access_token.generate_token')}
              </Button>
              {systemToken && <div className="mt-2 p-2 bg-muted rounded text-sm font-mono break-all">{systemToken}</div>}
            </div>
            <div>
              <Button onClick={onGetAffLink} variant="outline" className="w-full">
                {t('personal_settings.access_token.get_invite_link')}
              </Button>
              {affLink && <div className="mt-2 p-2 bg-muted rounded text-sm break-all">{affLink}</div>}
            </div>
          </div>
        </CardContent>
      </Card>

      {(systemStatus.lark_client_id || systemStatus.oidc) && (
        <Card>
          <CardHeader>
            <CardTitle>{t('personal_settings.oauth_binding.title')}</CardTitle>
            <CardDescription>{t('personal_settings.oauth_binding.description')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {oauthBindingError && <div className="text-sm text-destructive font-medium">{oauthBindingError}</div>}
            {systemStatus.lark_client_id && (
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between rounded-lg border bg-background p-3">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="font-medium">{t('personal_settings.oauth_binding.lark_label')}</span>
                  {oauthBindings.lark_id ? (
                    <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                      {t('personal_settings.oauth_binding.bound_lark')}
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground border-dashed">
                      {t('personal_settings.oauth_binding.not_bound')}
                    </Badge>
                  )}
                </div>
                {!oauthBindings.lark_id && (
                  <Button onClick={onBindLark} disabled={oauthBindingPending !== null} className="w-full sm:w-auto">
                    {oauthBindingPending === 'lark'
                      ? t('personal_settings.oauth_binding.binding')
                      : t('personal_settings.oauth_binding.bind_lark')}
                  </Button>
                )}
              </div>
            )}
            {systemStatus.oidc && (
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between rounded-lg border bg-background p-3">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="font-medium">{t('personal_settings.oauth_binding.oidc_label')}</span>
                  {oauthBindings.oidc_id ? (
                    <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                      {t('personal_settings.oauth_binding.bound_oidc')}
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground border-dashed">
                      {t('personal_settings.oauth_binding.not_bound')}
                    </Badge>
                  )}
                </div>
                {!oauthBindings.oidc_id && (
                  <Button onClick={onBindOidc} disabled={oauthBindingPending !== null} className="w-full sm:w-auto">
                    {oauthBindingPending === 'oidc'
                      ? t('personal_settings.oauth_binding.binding')
                      : t('personal_settings.oauth_binding.bind_oidc')}
                  </Button>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </>
  );
}
