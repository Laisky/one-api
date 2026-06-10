import { api } from '@/lib/api';

/**
 * Request an OAuth state token from the backend to prevent CSRF.
 * The value is stored in the server session and must be sent to GitHub.
 */
export async function getOAuthState(): Promise<string> {
  const res = await api.get('/api/oauth/state'); // Unified API call - complete URL with /api prefix
  const { success, data, message } = res.data || {};
  if (success && typeof data === 'string' && data.length > 0) return data;
  throw new Error(message || '');
}

/**
 * Build the GitHub OAuth URL.
 * If redirectUri is provided, it must match the one configured in GitHub settings.
 */
export function buildGitHubOAuthUrl(clientId: string, state: string, redirectUri?: string): string {
  const base = 'https://github.com/login/oauth/authorize';
  const params = new URLSearchParams();
  params.set('client_id', clientId);
  params.set('state', state);
  params.set('scope', 'user:email');
  if (redirectUri) params.set('redirect_uri', redirectUri);
  return `${base}?${params.toString()}`;
}

/**
 * Build the OIDC authorization URL using the configured authorization endpoint.
 * The redirect_uri must match the one registered with the OIDC provider.
 */
export function buildOidcOAuthUrl(authorizationEndpoint: string, clientId: string, state: string, redirectUri: string): string {
  const params = new URLSearchParams();
  params.set('client_id', clientId);
  params.set('redirect_uri', redirectUri);
  params.set('response_type', 'code');
  params.set('scope', 'openid profile email');
  params.set('state', state);
  return `${authorizationEndpoint}?${params.toString()}`;
}

/**
 * Build the Lark OAuth URL.
 * The redirect_uri must match the one configured in the Lark application.
 */
export function buildLarkOAuthUrl(clientId: string, state: string, redirectUri: string): string {
  const params = new URLSearchParams();
  params.set('app_id', clientId);
  params.set('redirect_uri', redirectUri);
  params.set('state', state);
  return `https://open.larksuite.com/open-apis/authen/v1/index?${params.toString()}`;
}
