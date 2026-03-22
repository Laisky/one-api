import Turnstile from '@/components/Turnstile';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useNotifications } from '@/components/ui/notifications';
import { Separator } from '@/components/ui/separator';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import { loadSystemStatus, type SystemStatus } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { browserSupportsWebAuthn, startRegistration } from '@simplewebauthn/browser';
import QRCode from 'qrcode';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as z from 'zod';

const personalSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  email: z.string().email('Valid email is required').optional(),
});

type PersonalForm = z.infer<typeof personalSchema>;

export function PersonalSettings() {
  const { t } = useTranslation();
  const { user, updateUser } = useAuthStore();
  const { notify } = useNotifications();
  const [loading, setLoading] = useState(false);
  const [systemToken, setSystemToken] = useState('');
  const [affLink, setAffLink] = useState('');
  const { isMobile } = useResponsive();

  // TOTP related state
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

  // Passkey related state
  interface PasskeyInfo {
    id: number;
    credential_name: string;
    sign_count: number;
    created_at: number;
  }
  const [passkeys, setPasskeys] = useState<PasskeyInfo[]>([]);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [passkeyError, setPasskeyError] = useState('');
  const [showPasskeyName, setShowPasskeyName] = useState(false);
  const [passkeyName, setPasskeyName] = useState('');
  const passkeySupported = typeof window !== 'undefined' && browserSupportsWebAuthn();

  // Password change state
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordError, setPasswordError] = useState('');

  // System status state
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({});
  const [emailVerificationCode, setEmailVerificationCode] = useState('');
  const [emailAction, setEmailAction] = useState<'send' | 'bind' | null>(null);
  const [emailVerificationError, setEmailVerificationError] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');

  const turnstileEnabled = Boolean(systemStatus.turnstile_check);
  const turnstileRenderable = turnstileEnabled && Boolean(systemStatus.turnstile_site_key);

  // Load system status
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
      const res = await api.get('/api/user/totp/setup');
      if (res.data.success) {
        setTotpSecret(res.data.data.secret);
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
        ctx.fillText('Two-Factor Authentication', canvas.width / 2, padding + 28);

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

      const attResp = await startRegistration({ optionsJSON: beginRes.data.data });

      const finishRes = await api.post(
        `/api/user/passkey/register/finish?name=${encodeURIComponent(name)}`,
        attResp
      );
      if (finishRes.data.success) {
        setShowPasskeyName(false);
        setPasskeyName('');
        await loadPasskeys();
      } else {
        setPasskeyError(finishRes.data.message || t('personal_settings.passkey.errors.register_failed'));
      }
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : t('personal_settings.passkey.errors.register_failed');
      if (!msg.includes('cancelled') && !msg.includes('AbortError')) {
        setPasskeyError(msg);
      }
    } finally {
      setPasskeyLoading(false);
    }
  };

  // Delete a passkey
  const deletePasskey = async (id: number) => {
    if (!confirm(t('personal_settings.passkey.delete_confirm'))) return;
    setPasskeyLoading(true);
    try {
      const res = await api.delete(`/api/user/passkey/${id}`);
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

  const generateAccessToken = async () => {
    try {
      const res = await api.get('/api/user/token');
      const { success, message, data } = res.data;
      if (success) {
        setSystemToken(data);
        setAffLink('');
        await navigator.clipboard.writeText(data);
      } else {
        console.error('Failed to generate token:', message);
      }
    } catch (error) {
      console.error('Error generating token:', error);
    }
  };

  const getAffLink = async () => {
    try {
      const res = await api.get('/api/user/aff');
      const { success, message, data } = res.data;
      if (success) {
        const link = `${window.location.origin}/register?aff=${data}`;
        setAffLink(link);
        setSystemToken('');
        await navigator.clipboard.writeText(link);
      } else {
        console.error('Failed to get aff link:', message);
      }
    } catch (error) {
      console.error('Error getting aff link:', error);
    }
  };

  const sendEmailVerificationCode = async () => {
    const email = form.getValues('email')?.trim() || '';
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!emailRegex.test(email)) {
      form.setError('email', {
        message: t('auth.register.email_invalid'),
      });
      return;
    }

    if (turnstileEnabled && !turnstileToken) {
      form.setError('email', {
        message: t('auth.login.turnstile_required'),
      });
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
        if (turnstileEnabled) {
          setTurnstileToken('');
        }
        return;
      }

      form.setError('email', {
        message: message || t('personal_settings.profile_info.failed'),
      });
    } catch (error) {
      form.setError('email', {
        message: error instanceof Error ? error.message : t('personal_settings.profile_info.failed'),
      });
    } finally {
      setEmailAction(null);
    }
  };

  const bindEmail = async () => {
    const email = form.getValues('email')?.trim() || '';
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

    if (!emailRegex.test(email)) {
      form.setError('email', {
        message: t('auth.register.email_invalid'),
      });
      return;
    }

    if (!emailVerificationCode.trim()) {
      setEmailVerificationError(t('auth.register.verification_code_required'));
      return;
    }

    setEmailAction('bind');
    setEmailVerificationError('');

    try {
      const response = await api.get(
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

  // Security status indicators
  const hasPasskeys = passkeys.length > 0;
  const securityScore = (hasPasskeys ? 1 : 0) + (totpEnabled ? 1 : 0) + 1; // password always counts as 1

  return (
    <div className="space-y-6">
      {/* Profile Information Card */}
      <Card>
        <CardHeader>
          <CardTitle>{t('personal_settings.profile_info.title')}</CardTitle>
          <CardDescription>{t('personal_settings.profile_info.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="username"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('personal_settings.profile_info.username')}</FormLabel>
                      <FormControl>
                        <Input {...field} disabled />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="display_name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('personal_settings.profile_info.display_name')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('personal_settings.profile_info.display_name_placeholder')} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem className="space-y-3">
                    <FormLabel>{t('personal_settings.profile_info.email')}</FormLabel>
                    <FormControl>
                      <Input type="email" placeholder={t('personal_settings.profile_info.email_placeholder')} {...field} />
                    </FormControl>
                    <div className="flex flex-col gap-3">
                      <p className="text-sm text-muted-foreground">{t('personal_settings.profile_info.email_help')}</p>
                      <div className="flex flex-col gap-3 md:flex-row items-end">
                        <div className="flex-1 w-full space-y-1">
                          <Input
                            value={emailVerificationCode}
                            onChange={(e) => {
                              setEmailVerificationCode(e.target.value);
                              if (emailVerificationError) {
                                setEmailVerificationError('');
                              }
                            }}
                            placeholder={t('personal_settings.profile_info.email_verification_code_placeholder')}
                          />
                        </div>
                        <div className="flex flex-col gap-2 sm:flex-row flex-shrink-0">
                          <Button
                            type="button"
                            variant="outline"
                            onClick={sendEmailVerificationCode}
                            disabled={emailAction !== null || (turnstileEnabled && !turnstileToken)}
                          >
                            {emailAction === 'send'
                              ? t('personal_settings.profile_info.sending_code')
                              : t('personal_settings.profile_info.send_code')}
                          </Button>
                          <Button type="button" onClick={bindEmail} disabled={emailAction !== null}>
                            {emailAction === 'bind'
                              ? t('personal_settings.profile_info.binding_email')
                              : t('personal_settings.profile_info.bind_email')}
                          </Button>
                        </div>
                      </div>
                      {turnstileRenderable && systemStatus.turnstile_site_key && (
                        <Turnstile
                          siteKey={systemStatus.turnstile_site_key}
                          onVerify={(token) => setTurnstileToken(token)}
                          onExpire={() => setTurnstileToken('')}
                        />
                      )}
                      {emailVerificationError && <div className="text-sm text-destructive">{emailVerificationError}</div>}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {form.formState.errors.root && <div className="text-sm text-destructive">{form.formState.errors.root.message}</div>}

              <Button type="submit" disabled={loading}>
                {loading ? t('personal_settings.profile_info.updating') : t('personal_settings.profile_info.update_button')}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>

      {/* Access Token & Invitation Card */}
      <Card>
        <CardHeader>
          <CardTitle>{t('personal_settings.access_token.title')}</CardTitle>
          <CardDescription>{t('personal_settings.access_token.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Button onClick={generateAccessToken} className="w-full">
                {t('personal_settings.access_token.generate_token')}
              </Button>
              {systemToken && <div className="mt-2 p-2 bg-muted rounded text-sm font-mono break-all">{systemToken}</div>}
            </div>

            <div>
              <Button onClick={getAffLink} variant="outline" className="w-full">
                {t('personal_settings.access_token.get_invite_link')}
              </Button>
              {affLink && <div className="mt-2 p-2 bg-muted rounded text-sm break-all">{affLink}</div>}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* ========== Account Security Card ========== */}
      <Card>
        <CardHeader>
          <CardTitle>{t('personal_settings.security.title')}</CardTitle>
          <CardDescription>{t('personal_settings.security.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">

          {/* --- Security Status Overview --- */}
          <div className="rounded-lg border bg-muted/30 p-4">
            <h4 className="text-sm font-medium mb-3">{t('personal_settings.security.status.title')}</h4>
            <div className="flex flex-wrap gap-2">
              {hasPasskeys ? (
                <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                  {t('personal_settings.security.status.passkey_on')}
                </Badge>
              ) : (
                <Badge variant="outline" className="text-muted-foreground border-dashed">
                  {t('personal_settings.security.status.passkey_off')}
                </Badge>
              )}
              {totpEnabled ? (
                <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                  {t('personal_settings.security.status.totp_on')}
                </Badge>
              ) : (
                <Badge variant="outline" className="text-muted-foreground border-dashed">
                  {t('personal_settings.security.status.totp_off')}
                </Badge>
              )}
              <Badge className="bg-emerald-100 text-emerald-800 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-400 dark:border-emerald-800">
                {t('personal_settings.security.status.password_on')}
              </Badge>
            </div>
            {securityScore < 3 && (
              <p className="text-xs text-muted-foreground mt-2">
                {securityScore === 1
                  ? t('personal_settings.passkey.no_passkeys_desc')
                  : ''}
              </p>
            )}
          </div>

          <Separator />

          {/* --- Passkeys Section (Primary / Recommended) --- */}
          <div className="space-y-4">
            <div className="flex items-center gap-2">
              <h3 className="text-base font-semibold">{t('personal_settings.passkey.title')}</h3>
              <Badge className="bg-primary/10 text-primary border-primary/20 text-xs">
                {t('personal_settings.passkey.recommended')}
              </Badge>
            </div>
            <p className="text-sm text-muted-foreground">{t('personal_settings.passkey.description')}</p>

            {passkeyError && <div className="text-sm text-destructive font-medium">{passkeyError}</div>}

            {!passkeySupported ? (
              <Alert>
                <AlertTitle>{t('personal_settings.passkey.errors.not_supported')}</AlertTitle>
                <AlertDescription>{t('personal_settings.passkey.not_supported_desc')}</AlertDescription>
              </Alert>
            ) : (
              <>
                {passkeys.length > 0 ? (
                  <div className="space-y-2">
                    {passkeys.map((pk) => (
                      <div key={pk.id} className="flex items-center justify-between p-3 border rounded-lg bg-background">
                        <div className="flex-1 min-w-0">
                          <div className="font-medium truncate">{pk.credential_name}</div>
                          <div className="text-xs text-muted-foreground">
                            {t('personal_settings.passkey.registered')}: {new Date(pk.created_at).toLocaleDateString()}
                            {' · '}
                            {t('personal_settings.passkey.sign_count')}: {pk.sign_count}
                          </div>
                        </div>
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => deletePasskey(pk.id)}
                          disabled={passkeyLoading}
                        >
                          {t('personal_settings.passkey.delete_button')}
                        </Button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <Alert className="bg-info-muted border-info-border">
                    <AlertTitle className="text-info-foreground">{t('personal_settings.passkey.no_passkeys')}</AlertTitle>
                    <AlertDescription>{t('personal_settings.passkey.no_passkeys_desc')}</AlertDescription>
                  </Alert>
                )}

                {showPasskeyName ? (
                  <div className="flex flex-col space-y-2">
                    <FormLabel>{t('personal_settings.passkey.name_label')}</FormLabel>
                    <Input
                      placeholder={t('personal_settings.passkey.name_placeholder')}
                      value={passkeyName}
                      onChange={(e) => setPasskeyName(e.target.value)}
                      maxLength={128}
                    />
                    <div className="flex gap-2">
                      <Button onClick={registerPasskey} disabled={passkeyLoading}>
                        {passkeyLoading ? t('personal_settings.passkey.processing') : t('personal_settings.passkey.register_button')}
                      </Button>
                      <Button variant="outline" onClick={() => { setShowPasskeyName(false); setPasskeyName(''); }} disabled={passkeyLoading}>
                        {t('personal_settings.totp.cancel')}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Button onClick={() => setShowPasskeyName(true)} disabled={passkeyLoading} className="w-full md:w-auto">
                    {t('personal_settings.passkey.register_button')}
                  </Button>
                )}
              </>
            )}
          </div>

          <Separator />

          {/* --- TOTP Section --- */}
          <div className="space-y-4">
            <h3 className="text-base font-semibold">{t('personal_settings.totp.title')}</h3>
            <p className="text-sm text-muted-foreground">{t('personal_settings.totp.description')}</p>

            {totpError && <div className="text-sm text-destructive font-medium">{totpError}</div>}

            {totpEnabled ? (
              <Alert className="bg-success-muted border-success-border">
                <div className="flex flex-col space-y-3">
                  <div>
                    <AlertTitle className="text-success-foreground">{t('personal_settings.totp.enabled_title')}</AlertTitle>
                    <AlertDescription>{t('personal_settings.totp.enabled_desc')}</AlertDescription>
                  </div>
                  <div className="flex flex-col space-y-2">
                    <Input
                      placeholder={t('personal_settings.totp.disable_placeholder')}
                      value={totpCode}
                      onChange={(e) => setTotpCode(e.target.value)}
                    />
                    {disableTotpError && <div className="text-sm text-destructive font-medium">{disableTotpError}</div>}
                    <Button variant="destructive" onClick={disableTotp} disabled={totpLoading} className="w-full md:w-auto">
                      {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.disable_button')}
                    </Button>
                  </div>
                </div>
              </Alert>
            ) : (
              <div className="space-y-2">
                {setupTotpError && <div className="text-sm text-destructive font-medium">{setupTotpError}</div>}
                <Button variant="default" onClick={setupTotp} disabled={totpLoading} className="w-full md:w-auto">
                  {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.enable_button')}
                </Button>
              </div>
            )}
          </div>

          <Separator />

          {/* --- Password Section (Legacy / Fallback) --- */}
          <div className="space-y-4">
            <h3 className="text-base font-semibold">{t('personal_settings.security.password.title')}</h3>
            <p className="text-sm text-muted-foreground">{t('personal_settings.security.password.description')}</p>

            {passwordError && <div className="text-sm text-destructive font-medium">{passwordError}</div>}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1">
                <FormLabel>{t('personal_settings.security.password.new_password')}</FormLabel>
                <Input
                  type="password"
                  placeholder={t('personal_settings.security.password.new_password_placeholder')}
                  value={newPassword}
                  onChange={(e) => {
                    setNewPassword(e.target.value);
                    if (passwordError) setPasswordError('');
                  }}
                />
              </div>
              <div className="space-y-1">
                <FormLabel>{t('personal_settings.security.password.confirm_password')}</FormLabel>
                <Input
                  type="password"
                  placeholder={t('personal_settings.security.password.confirm_password_placeholder')}
                  value={confirmPassword}
                  onChange={(e) => {
                    setConfirmPassword(e.target.value);
                    if (passwordError) setPasswordError('');
                  }}
                />
              </div>
            </div>
            <Button onClick={updatePassword} disabled={passwordLoading || !newPassword} className="w-full md:w-auto">
              {passwordLoading
                ? t('personal_settings.security.password.updating')
                : t('personal_settings.security.password.update_button')}
            </Button>
          </div>

        </CardContent>
      </Card>

      {/* TOTP Setup Dialog */}
      <Dialog open={showTotpSetup} onOpenChange={(open) => !totpLoading && setShowTotpSetup(open)}>
        <DialogContent className={`${isMobile ? 'max-w-[95vw] p-4 max-h-[90vh] overflow-y-auto' : 'max-w-[500px]'}`}>
          <DialogHeader>
            <DialogTitle className={isMobile ? 'text-base' : ''}>{t('personal_settings.totp.setup_title')}</DialogTitle>
            <DialogDescription className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.setup_desc')}</DialogDescription>
          </DialogHeader>

          <div className={`space-y-${isMobile ? '3' : '4'}`}>
            <Alert className={isMobile ? 'text-xs' : ''}>
              <AlertTitle className={isMobile ? 'text-sm' : ''}>{t('personal_settings.totp.setup_instructions_title')}</AlertTitle>
              <AlertDescription>
                <ol className={`${isMobile ? 'pl-3 mt-1 space-y-0.5 text-xs' : 'pl-4 mt-2 space-y-1'}`}>
                  <li>{t('personal_settings.totp.setup_step1')}</li>
                  <li>{t('personal_settings.totp.setup_step2')}</li>
                  <li>{t('personal_settings.totp.setup_step3')}</li>
                  <li>{t('personal_settings.totp.setup_step4')}</li>
                </ol>
              </AlertDescription>
            </Alert>

            {totpQRCode && (
              <div className={`flex justify-center ${isMobile ? 'my-2' : 'my-4'}`}>
                <img
                  src={totpQRCode}
                  alt="TOTP QR Code"
                  className={`rounded-lg shadow-md ${isMobile ? 'max-w-[240px] w-full h-auto' : 'max-w-full'}`}
                />
              </div>
            )}

            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.secret_key')}</FormLabel>
              <Input value={totpSecret} readOnly className={`font-mono ${isMobile ? 'text-xs h-9' : ''}`} />
            </div>

            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>{t('personal_settings.totp.verify_code')}</FormLabel>
              <Input
                placeholder={
                  isMobile ? t('personal_settings.totp.verify_placeholder_mobile') : t('personal_settings.totp.verify_placeholder')
                }
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                maxLength={6}
                className={isMobile ? 'text-base h-10' : ''}
              />
              {confirmTotpError && (
                <div className={`${isMobile ? 'text-xs' : 'text-sm'} text-destructive font-medium mt-1`}>{confirmTotpError}</div>
              )}
            </div>
          </div>

          <DialogFooter className={isMobile ? 'flex-col space-y-2 sm:space-y-0' : ''}>
            <Button
              variant="outline"
              onClick={() => setShowTotpSetup(false)}
              disabled={totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {t('personal_settings.totp.cancel')}
            </Button>
            <Button
              onClick={confirmTotp}
              disabled={!totpCode || totpCode.length !== 6 || totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {totpLoading ? t('personal_settings.totp.processing') : t('personal_settings.totp.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default PersonalSettings;
