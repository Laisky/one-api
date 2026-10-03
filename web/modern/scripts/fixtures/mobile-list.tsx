// Browser-only fixture: real list pages, synthetic records, no live network.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { ChannelsPage } from '@/pages/channels/ChannelsPage';
import { UsersPage } from '@/pages/users/UsersPage';
import { RedemptionsPage } from '@/pages/redemptions/RedemptionsPage';
import { NotificationsProvider } from '@/components/ui/notifications';
import { ThemeProvider } from '@/components/theme-provider';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { changeAppLanguage } from '@/i18n';

/** MobileFixtureWindow holds test-only inputs and records attempted synthetic API requests. */
interface MobileFixtureWindow extends Window {
  toolbarFixture?: { language?: string; page?: string };
  toolbarCalls: { url: string; method?: string; search: string; data: unknown }[];
}
const fixtureWindow = window as unknown as MobileFixtureWindow;
fixtureWindow.toolbarCalls = [];
const rows = Array.from({ length: 25 }, (_, index) => ({
  uuid: `018fcf6d-c484-7000-8000-${String(index + 1).padStart(12, '0')}`,
  name: ['OpenAI Production', 'A-very-long-provider-name-without-whitespace-that-must-not-overflow-a-narrow-phone'][index] ?? `Provider ${index + 1}`,
  username: ['github_synthetic_user_01', 'github_verylongusernamewithoutwhitespace0123456789abcdef'][index] ?? `user_${index + 1}`,
  display_name: 'Synthetic GitHub User',
  role: 1,
  type: 1,
  status: 1,
  models: 'gpt-4o,gpt-4.1',
  group: 'default',
  created_time: 1780000000,
  created_at: 1780000000,
  updated_at: 1780000000,
  redeemed_time: 0,
  priority: 0,
  weight: 0,
  balance: 100,
  quota: 500000,
  used_quota: 125000,
  key: 'synthetic-redemption-key-not-a-real-credential',
}));

api.defaults.adapter = async (config) => {
  const url = new URL(config.url || '/', 'https://fixture.invalid');
  const payload = typeof config.data === 'string' ? JSON.parse(config.data) : config.data;
  fixtureWindow.toolbarCalls.push({ url: url.pathname, method: config.method, search: url.search, data: payload });
  if (config.method && config.method !== 'get') throw new Error('This layout fixture must not mutate records');
  const page = Number(url.searchParams.get('p') || 0);
  const size = Number(url.searchParams.get('size') || 10);
  return { config, data: { success: true, data: rows.slice(page * size, (page + 1) * size), total: rows.length }, status: 200, statusText: 'OK', headers: {} };
};

/** mountFixture mounts original pages after setting a synthetic administrator and requested locale. */
async function mountFixture(): Promise<void> {
  useAuthStore.setState({
    user: { id: 'fixture-admin', username: 'fixture-admin', role: 100, status: 1, quota: 0, used_quota: 0, group: 'default' },
  });
  await changeAppLanguage(fixtureWindow.toolbarFixture?.language ?? 'en');
  const kind = fixtureWindow.toolbarFixture?.page;
  const page = kind === 'users' ? <UsersPage /> : kind === 'redemptions' ? <RedemptionsPage /> : <ChannelsPage />;
  createRoot(document.getElementById('root')!).render(
    <ThemeProvider defaultTheme="light" storageKey="one-api-theme">
      <NotificationsProvider><MemoryRouter>{page}</MemoryRouter></NotificationsProvider>
    </ThemeProvider>
  );
}
void mountFixture();
