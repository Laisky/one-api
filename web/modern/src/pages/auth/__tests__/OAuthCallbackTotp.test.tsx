import { api, isSafeInternalPath } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ComponentType } from 'react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { vi } from 'vitest';
import { GitHubOAuthPage } from '../GitHubOAuthPage';
import { LarkOAuthPage } from '../LarkOAuthPage';
import { OidcOAuthPage } from '../OidcOAuthPage';
import { WeChatOAuthPage } from '../WeChatOAuthPage';

vi.mock('@/lib/stores/auth');
vi.mock('@/lib/api');

const mockLogin = vi.fn();
const mockUseAuthStore = vi.mocked(useAuthStore);
const mockApiGet = vi.mocked(api.get);
const mockApiPost = vi.mocked(api.post);
const mockIsSafeInternalPath = vi.mocked(isSafeInternalPath);

const TOTP_REQUIRED_BODY = { success: false, message: 'totp_required', data: { totp_required: true } };
const USER = { id: 7, username: 'two-factor-user', role: 1 };
// The OAuth state carries the post-login target exactly like the production state strings do.
const STATE_WITH_REDIRECT = 'xyz redirect_to=%2Ftokens';

interface CallbackCase {
  name: string;
  Component: ComponentType;
  path: string;
  method: 'get' | 'post';
  callbackPrefix: string;
  successMessage: string;
}

const cases: CallbackCase[] = [
  {
    name: 'GitHubOAuthPage',
    Component: GitHubOAuthPage,
    path: '/oauth/github',
    method: 'get',
    callbackPrefix: '/api/oauth/github?',
    successMessage: 'GitHub login successful!',
  },
  {
    name: 'OidcOAuthPage',
    Component: OidcOAuthPage,
    path: '/oauth/oidc',
    method: 'get',
    callbackPrefix: '/api/oauth/oidc?',
    successMessage: 'OIDC login successful!',
  },
  {
    name: 'LarkOAuthPage',
    Component: LarkOAuthPage,
    path: '/oauth/lark',
    method: 'get',
    callbackPrefix: '/api/oauth/lark?',
    successMessage: 'Lark login successful!',
  },
  {
    name: 'WeChatOAuthPage',
    Component: WeChatOAuthPage,
    path: '/oauth/wechat',
    method: 'post',
    callbackPrefix: '/api/oauth/wechat?',
    successMessage: 'WeChat login successful!',
  },
];

/** RouteProbe renders the route name together with the navigation-state message so tests can assert both. */
function RouteProbe({ name }: { name: string }) {
  const location = useLocation();
  const message = (location.state as { message?: string } | null)?.message ?? '';
  return <div>{`${name}:${message}`}</div>;
}

/** httpError builds an axios-shaped rejection carrying an HTTP status and JSON body. */
const httpError = (status: number, data: unknown) =>
  Object.assign(new Error(`Request failed with status code ${status}`), { response: { status, data } });

/**
 * installApi routes the mocked api client: the provider callback answers `callbackBody`, and each
 * POST /api/oauth/totp consumes the next entry of `totpReplies` (a body to resolve or an Error to reject).
 */
const installApi = (tc: CallbackCase, callbackBody: unknown, totpReplies: Array<unknown> = []) => {
  const replies = [...totpReplies];

  /** route answers one mocked request for `method`, failing loudly on anything the flow should not call. */
  const route = async (method: 'get' | 'post', url: string) => {
    if (method === 'post' && url === '/api/oauth/totp') {
      const reply = replies.shift();
      if (reply instanceof Error) throw reply;
      if (reply === undefined) throw new Error('no TOTP reply queued');
      return { status: 200, data: reply } as any;
    }
    if (method === tc.method && url.startsWith(tc.callbackPrefix)) return { status: 200, data: callbackBody } as any;
    throw new Error(`unexpected ${method.toUpperCase()} ${url}`);
  };

  mockApiGet.mockImplementation((url: string) => route('get', url));
  mockApiPost.mockImplementation((url: string) => route('post', url));
};

/** callbackCallCount returns how many times the provider callback endpoint was requested. */
const callbackCallCount = (tc: CallbackCase) =>
  (tc.method === 'get' ? mockApiGet : mockApiPost).mock.calls.filter(([url]) => String(url).startsWith(tc.callbackPrefix)).length;

/** totpCalls returns the POST /api/oauth/totp invocations as [url, body] pairs. */
const totpCalls = () => mockApiPost.mock.calls.filter(([url]) => url === '/api/oauth/totp');

/** renderPage mounts the callback page under a router that exposes the post-login destinations. */
const renderPage = (tc: CallbackCase, state = 'xyz') =>
  render(
    <MemoryRouter initialEntries={[`${tc.path}?code=abc&state=${encodeURIComponent(state)}`]}>
      <Routes>
        <Route path={tc.path} element={<tc.Component />} />
        <Route path="/" element={<RouteProbe name="home" />} />
        <Route path="/tokens" element={<RouteProbe name="tokens" />} />
        <Route path="/settings" element={<RouteProbe name="settings" />} />
        <Route path="/login" element={<RouteProbe name="login" />} />
      </Routes>
    </MemoryRouter>
  );

