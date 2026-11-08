import type { ComponentType } from 'react';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { Footer } from '@/components/layout/Footer';
import i18n, { changeAppLanguage } from '@/i18n';
import { LoginPage } from '@/pages/auth/LoginPage.impl';
import { PasswordResetPage } from '@/pages/auth/PasswordResetPage';
import { RegisterPage } from '@/pages/auth/RegisterPage';
import { DashboardPage } from '@/pages/dashboard/DashboardPage';
import { DomMutationBoundary, translateText } from './dom-translation';

vi.unmock('react-i18next');

vi.mock('@/hooks/useSystemStatus', () => ({
  /** useSystemStatus supplies enabled password forms without remote integrations or custom footer markup. */
  useSystemStatus: () => ({ systemStatus: { password_login: true, password_register: true, turnstile_check: false } }),
}));
vi.mock('@/hooks/useResponsive', () => ({
  /** useResponsive selects desktop layout so the existing footer version is visible. */
  useResponsive: () => ({ isMobile: false, isTablet: false }),
  /** useIsTouchDevice keeps desktop control behavior in the component fixtures. */
  useIsTouchDevice: () => false,
}));
vi.mock('@/lib/stores/auth', () => ({
  /** useAuthStore supplies a signed-in dashboard viewer and an unused login action. */
  useAuthStore: () => ({ user: { role: 1 }, login: vi.fn() }),
}));
vi.mock('@/pages/dashboard/hooks/useDashboardData', () => ({
  /** useDashboardData supplies an updated but empty dashboard without making requests. */
  useDashboardData: () => ({
    rows: [],
    userRows: [],
    tokenRows: [],
    toolRows: [],
    toolUserRows: [],
    toolTokenRows: [],
    lastUpdated: 1_799_999_999,
    loading: false,
    dateError: '',
  }),
}));

// Chart and filter implementations are unrelated to the native timestamp prefix under test.
vi.mock('@/pages/dashboard/components/DashboardFilter', () => ({ DashboardFilter: () => null }));
vi.mock('@/pages/dashboard/components/OverviewCards', () => ({ OverviewCards: () => null }));
vi.mock('@/pages/dashboard/components/TopModels', () => ({ TopModels: () => null }));
vi.mock('@/pages/dashboard/components/Insights', () => ({ Insights: () => null }));
vi.mock('@/pages/dashboard/components/TimeSeriesCharts', () => ({ TimeSeriesCharts: () => null }));
vi.mock('@/pages/dashboard/components/UsageCharts', () => ({ UsageCharts: () => null }));
vi.mock('@/pages/dashboard/components/ToolUsageCharts', () => ({ ToolUsageCharts: () => null }));
vi.mock('@/pages/dashboard/components/CacheHeatmaps', () => ({ CacheHeatmaps: () => null }));

/** LocaleCase identifies a live component and the native translated text region checked after a locale change. */
interface LocaleCase {
  name: string;
  Component: ComponentType;
  textKey: string;
  linkKey?: string;
  href?: string;
}

const cases: LocaleCase[] = [
  {
    name: 'login registration prompt',
    Component: LoginPage,
    textKey: 'auth.login.no_account',
    linkKey: 'auth.login.sign_up',
    href: '/register',
  },
  {
    name: 'password-reset login prompt',
    Component: PasswordResetPage,
    textKey: 'auth.reset.remember_password',
    linkKey: 'common.login',
    href: '/login',
  },
  {
    name: 'registration login prompt',
    Component: RegisterPage,
    textKey: 'auth.register.already_have_account',
    linkKey: 'auth.register.sign_in',
    href: '/login',
  },
  { name: 'dashboard timestamp prefix', Component: DashboardPage, textKey: 'dashboard.overview.updated' },
];

beforeEach(async () => {
  await act(async () => {
    await changeAppLanguage('en');
  });
});

describe.each(cases)('$name after browser translation', ({ Component, textKey, linkKey, href }) => {
  it('refreshes visible text for the selected app locale while preserving its neighboring content', async () => {
    const onCaughtError = vi.fn();
    const { container, unmount } = render(
      <MemoryRouter>
        <DomMutationBoundary>
          <Component />
        </DomMutationBoundary>
      </MemoryRouter>,
      { onCaughtError }
    );
    const region = screen.getByText(i18n.t(textKey));
    const neighbor = linkKey ? screen.getByRole('link', { name: i18n.t(linkKey) }) : container.querySelector('.font-mono');
    expect(neighbor).not.toBeNull();
    expect(translateText(region)).toBeGreaterThan(0);

    await act(async () => {
      await changeAppLanguage('fr');
    });

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.queryByText(i18n.t(textKey))).toBeInTheDocument();
    if (linkKey) {
      const link = screen.getByRole('link', { name: i18n.t(linkKey) });
      expect(link).toBe(neighbor);
      expect(link).toHaveAttribute('href', href);
    } else {
      expect(container.querySelector('.font-mono')).toBe(neighbor);
    }
    unmount();
    expect(onCaughtError.mock.calls.length).toBe(0);
  });
});

it('refreshes the native footer version label after browser translation and an app locale change', async () => {
  const onCaughtError = vi.fn();
  const { unmount } = render(
    <DomMutationBoundary>
      <Footer />
    </DomMutationBoundary>,
    { onCaughtError }
  );
  const version = screen.getByText(/^Version:/);
  const value = version.textContent?.slice(`${i18n.t('common.version')}: `.length);
  expect(value).toBeTruthy();
  expect(translateText(version)).toBeGreaterThan(0);

  await act(async () => {
    await changeAppLanguage('es');
  });

  expect(onCaughtError.mock.calls.length).toBe(0);
  expect(screen.queryByText(`${i18n.t('common.version')}: ${value}`)).toBeInTheDocument();
  unmount();
  expect(onCaughtError.mock.calls.length).toBe(0);
});
