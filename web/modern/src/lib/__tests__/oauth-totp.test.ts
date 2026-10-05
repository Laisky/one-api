import { api } from '@/lib/api';
import { vi } from 'vitest';
import { isTotpExpiredResponse, isTotpRequiredResponse, resolveOAuthStateRedirect, verifyOAuthTotpCode } from '../oauth-totp';

describe('isTotpRequiredResponse', () => {
  it('recognises the password-login totp_required challenge', () => {
    expect(isTotpRequiredResponse({ success: false, message: 'totp_required', data: { totp_required: true } })).toBe(true);
    expect(isTotpRequiredResponse({ success: false, message: 'totp_required' })).toBe(true);
    expect(isTotpRequiredResponse({ success: false, message: '', data: { totp_required: true } })).toBe(true);
  });

  it('ignores successes, ordinary failures, and malformed bodies', () => {
    expect(isTotpRequiredResponse({ success: true, message: 'totp_required', data: { totp_required: true } })).toBe(false);
    expect(isTotpRequiredResponse({ success: false, message: 'oauth failed', data: null })).toBe(false);
    expect(isTotpRequiredResponse({ success: false, message: 'bind' })).toBe(false);
    expect(isTotpRequiredResponse(null)).toBe(false);
    expect(isTotpRequiredResponse('totp_required')).toBe(false);
  });
});

describe('isTotpExpiredResponse', () => {
  it('only matches a failed envelope with data.totp_expired', () => {
    expect(isTotpExpiredResponse({ success: false, message: 'expired', data: { totp_expired: true } })).toBe(true);
    expect(isTotpExpiredResponse({ success: false, message: 'Invalid TOTP code' })).toBe(false);
    expect(isTotpExpiredResponse({ success: true, data: { totp_expired: true } })).toBe(false);
  });
});

describe('resolveOAuthStateRedirect', () => {
  it('returns a decoded same-origin path carried in the state', () => {
    expect(resolveOAuthStateRedirect('nonce redirect_to=%2Ftokens%3Fp%3D1')).toBe('/tokens?p=1');
  });

  it('falls back for missing, unsafe, login-loop, and undecodable targets', () => {
    expect(resolveOAuthStateRedirect(null)).toBe('/');
    expect(resolveOAuthStateRedirect('nonce')).toBe('/');
    expect(resolveOAuthStateRedirect('nonce redirect_to=')).toBe('/');
    expect(resolveOAuthStateRedirect('nonce redirect_to=%2F%2Fevil.example')).toBe('/');
    expect(resolveOAuthStateRedirect('nonce redirect_to=https%3A%2F%2Fevil.example')).toBe('/');
    expect(resolveOAuthStateRedirect('nonce redirect_to=%2Flogin%3Fx%3D1')).toBe('/');
    expect(resolveOAuthStateRedirect('nonce redirect_to=%E0%A4%A', '/dashboard')).toBe('/dashboard');
  });
});

describe('verifyOAuthTotpCode', () => {
  const postSpy = vi.spyOn(api, 'post');

  beforeEach(() => {
    postSpy.mockReset();
  });

  afterAll(() => {
    postSpy.mockRestore();
  });

  it('posts the code as totp_code and returns the user on success', async () => {
    postSpy.mockResolvedValueOnce({ status: 200, data: { success: true, message: '', data: { id: 1 } } } as any);

    await expect(verifyOAuthTotpCode('123456')).resolves.toEqual({ kind: 'success', user: { id: 1 } });
    expect(postSpy).toHaveBeenCalledWith('/api/oauth/totp', { totp_code: '123456' });
  });

  it('classifies a wrong code, an expired challenge, throttling, and transport failures', async () => {
    postSpy.mockResolvedValueOnce({ status: 200, data: { success: false, message: 'Invalid TOTP code' } } as any);
    await expect(verifyOAuthTotpCode('000000')).resolves.toEqual({ kind: 'invalid', message: 'Invalid TOTP code' });

    postSpy.mockResolvedValueOnce({ status: 200, data: { success: false, message: 'gone', data: { totp_expired: true } } } as any);
    await expect(verifyOAuthTotpCode('000000')).resolves.toEqual({ kind: 'expired', message: 'gone' });

    postSpy.mockRejectedValueOnce(
      Object.assign(new Error('Request failed with status code 429'), {
        response: { status: 429, data: { success: false, message: 'slow down' } },
      })
    );
    await expect(verifyOAuthTotpCode('000000')).resolves.toEqual({ kind: 'rate_limited', message: 'slow down' });

    postSpy.mockRejectedValueOnce(
      Object.assign(new Error('Request failed with status code 500'), { response: { status: 500, data: { success: false } } })
    );
    await expect(verifyOAuthTotpCode('000000')).resolves.toEqual({ kind: 'error', message: '' });

    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    postSpy.mockRejectedValueOnce(new Error('Network Error'));
    await expect(verifyOAuthTotpCode('000000')).resolves.toEqual({ kind: 'error', message: 'Network Error' });
    expect(consoleSpy).toHaveBeenCalledWith('OAuth TOTP verification request failed: Network Error');
    consoleSpy.mockRestore();
  });
});
