import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { X } from 'lucide-react';
import { browserSupportsWebAuthn } from '@simplewebauthn/browser';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

const STORAGE_KEY = 'passkey_prompt_dismissed';

/**
 * A one-time dismissible banner shown to logged-in users who have not yet
 * registered a passkey.  Once dismissed the banner never appears again
 * (persisted in localStorage).
 */
export function PasskeyPromptBanner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { isAuthenticated } = useAuthStore();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    // Gate: must be authenticated, browser must support WebAuthn, and not
    // already dismissed.
    if (!isAuthenticated) return;
    if (!browserSupportsWebAuthn()) return;
    if (localStorage.getItem(STORAGE_KEY) === '1') return;

    let cancelled = false;

    (async () => {
      try {
        const res = await api.get('/api/user/passkey');
        if (cancelled) return;
        if (res.data.success) {
          const passkeys: unknown[] = res.data.data ?? [];
          if (passkeys.length === 0) {
            setVisible(true);
          }
        }
      } catch {
        // Silently ignore – the banner is a nice-to-have, not critical.
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isAuthenticated]);

  const dismiss = () => {
    localStorage.setItem(STORAGE_KEY, '1');
    setVisible(false);
  };

  const goToSettings = () => {
    dismiss();
    navigate('/settings');
  };

  if (!visible) return null;

  return (
    <div
      className={cn(
        'relative mx-auto w-full max-w-screen-xl',
        'flex items-center justify-between gap-3',
        'rounded-md border border-info-border bg-info-muted px-4 py-2.5 text-sm text-info-foreground',
        'mx-2 md:mx-4 mt-2'
      )}
      role="status"
    >
      <div className="flex items-center gap-2 min-w-0">
        {/* Key icon */}
        <svg
          className="h-4 w-4 shrink-0"
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
        <span>{t('passkey_prompt.message')}</span>
      </div>

      <div className="flex items-center gap-2 shrink-0">
        <Button size="sm" variant="default" onClick={goToSettings}>
          {t('passkey_prompt.setup_button')}
        </Button>
        <button
          onClick={dismiss}
          className="rounded p-1 text-current/70 hover:text-current focus:outline-none focus:ring-2 focus:ring-current"
          aria-label={t('passkey_prompt.dismiss')}
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
