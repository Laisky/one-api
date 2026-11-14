import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
  default: { get: vi.fn() },
}));
vi.mock('@/lib/stores/auth', () => ({ useAuthStore: vi.fn() }));
vi.mock('@simplewebauthn/browser', () => ({ browserSupportsWebAuthn: vi.fn(() => true) }));

import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { PasskeyPromptBanner } from '../PasskeyPromptBanner';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;
const mockedUseAuthStore = useAuthStore as unknown as ReturnType<typeof vi.fn>;
const BANNER_TEXT = /set up a passkey/i;

let navigateTo: (path: string) => void = () => {};

/** NavigationProbe exposes the router's navigate function to the test body. */
function NavigationProbe() {
  const navigate = useNavigate();
  navigateTo = navigate;
  return null;
}

/** renderBanner mounts the banner inside a router at the given path, like Layout does. */
const renderBanner = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <NavigationProbe />
      <PasskeyPromptBanner />
    </MemoryRouter>
  );

/** signIn makes the mocked auth store report the given user as signed in (or signed out). */
const signIn = (userId: string | null) => {
  mockedUseAuthStore.mockReturnValue(
    userId ? { isAuthenticated: true, user: { id: userId, username: userId } } : { isAuthenticated: false, user: null }
  );
};

let userCounter = 0;
/** uniqueUser returns a user id no earlier test used, so the module-level session cache starts cold. */
const uniqueUser = () => {
  userCounter += 1;
  return `user-${userCounter}`;
};

describe('PasskeyPromptBanner', () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockedGet.mockReset();
    mockedGet.mockResolvedValue({ data: { success: true, data: [] } });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('asks for the passkey list once per session across route changes and remounts', async () => {
    signIn(uniqueUser());
    const first = renderBanner('/dashboard');
    expect(await screen.findByText(BANNER_TEXT)).toBeInTheDocument();

    for (const path of ['/tokens', '/logs', '/channels', '/settings']) {
      act(() => navigateTo(path));
    }
    // Layout remounts when navigation crosses public/protected route trees.
    first.unmount();
    renderBanner('/models');
    expect(await screen.findByText(BANNER_TEXT)).toBeInTheDocument();
    renderBanner('/topup').unmount();

    expect(mockedGet).toHaveBeenCalledTimes(1);
    expect(mockedGet).toHaveBeenCalledWith('/api/user/passkey');
  });

  it('never probes on sign-in routes and probes once after leaving them', async () => {
    signIn(uniqueUser());
    renderBanner('/login');
    act(() => navigateTo('/oauth/github'));
    act(() => navigateTo('/register'));
    expect(mockedGet).not.toHaveBeenCalled();

    act(() => navigateTo('/dashboard'));
    expect(await screen.findByText(BANNER_TEXT)).toBeInTheDocument();
    expect(mockedGet).toHaveBeenCalledTimes(1);
  });

  it('stays hidden when the account already has a passkey, without re-asking', async () => {
    mockedGet.mockResolvedValue({ data: { success: true, data: [{ id: 1 }] } });
    signIn(uniqueUser());
    const view = renderBanner('/dashboard');
    await waitFor(() => expect(mockedGet).toHaveBeenCalledTimes(1));

    view.unmount();
    renderBanner('/tokens');
    await act(async () => {});
    expect(screen.queryByText(BANNER_TEXT)).not.toBeInTheDocument();
    expect(mockedGet).toHaveBeenCalledTimes(1);
  });

  it('probes again after the session ends or another user signs in', async () => {
    const user = uniqueUser();
    signIn(user);
    const view = renderBanner('/dashboard');
    await screen.findByText(BANNER_TEXT);
    expect(mockedGet).toHaveBeenCalledTimes(1);

    signIn(null);
    view.rerender(
      <MemoryRouter initialEntries={['/dashboard']}>
        <PasskeyPromptBanner />
      </MemoryRouter>
    );
    expect(screen.queryByText(BANNER_TEXT)).not.toBeInTheDocument();
    view.unmount();

    signIn(user);
    const again = renderBanner('/dashboard');
    await screen.findByText(BANNER_TEXT);
    expect(mockedGet).toHaveBeenCalledTimes(2);
    again.unmount();

    signIn(uniqueUser());
    renderBanner('/dashboard');
    await screen.findByText(BANNER_TEXT);
    expect(mockedGet).toHaveBeenCalledTimes(3);
  });

  it('retries on a later mount when the probe failed', async () => {
    vi.spyOn(console, 'debug').mockImplementation(() => {});
    mockedGet.mockRejectedValueOnce(new Error('offline'));
    signIn(uniqueUser());
    const view = renderBanner('/dashboard');
    await waitFor(() => expect(console.debug).toHaveBeenCalledWith('[passkey-prompt] passkey probe failed; banner stays hidden'));
    expect(screen.queryByText(BANNER_TEXT)).not.toBeInTheDocument();
    view.unmount();

    renderBanner('/dashboard');
    expect(await screen.findByText(BANNER_TEXT)).toBeInTheDocument();
    expect(mockedGet).toHaveBeenCalledTimes(2);
  });

  it('does not probe once dismissed and remembers the dismissal', async () => {
    signIn(uniqueUser());
    const view = renderBanner('/dashboard');
    await screen.findByText(BANNER_TEXT);

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(screen.queryByText(BANNER_TEXT)).not.toBeInTheDocument();
    expect(window.localStorage.getItem('passkey_prompt_dismissed')).toBe('1');
    view.unmount();

    signIn(uniqueUser());
    renderBanner('/dashboard');
    await act(async () => {});
    expect(screen.queryByText(BANNER_TEXT)).not.toBeInTheDocument();
    expect(mockedGet).toHaveBeenCalledTimes(1);
  });
});
