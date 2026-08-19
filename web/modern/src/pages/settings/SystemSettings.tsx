import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { useNotifications } from '@/components/ui/notifications';
import { TooltipProvider } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EmailDomainWhitelistItem, OptionItem, type EnumChoice, type OptionRow } from './SystemSettingsFields';

interface OptionGroup {
  id: string;
  title: string;
  description?: string;
  keys: string[];
}

const OPTION_GROUPS: OptionGroup[] = [
  {
    id: 'authentication',
    title: 'Authentication & Registration',
    description: 'Control how users sign up and sign in to your workspace.',
    keys: [
      'PasswordLoginEnabled',
      'PasswordRegisterEnabled',
      'RegisterEnabled',
      'EmailVerificationEnabled',
      'EmailDomainRestrictionEnabled',
      'EmailDomainWhitelist',
    ],
  },
  {
    id: 'oauth',
    title: 'OAuth / SSO Providers',
    description: 'Connect third-party identity providers for seamless sign-in.',
    keys: [
      'GitHubOAuthEnabled',
      'GitHubClientId',
      'GitHubClientSecret',
      'OidcEnabled',
      'OidcClientId',
      'OidcClientSecret',
      'OidcWellKnown',
      'OidcAuthorizationEndpoint',
      'OidcTokenEndpoint',
      'OidcUserinfoEndpoint',
      'LarkClientId',
      'LarkClientSecret',
      'WeChatAuthEnabled',
      'WeChatServerAddress',
      'WeChatServerToken',
      'WeChatAccountQRCodeImageURL',
    ],
  },
  {
    id: 'security',
    title: 'Anti-bot & Security',
    description: 'Configure bot protection and security checks.',
    keys: ['TurnstileCheckEnabled', 'TurnstileSiteKey', 'TurnstileSecretKey'],
  },
  {
    id: 'email',
    title: 'Email',
    description: 'Set up outbound email delivery.',
    keys: ['EmailProvider', 'SMTPServer', 'SMTPPort', 'SMTPAccount', 'SMTPToken', 'SMTPFrom', 'ResendAPIKey'],
  },
  {
    id: 'branding',
    title: 'Branding & Content',
    description: 'Customize the look and feel of the product experience.',
    keys: ['SystemName', 'Logo', 'Footer', 'Notice', 'About', 'HomePageContent', 'Theme'],
  },
  {
    id: 'links',
    title: 'Links',
    description: 'Control external links exposed to your end users.',
    keys: ['TopUpLink', 'ChatLink', 'ServerAddress'],
  },
  {
    id: 'quota',
    title: 'Quota & Billing',
    description: 'Manage quotas, billing ratios, and currency presentation.',
    keys: [
      'QuotaForNewUser',
      'QuotaForInviter',
      'QuotaForInvitee',
      'QuotaRemindThreshold',
      'PreConsumedQuota',
      'QuotaPerUnit',
      'DisplayInCurrencyEnabled',
      'DisplayTokenStatEnabled',
      'ApproximateTokenEnabled',
    ],
  },
  {
    id: 'channels',
    title: 'Channels & Reliability',
    description: 'Automatically react to upstream channel health and retry behavior.',
    keys: ['AutomaticDisableChannelEnabled', 'AutomaticEnableChannelEnabled', 'ChannelDisableThreshold', 'RetryTimes'],
  },
  {
    id: 'logging',
    title: 'Logging, Metrics & Integrations',
    description: 'Tune observability and downstream integrations.',
    keys: ['LogConsumeEnabled', 'MessagePusherAddress', 'MessagePusherToken'],
  },
];

const SENSITIVE_OPTION_KEYS = new Set<string>([
  'SMTPToken',
  'ResendAPIKey',
  'TurnstileSecretKey',
  'GitHubClientSecret',
  'OidcClientSecret',
  'LarkClientSecret',
  'WeChatServerToken',
  'MessagePusherToken',
]);

