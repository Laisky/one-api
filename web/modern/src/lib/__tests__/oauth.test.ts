import { buildLarkOAuthUrl } from '../oauth';

describe('buildLarkOAuthUrl', () => {
  it('includes the OAuth state and redirect URI in the authorize URL', () => {
    const url = new URL(buildLarkOAuthUrl('lark-client', 'opaque-state', 'https://app.example.com/oauth/lark'));

    expect(url.origin).toBe('https://open.larksuite.com');
    expect(url.pathname).toBe('/open-apis/authen/v1/index');
    expect(url.searchParams.get('app_id')).toBe('lark-client');
    expect(url.searchParams.get('state')).toBe('opaque-state');
    expect(url.searchParams.get('redirect_uri')).toBe('https://app.example.com/oauth/lark');
  });
});
