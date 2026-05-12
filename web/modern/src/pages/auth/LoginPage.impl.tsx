import Turnstile from '@/components/Turnstile';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { useSystemStatus } from '@/hooks/useSystemStatus';
import { api, isSafeInternalPath } from '@/lib/api';
import { buildGitHubOAuthUrl, buildOidcOAuthUrl, getOAuthState } from '@/lib/oauth';
import { useAuthStore } from '@/lib/stores/auth';
import { zodResolver } from '@hookform/resolvers/zod';
import { browserSupportsWebAuthn, startAuthentication } from '@simplewebauthn/browser';
import { useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import * as z from 'zod';

const loginSchema = (t: (key: string) => string) =>
  z.object({
    username: z.string().min(1, t('auth.login.username_required')),
    password: z.string().min(1, t('auth.login.password_required')),
    totp_code: z
      .string()
      .optional()
      .refine((val) => !val || val.length === 6, {
        message: t('auth.login.totp_invalid'),
      }),
  });

type LoginForm = z.infer<ReturnType<typeof loginSchema>>;

export function LoginPage() {
  const { t } = useTranslation();
  const [isLoading, setIsLoading] = useState(false);
  const [totpRequired, setTotpRequired] = useState(false);
  const [successMessage, setSuccessMessage] = useState<string>('');
  const [totpValue, setTotpValue] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [turnstileRequired, setTurnstileRequired] = useState(false);
  const totpRef = useRef<HTMLInputElement | null>(null);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const { login } = useAuthStore();
  const { systemStatus } = useSystemStatus();
  // Default to enabled when the flag is missing (e.g. status hasn't loaded yet
  // or the backend predates this field) so we don't accidentally lock out
  // users on slow networks. Only an explicit `false` hides the password form.
  const passwordLoginEnabled = systemStatus?.password_login !== false;
  const passwordRegisterEnabled = systemStatus?.password_register !== false;
  const turnstileEnabled = Boolean(systemStatus?.turnstile_check);
  // Only show Turnstile after the server tells us it's required (i.e. after a failed login attempt).
  const turnstileRenderable = turnstileRequired && turnstileEnabled && Boolean(systemStatus?.turnstile_site_key);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const passkeySupported = typeof window !== 'undefined' && browserSupportsWebAuthn();
  const [wechatOpen, setWechatOpen] = useState(false);
  const [wechatCode, setWechatCode] = useState('');
  const [wechatLoading, setWechatLoading] = useState(false);
  const [wechatError, setWechatError] = useState('');

  const onPasskeyLogin = async () => {
    setPasskeyLoading(true);
    form.clearErrors('root');
    try {
      // Step 1: Get assertion options
      const beginRes = await api.post('/api/user/passkey/login/begin');
      if (!beginRes.data.success) {
        form.setError('root', { message: beginRes.data.message || t('auth.login.passkey_failed') });
        return;
      }

      // Step 2: Authenticate via browser WebAuthn API
      const assertionResp = await startAuthentication({ optionsJSON: beginRes.data.data.publicKey });

      // Step 3: Send assertion to server
      const finishRes = await api.post('/api/user/passkey/login/finish', assertionResp);
      const { success, message, data: respData } = finishRes.data;

      if (success) {
        login(respData, '');
        const redirectTo = searchParams.get('redirect_to');
        if (redirectTo) {
          try {
            const decodedPath = decodeURIComponent(redirectTo);
            // Reject anything that isn't a same-origin internal path (e.g. `//evil.com`).
            // Also bounce back to /dashboard if the target is another /login URL — that
            // would just send us back here.
            if (isSafeInternalPath(decodedPath) && !decodedPath.startsWith('/login')) {
              navigate(decodedPath);
            } else {
              navigate('/dashboard');
            }
          } catch {
            navigate('/dashboard');
          }
        } else {
          navigate('/dashboard');
        }
      } else {
        form.setError('root', { message: message || t('auth.login.passkey_failed') });
      }
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : t('auth.login.passkey_failed');
      // Don't show error if user cancelled
      if (!msg.includes('cancelled') && !msg.includes('AbortError')) {
        form.setError('root', { message: msg });
      }
    } finally {
      setPasskeyLoading(false);
    }
  };

  const form = useForm<LoginForm>({
    resolver: zodResolver(loginSchema(t)),
    defaultValues: { username: '', password: '', totp_code: '' },
  });

  useEffect(() => {
    // Check for expired session
    if (searchParams.get('expired')) {
      console.warn(t('auth.login.session_expired'));
    }

    // Handle success messages from navigation state
    if (location.state?.message) {
      setSuccessMessage(location.state.message);
      // Clear the state to prevent showing the message on refresh
      window.history.replaceState({}, document.title);
    }
  }, [searchParams, location.state]);

  const onGitHubOAuth = async () => {
    if (!systemStatus.github_client_id) return;
    try {
      // Request state from backend to prevent CSRF
      const state = await getOAuthState();
      const redirectUri = `${window.location.origin}/oauth/github`;
      const url = buildGitHubOAuthUrl(systemStatus.github_client_id, state, redirectUri);
      window.location.href = url;
    } catch (e) {
      // Fallback: try without state if backend unavailable
      const redirectUri = `${window.location.origin}/oauth/github`;
      const url = buildGitHubOAuthUrl(systemStatus.github_client_id, '', redirectUri);
      window.location.href = url;
    }
  };

  const onLarkOAuth = () => {
    if (systemStatus.lark_client_id) {
      const redirectUri = `${window.location.origin}/oauth/lark`;
      window.location.href = `https://open.larksuite.com/open-apis/authen/v1/index?app_id=${encodeURIComponent(systemStatus.lark_client_id)}&redirect_uri=${encodeURIComponent(redirectUri)}`;
    }
  };

  const onOidcOAuth = async () => {
    if (!systemStatus.oidc_client_id || !systemStatus.oidc_authorization_endpoint) return;
    try {
      const state = await getOAuthState();
      const redirectUri = `${window.location.origin}/oauth/oidc`;
      const url = buildOidcOAuthUrl(systemStatus.oidc_authorization_endpoint, systemStatus.oidc_client_id, state, redirectUri);
      window.location.href = url;
    } catch {
      const redirectUri = `${window.location.origin}/oauth/oidc`;
      const url = buildOidcOAuthUrl(systemStatus.oidc_authorization_endpoint, systemStatus.oidc_client_id, '', redirectUri);
      window.location.href = url;
    }
  };

  const onWeChatOpen = () => {
    setWechatCode('');
    setWechatError('');
    setWechatOpen(true);
  };

  const onWeChatSubmit = async () => {
    const code = wechatCode.trim();
    if (!code) {
      setWechatError(t('auth.login.wechat_code_required'));
      return;
    }
    setWechatLoading(true);
    setWechatError('');
    try {
      const response = await api.get(`/api/oauth/wechat?code=${encodeURIComponent(code)}`);
      const { success, message, data } = response.data;
      if (!success) {
        setWechatError(message || t('auth.login.wechat_failed'));
        return;
      }

      if (message === 'bind') {
        setWechatOpen(false);
        navigate('/settings', { state: { message: t('auth.oauth.wechat.bind_success') } });
        return;
      }

      login(data, '');
      setWechatOpen(false);

      const redirectTo = searchParams.get('redirect_to');
      if (redirectTo) {
        try {
          const decodedPath = decodeURIComponent(redirectTo);
          if (isSafeInternalPath(decodedPath) && !decodedPath.startsWith('/login')) {
            navigate(decodedPath);
            return;
          }
        } catch (err) {
          console.error('Invalid redirect_to parameter:', err);
        }
      }
      navigate('/dashboard');
    } catch (error) {
      setWechatError(error instanceof Error ? error.message : t('auth.login.wechat_failed'));
    } finally {
      setWechatLoading(false);
    }
  };

  const onSubmit = async (data: LoginForm) => {
    // Only gate on Turnstile if it's been required (after a prior failed attempt).
    if (turnstileRequired && turnstileEnabled && !turnstileToken) {
      form.setError('root', { message: t('auth.login.turnstile_required') });
      return;
    }
    setIsLoading(true);
    try {
      const payload: Record<string, string> = {
        username: data.username,
        password: data.password,
      };
      if (totpRequired && totpValue) payload.totp_code = totpValue;
      // Only include Turnstile token when it's required and available.
      const query = turnstileRequired && turnstileToken ? `?turnstile=${encodeURIComponent(turnstileToken)}` : '';
      const response = await api.post(`/api/user/login${query}`, payload);
      const { success, message, data: respData } = response.data;
      const m = typeof message === 'string' ? message.trim().toLowerCase() : '';
      const dataTotp = !!(
        respData &&
        (respData.totp_required === true || respData.totp_required === 'true' || respData.totp_required === 1)
      );
      const needsTotp = !success && (dataTotp || m === 'totp_required' || m.includes('totp'));

      // Check if the server is now requiring Turnstile (after failed login).
      if (!success && respData?.turnstile_required) {
        setTurnstileRequired(true);
        setTurnstileToken('');
      }

      if (needsTotp) {
        setTotpRequired(true);
        setTotpValue('');
        form.setValue('totp_code', '');
        form.setError('root', { message: t('auth.login.totp_required') });
        return;
      }

      if (success) {
        login(respData, '');

        // Get redirect_to parameter from URL
        const redirectTo = searchParams.get('redirect_to');

        // Handle default root password warning
        if (data.username === 'root' && data.password === '123456') {
          navigate('/users/edit');
          console.warn(t('auth.login.root_password_warning'));
        } else if (redirectTo) {
          // Decode and navigate to the original page, enforcing same-origin
          // and rejecting any /login target to break potential redirect loops.
          try {
            const decodedPath = decodeURIComponent(redirectTo);
            if (isSafeInternalPath(decodedPath) && !decodedPath.startsWith('/login')) {
              navigate(decodedPath);
            } else {
              navigate('/dashboard');
            }
          } catch (error) {
            console.error('Invalid redirect_to parameter:', error);
            navigate('/dashboard');
          }
        } else {
          navigate('/dashboard');
        }
      } else {
        form.setError('root', {
          message: m === 'totp_required' ? t('auth.login.totp_required') : message || t('auth.login.failed'),
        });
      }
    } catch (error) {
      form.setError('root', {
        message: error instanceof Error ? error.message : t('auth.login.failed'),
      });
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (totpRequired && totpRef.current) totpRef.current.focus();
  }, [totpRequired]);

  const hasOAuthOptions = systemStatus.github_oauth || systemStatus.lark_client_id || systemStatus.oidc || systemStatus.wechat_login;

  const handleTurnstileVerify = (token: string) => {
    setTurnstileToken(token);
    if (!totpRequired && form.formState.errors.root?.message) {
      form.clearErrors('root');
    }
  };

  const handleTurnstileExpire = () => {
    setTurnstileToken('');
  };

  return (
    <div className="flex items-center justify-center min-h-[calc(100dvh-12rem)] py-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          {systemStatus.logo && (
            <div className="flex justify-center mb-4">
              <img
                src={systemStatus.logo}
                alt={systemStatus.system_name ? `${systemStatus.system_name} logo` : 'Site logo'}
                className="h-12 w-auto"
                decoding="async"
              />
            </div>
          )}
          <CardTitle className="text-2xl">
            {t('auth.login.title')}
            {systemStatus.system_name ? ` ${t('common.to')} ${systemStatus.system_name}` : ''}
          </CardTitle>
          <CardDescription>{t('auth.login.subtitle')}</CardDescription>
        </CardHeader>
        <CardContent>
          {!passwordLoginEnabled && (
            <div
              data-testid="password-login-disabled-notice"
              className="mb-4 text-sm rounded-md border border-warning-border bg-warning-muted p-3 text-warning-foreground"
            >
              {hasOAuthOptions || passkeySupported
                ? t('auth.login.password_login_disabled')
                : t('auth.login.password_login_disabled_no_methods')}
            </div>
          )}
          <Form {...form}>
            <form data-testid="login-form" onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel htmlFor="login-username">{t('common.username')}</FormLabel>
                    <FormControl>
                      <Input id="login-username" {...field} disabled={totpRequired} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel htmlFor="login-password">{t('common.password')}</FormLabel>
                    <FormControl>
                      <Input id="login-password" type="password" {...field} disabled={totpRequired} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {totpRequired && (
                <FormField
                  control={form.control}
                  name="totp_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('auth.login.totp_label')}</FormLabel>
                      <FormControl>
                        <Input
                          maxLength={6}
                          placeholder={t('auth.login.totp_placeholder')}
                          {...field}
                          ref={totpRef}
                          inputMode="numeric"
                          pattern="[0-9]*"
                          onChange={(e) => {
                            field.onChange(e);
                            setTotpValue(e.target.value);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              {successMessage && (
                <div className="text-sm text-success-foreground bg-success-muted p-3 rounded-md border border-success-border">
                  {successMessage}
                </div>
              )}
              {form.formState.errors.root && (
                <div className="text-sm text-destructive">
                  {totpRequired ? t('auth.login.totp_required') : form.formState.errors.root.message}
                </div>
              )}
              <Button
                type="submit"
                className="w-full"
                disabled={
                  isLoading || (totpRequired && totpValue.length !== 6) || (turnstileRequired && turnstileEnabled && !turnstileToken)
                }
              >
                {isLoading ? t('auth.login.signing_in') : totpRequired ? t('auth.login.verify_totp') : t('auth.login.title')}
              </Button>

              {turnstileRenderable && systemStatus?.turnstile_site_key && (
                <Turnstile
                  siteKey={systemStatus.turnstile_site_key}
                  onVerify={handleTurnstileVerify}
                  onExpire={handleTurnstileExpire}
                  className="mt-2 flex justify-center"
                />
              )}

              {totpRequired && (
                <Button
                  type="button"
                  variant="outline"
                  className="w-full"
                  onClick={() => {
                    setTotpRequired(false);
                    setTotpValue('');
                    form.setValue('totp_code', '');
                    form.clearErrors('root');
                  }}
                >
                  {t('auth.login.back_to_login')}
                </Button>
              )}

              {passkeySupported && !totpRequired && (
                <>
                  <div className="relative my-2">
                    <div className="absolute inset-0 flex items-center">
                      <span className="w-full border-t" />
                    </div>
                    <div className="relative flex justify-center text-xs uppercase">
                      <span className="bg-card px-2 text-muted-foreground">{t('auth.login.or_use_passkey')}</span>
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full"
                    onClick={onPasskeyLogin}
                    disabled={passkeyLoading || isLoading}
                  >
                    <svg
                      className="w-4 h-4 mr-2"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      aria-hidden="true"
                    >
                      <path d="M2 18v3c0 .6.4 1 1 1h4v-3h3v-3h2l1.4-1.4a6.5 6.5 0 1 0-4-4Z" />
                      <circle cx="16.5" cy="7.5" r=".5" />
                    </svg>
                    {passkeyLoading ? t('auth.login.passkey_signing_in') : t('auth.login.passkey_login')}
                  </Button>
                </>
              )}

              <div className="text-center text-sm space-y-2">
                <Link to="/reset" className="text-primary hover:underline">
                  {t('auth.login.forgot_password')}
                </Link>
                {passwordRegisterEnabled && (
                  <div>
                    {t('auth.login.no_account')}{' '}
                    <Link to="/register" className="text-primary hover:underline">
                      {t('auth.login.sign_up')}
                    </Link>
                  </div>
                )}
              </div>
            </form>
          </Form>

          {hasOAuthOptions && (
            <>
              <Separator className="my-4" />
              <div className="text-center">
                <p className="text-sm text-muted-foreground mb-4">{t('auth.login.or_continue_with')}</p>
                <div className="flex justify-center gap-2">
                  {systemStatus.github_oauth && (
                    <Button variant="outline" size="sm" onClick={onGitHubOAuth}>
                      <svg className="w-4 h-4 mr-2" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                        <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                      </svg>
                      GitHub
                    </Button>
                  )}
                  {systemStatus.wechat_login && (
                    <Button variant="outline" size="sm" onClick={onWeChatOpen}>
                      <svg className="w-4 h-4 mr-2" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                        <path d="M8.5 4C4.36 4 1 6.69 1 10c0 1.89 1.1 3.56 2.79 4.65L3 17l2.74-1.43c.74.18 1.51.28 2.31.28.27 0 .54-.01.8-.04-.17-.5-.26-1.03-.26-1.58 0-3.18 3.13-5.77 7-5.77.13 0 .26 0 .39.01C15.3 5.84 12.16 4 8.5 4zm-2 2c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm5 0c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm4.5 4.5c-3.59 0-6.5 2.24-6.5 5s2.91 5 6.5 5c.63 0 1.24-.07 1.81-.21L20 21.5l-.6-2.04C21.65 18.42 23 16.78 23 14.5c0-2.76-2.91-5-6.5-5zm-2 2.25c.41 0 .75.34.75.75s-.34.75-.75.75-.75-.34-.75-.75.34-.75.75-.75zm4 0c.41 0 .75.34.75.75s-.34.75-.75.75-.75-.34-.75-.75.34-.75.75-.75z" />
                      </svg>
                      WeChat
                    </Button>
                  )}
                  {systemStatus.lark_client_id && (
                    <Button variant="outline" size="sm" onClick={onLarkOAuth}>
                      <svg className="w-4 h-4 mr-2" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                        <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" />
                      </svg>
                      Lark
                    </Button>
                  )}
                  {systemStatus.oidc && (
                    <Button variant="outline" size="sm" onClick={onOidcOAuth}>
                      <svg
                        className="w-4 h-4 mr-2"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        aria-hidden="true"
                      >
                        <path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4" />
                      </svg>
                      OIDC
                    </Button>
                  )}
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>
      <Dialog
        open={wechatOpen}
        onOpenChange={(open) => {
          if (!wechatLoading) setWechatOpen(open);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('auth.login.wechat_modal_title')}</DialogTitle>
            <DialogDescription>{t('auth.login.wechat_modal_description')}</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col items-center gap-3">
            {systemStatus.wechat_qrcode && (
              <img src={systemStatus.wechat_qrcode} alt="WeChat QR code" className="max-h-64 w-auto rounded-md border" />
            )}
            <Input
              type="text"
              autoFocus
              value={wechatCode}
              placeholder={t('auth.login.wechat_code_placeholder')}
              disabled={wechatLoading}
              onChange={(e) => {
                setWechatCode(e.target.value);
                if (wechatError) setWechatError('');
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void onWeChatSubmit();
                }
              }}
            />
            {wechatError && <p className="text-sm text-destructive w-full">{wechatError}</p>}
          </div>
          <DialogFooter>
            <Button type="button" onClick={() => void onWeChatSubmit()} disabled={wechatLoading}>
              {wechatLoading ? t('auth.login.signing_in') : t('auth.login.title')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default LoginPage;
