import { act, cleanup, render } from '@testing-library/react';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, useNavigate } from 'react-router-dom';

const SEO_TITLE = 'One API SEO title';

let usePageTitle: typeof import('../usePageTitle').usePageTitle;
let getPageTitleKey: typeof import('../usePageTitle').getPageTitleKey;

let navigate: ReturnType<typeof useNavigate>;

/** TitleProbe applies the hook and exposes the router's navigate function to the test. */
function TitleProbe({ systemName }: { systemName?: string }) {
  usePageTitle(systemName);
  navigate = useNavigate();
  return null;
}

/** renderAt mounts the probe at a pathname with an optional configured system name. */
function renderAt(pathname: string, systemName?: string) {
  return render(
    <MemoryRouter initialEntries={[pathname]}>
      <TitleProbe systemName={systemName} />
    </MemoryRouter>
  );
}

describe('usePageTitle', () => {
  beforeAll(async () => {
    // The hook captures the index.html title when its module first loads.
    document.title = SEO_TITLE;
    vi.resetModules();
    ({ usePageTitle, getPageTitleKey } = await import('../usePageTitle'));
  });

  afterEach(() => {
    cleanup();
    localStorage.clear();
    document.title = SEO_TITLE;
  });

  it('maps the first path segment to the header navigation label key', () => {
    expect(getPageTitleKey('/')).toBeNull();
    expect(getPageTitleKey('')).toBeNull();
    expect(getPageTitleKey('/tokens')).toBe('common.tokens');
    expect(getPageTitleKey('/tokens/edit/3')).toBe('common.tokens');
    expect(getPageTitleKey('/chat')).toBe('common.playground');
    expect(getPageTitleKey('/user/reset')).toBe('auth.reset_confirm.title');
    expect(getPageTitleKey('/does-not-exist')).toBe('notFound.message');
  });

  it('front-loads the page label before the configured system name', () => {
    renderAt('/logs', 'Acme Gateway');
    expect(document.title).toBe('Logs | Acme Gateway');
  });

  it('falls back to the cached system name, then the default brand', () => {
    localStorage.setItem('system_name', 'Cached Gateway');
    renderAt('/dashboard', '  ');
    expect(document.title).toBe('Dashboard | Cached Gateway');
    cleanup();

    localStorage.clear();
    renderAt('/dashboard');
    expect(document.title).toBe('Dashboard | One API');
  });

  it('updates the title on navigation and restores the SEO title on the home page', () => {
    renderAt('/tokens/edit/7', 'Acme');
    expect(document.title).toBe('Tokens | Acme');

    act(() => {
      navigate('/settings/system');
    });
    expect(document.title).toBe('Settings | Acme');

    act(() => {
      navigate('/');
    });
    expect(document.title).toBe(SEO_TITLE);

    act(() => {
      navigate('/missing-page');
    });
    expect(document.title).toBe('Page not found | Acme');
  });
});