/** enterCode types a TOTP code into the prompt and submits it with the verify button. */
const enterCode = async (code: string) => {
  const input = await screen.findByLabelText('TOTP Code');
  fireEvent.change(input, { target: { value: code } });
  fireEvent.click(screen.getByRole('button', { name: 'Verify TOTP' }));
};

describe.each(cases)('$name two-factor OAuth completion', (tc) => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockReset();
    mockApiPost.mockReset();
    mockIsSafeInternalPath.mockImplementation((raw: string) => typeof raw === 'string' && raw.startsWith('/') && !raw.startsWith('//'));
    mockUseAuthStore.mockReturnValue({ login: mockLogin } as any);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('logs in directly without a TOTP prompt when the account has no two-factor (positive control)', async () => {
    installApi(tc, { success: true, message: '', data: USER });

    renderPage(tc, STATE_WITH_REDIRECT);

    expect(await screen.findByText(`tokens:${tc.successMessage}`)).toBeInTheDocument();
    expect(mockLogin).toHaveBeenCalledWith(USER, '');
    expect(callbackCallCount(tc)).toBe(1);
    expect(totpCalls()).toHaveLength(0);
  });

  it('shows the TOTP prompt and never retries the consumed callback', async () => {
    vi.useFakeTimers();
    installApi(tc, TOTP_REQUIRED_BODY);

    renderPage(tc);

    await vi.waitFor(() => {
      expect(screen.getByTestId('oauth-totp-prompt')).toBeInTheDocument();
    });
    expect(screen.getByText('Two-Factor Authentication')).toBeInTheDocument();

    // Run well past every retry backoff (2s + 4s + 6s) and the 2s failure redirect.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(20000);
    });

    expect(callbackCallCount(tc)).toBe(1);
    expect(screen.getByTestId('oauth-totp-prompt')).toBeInTheDocument();
    expect(screen.queryByText(/^login:/)).not.toBeInTheDocument();
    expect(mockLogin).not.toHaveBeenCalled();
  });

  it('treats a data.totp_required flag without the message as a TOTP challenge', async () => {
    installApi(tc, { success: false, message: '', data: { totp_required: true } });

    renderPage(tc);

    expect(await screen.findByTestId('oauth-totp-prompt')).toBeInTheDocument();
    expect(callbackCallCount(tc)).toBe(1);
  });

  it('posts the code to /api/oauth/totp and completes the login with the safe redirect', async () => {
    installApi(tc, TOTP_REQUIRED_BODY, [{ success: true, message: '', data: USER }]);

    renderPage(tc, STATE_WITH_REDIRECT);
    await enterCode('123456');

    expect(await screen.findByText(`tokens:${tc.successMessage}`)).toBeInTheDocument();
    expect(totpCalls()).toEqual([['/api/oauth/totp', { totp_code: '123456' }]]);
    expect(mockLogin).toHaveBeenCalledTimes(1);
    expect(mockLogin).toHaveBeenCalledWith(USER, '');
    expect(callbackCallCount(tc)).toBe(1);
  });

  it('keeps the prompt with the error on a wrong code and accepts a re-entered code', async () => {
    installApi(tc, TOTP_REQUIRED_BODY, [
      { success: false, message: 'Invalid TOTP code' },
      { success: true, message: '', data: USER },
    ]);

    renderPage(tc);
    await enterCode('000000');

    expect(await screen.findByText('Invalid TOTP code')).toBeInTheDocument();
    expect(screen.getByTestId('oauth-totp-prompt')).toBeInTheDocument();
    expect(screen.getByLabelText('TOTP Code')).toHaveValue('');
    expect(mockLogin).not.toHaveBeenCalled();

    await enterCode('654321');

    expect(await screen.findByText(`home:${tc.successMessage}`)).toBeInTheDocument();
    expect(totpCalls().map(([, body]) => body)).toEqual([{ totp_code: '000000' }, { totp_code: '654321' }]);
    expect(mockLogin).toHaveBeenCalledWith(USER, '');
    expect(callbackCallCount(tc)).toBe(1);
  });

  it('shows the throttling message on HTTP 429 and lets the user retry', async () => {
    installApi(tc, TOTP_REQUIRED_BODY, [
      httpError(429, { success: false, message: 'Too many TOTP verification attempts. Please wait before trying again.' }),
    ]);

    renderPage(tc);
    await enterCode('123456');

    expect(await screen.findByText('Too many verification attempts. Please wait a moment before trying again.')).toBeInTheDocument();
    expect(screen.getByLabelText('TOTP Code')).not.toBeDisabled();
    expect(mockLogin).not.toHaveBeenCalled();
  });

  it('reports an expired challenge, disables verification, and offers back to login', async () => {
    installApi(tc, TOTP_REQUIRED_BODY, [
      {
        success: false,
        message: 'Two-factor sign-in has expired or was not started. Please sign in again.',
        data: { totp_expired: true },
      },
    ]);

    renderPage(tc);
    await enterCode('123456');

    expect(await screen.findByText('Two-factor sign-in has expired or was not started. Please sign in again.')).toBeInTheDocument();
    expect(screen.getByLabelText('TOTP Code')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Verify TOTP' })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'Back to Login' }));

    await waitFor(() => {
      expect(screen.getByText(/^login:/)).toBeInTheDocument();
    });
    expect(mockLogin).not.toHaveBeenCalled();
    expect(callbackCallCount(tc)).toBe(1);
  });
});