// EMAIL_PROVIDER_AUTO is the UI sentinel for the empty ("auto") EmailProvider value,
// since Radix Select items cannot use an empty string as their value.
const EMAIL_PROVIDER_AUTO = '__auto__';

// ENUM_OPTION_KEYS lists option keys whose value is a fixed enum, rendered as a select.
const ENUM_OPTION_KEYS: Record<string, EnumChoice[]> = {
  EmailProvider: [
    { value: EMAIL_PROVIDER_AUTO, storedValue: '', labelKey: 'system_settings.email_provider.auto' },
    { value: 'smtp', labelKey: 'system_settings.email_provider.smtp' },
    { value: 'resend', labelKey: 'system_settings.email_provider.resend' },
  ],
};

const OPTION_GROUP_KEY_SET = new Set(OPTION_GROUPS.flatMap((group) => group.keys));

// BOOLEAN_OPTION_KEYS must stay aligned with backend option typing in `model/option.go` and related config defaults.
// Do not rely on string suffix heuristics here—explicitly list each boolean config flag so future options remain typed correctly.
const BOOLEAN_OPTION_KEYS = new Set<string>([
  'PasswordLoginEnabled',
  'PasswordRegisterEnabled',
  'RegisterEnabled',
  'EmailVerificationEnabled',
  'EmailDomainRestrictionEnabled',
  'GitHubOAuthEnabled',
  'OidcEnabled',
  'WeChatAuthEnabled',
  'TurnstileCheckEnabled',
  'AutomaticDisableChannelEnabled',
  'AutomaticEnableChannelEnabled',
  'ApproximateTokenEnabled',
  'LogConsumeEnabled',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
]);

const isBooleanOptionKey = (key: string) => BOOLEAN_OPTION_KEYS.has(key);

const OIDC_DISCOVERY_KEY_MAP: Record<string, string> = {
  authorization_endpoint: 'OidcAuthorizationEndpoint',
  token_endpoint: 'OidcTokenEndpoint',
  userinfo_endpoint: 'OidcUserinfoEndpoint',
};

