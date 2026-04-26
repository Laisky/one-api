import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), delete: vi.fn(), post: vi.fn() },
  default: { get: vi.fn(), put: vi.fn(), delete: vi.fn(), post: vi.fn() },
}));

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

import { api } from '@/lib/api';
import { buildThirdPartyClientUrl, resolveThirdPartyClientContext, TokensPage, type Token } from '../TokensPage.impl';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;

const sampleToken: Token = {
  id: 42,
  name: 'Sample',
  key: 'abcdef1234567890',
  status: 1,
  remain_quota: 1000,
  unlimited_quota: false,
  used_quota: 0,
  created_time: 0,
  accessed_time: 0,
  expired_time: -1,
};

const setLocalStorage = (entries: Record<string, string>) => {
  for (const [k, v] of Object.entries(entries)) {
    window.localStorage.setItem(k, v);
  }
};

describe('resolveThirdPartyClientContext', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('reads chat_link and server_address from status JSON', () => {
    setLocalStorage({
      chat_link: 'https://chat.example.com',
      status: JSON.stringify({ server_address: 'https://api.example.com' }),
    });
    const ctx = resolveThirdPartyClientContext();
    expect(ctx.chatLink).toBe('https://chat.example.com');
    expect(ctx.serverAddress).toBe('https://api.example.com');
    expect(ctx.encodedServerAddress).toBe(encodeURIComponent('https://api.example.com'));
  });

  it('falls back to window.location.origin when no server_address is configured', () => {
    setLocalStorage({ chat_link: 'https://chat.example.com' });
    const ctx = resolveThirdPartyClientContext();
    expect(ctx.serverAddress).toBe(window.location.origin);
  });

  it('returns empty chatLink when not set', () => {
    const ctx = resolveThirdPartyClientContext();
    expect(ctx.chatLink).toBe('');
  });
});

describe('buildThirdPartyClientUrl', () => {
  const ctx = {
    chatLink: 'https://chat.example.com',
    serverAddress: 'https://api.example.com',
    encodedServerAddress: encodeURIComponent('https://api.example.com'),
  };

  it('builds the ChatGPT Next Web url with the key', () => {
    const url = buildThirdPartyClientUrl('next', 'KEY', ctx);
    expect(url).toBe('https://chat.example.com/#/?settings={"key":"sk-KEY","url":"https://api.example.com"}');
  });

  it('builds the AMA url', () => {
    const url = buildThirdPartyClientUrl('ama', 'KEY', ctx);
    expect(url).toBe(`ama://set-api-key?server=${encodeURIComponent('https://api.example.com')}&key=sk-KEY`);
  });

  it('builds the OpenCat url', () => {
    const url = buildThirdPartyClientUrl('opencat', 'KEY', ctx);
    expect(url).toBe(`opencat://team/join?domain=${encodeURIComponent('https://api.example.com')}&token=sk-KEY`);
  });

  it('builds the LobeChat url', () => {
    const url = buildThirdPartyClientUrl('lobechat', 'KEY', ctx);
    expect(url).toBe(
      'https://chat.example.com/?settings={"keyVaults":{"openai":{"apiKey":"sk-KEY","baseURL":"https://api.example.com/v1"}}}'
    );
  });

  it('returns null for next and lobechat when chatLink is empty', () => {
    const noChat = { ...ctx, chatLink: '' };
    expect(buildThirdPartyClientUrl('next', 'KEY', noChat)).toBeNull();
    expect(buildThirdPartyClientUrl('lobechat', 'KEY', noChat)).toBeNull();
  });
});

describe('TokensPage share dropdown integration', () => {
  const originalOpen = window.open;

  beforeEach(() => {
    window.localStorage.clear();
    mockedGet.mockReset();
    mockedGet.mockImplementation((url: string) => {
      if (url.startsWith('/api/token/')) {
        return Promise.resolve({ data: { success: true, data: [sampleToken], total: 1 } });
      }
      return Promise.resolve({ data: { success: true, data: [], total: 0 } });
    });
  });

  afterEach(() => {
    window.open = originalOpen;
  });

  const renderPage = () =>
    render(
      <MemoryRouter>
        <TokensPage />
      </MemoryRouter>
    );

  it('opens ChatGPT Next Web URL when the dropdown item is selected', async () => {
    setLocalStorage({
      chat_link: 'https://chat.example.com',
      status: JSON.stringify({ server_address: 'https://api.example.com' }),
    });
    const openSpy = vi.fn();
    window.open = openSpy as unknown as typeof window.open;

    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Sample')).toBeInTheDocument();
    });

    const trigger = screen.getByRole('button', { name: /Open in client/i });
    await user.click(trigger);

    const item = await screen.findByTestId(`token-share-next-${sampleToken.id}`);
    await user.click(item);

    expect(openSpy).toHaveBeenCalledTimes(1);
    const [calledUrl, target] = openSpy.mock.calls[0];
    expect(target).toBe('_blank');
    expect(calledUrl).toContain('sk-' + sampleToken.key);
    expect(calledUrl).toContain('https://chat.example.com');
    expect(calledUrl).toContain('https://api.example.com');
  });

  it('hides ChatGPT Next Web and LobeChat when chat_link is empty', async () => {
    setLocalStorage({
      status: JSON.stringify({ server_address: 'https://api.example.com' }),
    });
    const openSpy = vi.fn();
    window.open = openSpy as unknown as typeof window.open;

    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Sample')).toBeInTheDocument();
    });

    const trigger = screen.getByRole('button', { name: /Open in client/i });
    await user.click(trigger);

    await screen.findByTestId(`token-share-ama-${sampleToken.id}`);
    expect(screen.queryByTestId(`token-share-next-${sampleToken.id}`)).not.toBeInTheDocument();
    expect(screen.queryByTestId(`token-share-lobechat-${sampleToken.id}`)).not.toBeInTheDocument();
    expect(screen.getByTestId(`token-share-opencat-${sampleToken.id}`)).toBeInTheDocument();
  });
});
