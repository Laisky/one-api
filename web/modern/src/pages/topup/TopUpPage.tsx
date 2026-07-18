import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { api } from '@/lib/api';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/lib/stores/auth';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  AlertCircle,
  ArrowUpRight,
  CheckCircle2,
  Clock,
  CreditCard,
  Mail,
  RefreshCw,
  Ticket,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router-dom';
import * as z from 'zod';

const QUOTA_PER_UNIT_FALLBACK = 500000;
const PRESET_AMOUNTS = [5, 10, 20, 50, 100] as const;
const MIN_TOPUP_USD = 5;

function readQuotaPerUnit(): number {
  const raw = parseFloat(localStorage.getItem('quota_per_unit') || `${QUOTA_PER_UNIT_FALLBACK}`);
  return Number.isFinite(raw) && raw > 0 ? raw : QUOTA_PER_UNIT_FALLBACK;
}

function formatUSD(quota: number): string {
  const usd = quota / readQuotaPerUnit();
  return usd.toLocaleString('en-US', { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

export function TopUpPage() {
  const { user, updateUser } = useAuthStore();
  const location = useLocation();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`topup.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [userQuota, setUserQuota] = useState(user?.quota ?? 0);
  const [userData, setUserData] = useState<any>(null);
  const [topUpLink, setTopUpLink] = useState('');
  const [isRefreshing, setIsRefreshing] = useState(false);

  type HistoryEntry = {
    id: number;
    created_at: number;
    quota: number;
    content?: string;
    source: 'stripe' | 'code' | 'admin' | 'system' | 'other';
  };
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [isHistoryLoading, setIsHistoryLoading] = useState(false);

  const stripeOutcome = useMemo<'success' | 'cancel' | null>(() => {
    if (location.pathname.endsWith('/topup/success')) return 'success';
    if (location.pathname.endsWith('/topup/cancel')) return 'cancel';
    return null;
  }, [location.pathname]);

  // ─── Forms ────────────────────────────────────────────────────────
  const stripeSchema = z.object({
    amount_usd: z.coerce
      .number({ invalid_type_error: tr('stripe.required', 'Enter an amount in USD') })
      .min(MIN_TOPUP_USD, tr('stripe.min', `Minimum is $${MIN_TOPUP_USD}`, { value: MIN_TOPUP_USD }))
      .max(100000, tr('stripe.max', 'Amount too large')),
  });
  type StripeForm = z.infer<typeof stripeSchema>;
  const stripeForm = useForm<StripeForm>({
    resolver: zodResolver(stripeSchema),
    defaultValues: { amount_usd: MIN_TOPUP_USD },
  });
  const [isStripeSubmitting, setIsStripeSubmitting] = useState(false);

  const codeSchema = z.object({
    redemption_code: z.string().min(1, tr('redeem.required', 'Redemption code is required')),
  });
  type CodeForm = z.infer<typeof codeSchema>;
  const codeForm = useForm<CodeForm>({
    resolver: zodResolver(codeSchema),
    defaultValues: { redemption_code: '' },
  });
  const [isCodeSubmitting, setIsCodeSubmitting] = useState(false);

  // ─── Data ──────────────────────────────────────────────────────────
  const loadUserData = async () => {
    setIsRefreshing(true);
    try {
      const res = await api.get('/api/user/self');
      const { success, data } = res.data;
      if (success) {
        setUserQuota(data.quota);
        setUserData(data);
        updateUser(data);
      }
    } catch (error) {
      console.error('Error loading user data:', error);
    } finally {
      setIsRefreshing(false);
    }
  };

  const loadSystemStatus = () => {
    const status = localStorage.getItem('status');
    if (!status) return;
    try {
      const parsed = JSON.parse(status);
      if (parsed.top_up_link) setTopUpLink(parsed.top_up_link);
    } catch (error) {
      console.error('Error parsing system status:', error);
    }
  };

  const classifySource = (content: string): HistoryEntry['source'] => {
    const c = content.toLowerCase();
    if (c.includes('stripe')) return 'stripe';
    if (c.includes('redemption') || c.includes('redeem') || c.includes('code')) return 'code';
    if (c.includes('admin') || c.includes('via api')) return 'admin';
    if (c.includes('welcome') || c.includes('system')) return 'system';
    return 'other';
  };

  const loadHistory = async () => {
    setIsHistoryLoading(true);
    try {
      // type=1 → LogTypeTopup (covers both Stripe and redemption code grants)
      const res = await api.get('/api/log/self?type=1&p=0&size=10&sort=created_at&order=desc');
      const { success, data, items } = res.data || {};
      const rows: any[] = Array.isArray(items)
        ? items
        : Array.isArray(data)
        ? data
        : Array.isArray(data?.items)
        ? data.items
        : [];
      if (success || rows.length) {
        setHistory(
          rows.map((r) => ({
            id: r.id,
            created_at: r.created_at,
            quota: r.quota ?? 0,
            content: r.content || '',
            source: classifySource(r.content || ''),
          }))
        );
      }
    } catch (error) {
      console.error('Error loading topup history:', error);
    } finally {
      setIsHistoryLoading(false);
    }
  };

  useEffect(() => {
    loadUserData();
    loadSystemStatus();
    loadHistory();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ─── Handlers ──────────────────────────────────────────────────────
  const onStripeSubmit = async (data: StripeForm) => {
    setIsStripeSubmitting(true);
    try {
      const res = await api.post('/api/user/topup/stripe', { amount_usd: data.amount_usd });
      const { success, message, data: payload } = res.data;
      if (success && payload?.url) {
        window.location.href = payload.url;
        return;
      }
      stripeForm.setError('root', {
        message: message || tr('stripe.failed', 'Failed to create checkout session'),
      });
    } catch (error) {
      stripeForm.setError('root', {
        message: error instanceof Error ? error.message : tr('stripe.failed', 'Failed to create checkout session'),
      });
    } finally {
      setIsStripeSubmitting(false);
    }
  };

  const onCodeSubmit = async (data: CodeForm) => {
    setIsCodeSubmitting(true);
    try {
      const res = await api.post('/api/user/topup', { key: data.redemption_code });
      const { success, message, data: added } = res.data;
      if (success) {
        codeForm.reset();
        codeForm.setError('root', {
          type: 'success',
          message: tr('redeem.success', `Successfully redeemed! Added {{value}} tokens.`, {
            value: (added || 0).toLocaleString(),
          }),
        });
        loadUserData();
        loadHistory();
      } else {
        codeForm.setError('root', { message: message || tr('redeem.failed', 'Redemption failed') });
      }
    } catch (error) {
      codeForm.setError('root', {
        message: error instanceof Error ? error.message : tr('redeem.failed', 'Redemption failed'),
      });
    } finally {
      setIsCodeSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) return;
    try {
      const url = new URL(topUpLink);
      if (userData) {
        url.searchParams.append('username', userData.username);
        url.searchParams.append('user_id', userData.id.toString());
        const uuid =
          (globalThis as any).crypto?.randomUUID?.() ??
          'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
            const r = (Math.random() * 16) | 0;
            const v = c === 'x' ? r : (r & 0x3) | 0x8;
            return v.toString(16);
          });
        url.searchParams.append('transaction_id', uuid);
      }
      window.open(url.toString(), '_blank');
    } catch (error) {
      console.error('Error opening top-up link:', error);
    }
  };

  // ─── Derived ───────────────────────────────────────────────────────
  const balanceUSD = formatUSD(userQuota);
  const tokensLabel = `${userQuota.toLocaleString()} ${tr('tokens', 'tokens')}`;
  const watchedAmount = stripeForm.watch('amount_usd');
  const userEmail = (userData?.email as string | undefined) || (user as any)?.email || '';

  const setPreset = (value: number) => {
    stripeForm.setValue('amount_usd', value, { shouldValidate: true, shouldDirty: true });
  };

  const formatHistoryDate = (ts: number) =>
    new Date(ts * 1000).toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });

  const sourceLabel = (s: HistoryEntry['source']): string => {
    switch (s) {
      case 'stripe':
        return tr('history.source.stripe', 'Card (Stripe)');
      case 'code':
        return tr('history.source.code', 'Redemption code');
      case 'admin':
        return tr('history.source.admin', 'Admin grant');
      case 'system':
        return tr('history.source.system', 'System bonus');
      default:
        return tr('history.source.other', 'Other');
    }
  };

  return (
    <ResponsivePageContainer
      title={tr('title', 'Billing')}
      description={tr('description', 'Manage your balance and add credits')}
      className="max-w-4xl"
    >
      <div className="space-y-6">
        {/* ── Outcome banners ───────────────────────────────────────── */}
        {stripeOutcome === 'success' && (
          <div className="flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm">
            <CheckCircle2 className="h-4 w-4 mt-0.5 text-success flex-shrink-0" />
            <div className="text-success-foreground">
              <p className="font-medium mb-0.5">{tr('stripe.outcome_success_title', 'Payment received')}</p>
              <p className="text-success-foreground/80">
                {tr(
                  'stripe.outcome_success',
                  'Your balance will update within a moment once Stripe confirms the charge.'
                )}
              </p>
            </div>
          </div>
        )}
        {stripeOutcome === 'cancel' && (
          <div className="flex items-start gap-3 rounded-lg border border-warning-border bg-warning-muted px-4 py-3 text-sm">
            <AlertCircle className="h-4 w-4 mt-0.5 text-warning flex-shrink-0" />
            <div className="text-warning-foreground">
              <p className="font-medium mb-0.5">{tr('stripe.outcome_cancel_title', 'Payment canceled')}</p>
              <p className="text-warning-foreground/80">
                {tr('stripe.outcome_cancel', 'You have not been charged.')}
              </p>
            </div>
          </div>
        )}

        {/* ── Top row: Balance + Add credits ───────────────────────── */}
        <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
          {/* Balance — left, narrow */}
          <Card className="lg:col-span-2">
            <CardContent className="p-6 h-full flex flex-col">
              <div className="flex items-start justify-between gap-2">
                <p className="text-sm text-muted-foreground">{tr('balance.title', 'Current balance')}</p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={loadUserData}
                  disabled={isRefreshing}
                  className="-mr-2 -mt-2 h-8 text-muted-foreground hover:text-foreground"
                >
                  <RefreshCw className={cn('h-4 w-4', isRefreshing && 'animate-spin')} />
                </Button>
              </div>
              <p className="mt-1 text-4xl font-semibold tracking-tight tabular-nums text-foreground">
                {balanceUSD}
              </p>
              <p className="mt-1 text-xs text-muted-foreground tabular-nums">{tokensLabel}</p>
              <p className="mt-auto pt-6 text-xs text-muted-foreground">
                {tr('balance.note', 'Credits are billed in USD and never expire.')}
              </p>
            </CardContent>
          </Card>

          {/* Add credits — right, wider */}
          <Card className="lg:col-span-3">
            <CardHeader>
              <div className="flex items-center gap-2">
                <CreditCard className="h-4 w-4 text-muted-foreground" />
                <CardTitle>{tr('stripe.title', 'Add credits')}</CardTitle>
              </div>
              <CardDescription>
                {tr(
                  'stripe.description',
                  'Pay by card via Stripe. USD only, $20 minimum. A receipt is emailed to your registered address.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
            <Form {...stripeForm}>
              <form onSubmit={stripeForm.handleSubmit(onStripeSubmit)} className="space-y-5">
                {/* Preset chips */}
                <div className="flex flex-wrap gap-2">
                  {PRESET_AMOUNTS.map((amount) => {
                    const active = Number(watchedAmount) === amount;
                    return (
                      <button
                        type="button"
                        key={amount}
                        onClick={() => setPreset(amount)}
                        className={cn(
                          'inline-flex items-center justify-center rounded-md border px-3 py-1.5 text-sm font-medium tabular-nums transition-colors',
                          active
                            ? 'border-primary bg-primary text-primary-foreground'
                            : 'border-border bg-background text-foreground hover:bg-muted'
                        )}
                      >
                        ${amount}
                      </button>
                    );
                  })}
                </div>

                <FormField
                  control={stripeForm.control}
                  name="amount_usd"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{tr('stripe.label', 'Amount (USD)')}</FormLabel>
                      <FormControl>
                        <div className="relative">
                          <span className="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-muted-foreground">
                            $
                          </span>
                          <Input
                            type="number"
                            inputMode="decimal"
                            min={MIN_TOPUP_USD}
                            step="1"
                            placeholder={String(MIN_TOPUP_USD)}
                            className="pl-7 tabular-nums"
                            {...field}
                          />
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {stripeForm.formState.errors.root && (
                  <div className="text-sm text-destructive">{stripeForm.formState.errors.root.message}</div>
                )}

                <div className="flex flex-col-reverse sm:flex-row sm:items-center sm:justify-between gap-3 pt-1">
                  <p className="text-xs text-muted-foreground">
                    {tr(
                      'stripe.note',
                      'You will be redirected to Stripe Checkout. Your balance will update once payment is confirmed.'
                    )}
                  </p>
                  <Button type="submit" disabled={isStripeSubmitting} className="sm:w-auto">
                    {isStripeSubmitting
                      ? tr('stripe.processing', 'Redirecting…')
                      : tr('stripe.button', 'Continue to Stripe')}
                  </Button>
                </div>
              </form>
            </Form>
          </CardContent>
          </Card>
        </div>

        {/* ── Redemption code — secondary, compact ─────────────────── */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Ticket className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-base">{tr('redeem.title', 'Redeem a code')}</CardTitle>
            </div>
            <CardDescription>
              {tr('redeem.description', 'Have a redemption code? Add credits without paying.')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...codeForm}>
              <form
                onSubmit={codeForm.handleSubmit(onCodeSubmit)}
                className="flex flex-col sm:flex-row sm:items-start gap-3"
              >
                <FormField
                  control={codeForm.control}
                  name="redemption_code"
                  render={({ field }) => (
                    <FormItem className="flex-1">
                      <FormControl>
                        <Input
                          autoComplete="off"
                          spellCheck={false}
                          placeholder={tr('redeem.placeholder', 'Enter your redemption code')}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button type="submit" variant="outline" disabled={isCodeSubmitting} className="sm:w-auto">
                  {isCodeSubmitting
                    ? tr('redeem.processing', 'Redeeming...')
                    : tr('redeem.button', 'Redeem')}
                </Button>
              </form>

              {codeForm.formState.errors.root && (
                <div
                  className={cn(
                    'mt-3 text-sm',
                    codeForm.formState.errors.root.type === 'success' ? 'text-success' : 'text-destructive'
                  )}
                >
                  {codeForm.formState.errors.root.message}
                </div>
              )}
            </Form>
          </CardContent>
        </Card>

        {/* ── History (Stripe + redemption) ─────────────────────────── */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-2">
                <Clock className="h-4 w-4 text-muted-foreground" />
                <CardTitle className="text-base">{tr('history.title', 'Recent activity')}</CardTitle>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={loadHistory}
                disabled={isHistoryLoading}
                className="h-8 text-muted-foreground hover:text-foreground"
              >
                <RefreshCw className={cn('h-4 w-4', isHistoryLoading && 'animate-spin')} />
              </Button>
            </div>
            <CardDescription>
              {tr('history.description', 'The last 10 top-ups and code redemptions on this account.')}
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {history.length === 0 && !isHistoryLoading ? (
              <p className="px-6 pb-6 text-sm text-muted-foreground">
                {tr('history.empty', 'No top-ups yet. Add credits or redeem a code to get started.')}
              </p>
            ) : (
              <div className="border-t">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-xs uppercase tracking-wider text-muted-foreground">
                      <th className="text-left font-medium px-6 py-2.5">{tr('history.col.date', 'Date')}</th>
                      <th className="text-left font-medium px-6 py-2.5">{tr('history.col.source', 'Source')}</th>
                      <th className="text-right font-medium px-6 py-2.5">{tr('history.col.amount', 'Amount')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((entry) => (
                      <tr key={entry.id} className="border-t hover:bg-muted/30">
                        <td className="px-6 py-3 tabular-nums text-foreground/90 whitespace-nowrap">
                          {formatHistoryDate(entry.created_at)}
                        </td>
                        <td className="px-6 py-3 text-foreground/80">
                          <div className="flex items-center gap-2">
                            <span
                              className={cn(
                                'inline-block h-1.5 w-1.5 rounded-full',
                                entry.source === 'stripe' && 'bg-primary',
                                entry.source === 'code' && 'bg-accent',
                                entry.source === 'admin' && 'bg-warning',
                                entry.source === 'system' && 'bg-info',
                                entry.source === 'other' && 'bg-muted-foreground/40'
                              )}
                            />
                            <span>{sourceLabel(entry.source)}</span>
                          </div>
                        </td>
                        <td className="px-6 py-3 text-right tabular-nums text-foreground font-medium">
                          {formatUSD(entry.quota)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>

        {/* ── External top-up (only if configured) ─────────────────── */}
        {topUpLink && (
          <Card>
            <CardContent className="flex items-center justify-between gap-4 p-4">
              <div className="min-w-0">
                <p className="text-sm font-medium text-foreground">
                  {tr('online.title', 'Online payment portal')}
                </p>
                <p className="text-xs text-muted-foreground truncate">
                  {tr('online.description', 'Purchase quota through our external payment system')}
                </p>
              </div>
              <Button variant="ghost" size="sm" onClick={openTopUpLink} className="flex-shrink-0">
                {tr('online.button', 'Open')}
                <ArrowUpRight className="h-4 w-4 ml-1" />
              </Button>
            </CardContent>
          </Card>
        )}

        {/* ── Notes footer (business-aligned) ──────────────────────── */}
        <div className="pt-2">
          <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-3">
            {tr('notes.title', 'Good to know')}
          </p>
          <ul className="space-y-2 text-sm text-muted-foreground">
            <li className="flex items-start gap-2">
              <Mail className="h-4 w-4 mt-0.5 text-muted-foreground/70 flex-shrink-0" />
              <span>
                {userEmail
                  ? tr(
                      'notes.receipt_with_email',
                      'After each card payment, Stripe emails an itemized receipt to {{email}}.',
                      { email: userEmail }
                    )
                  : tr(
                      'notes.receipt_no_email',
                      'After each card payment, Stripe emails a receipt. Add an email to your account to receive it automatically.'
                    )}
              </span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-muted-foreground/50 select-none mt-1">•</span>
              <span>
                {tr(
                  'notes.expiry',
                  'Credits never expire. Card payments are USD only with a $20 minimum; redemption codes have no minimum.'
                )}
              </span>
            </li>
            <li className="flex items-start gap-2">
              <span className="text-muted-foreground/50 select-none mt-1">•</span>
              <span>
                {tr(
                  'notes.refund',
                  'Refund requests are handled case-by-case. Contact support with the receipt from Stripe.'
                )}
              </span>
            </li>
          </ul>
        </div>
      </div>
    </ResponsivePageContainer>
  );
}

export default TopUpPage;
