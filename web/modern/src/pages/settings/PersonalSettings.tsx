import { useConfirmDialog } from '@/components/ui/confirm-dialog';
import { useNotifications } from '@/components/ui/notifications';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { buildLarkOAuthUrl, buildOidcOAuthUrl, getOAuthState } from '@/lib/oauth';
import { useAuthStore } from '@/lib/stores/auth';
import { copyToClipboard, loadSystemStatus, type SystemStatus } from '@/lib/utils';
import { zodResolver } from '@/lib/zod-resolver';
import { browserSupportsWebAuthn, startRegistration } from '@simplewebauthn/browser';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as z from 'zod';
import { isPasskeyCeremonyCancelled } from '@/pages/auth/passkey-errors';
import { PersonalAccessAndBindingsCards } from './PersonalAccessAndBindingsCards';
import { PersonalProfileCard } from './PersonalProfileCard';
import { PersonalSecurityCard } from './PersonalSecurityCard';
import type { OAuthBindings, PasskeyInfo, PersonalForm } from './personal-settings-types';

export function PersonalSettings() {
  const { t } = useTranslation();
  const { user, updateUser } = useAuthStore();
  const { notify } = useNotifications();
  const [confirmAction, ConfirmActionDialog] = useConfirmDialog();
  const [loading, setLoading] = useState(false);
  const [systemToken, setSystemToken] = useState('');
  const [affLink, setAffLink] = useState('');
  const { isMobile } = useResponsive();

  const [totpEnabled, setTotpEnabled] = useState(false);
  const [showTotpSetup, setShowTotpSetup] = useState(false);
  const [totpSecret, setTotpSecret] = useState('');
  const [totpQRCode, setTotpQRCode] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [totpLoading, setTotpLoading] = useState(false);
  const [totpError, setTotpError] = useState('');
  const [setupTotpError, setSetupTotpError] = useState('');
  const [confirmTotpError, setConfirmTotpError] = useState('');
  const [disableTotpError, setDisableTotpError] = useState('');

  const [passkeys, setPasskeys] = useState<PasskeyInfo[]>([]);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [passkeyError, setPasskeyError] = useState('');
  const [showPasskeyName, setShowPasskeyName] = useState(false);
  const [passkeyName, setPasskeyName] = useState('');
  const passkeySupported = typeof window !== 'undefined' && browserSupportsWebAuthn();

  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordError, setPasswordError] = useState('');

  const [systemStatus, setSystemStatus] = useState<SystemStatus>({});
  const [emailVerificationCode, setEmailVerificationCode] = useState('');
  const [emailAction, setEmailAction] = useState<'send' | 'bind' | null>(null);
  const [emailVerificationError, setEmailVerificationError] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');

  const [oauthBindings, setOauthBindings] = useState<OAuthBindings>({
    github_id: '',
    wechat_id: '',
    lark_id: '',
    oidc_id: '',
  });
  const [oauthBindingError, setOauthBindingError] = useState('');
  const [oauthBindingPending, setOauthBindingPending] = useState<'lark' | 'oidc' | null>(null);

  const turnstileEnabled = Boolean(systemStatus.turnstile_check);
  const turnstileRenderable = turnstileEnabled && Boolean(systemStatus.turnstile_site_key);

  const personalSchema = useMemo(
    () =>
      z.object({
        username: z.string().min(1, t('personal_settings.profile_info.username_required')),
        display_name: z.string().optional(),
        email: z.string().email(t('personal_settings.profile_info.email_required')).optional(),
      }),
    [t]
  );

  const loadStatus = async () => {
    try {
      const status = await loadSystemStatus();
      if (status) {
        setSystemStatus(status);
      }
    } catch (error) {
      console.error('Failed to load system status:', error);
    }
  };

  const form = useForm<PersonalForm>({
    resolver: zodResolver(personalSchema),
    defaultValues: {
      username: user?.username || '',
      display_name: user?.display_name || '',
      email: user?.email || '',
    },
  });

  const syncProfile = (profile: PersonalForm) => {
    updateUser(profile);
    form.reset({
      username: profile.username || '',
      display_name: profile.display_name || '',
      email: profile.email || '',
    });
    setEmailVerificationCode('');
    setEmailVerificationError('');
  };

  const loadProfile = async (showNotification = false) => {
    try {
      const response = await api.get('/api/user/self');
      const { success, message, data } = response.data;
      if (success && data) {
        syncProfile({
          username: data.username || '',
          display_name: data.display_name || '',
          email: data.email || '',
        });
        setOauthBindings({
          github_id: data.github_id || '',
          wechat_id: data.wechat_id || '',
          lark_id: data.lark_id || '',
          oidc_id: data.oidc_id || '',
        });
        return true;
      }

      const errorMessage = message || t('personal_settings.profile_info.load_failed');
      form.setError('root', { message: errorMessage });
      if (showNotification) {
        notify({
          type: 'error',
          message: errorMessage,
        });
      }
      return false;
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : t('personal_settings.profile_info.load_failed');
      form.setError('root', { message: errorMessage });
      if (showNotification) {
        notify({
          type: 'error',
          message: errorMessage,
        });
      }
      return false;
    }
  };

  // Load TOTP status when component mounts
  const loadTotpStatus = async () => {
    try {
      setTotpError('');
      const res = await api.get('/api/user/totp/status');
      if (res.data.success) {
        setTotpEnabled(res.data.data.totp_enabled);
      } else {
        setTotpError(res.data.message || t('personal_settings.totp.errors.load_status'));
      }
    } catch (error) {
      setTotpError(error instanceof Error ? error.message : t('personal_settings.totp.errors.load_status'));
    }
  };

  useEffect(() => {
    loadStatus();
    loadTotpStatus();
    loadProfile();
    loadPasskeys();
  }, []);

  // Setup TOTP for the user
  const setupTotp = async () => {
    setTotpLoading(true);
    setSetupTotpError('');
    try {
      const res = await api.post('/api/user/totp/setup');
      if (res.data.success) {
        setTotpSecret(res.data.data.secret);
        // Loaded on demand so the QR encoder stays out of the shared settings chunk.
        const { default: QRCode } = await import('qrcode');
        const qrCodeDataURL = await QRCode.toDataURL(res.data.data.qr_code, {
          width: 256,
          margin: 2,
        });

        const systemName = systemStatus.system_name || 'One API';
        const compositeImage = await createQRCodeWithText(qrCodeDataURL, systemName);
        setTotpQRCode(compositeImage);
        setShowTotpSetup(true);
      } else {
        setSetupTotpError(res.data.message || t('personal_settings.totp.errors.setup_failed'));
      }
    } catch (error) {
      setSetupTotpError(error instanceof Error ? error.message : t('personal_settings.totp.errors.setup_failed'));
    }
    setTotpLoading(false);
  };

  // Create QR code with text overlay
  const createQRCodeWithText = async (qrCodeDataURL: string, text: string): Promise<string> => {
    return new Promise((resolve) => {
      const canvas = document.createElement('canvas');
      const ctx = canvas.getContext('2d')!;
      const img = new Image();

      img.onload = () => {
        const padding = 30;
        const textHeight = 40;
        canvas.width = img.width + padding * 2;
        canvas.height = img.height + textHeight + padding * 2;

        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        ctx.fillStyle = '#000000';
        ctx.font = 'bold 18px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(text, canvas.width / 2, padding + 10);

        ctx.font = '12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
        ctx.fillStyle = '#666666';
        ctx.fillText(t('personal_settings.totp.qr_caption'), canvas.width / 2, padding + 28);

        ctx.drawImage(img, padding, padding + textHeight, img.width, img.height);

        resolve(canvas.toDataURL('image/png'));
      };

      img.src = qrCodeDataURL;
    });
  };

  // Confirm TOTP setup with verification code
  const confirmTotp = async () => {
    setConfirmTotpError('');
    if (!/^\d{6}$/.test(totpCode)) {
      setConfirmTotpError(t('personal_settings.totp.errors.invalid_code'));
      return;
    }

    setTotpLoading(true);
    try {
      const res = await api.post('/api/user/totp/confirm', {
        totp_code: totpCode,
      });

      if (res.data.success) {
        setConfirmTotpError('');
        setTotpEnabled(true);
        setShowTotpSetup(false);
        setTotpCode('');
        setTotpSecret('');
        setTotpQRCode('');
      } else {
        setConfirmTotpError(res.data.message || t('personal_settings.totp.errors.confirm_failed'));
      }
    } catch (error) {
      setConfirmTotpError(error instanceof Error ? error.message : t('personal_settings.totp.errors.confirm_failed'));
    } finally {
      setTotpLoading(false);
    }
  };

  // Disable TOTP for the user
  const disableTotp = async () => {
    setDisableTotpError('');
    if (!totpCode) {
      setDisableTotpError(t('personal_settings.totp.errors.missing_code'));
      return;
    }

    setTotpLoading(true);
    try {
      const res = await api.post('/api/user/totp/disable', {
        totp_code: totpCode,
      });

      if (res.data.success) {
        setDisableTotpError('');
        setTotpEnabled(false);
        setTotpCode('');
      } else {
        setDisableTotpError(res.data.message || t('personal_settings.totp.errors.disable_failed'));
      }
    } catch (error) {
      setDisableTotpError(error instanceof Error ? error.message : t('personal_settings.totp.errors.disable_failed'));
    }
    setTotpLoading(false);
  };

  // Load passkey list
  const loadPasskeys = async () => {
    try {
      setPasskeyError('');
      const res = await api.get('/api/user/passkey');
      if (res.data.success) {
        setPasskeys(res.data.data || []);
      } else {
        setPasskeyError(res.data.message || t('personal_settings.passkey.errors.load_failed'));
      }
    } catch (error) {
      setPasskeyError(error instanceof Error ? error.message : t('personal_settings.passkey.errors.load_failed'));
    }
  };

  // Register a new passkey
  const registerPasskey = async () => {
    if (!passkeySupported) {
      setPasskeyError(t('personal_settings.passkey.errors.not_supported'));
      return;
    }

    const name = passkeyName.trim() || 'Passkey';
    setPasskeyLoading(true);
    setPasskeyError('');
    try {
      const beginRes = await api.post('/api/user/passkey/register/begin');
      if (!beginRes.data.success) {
        setPasskeyError(beginRes.data.message || t('personal_settings.passkey.errors.register_failed'));
        return;
      }

      const attResp = await startRegistration({ optionsJSON: beginRes.data.data.publicKey });

      const finishRes = await api.post(`/api/user/passkey/register/finish?name=${encodeURIComponent(name)}`, attResp);
      if (finishRes.data.success) {
        setShowPasskeyName(false);
        setPasskeyName('');
        // The account now has a passkey, so the "set up a passkey" banner (PasskeyPromptBanner) is no longer needed.
        try {
          localStorage.setItem('passkey_prompt_dismissed', '1');
        } catch (storageError) {
          // Storage can be blocked (private mode, site-data settings); the banner then simply stays dismissible.
          console.warn(`Could not persist the passkey prompt dismissal: ${storageError instanceof Error ? storageError.message : String(storageError)}`);
        }
        await loadPasskeys();
      } else {
        setPasskeyError(finishRes.data.message || t('personal_settings.passkey.errors.register_failed'));
      }
    } catch (error: unknown) {
      // A dismissed prompt (NotAllowedError) or aborted ceremony (AbortError) is a cancellation, not a failure.
      if (!isPasskeyCeremonyCancelled(error)) {
        setPasskeyError(error instanceof Error && error.message ? error.message : t('personal_settings.passkey.errors.register_failed'));
      }
    } finally {
      setPasskeyLoading(false);
    }
  };

  // Delete a passkey
  const deletePasskey = async (passkey: PasskeyInfo) => {
    const confirmed = await confirmAction({
      title: t('personal_settings.passkey.delete_button'),
      description: t('personal_settings.passkey.delete_confirm'),
      details: [
        {
          label: t('personal_settings.passkey.name_label'),
          value: passkey.credential_name,
        },
        {
          label: t('personal_settings.passkey.registered'),
          value: new Date(passkey.created_at).toLocaleDateString(),
        },
      ],
      variant: 'destructive',
    });
    if (!confirmed) return;

    setPasskeyLoading(true);
    try {
      const passkeyRef = passkey.uuid || passkey.id;
      const res = await api.delete(`/api/user/passkey/${passkeyRef}`);
      if (res.data.success) {
        await loadPasskeys();
      } else {
        setPasskeyError(res.data.message || t('personal_settings.passkey.errors.delete_failed'));
      }
    } catch (error) {
      setPasskeyError(error instanceof Error ? error.message : t('personal_settings.passkey.errors.delete_failed'));
    } finally {
      setPasskeyLoading(false);
    }
  };

  // Update password
  const updatePassword = async () => {
    setPasswordError('');
    if (!newPassword) return;
    if (newPassword !== confirmPassword) {
      setPasswordError(t('personal_settings.security.password.mismatch'));
      return;
    }

    setPasswordLoading(true);
    try {
      const response = await api.put('/api/user/self', { password: newPassword });
      const { success, message } = response.data;
      if (success) {
        setNewPassword('');
        setConfirmPassword('');
        notify({
          type: 'success',
          message: t('personal_settings.security.password.success'),
        });
      } else {
        setPasswordError(message || t('personal_settings.security.password.failed'));
      }
    } catch (error) {
      setPasswordError(error instanceof Error ? error.message : t('personal_settings.security.password.failed'));
    } finally {
      setPasswordLoading(false);
    }
  };

  /**
   * copyGeneratedValue copies a freshly generated token or invitation link, which stays visible on the page.
   * It takes the value to copy and returns once done; a failed copy is reported as a notification (the value itself
   * is never logged) so the user knows to copy it manually instead of assuming it reached the clipboard.
   */
  const copyGeneratedValue = async (value: string) => {
    try {
      await copyToClipboard(value);
    } catch {
      notify({
        type: 'error',
        message: t('personal_settings.access_token.copy_failed'),
      });
    }
  };

  const generateAccessToken = async () => {
    try {
      const res = await api.post('/api/user/token');
      const { success, message, data } = res.data;
      if (success) {
        setSystemToken(data);
        setAffLink('');
        await copyGeneratedValue(data);
      } else {
        notify({
          type: 'error',
          message: message || t('personal_settings.access_token.generate_failed'),
        });
      }
    } catch (error) {
      notify({
        type: 'error',
        message: error instanceof Error ? error.message : t('personal_settings.access_token.generate_failed'),
      });
    }
  };

  const getAffLink = async () => {
    try {
      const res = await api.post('/api/user/aff');
      const { success, message, data } = res.data;
      if (success) {
        const link = `${window.location.origin}/register?aff=${data}`;
        setAffLink(link);
        setSystemToken('');
        await copyGeneratedValue(link);
      } else {
        notify({
          type: 'error',
          message: message || t('personal_settings.access_token.invite_failed'),
        });
      }
    } catch (error) {
      notify({
        type: 'error',
        message: error instanceof Error ? error.message : t('personal_settings.access_token.invite_failed'),
      });
    }
  };

  /**
   * setEmailError shows an email-field error and focuses the field. FormMessage is not a live region, so focusing
   * the field is what makes screen readers announce the linked message. It takes the message and returns nothing.
   */
  const setEmailError = (message: string) => {
    form.setError('email', { message }, { shouldFocus: true });
  };

  const sendEmailVerificationCode = async () => {
    const email = form.getValues('email')?.trim() || '';
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!emailRegex.test(email)) {
      setEmailError(t('auth.register.email_invalid'));
      return;
    }

    if (turnstileEnabled && !turnstileToken) {
      setEmailError(t('auth.login.turnstile_required'));
      return;
    }

    setEmailAction('send');
    setEmailVerificationError('');

    try {
      const turnstileParam = turnstileEnabled && turnstileToken ? `&turnstile=${encodeURIComponent(turnstileToken)}` : '';
      const response = await api.get(`/api/verification?email=${encodeURIComponent(email)}${turnstileParam}`);
      const { success, message } = response.data;

      if (success) {
        form.clearErrors('email');
        notify({
          type: 'success',
          message: message || t('personal_settings.profile_info.send_code_success'),
        });
        // Keep the Turnstile token: the backend marks the session verified after this
        // /api/verification call (middleware.TurnstileCheck) and skips the check on later
        // requests, so wiping it here would only strand the "Send Code" button (which is
        // gated on `turnstileEnabled && !turnstileToken`) with no way to re-issue a token.
        return;
      }

      setEmailError(message || t('personal_settings.profile_info.failed'));
    } catch (error) {
      setEmailError(error instanceof Error ? error.message : t('personal_settings.profile_info.failed'));
    } finally {
      setEmailAction(null);
    }
  };

  const bindEmail = async () => {
    const email = form.getValues('email')?.trim() || '';
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!emailRegex.test(email)) {
      setEmailError(t('auth.register.email_invalid'));
      return;
    }

    if (!emailVerificationCode.trim()) {
      setEmailVerificationError(t('auth.register.verification_code_required'));
      return;
    }

    setEmailAction('bind');
    setEmailVerificationError('');

    try {
      const response = await api.post(
        `/api/oauth/email/bind?email=${encodeURIComponent(email)}&code=${encodeURIComponent(emailVerificationCode.trim())}`
      );
      const { success, message } = response.data;

      if (success) {
        await loadProfile();
        notify({
          type: 'success',
          message: t('personal_settings.profile_info.bind_success'),
        });
        return;
      }

      setEmailVerificationError(message || t('personal_settings.profile_info.failed'));
    } catch (error) {
      setEmailVerificationError(error instanceof Error ? error.message : t('personal_settings.profile_info.failed'));
    } finally {
      setEmailAction(null);
    }
  };

  // Bind a Lark account by redirecting to the Lark OAuth authorize URL.
  // The OAuth callback (/oauth/lark) detects bind vs login by inspecting the
  // backend response message, so no extra "intent" query param is needed.
  const onBindLark = async () => {
    setOauthBindingError('');
    if (!systemStatus.lark_client_id) {
      setOauthBindingError(t('personal_settings.oauth_binding.bind_failed'));
      return;
    }
    setOauthBindingPending('lark');
    try {
      const state = await getOAuthState();
      const redirectUri = `${window.location.origin}/oauth/lark`;
      window.location.href = buildLarkOAuthUrl(systemStatus.lark_client_id, state, redirectUri);
    } catch (error) {
      setOauthBindingPending(null);
      setOauthBindingError(error instanceof Error && error.message ? error.message : t('auth.oauth.state_failed'));
    }
  };

  // Bind an OIDC account by redirecting to the configured authorization endpoint.
  const onBindOidc = async () => {
    setOauthBindingError('');
    if (!systemStatus.oidc_client_id || !systemStatus.oidc_authorization_endpoint) {
      setOauthBindingError(t('personal_settings.oauth_binding.bind_failed'));
      return;
    }
    setOauthBindingPending('oidc');
    try {
      const state = await getOAuthState();
      const redirectUri = `${window.location.origin}/oauth/oidc`;
      const url = buildOidcOAuthUrl(systemStatus.oidc_authorization_endpoint, systemStatus.oidc_client_id, state, redirectUri);
      window.location.href = url;
    } catch (error) {
      setOauthBindingPending(null);
      setOauthBindingError(error instanceof Error && error.message ? error.message : t('auth.oauth.state_failed'));
    }
  };

  const onSubmit = async (data: PersonalForm) => {
    setLoading(true);
    try {
      const payload = { ...data };
      delete (payload as Record<string, unknown>).email;

      const response = await api.put('/api/user/self', payload);
      const { success, message } = response.data;
      if (success) {
        const refreshed = await loadProfile();
        if (!refreshed) {
          syncProfile({ ...data });
        }
        notify({
          type: 'success',
          message: message || t('personal_settings.profile_info.success'),
        });
      } else {
        form.setError('root', {
          message: message || t('personal_settings.profile_info.failed'),
        });
      }
    } catch (error) {
      form.setError('root', {
        message: error instanceof Error ? error.message : t('personal_settings.profile_info.failed'),
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <PersonalProfileCard
        t={t}
        form={form}
        loading={loading}
        onSubmit={onSubmit}
        emailVerificationCode={emailVerificationCode}
        onEmailVerificationCodeChange={(value) => {
          setEmailVerificationCode(value);
          if (emailVerificationError) {
            setEmailVerificationError('');
          }
        }}
        emailVerificationError={emailVerificationError}
        emailAction={emailAction}
        onSendEmailVerificationCode={sendEmailVerificationCode}
        onBindEmail={bindEmail}
        turnstileEnabled={turnstileEnabled}
        turnstileToken={turnstileToken}
        turnstileSiteKey={systemStatus.turnstile_site_key}
        turnstileRenderable={turnstileRenderable}
        onTurnstileVerify={(token) => setTurnstileToken(token)}
        onTurnstileExpire={() => setTurnstileToken('')}
      />

      <PersonalAccessAndBindingsCards
        t={t}
        systemToken={systemToken}
        affLink={affLink}
        onGenerateAccessToken={generateAccessToken}
        onGetAffLink={getAffLink}
        systemStatus={systemStatus}
        oauthBindings={oauthBindings}
        oauthBindingError={oauthBindingError}
        oauthBindingPending={oauthBindingPending}
        onBindLark={onBindLark}
        onBindOidc={onBindOidc}
      />
      <PersonalSecurityCard
        t={t}
        passkey={{
          passkeys,
          passkeySupported,
          passkeyError,
          passkeyLoading,
          showPasskeyName,
          passkeyName,
          onPasskeyNameChange: setPasskeyName,
          onOpenPasskeyName: () => setShowPasskeyName(true),
          onCancelPasskeyName: () => {
            setShowPasskeyName(false);
            setPasskeyName('');
          },
          onRegisterPasskey: registerPasskey,
          onDeletePasskey: deletePasskey,
        }}
        totp={{
          totpEnabled,
          totpError,
          setupTotpError,
          disableTotpError,
          totpCode,
          onTotpCodeChange: setTotpCode,
          totpLoading,
          onSetupTotp: setupTotp,
          onDisableTotp: disableTotp,
          showTotpSetup,
          onShowTotpSetupChange: (open) => !totpLoading && setShowTotpSetup(open),
          totpQRCode,
          totpSecret,
          confirmTotpError,
          onConfirmTotp: confirmTotp,
          isMobile,
        }}
        password={{
          newPassword,
          confirmPassword,
          passwordError,
          passwordLoading,
          onNewPasswordChange: (value) => {
            setNewPassword(value);
            if (passwordError) setPasswordError('');
          },
          onConfirmPasswordChange: (value) => {
            setConfirmPassword(value);
            if (passwordError) setPasswordError('');
          },
          onUpdatePassword: updatePassword,
        }}
      />
      <ConfirmActionDialog />
    </div>
  );
}

export default PersonalSettings;
