import { PasskeyPromptBanner } from '@/components/auth/PasskeyPromptBanner';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'react-router-dom';
import { Footer } from './Footer';
import { Header } from './Header';
import { NoticeBanner } from './NoticeBanner';
import { PageLoader } from './PageLoader';

/** MAIN_CONTENT_ID is the skip link target; it must stay unique on the page. */
export const MAIN_CONTENT_ID = 'main-content';

/**
 * Layout renders the shared application chrome around the routed page: a skip link, the header,
 * the banner slot, the main landmark, and the footer. The routed page is wrapped in its own Suspense
 * boundary so the chrome (and its status/notice requests) renders while the page chunk downloads.
 *
 * @returns The page layout element.
 */
export function Layout() {
  const { isMobile } = useResponsive();
  const { t } = useTranslation();

  return (
    <div
      className={cn(
        // Grid rows: header (auto) | banner slot (auto) | main (1fr) | footer (auto).
        // All banners must live in the dedicated slot div so the `1fr` track
        // stays on `<main>` — otherwise an extra grid child would land on the
        // `1fr` row and stretch the banner to fill the viewport. The skip link
        // is absolutely positioned (.skip-link), so it takes no grid track.
        //
        // `grid-cols-[minmax(0,1fr)]` is load-bearing for mobile: the default
        // `1fr` column resolves to `minmax(auto, 1fr)`, which uses the widest
        // child's min-content as the column floor. Without this, an unshrinkable
        // header action group leaks horizontal overflow into every page.
        'grid grid-cols-[minmax(0,1fr)] grid-rows-[auto_auto_1fr_auto] bg-background',
        // Use dynamic viewport height to avoid iOS/Android 100vh bugs causing extra blank space
        'min-h-screen-dvh',
        // Full width root
        'w-full'
      )}
    >
      {/* First focusable element: lets keyboard users bypass the repeated header navigation. */}
      <a href={`#${MAIN_CONTENT_ID}`} className="skip-link">
        {t('common.skip_to_content')}
      </a>
      <Header />
      <div>
        <PasskeyPromptBanner />
        <NoticeBanner />
      </div>

      <main
        id={MAIN_CONTENT_ID}
        // Programmatically focusable (not a tab stop) so the skip link moves focus, not only scroll.
        tabIndex={-1}
        className={cn(
          // Row 2 of grid grows to fill available space
          'w-full min-h-0',
          // Responsive padding and spacing
          isMobile ? 'px-2 py-4' : 'px-4 py-6',
          // Ensure proper spacing from header
          'mt-0',
          // Keep the skip-link target clear of the sticky h-16 header; main is not a tab stop,
          // so its programmatic focus needs no ring.
          'scroll-mt-16 focus:outline-none'
        )}
      >
        <div className="w-full max-w-full">
          <Suspense fallback={<PageLoader />}>
            <Outlet />
          </Suspense>
        </div>
      </main>

      <Footer />
    </div>
  );
}
