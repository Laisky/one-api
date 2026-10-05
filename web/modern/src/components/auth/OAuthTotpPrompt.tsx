import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { OAUTH_TOTP_CODE_PATTERN, verifyOAuthTotpCode, type OAuthLoginUser } from '@/lib/oauth-totp';
import { cn } from '@/lib/utils';
import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

/** OAuthTotpPromptProps configures the TOTP step that completes a pending OAuth or WeChat login. */
export interface OAuthTotpPromptProps {
  /** onSuccess receives the authenticated user once POST /api/oauth/totp accepts the code. */
  onSuccess: (user: OAuthLoginUser) => void;
  /** onBackToLogin abandons the pending challenge and returns the user to the login entry point. */
  onBackToLogin: () => void;
  /** className is appended to the root form element for layout tweaks by the host page or dialog. */
  className?: string;
}

/**
 * OAuthTotpPrompt renders the six-digit TOTP form shown after an OAuth or WeChat login answers `totp_required`.
 * It takes success and back-to-login callbacks, posts the code to POST /api/oauth/totp, keeps the prompt open with
 * an error on a wrong or throttled code, disables verification once the pending challenge has expired, and returns
 * the form element. It never re-sends the consumed OAuth code; only the TOTP code is submitted.
 */
export function OAuthTotpPrompt({ onSuccess, onBackToLogin, className }: OAuthTotpPromptProps) {
  const { t } = useTranslation();
  const inputId = useId();
  const errorId = useId();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [expired, setExpired] = useState(false);
  const [verifying, setVerifying] = useState(false);

  // Keep the code input focused whenever it is editable: on mount and again after each failed attempt
  // re-enables it, so the user can immediately type the next code.
  useEffect(() => {
    if (!verifying && !expired) inputRef.current?.focus();
  }, [verifying, expired]);

  /**
   * handleSubmit validates the code, submits it, and maps the classified result onto the prompt state.
   * It takes the optional form event and returns once the request settles.
   */
  const handleSubmit = async (event?: FormEvent<HTMLFormElement>) => {
    event?.preventDefault();
    if (verifying || expired) return;

    const totpCode = code.trim();
    if (!OAUTH_TOTP_CODE_PATTERN.test(totpCode)) {
      setError(t('auth.login.totp_invalid'));
      return;
    }

    setVerifying(true);
    setError('');
    const result = await verifyOAuthTotpCode(totpCode);

    switch (result.kind) {
      case 'success':
        onSuccess(result.user);
        return;
      case 'expired':
        setExpired(true);
        setError(t('auth.oauth.totp.expired'));
        break;
      case 'rate_limited':
        setError(t('auth.oauth.totp.rate_limited'));
        break;
      case 'invalid':
        setCode('');
        setError(result.message || t('auth.oauth.totp.invalid_code'));
        break;
      default:
        setError(result.message || t('auth.oauth.totp.failed'));
        break;
    }
    setVerifying(false);
  };

  return (
    <form data-testid="oauth-totp-prompt" className={cn('space-y-4', className)} onSubmit={(event) => void handleSubmit(event)} noValidate>
      <div className="space-y-2">
        <Label htmlFor={inputId}>{t('auth.login.totp_label')}</Label>
        <Input
          id={inputId}
          ref={inputRef}
          value={code}
          autoComplete="one-time-code"
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={6}
          placeholder={t('auth.login.totp_placeholder')}
          disabled={verifying || expired}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : undefined}
          onChange={(event) => {
            setCode(event.target.value.replace(/\D/g, '').slice(0, 6));
            if (error && !expired) setError('');
          }}
        />
      </div>
      {error && (
        <p id={errorId} role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Button type="submit" className="w-full" disabled={verifying || expired || code.length !== 6}>
        {verifying ? t('auth.oauth.totp.verifying') : t('auth.login.verify_totp')}
      </Button>
      <Button type="button" variant="outline" className="w-full" onClick={onBackToLogin} disabled={verifying}>
        {t('auth.login.back_to_login')}
      </Button>
    </form>
  );
}

export default OAuthTotpPrompt;
