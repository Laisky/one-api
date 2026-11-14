import { cleanup, render, screen } from '@testing-library/react';
import { lazy } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';

// The chrome components are replaced with landmarks so this suite covers only Layout's own structure.
vi.mock('../Header', () => ({
  Header: () => (
    <header>
      <nav aria-label="Primary">
        <a href="/dashboard">Dashboard</a>
      </nav>
    </header>
  ),
}));
vi.mock('../Footer', () => ({ Footer: () => <footer>Footer</footer> }));
vi.mock('../NoticeBanner', () => ({ NoticeBanner: () => null }));
vi.mock('@/components/auth/PasskeyPromptBanner', () => ({ PasskeyPromptBanner: () => null }));
vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false, isDesktop: true, isLarge: false }),
}));

import { Layout, MAIN_CONTENT_ID } from '../Layout';

/** NeverLoadingPage simulates a lazily loaded route chunk that has not arrived yet. */
const NeverLoadingPage = lazy(() => new Promise<{ default: () => null }>(() => {}));

/** renderLayout mounts Layout at a route whose page is the given element. */
function renderLayout(page: React.ReactNode) {
  return render(
    <MemoryRouter initialEntries={['/page']}>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route path="page" element={page} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

describe('Layout accessibility', () => {
  afterEach(() => {
    cleanup();
  });

  it('renders a skip link as the first focusable element, before the header navigation', () => {
    const { container } = renderLayout(<h1>Page body</h1>);

    const focusables = container.querySelectorAll<HTMLElement>('a[href], button, [tabindex]:not([tabindex="-1"])');
    const skipLink = screen.getByRole('link', { name: 'Skip to main content' });
    expect(focusables[0]).toBe(skipLink);
    expect(skipLink).toHaveAttribute('href', `#${MAIN_CONTENT_ID}`);
    expect(skipLink).toHaveClass('skip-link');
  });

  it('targets a programmatically focusable main landmark that is not a tab stop', () => {
    renderLayout(<h1>Page body</h1>);

    const main = screen.getByRole('main');
    expect(main).toHaveAttribute('id', MAIN_CONTENT_ID);
    expect(main).toHaveAttribute('tabindex', '-1');
    expect(main).toContainElement(screen.getByRole('heading', { name: 'Page body' }));
    expect(document.querySelectorAll(`#${MAIN_CONTENT_ID}`)).toHaveLength(1);

    main.focus();
    expect(document.activeElement).toBe(main);
  });

  it('keeps the header and footer rendered while the routed page chunk is still loading', () => {
    const { container } = renderLayout(<NeverLoadingPage />);

    expect(screen.getByRole('banner')).toBeInTheDocument();
    expect(screen.getByRole('contentinfo')).toBeInTheDocument();
    expect(screen.getByRole('main').querySelector('.animate-spin')).not.toBeNull();
    expect(container.querySelector('.skip-link')).not.toBeNull();
  });
});
