import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { KeyRound } from 'lucide-react';
import { browserSupportsWebAuthn } from '@simplewebauthn/browser';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { Button } from '@/components/ui/button';
import { Banner, BannerActions, BannerContent, BannerDescription, BannerIcon } from '@/components/ui/banner';

const STORAGE_KEY = 'passkey_prompt_dismissed';

// Routes where the banner must never fire an authenticated probe, even if
// the store's `isAuthenticated` flag is stale (e.g. zustand persist
// re-hydrated true after iOS ITP cleared the session cookie). Probing here
// triggers a 401 → /login redirect loop.
const AUTH_PUBLIC_PATHS = new Set(['/login', '/register', '/reset', '/user/reset']);

/**
 * One-time dismissible banner shown to logged-in users who have not yet
 * registered a passkey. Once dismissed (localStorage), the banner stays hidden.
 */
export function PasskeyPromptBanner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { isAuthenticated } = useAuthStore();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!isAuthenticated) return;
    if (AUTH_PUBLIC_PATHS.has(location.pathname) || location.pathname.startsWith('/oauth/')) return;
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
  }, [isAuthenticated, location.pathname]);

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
    <Banner variant="info" density="slim" onDismiss={dismiss} dismissLabel={t('passkey_prompt.dismiss')} className="mx-2 mt-2 md:mx-4">
      <BannerIcon>
        <KeyRound />
      </BannerIcon>
      <BannerContent>
        <BannerDescription>{t('passkey_prompt.message')}</BannerDescription>
      </BannerContent>
      <BannerActions>
        <Button size="sm" variant="default" onClick={goToSettings}>
          {t('passkey_prompt.setup_button')}
        </Button>
      </BannerActions>
    </Banner>
  );
}
