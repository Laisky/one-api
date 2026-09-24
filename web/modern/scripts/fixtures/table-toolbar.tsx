// Browser-only fixture: render the real pages offline with synthetic records.
// It is not imported by the application and never calls a backend.
import React from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { ChannelsPage } from '@/pages/channels/ChannelsPage';
import { RedemptionsPage } from '@/pages/redemptions/RedemptionsPage';
import { NotificationsProvider } from '@/components/ui/notifications';
import { ThemeProvider } from '@/components/theme-provider';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { changeAppLanguage } from '@/i18n';

/** ToolbarFixtureWindow holds only synthetic browser-test inputs and recorded mock requests. */
interface ToolbarFixtureWindow extends Window {
  toolbarFixture?: { language?: string; page?: string };
  toolbarCalls: { url: string; method?: string; search: string; data: unknown }[];
}
const fixtureWindow = window as unknown as ToolbarFixtureWindow;
fixtureWindow.toolbarCalls = [];
const rows = Array.from({ length: 125 }, (_, index) => ({
  uuid: `018fcf6d-c484-7000-8000-${String(index + 1).padStart(12, '0')}`,
  name: ['OpenAI Production', 'OpenAI Secondary', 'OpenAI Development'][index] ?? `Provider ${index + 1}`,
  type: 1,
  status: 1,
  models: 'gpt-4o,gpt-4.1',
  group: 'default',
  created_time: 1780000000,
  priority: 0,
  weight: 0,
  balance: 100,
  quota: 500000,
}));

// An Axios adapter, not a live endpoint: confirmations exercise real page logic
// while the request payloads remain available for assertions in the driver.
api.defaults.adapter = async (config) => {
  const url = new URL(config.url || '/', 'https://fixture.invalid');
  const payload = typeof config.data === 'string' ? JSON.parse(config.data) : config.data;
  fixtureWindow.toolbarCalls.push({ url: url.pathname, method: config.method, search: url.search, data: payload });
  let data: unknown;
  if (url.pathname === '/api/channel/selection') {
    const selection = payload.selection;
    data = rows.filter((row) => (selection.mode === 'ids' ? selection.ids.includes(row.uuid) : !selection.excluded_ids.includes(row.uuid)));
  } else if (url.pathname === '/api/channel/reset_models') {
    data = {
      results: payload.selection.ids.map((uuid: string) => ({ uuid, name: rows.find((row) => row.uuid === uuid)?.name, success: true })),
    };
  } else if (config.method === 'put' && url.pathname === '/api/channel/' && url.searchParams.get('status_only') === '1') {
    const channel = rows.find((row) => row.uuid === payload.uuid);
    if (!channel || ![1, 2].includes(payload.status)) throw new Error('Invalid fixture status update');
    channel.status = payload.status;
    data = null;
  } else {
    const page = Number(url.searchParams.get('p') || 0);
    const size = Number(url.searchParams.get('size') || 10);
    data = rows.slice(page * size, (page + 1) * size);
  }
  return { config, data: { success: true, data, total: rows.length }, status: 200, statusText: 'OK', headers: {} };
};

/** mountFixture loads the requested locale and renders actual table pages without network or real credentials. */
async function mountFixture(): Promise<void> {
  useAuthStore.setState({
    user: { id: 'fixture-admin', username: 'fixture-admin', role: 100, status: 1, quota: 0, used_quota: 0, group: 'default' },
  });
  await changeAppLanguage(fixtureWindow.toolbarFixture?.language ?? 'en');
  createRoot(document.getElementById('root')!).render(
    <ThemeProvider defaultTheme="light" storageKey="one-api-theme">
      <NotificationsProvider>
        <MemoryRouter>{fixtureWindow.toolbarFixture?.page === 'redemptions' ? <RedemptionsPage /> : <ChannelsPage />}</MemoryRouter>
      </NotificationsProvider>
    </ThemeProvider>
  );
}
void mountFixture();