export function SystemSettings() {
  const { t } = useTranslation();
  const [options, setOptions] = useState<OptionRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [hasLoaded, setHasLoaded] = useState(false);
  const { notify } = useNotifications();

  const OPTION_GROUPS: OptionGroup[] = useMemo(
    () => [
      {
        id: 'authentication',
        title: t('system_settings.groups.authentication.title'),
        description: t('system_settings.groups.authentication.description'),
        keys: [
          'PasswordLoginEnabled',
          'PasswordRegisterEnabled',
          'RegisterEnabled',
          'EmailVerificationEnabled',
          'EmailDomainRestrictionEnabled',
          'EmailDomainWhitelist',
        ],
      },
      {
        id: 'oauth',
        title: t('system_settings.groups.oauth.title'),
        description: t('system_settings.groups.oauth.description'),
        keys: [
          'GitHubOAuthEnabled',
          'GitHubClientId',
          'GitHubClientSecret',
          'OidcEnabled',
          'OidcClientId',
          'OidcClientSecret',
          'OidcWellKnown',
          'OidcAuthorizationEndpoint',
          'OidcTokenEndpoint',
          'OidcUserinfoEndpoint',
          'LarkClientId',
          'LarkClientSecret',
          'WeChatAuthEnabled',
          'WeChatServerAddress',
          'WeChatServerToken',
          'WeChatAccountQRCodeImageURL',
        ],
      },
      {
        id: 'security',
        title: t('system_settings.groups.security.title'),
        description: t('system_settings.groups.security.description'),
        keys: ['TurnstileCheckEnabled', 'TurnstileSiteKey', 'TurnstileSecretKey'],
      },
      {
        id: 'email',
        title: t('system_settings.groups.email.title'),
        description: t('system_settings.groups.email.description'),
        keys: ['EmailProvider', 'SMTPServer', 'SMTPPort', 'SMTPAccount', 'SMTPToken', 'SMTPFrom', 'ResendAPIKey'],
      },
      {
        id: 'branding',
        title: t('system_settings.groups.branding.title'),
        description: t('system_settings.groups.branding.description'),
        keys: ['SystemName', 'Logo', 'Footer', 'Notice', 'About', 'HomePageContent', 'Theme'],
      },
      {
        id: 'links',
        title: t('system_settings.groups.links.title'),
        description: t('system_settings.groups.links.description'),
        keys: ['TopUpLink', 'ChatLink', 'ServerAddress'],
      },
      {
        id: 'quota',
        title: t('system_settings.groups.quota.title'),
        description: t('system_settings.groups.quota.description'),
        keys: [
          'QuotaForNewUser',
          'QuotaForInviter',
          'QuotaForInvitee',
          'QuotaRemindThreshold',
          'PreConsumedQuota',
          'QuotaPerUnit',
          'DisplayInCurrencyEnabled',
          'DisplayTokenStatEnabled',
          'ApproximateTokenEnabled',
        ],
      },
      {
        id: 'channels',
        title: t('system_settings.groups.channels.title'),
        description: t('system_settings.groups.channels.description'),
        keys: ['AutomaticDisableChannelEnabled', 'AutomaticEnableChannelEnabled', 'ChannelDisableThreshold', 'RetryTimes'],
      },
      {
        id: 'logging',
        title: t('system_settings.groups.logging.title'),
        description: t('system_settings.groups.logging.description'),
        keys: ['LogConsumeEnabled', 'MessagePusherAddress', 'MessagePusherToken'],
      },
    ],
    [t]
  );

  // Map each option key to a concise, user-friendly description for tooltips
  const descriptions = useMemo<Record<string, string>>(
    () => ({
      // Authentication & Registration
      PasswordLoginEnabled: t('system_settings.descriptions.PasswordLoginEnabled'),
      PasswordRegisterEnabled: t('system_settings.descriptions.PasswordRegisterEnabled'),
      RegisterEnabled: t('system_settings.descriptions.RegisterEnabled'),
      EmailVerificationEnabled: t('system_settings.descriptions.EmailVerificationEnabled'),
      EmailDomainRestrictionEnabled: t('system_settings.descriptions.EmailDomainRestrictionEnabled'),
      EmailDomainWhitelist: t('system_settings.descriptions.EmailDomainWhitelist'),

      // OAuth / SSO Providers
      GitHubOAuthEnabled: t('system_settings.descriptions.GitHubOAuthEnabled'),
      GitHubClientId: t('system_settings.descriptions.GitHubClientId'),
      GitHubClientSecret: t('system_settings.descriptions.GitHubClientSecret'),
      OidcEnabled: t('system_settings.descriptions.OidcEnabled'),
      OidcClientId: t('system_settings.descriptions.OidcClientId'),
      OidcClientSecret: t('system_settings.descriptions.OidcClientSecret'),
      OidcWellKnown: t('system_settings.descriptions.OidcWellKnown'),
      OidcAuthorizationEndpoint: t('system_settings.descriptions.OidcAuthorizationEndpoint'),
      OidcTokenEndpoint: t('system_settings.descriptions.OidcTokenEndpoint'),
      OidcUserinfoEndpoint: t('system_settings.descriptions.OidcUserinfoEndpoint'),
      LarkClientId: t('system_settings.descriptions.LarkClientId'),
      LarkClientSecret: t('system_settings.descriptions.LarkClientSecret'),
      WeChatAuthEnabled: t('system_settings.descriptions.WeChatAuthEnabled'),
      WeChatServerAddress: t('system_settings.descriptions.WeChatServerAddress'),
      WeChatServerToken: t('system_settings.descriptions.WeChatServerToken'),
      WeChatAccountQRCodeImageURL: t('system_settings.descriptions.WeChatAccountQRCodeImageURL'),

      // Anti-bot / Security
      TurnstileCheckEnabled: t('system_settings.descriptions.TurnstileCheckEnabled'),
      TurnstileSiteKey: t('system_settings.descriptions.TurnstileSiteKey'),
      TurnstileSecretKey: t('system_settings.descriptions.TurnstileSecretKey'),

      // Email
      EmailProvider: t('system_settings.descriptions.EmailProvider'),
      SMTPServer: t('system_settings.descriptions.SMTPServer'),
      SMTPPort: t('system_settings.descriptions.SMTPPort'),
      SMTPAccount: t('system_settings.descriptions.SMTPAccount'),
      SMTPToken: t('system_settings.descriptions.SMTPToken'),
      SMTPFrom: t('system_settings.descriptions.SMTPFrom'),
      ResendAPIKey: t('system_settings.descriptions.ResendAPIKey'),

      // Branding & Content
      SystemName: t('system_settings.descriptions.SystemName'),
      Logo: t('system_settings.descriptions.Logo'),
      Footer: t('system_settings.descriptions.Footer'),
      Notice: t('system_settings.descriptions.Notice'),
      About: t('system_settings.descriptions.About'),
      HomePageContent: t('system_settings.descriptions.HomePageContent'),
      Theme: t('system_settings.descriptions.Theme'),

      // Links
      TopUpLink: t('system_settings.descriptions.TopUpLink'),
      ChatLink: t('system_settings.descriptions.ChatLink'),
      ServerAddress: t('system_settings.descriptions.ServerAddress'),

      // Quota & Billing
      QuotaForNewUser: t('system_settings.descriptions.QuotaForNewUser'),
      QuotaForInviter: t('system_settings.descriptions.QuotaForInviter'),
      QuotaForInvitee: t('system_settings.descriptions.QuotaForInvitee'),
      QuotaRemindThreshold: t('system_settings.descriptions.QuotaRemindThreshold'),
      PreConsumedQuota: t('system_settings.descriptions.PreConsumedQuota'),
      GroupRatio: t('system_settings.descriptions.GroupRatio'),
      QuotaPerUnit: t('system_settings.descriptions.QuotaPerUnit'),
      DisplayInCurrencyEnabled: t('system_settings.descriptions.DisplayInCurrencyEnabled'),
      DisplayTokenStatEnabled: t('system_settings.descriptions.DisplayTokenStatEnabled'),
      ApproximateTokenEnabled: t('system_settings.descriptions.ApproximateTokenEnabled'),

      // Channels & Reliability
      AutomaticDisableChannelEnabled: t('system_settings.descriptions.AutomaticDisableChannelEnabled'),
      AutomaticEnableChannelEnabled: t('system_settings.descriptions.AutomaticEnableChannelEnabled'),
      ChannelDisableThreshold: t('system_settings.descriptions.ChannelDisableThreshold'),
      RetryTimes: t('system_settings.descriptions.RetryTimes'),

      // Logging / Metrics / Integrations
      LogConsumeEnabled: t('system_settings.descriptions.LogConsumeEnabled'),
      MessagePusherAddress: t('system_settings.descriptions.MessagePusherAddress'),
      MessagePusherToken: t('system_settings.descriptions.MessagePusherToken'),
    }),
    [t]
  );

  const load = async () => {
    setLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/option/');
      if (res.data?.success) setOptions(res.data.data || []);
    } finally {
      setLoading(false);
      setHasLoaded(true);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const save = useCallback(
    async (key: string, value: string | string[]) => {
      // Intercept array values for multi-tag options like EmailDomainWhitelist
      const serialized = Array.isArray(value) ? value.join(',') : value;
      // Never retain a submitted secret in client state. GetOptions strips sensitive
      // values on reload, so mirror that here: store '' for sensitive keys so the
      // parent → child prop sync can never repopulate the plaintext secret.
      const valueForState = SENSITIVE_OPTION_KEYS.has(key) ? '' : serialized;
      try {
        // Unified API call - complete URL with /api prefix
        const response = await api.put('/api/option/', { key, value: serialized });
        if (!response.data?.success) {
          throw new Error(response.data?.message || t('system_settings.save_failed'));
        }
        setOptions((prev) => {
          const index = prev.findIndex((opt) => opt.key === key);
          if (index === -1) {
            return [...prev, { key, value: valueForState }];
          }
          return prev.map((opt) => (opt.key === key ? { ...opt, value: valueForState } : opt));
        });
        notify({
          type: 'success',
          title: t('system_settings.saved_success'),
          message: t('system_settings.saved_message', { key }),
        });
      } catch (error: any) {
        console.error('Error saving option:', error);
        const errMsg = error?.response?.data?.message || error?.message || 'Unknown error';
        notify({
          type: 'error',
          title: t('system_settings.save_failed'),
          message: String(errMsg),
        });
        throw error;
      }
    },
    [notify, t]
  );

  // clearSensitive removes a stored secret via an explicit clear:true request.
  // Empty saves are ignored server-side for sensitive keys, so this is the only
  // way to wipe a persisted credential (e.g. a ResendAPIKey) from the admin UI.
  const clearSensitive = useCallback(
    async (key: string) => {
      try {
        const response = await api.put('/api/option/', { key, value: '', clear: true });
        if (!response.data?.success) {
          throw new Error(response.data?.message || t('system_settings.save_failed'));
        }
        setOptions((prev) => {
          const index = prev.findIndex((opt) => opt.key === key);
          if (index === -1) {
            return [...prev, { key, value: '' }];
          }
          return prev.map((opt) => (opt.key === key ? { ...opt, value: '' } : opt));
        });
        notify({
          type: 'success',
          title: t('system_settings.saved_success'),
          message: t('system_settings.saved_message', { key }),
        });
      } catch (error: any) {
        console.error('Error clearing option:', error);
        const errMsg = error?.response?.data?.message || error?.message || 'Unknown error';
        notify({
          type: 'error',
          title: t('system_settings.save_failed'),
          message: String(errMsg),
        });
        throw error;
      }
    },
    [notify, t]
  );

  const optionsMap = useMemo(() => {
    const map: Record<string, OptionRow> = {};
    for (const opt of options) {
      map[opt.key] = opt;
    }
    return map;
  }, [options]);

  const oidcWellKnownValue = optionsMap['OidcWellKnown']?.value ?? '';

  const handleOidcDiscovery = useCallback(async () => {
    const url = (oidcWellKnownValue || '').trim();
    if (!/^https?:\/\//i.test(url)) {
      notify({
        type: 'error',
        title: t('system_settings.oidc_discovery.failed_title'),
        message: t('system_settings.oidc_discovery.invalid_url'),
      });
      return;
    }

    try {
      // Direct browser fetch (NOT through api client) — IDP is external.
      const res = await fetch(url);
      const payload = await res.json();
      const targetKeys = Object.keys(OIDC_DISCOVERY_KEY_MAP);
      const missing = targetKeys.filter((k) => !payload?.[k]);
      if (missing.length > 0) {
        notify({
          type: 'error',
          title: t('system_settings.oidc_discovery.failed_title'),
          message: t('system_settings.oidc_discovery.missing_endpoints', { endpoints: missing.join(', ') }),
        });
        return;
      }

      // Save each endpoint via existing per-key save logic
      for (const sourceKey of targetKeys) {
        const optionKey = OIDC_DISCOVERY_KEY_MAP[sourceKey];
        const response = await api.put('/api/option/', { key: optionKey, value: String(payload[sourceKey]) });
        if (!response.data?.success) {
          throw new Error(response.data?.message || t('system_settings.oidc_discovery.failed_title'));
        }
      }

      // Update local options state in one pass
      setOptions((prev) => {
        const next = [...prev];
        for (const sourceKey of targetKeys) {
          const optionKey = OIDC_DISCOVERY_KEY_MAP[sourceKey];
          const value = String(payload[sourceKey]);
          const idx = next.findIndex((opt) => opt.key === optionKey);
          if (idx === -1) {
            next.push({ key: optionKey, value });
          } else {
            next[idx] = { ...next[idx], value };
          }
        }
        return next;
      });

      notify({
        type: 'success',
        title: t('system_settings.oidc_discovery.success_title'),
        message: t('system_settings.oidc_discovery.success_message'),
      });
    } catch (error: any) {
      console.error('OIDC discovery failed:', error);
      const errMsg = error?.message || 'Unknown error';
      notify({
        type: 'error',
        title: t('system_settings.oidc_discovery.failed_title'),
        message: String(errMsg),
      });
    }
  }, [notify, oidcWellKnownValue, t]);

  const uncategorizedOptions = useMemo(() => options.filter((opt) => !OPTION_GROUP_KEY_SET.has(opt.key)), [options]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle>{t('system_settings.title')}</CardTitle>
          <CardDescription>{t('system_settings.description')}</CardDescription>
        </div>
        <Button variant="outline" onClick={load} disabled={loading}>
          {t('system_settings.refresh')}
        </Button>
      </CardHeader>
      <CardContent>
        {options.length > 0 ? (
          <TooltipProvider>
            <div className="space-y-10">
              {OPTION_GROUPS.map((group) => {
                const groupOptions = group.keys.map((key) => {
                  const option = optionsMap[key] ?? { key, value: '' };
                  return {
                    option,
                    isSensitive: SENSITIVE_OPTION_KEYS.has(key),
                  };
                });

                return (
                  <section key={group.id} className="space-y-4">
                    <div className="space-y-1">
                      <h3 className="text-lg font-semibold leading-6">{group.title}</h3>
                      {group.description && <p className="text-sm text-muted-foreground">{group.description}</p>}
                    </div>
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                      {groupOptions.map(({ option, isSensitive }) => {
                        if (option.key === 'EmailDomainWhitelist') {
                          return (
                            <EmailDomainWhitelistItem
                              key={option.key}
                              option={option}
                              description={descriptions[option.key]}
                              onSave={save}
                            />
                          );
                        }
                        if (option.key === 'OidcWellKnown') {
                          return (
                            <OptionItem
                              key={option.key}
                              option={option}
                              description={descriptions[option.key]}
                              isSensitive={isSensitive}
                              isBoolean={isBooleanOptionKey(option.key)}
                              onSave={save}
                              onClear={clearSensitive}
                              extraAction={
                                <Button type="button" variant="outline" onClick={handleOidcDiscovery}>
                                  {t('system_settings.oidc_discovery.button')}
                                </Button>
                              }
                            />
                          );
                        }
                        return (
                          <OptionItem
                            key={option.key}
                            option={option}
                            description={descriptions[option.key]}
                            isSensitive={isSensitive}
                            isBoolean={isBooleanOptionKey(option.key)}
                            enumChoices={ENUM_OPTION_KEYS[option.key]}
                            onSave={save}
                            onClear={clearSensitive}
                          />
                        );
                      })}
                    </div>
                  </section>
                );
              })}

              {uncategorizedOptions.length > 0 && (
                <section className="space-y-4">
                  <div className="space-y-1">
                    <h3 className="text-lg font-semibold leading-6">{t('system_settings.groups.other.title')}</h3>
                    <p className="text-sm text-muted-foreground">{t('system_settings.groups.other.description')}</p>
                  </div>
                  <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                    {uncategorizedOptions.map((opt) => (
                      <OptionItem
                        key={opt.key}
                        option={opt}
                        description={descriptions[opt.key]}
                        isSensitive={SENSITIVE_OPTION_KEYS.has(opt.key)}
                        isBoolean={isBooleanOptionKey(opt.key)}
                        enumChoices={ENUM_OPTION_KEYS[opt.key]}
                        onSave={save}
                        onClear={clearSensitive}
                      />
                    ))}
                  </div>
                </section>
              )}
            </div>
          </TooltipProvider>
        ) : hasLoaded ? (
          <div className="text-center text-sm text-muted-foreground py-8">{t('system_settings.no_options')}</div>
        ) : (
          <div className="text-center text-sm text-muted-foreground py-8">{t('system_settings.loading')}</div>
        )}
      </CardContent>
    </Card>
  );
}

export default SystemSettings;
