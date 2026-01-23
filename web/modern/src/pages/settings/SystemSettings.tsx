import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { useNotifications } from '@/components/ui/notifications';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { Info } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface OptionRow {
  key: string;
  value: string;
}

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
    title: 'Email (SMTP)',
    description: 'Set up outbound email delivery.',
    keys: ['SMTPServer', 'SMTPPort', 'SMTPAccount', 'SMTPToken', 'SMTPFrom'],
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
      'GroupRatio',
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
  'GitHubClientSecret',
  'OidcClientSecret',
  'LarkClientSecret',
  'WeChatServerToken',
  'MessagePusherToken',
]);

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
        keys: ['SMTPServer', 'SMTPPort', 'SMTPAccount', 'SMTPToken', 'SMTPFrom'],
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
          'GroupRatio',
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

      // Email (SMTP)
      SMTPServer: t('system_settings.descriptions.SMTPServer'),
      SMTPPort: t('system_settings.descriptions.SMTPPort'),
      SMTPAccount: t('system_settings.descriptions.SMTPAccount'),
      SMTPToken: t('system_settings.descriptions.SMTPToken'),
      SMTPFrom: t('system_settings.descriptions.SMTPFrom'),

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
    async (key: string, value: string) => {
      try {
        // Unified API call - complete URL with /api prefix
        await api.put('/api/option/', { key, value });
        setOptions((prev) => {
          const index = prev.findIndex((opt) => opt.key === key);
          if (index === -1) {
            return [...prev, { key, value }];
          }
          return prev.map((opt) => (opt.key === key ? { ...opt, value } : opt));
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

  const optionsMap = useMemo(() => {
    const map: Record<string, OptionRow> = {};
    for (const opt of options) {
      map[opt.key] = opt;
    }
    return map;
  }, [options]);

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
                      {groupOptions.map(({ option, isSensitive }) => (
                        <OptionItem
                          key={option.key}
                          option={option}
                          description={descriptions[option.key]}
                          isSensitive={isSensitive}
                          isBoolean={isBooleanOptionKey(option.key)}
                          onSave={save}
                        />
                      ))}
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
                        onSave={save}
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

interface OptionItemProps {
  option: OptionRow;
  description?: string;
  onSave: (key: string, value: string) => Promise<void>;
  isSensitive?: boolean;
  isBoolean?: boolean;
}

function OptionItem({ option, description, onSave, isSensitive, isBoolean }: OptionItemProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState(option.value);
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    setValue(option.value);
  }, [option.value]);

  const handleSave = useCallback(
    async (overrideValue?: string) => {
      const nextValue = overrideValue ?? value;
      if (isSaving || nextValue === option.value) return;
      setIsSaving(true);
      try {
        await onSave(option.key, nextValue);
        if (isSensitive) {
          setValue('');
        } else {
          setValue(nextValue);
        }
      } catch (_error) {
        setValue(option.value);
      } finally {
        setIsSaving(false);
      }
    },
    [isSaving, isSensitive, onSave, option.key, option.value, value]
  );

  const handleBlur = useCallback(async () => {
    if (value === option.value) return;
    await handleSave();
  }, [handleSave, option.value, value]);

  const handleBooleanChange = useCallback(
    (newValue: string) => {
      setValue(newValue);
      handleSave(newValue);
    },
    [handleSave]
  );

  const placeholder = isSensitive ? t('system_settings.sensitive_placeholder') : undefined;
  const optionValueAriaLabel = t('system_settings.option_value_aria', {
    key: option.key,
  });

  return (
    <div className="border rounded-lg p-4 space-y-3">
      <div className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <span>{option.key}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="inline-flex items-center text-muted-foreground hover:text-foreground focus:outline-none"
              aria-label={t('system_settings.info_about', { key: option.key })}
            >
              <Info className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" align="start" className="max-w-[320px]">
            {description || t('system_settings.no_description')}
          </TooltipContent>
        </Tooltip>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        {isBoolean ? (
          <Select value={value === '' ? undefined : value} onValueChange={handleBooleanChange} disabled={isSaving}>
            <SelectTrigger className="flex-1" aria-label={optionValueAriaLabel} disabled={isSaving}>
              <SelectValue placeholder={t('system_settings.select_value')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="true">{t('system_settings.enabled')}</SelectItem>
              <SelectItem value="false">{t('system_settings.disabled')}</SelectItem>
            </SelectContent>
          </Select>
        ) : (
          <Input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onBlur={handleBlur}
            className="flex-1"
            aria-label={optionValueAriaLabel}
            placeholder={placeholder}
            disabled={isSaving}
          />
        )}
        <Button variant="outline" onClick={() => handleSave()} disabled={isSaving}>
          {isSaving ? t('system_settings.saving') : t('system_settings.save')}
        </Button>
      </div>
      {isSensitive && <p className="text-xs text-muted-foreground">{t('system_settings.sensitive_hint')}</p>}
    </div>
  );
}

export default SystemSettings;
