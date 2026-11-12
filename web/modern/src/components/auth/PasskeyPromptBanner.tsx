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
 * passkeyProbe caches, for the signed-in account identified by sessionKey, whether that account still has no
 * passkey. It lives in module scope because Layout (and with it this banner) remounts whenever navigation
 * crosses between public and protected routes; the server is asked at most once per session instead of on
 * every navigation.
 */
let passkeyProbe: { sessionKey: string; needsPasskey: Promise<boolean> } | null = null;

/**
 * isAuthPublicPath reports whether a pathname belongs to the sign-in/OAuth flows where probing is unsafe.
 *
 * @param pathname - the current router pathname.
 * @returns true for auth and OAuth callback routes.
 */
const isAuthPublicPath = (pathname: string): boolean => AUTH_PUBLIC_PATHS.has(pathname) || pathname.startsWith('/oauth/');

/**
 * isPromptDismissed reports whether the user already dismissed the prompt on this device.
 *
 * @returns true when the dismissal flag is stored, false when it is absent or storage is unavailable.
 */
const isPromptDismissed = (): boolean => {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1';
  } catch {
    return false;
  }
};

/**
 * probeNeedsPasskey asks the server whether the signed-in account has no passkey yet, reusing the answer for
 * the same session. Failed requests are not cached, so a later mount may retry.
 *
 * @param sessionKey - identifies the signed-in account; a different key starts a new probe.
 * @returns a promise resolving to true when the account has no passkey registered.
 */
const probeNeedsPasskey = (sessionKey: string): Promise<boolean> => {
  if (passkeyProbe?.sessionKey === sessionKey) {
    return passkeyProbe.needsPasskey;
  }

  const needsPasskey = api.get('/api/user/passkey').then((res) => {
    if (!res.data?.success) {
      return false;
    }
    const passkeys: unknown[] = res.data.data ?? [];
    return passkeys.length === 0;
  });
  const entry = { sessionKey, needsPasskey };
  passkeyProbe = entry;
  needsPasskey.catch(() => {
    if (passkeyProbe === entry) {
      passkeyProbe = null;
    }
  });
  return needsPasskey;
};

/**
 * PasskeyPromptBanner is a one-time dismissible banner shown to logged-in users who have not yet registered a
 * passkey. Once dismissed (localStorage), the banner stays hidden. The passkey list is fetched once per
 * signed-in session, only while the banner is eligible (authenticated, outside the auth flows, WebAuthn
 * supported, not dismissed), and never again on ordinary route changes.
 *
 * @returns the banner, or null when it should not be shown.
 */
export function PasskeyPromptBanner() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { isAuthenticated, user } = useAuthStore();
  const [visible, setVisible] = useState(false);

  const sessionKey = String(user?.uuid ?? user?.id ?? user?.username ?? '');
  const onAuthPublicPath = isAuthPublicPath(location.pathname);
  const eligible = isAuthenticated && !onAuthPublicPath && browserSupportsWebAuthn() && !isPromptDismissed();

  useEffect(() => {
    if (!isAuthenticated) {
      // The session ended: the next sign-in (possibly as someone else) must be probed again.
      passkeyProbe = null;
    }
  }, [isAuthenticated]);

  useEffect(() => {
    if (!eligible) return;

    let cancelled = false;
    probeNeedsPasskey(sessionKey).then(
      (needsPasskey) => {
        if (!cancelled) {
          setVisible(needsPasskey);
        }
      },
      () => {
        // The banner is a nice-to-have, so a failed probe only keeps it hidden.
        console.debug('[passkey-prompt] passkey probe failed; banner stays hidden');
      }
    );

    return () => {
      cancelled = true;
    };
  }, [eligible, sessionKey]);

  const dismiss = () => {
    localStorage.setItem(STORAGE_KEY, '1');
    setVisible(false);
  };

  const goToSettings = () => {
    dismiss();
    navigate('/settings');
  };

  if (!visible || !eligible) return null;

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
