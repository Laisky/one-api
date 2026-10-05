import { OAuthTotpPrompt } from '@/components/auth/OAuthTotpPrompt';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { api } from '@/lib/api';
import { isTotpRequiredResponse, resolveOAuthStateRedirect, type OAuthLoginUser } from '@/lib/oauth-totp';
import { useAuthStore } from '@/lib/stores/auth';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';

/**
 * OidcOAuthPage handles the OIDC login callback: it exchanges the code and state with the backend, retries
 * transient failures, prompts for a TOTP code when the account has two-factor authentication enabled, and returns
 * the callback card.
 */
export function OidcOAuthPage() {
  const [searchParams] = useSearchParams();
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState(() => t('auth.oauth.oidc.prompt.processing'));
  const [totpRequired, setTotpRequired] = useState(false);
  // The callback code/state are single-use; remember which pair was sent so a re-run of the effect
  // (for example React StrictMode's double invocation) never consumes or retries it twice.
  const sentRequestRef = useRef('');
  const navigate = useNavigate();
  const { login } = useAuthStore();

  /**
   * completeLogin stores the authenticated user and navigates to the safe `redirect_to` target carried in the
   * OAuth state, falling back to the home page. It takes the user payload and raw state and returns nothing.
   */
  const completeLogin = useCallback(
    (user: OAuthLoginUser, state: string | null) => {
      login(user, '');
      navigate(resolveOAuthStateRedirect(state), {
        state: { message: t('auth.oauth.oidc.login_success') },
      });
    },
    [login, navigate, t]
  );

  /**
   * sendCode exchanges the callback code and state with the backend and routes on the answer. It takes the code,
   * the state, and the current retry count, retries transient failures with linear backoff up to three times,
   * stops without retrying on a TOTP challenge, and resolves once the attempt has been handled.
   */
  const sendCode = useCallback(
    async (code: string, state: string, retryCount = 0): Promise<void> => {
      try {
        // Unified API call - complete URL with /api prefix
        const response = await api.get(`/api/oauth/oidc?code=${code}&state=${state}`);
        // Two-factor accounts get the password-login challenge. The code/state are already consumed, so
        // stop here (no retry) and let the user finish with POST /api/oauth/totp.
        if (isTotpRequiredResponse(response.data)) {
          setTotpRequired(true);
          return;
        }

        const { success, message, data } = response.data;

        if (success) {
          if (message === 'bind') {
            navigate('/settings', {
              state: { message: t('auth.oauth.oidc.bind_success') },
            });
          } else {
            completeLogin(data, state);
          }
        } else {
          throw new Error(message || t('auth.oauth.oidc.failed'));
        }
      } catch (error) {
        console.warn(`OIDC OAuth callback attempt ${retryCount + 1} failed: ${error instanceof Error ? error.message : String(error)}`);
        if (retryCount >= 3) {
          setPrompt(t('auth.oauth.oidc.prompt.failed'));
          setTimeout(() => {
            navigate('/login', {
              state: { message: t('auth.oauth.oidc.failed_redirect') },
            });
          }, 2000);
          return;
        }

        const nextRetry = retryCount + 1;
        setPrompt(t('auth.oauth.oidc.prompt.retry', { retry: nextRetry }));

        // Exponential backoff
        const delay = nextRetry * 2000;
        setTimeout(() => {
          sendCode(code, state, nextRetry);
        }, delay);
      }
    },
    [completeLogin, navigate, t]
  );

  useEffect(() => {
    const code = searchParams.get('code');
    const state = searchParams.get('state');

    if (!code || !state) {
      navigate('/login', {
        state: { message: t('auth.oauth.oidc.invalid_params') },
      });
      return;
    }

    const requestKey = `${code}\n${state}`;
    if (sentRequestRef.current === requestKey) return;
    sentRequestRef.current = requestKey;

    sendCode(code, state);
  }, [searchParams, navigate, sendCode, t]);

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">{totpRequired ? t('auth.oauth.totp.title') : t('auth.oauth.oidc.title')}</CardTitle>
          <CardDescription>{totpRequired ? t('auth.oauth.totp.description') : t('auth.oauth.oidc.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          {totpRequired ? (
            <OAuthTotpPrompt
              onSuccess={(user) => completeLogin(user, searchParams.get('state'))}
              onBackToLogin={() => navigate('/login')}
            />
          ) : (
            <div className="flex items-center justify-center py-8">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <span className="ml-3 text-sm text-muted-foreground">{prompt}</span>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default OidcOAuthPage;
