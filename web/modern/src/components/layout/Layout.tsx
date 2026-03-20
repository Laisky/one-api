import { Outlet } from 'react-router-dom';
import { Header } from './Header';
import { Footer } from './Footer';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';

export function Layout() {
  const { isMobile } = useResponsive();

  return (
    <div
      className={cn(
        // Grid layout prevents any accidental extra space after footer
        'grid grid-rows-[auto_1fr_auto] bg-background',
        // Use dynamic viewport height to avoid iOS/Android 100vh bugs causing extra blank space
        'min-h-screen-dvh',
        // Full width root
        'w-full'
      )}
    >
      <Header />

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
