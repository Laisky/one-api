import { PasskeyPromptBanner } from '@/components/auth/PasskeyPromptBanner';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { Outlet } from 'react-router-dom';
import { Footer } from './Footer';
import { Header } from './Header';
import { NoticeBanner } from './NoticeBanner';

export function Layout() {
  const { isMobile } = useResponsive();

  return (
    <div
      className={cn(
        // Grid rows: header (auto) | banner slot (auto) | main (1fr) | footer (auto).
        // All banners must live in the dedicated slot div so the `1fr` track
        // stays on `<main>` — otherwise an extra grid child would land on the
        // `1fr` row and stretch the banner to fill the viewport.
        'grid grid-rows-[auto_auto_1fr_auto] bg-background',
        // Use dynamic viewport height to avoid iOS/Android 100vh bugs causing extra blank space
        'min-h-screen-dvh',
        // Full width root
        'w-full'
      )}
    >
      <Header />
      <div>
        <PasskeyPromptBanner />
        <NoticeBanner />
      </div>

      <main
        className={cn(
          // Row 2 of grid grows to fill available space
          'w-full min-h-0',
          // Responsive padding and spacing
          isMobile ? 'px-2 py-4' : 'px-4 py-6',
          // Ensure proper spacing from header
          'mt-0'
        )}
      >
        <div className="w-full max-w-full">
          <Outlet />
        </div>
      </main>

      <Footer />
    </div>
  );
}
