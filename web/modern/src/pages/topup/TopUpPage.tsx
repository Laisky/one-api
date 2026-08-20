import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { api } from '@/lib/api';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/lib/stores/auth';
import { zodResolver } from '@/lib/zod-resolver';
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
import { useSearchParams } from 'react-router-dom';
import * as z from 'zod';

const QUOTA_PER_UNIT_FALLBACK = 500000;
const DEFAULT_MIN_TOPUP_USD = 5;
const BASE_PRESETS = [5, 10, 20, 50, 100] as const;

/** readQuotaPerUnit returns the tokens-per-USD unit from localStorage status, with a safe fallback. */
function readQuotaPerUnit(): number {
  const raw = parseFloat(localStorage.getItem('quota_per_unit') || `${QUOTA_PER_UNIT_FALLBACK}`);
  return Number.isFinite(raw) && raw > 0 ? raw : QUOTA_PER_UNIT_FALLBACK;
}

/** errorMessage returns a string-only description of an unknown catch value for safe console logging. */
function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message || error.name || 'Error';
  }
  return String(error);
}

/**
 * TopUpPage renders balance, optional Stripe Checkout top-up, redemption codes, and recent top-up history.
 * It has no props; user identity comes from the auth store and system status from localStorage /api/status.
 */
export function TopUpPage() {
  const { user, updateUser } = useAuthStore();
  const [searchParams] = useSearchParams();
  const { t, i18n } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`topup.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [userQuota, setUserQuota] = useState(user?.quota ?? 0);
  const [userData, setUserData] = useState<any>(null);
  const [topUpLink, setTopUpLink] = useState('');
  const [stripeEnabled, setStripeEnabled] = useState(false);
  const [minTopUpUSD, setMinTopUpUSD] = useState(DEFAULT_MIN_TOPUP_USD);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [fulfillmentStatus, setFulfillmentStatus] = useState<'processing' | 'paid' | 'failed' | null>(null);

  type HistoryEntry = {
    id: number;
    created_at: number;
    amount_cents: number;
    currency: string;
    quota: number;
    status: string;
    source: 'stripe' | 'code' | 'admin' | 'system' | 'other';
  };
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [isHistoryLoading, setIsHistoryLoading] = useState(false);

  const stripeOutcome = useMemo<'success' | 'cancel' | null>(() => {
    const flag = (searchParams.get('stripe') || '').toLowerCase();
    if (flag === 'success') return 'success';
    if (flag === 'cancel') return 'cancel';
    return null;
  }, [searchParams]);
  const returnSessionId = searchParams.get('session_id') || '';

  const presets = useMemo(
    () => BASE_PRESETS.filter((n) => n >= minTopUpUSD),
    [minTopUpUSD]
  );

  const stripeSchema = useMemo(
    () =>
      z.object({
        amount_usd: z.coerce
          .number({ error: tr('stripe.required', 'Enter an amount in USD') })
          .min(minTopUpUSD, tr('stripe.min', `Minimum is $${minTopUpUSD}`, { value: minTopUpUSD }))
          .max(100000, tr('stripe.max', 'Amount too large')),
      }),
    [minTopUpUSD, tr]
  );
  type StripeForm = z.infer<typeof stripeSchema>;
  const stripeForm = useForm<StripeForm>({
    resolver: zodResolver(stripeSchema),
    defaultValues: { amount_usd: minTopUpUSD },
  });
  const [isStripeSubmitting, setIsStripeSubmitting] = useState(false);

  useEffect(() => {
    stripeForm.setValue('amount_usd', minTopUpUSD, { shouldValidate: false });
  }, [minTopUpUSD, stripeForm]);

  const codeSchema = z.object({
    redemption_code: z.string().min(1, tr('redeem.required', 'Redemption code is required')),
  });
  type CodeForm = z.infer<typeof codeSchema>;
  const codeForm = useForm<CodeForm>({
    resolver: zodResolver(codeSchema),
    defaultValues: { redemption_code: '' },
  });
  const [isCodeSubmitting, setIsCodeSubmitting] = useState(false);

  /** formatUSD formats a token quota as a USD currency string in the active locale. */
  const formatUSD = useCallback(
    (quota: number): string => {
      const usd = quota / readQuotaPerUnit();
      return usd.toLocaleString(i18n.language || 'en', {
        style: 'currency',
        currency: 'USD',
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });
    },
    [i18n.language]
  );

  /** formatCents formats an immutable Stripe amount in cents as currency. */
  const formatCents = useCallback(
    (cents: number, currency = 'usd'): string => {
      return (cents / 100).toLocaleString(i18n.language || 'en', {
        style: 'currency',
        currency: (currency || 'usd').toUpperCase(),
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      });
    },
    [i18n.language]
  );

  /** formatHistoryDate formats a unix-second timestamp for the history list. */
  const formatHistoryDate = useCallback(
    (ts: number) =>
      new Date(ts * 1000).toLocaleString(i18n.language || 'en', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language]
  );

  /** loadUserData refreshes the signed-in user profile and quota from the API. */
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
      console.error(`Error loading user data: ${errorMessage(error)}`);
    } finally {
      setIsRefreshing(false);
    }
  };

  /** loadSystemStatus reads cached /api/status fields for top-up link and Stripe capability. */
  const loadSystemStatus = async () => {
    const apply = (parsed: Record<string, unknown>) => {
      if (typeof parsed.top_up_link === 'string') setTopUpLink(parsed.top_up_link);
      if (typeof parsed.quota_per_unit === 'number' && parsed.quota_per_unit > 0) {
        localStorage.setItem('quota_per_unit', String(parsed.quota_per_unit));
      }
      setStripeEnabled(Boolean(parsed.stripe_enabled));
      const min = Number(parsed.min_topup_usd);
      if (Number.isFinite(min) && min >= 1) {
        setMinTopUpUSD(Math.floor(min));
      }
    };
    try {
      const cached = localStorage.getItem('status');
      if (cached) apply(JSON.parse(cached));
    } catch (error) {
      console.error(`Error parsing cached system status: ${errorMessage(error)}`);
    }
    try {
      const res = await api.get('/api/status');
      const data = res.data?.data;
      if (data && typeof data === 'object') {
        localStorage.setItem('status', JSON.stringify(data));
        apply(data as Record<string, unknown>);
      }
    } catch (error) {
      console.error(`Error loading system status: ${errorMessage(error)}`);
    }
  };

  /** loadHistory loads immutable Stripe payment orders for the signed-in user. */
  const loadHistory = async () => {
    setIsHistoryLoading(true);
    try {
      const res = await api.get('/api/user/topup/stripe/orders');
      const { success, data } = res.data || {};
      const rows: any[] = Array.isArray(data) ? data : [];
      if (success || rows.length) {
        setHistory(
          rows.map((r) => ({
            id: r.id,
            created_at: Math.floor((r.created_at || 0) / 1000) || r.created_at,
            amount_cents: r.amount_cents ?? 0,
            currency: r.currency || 'usd',
            quota: r.quota ?? 0,
            status: r.status || 'pending',
            source: 'stripe' as const,
          }))
        );
      }
    } catch (error) {
      console.error(`Error loading topup history: ${errorMessage(error)}`);
    } finally {
      setIsHistoryLoading(false);
    }
  };

  /** pollReturnSession asks the server for fulfillment status after Stripe redirect. */
  const pollReturnSession = useCallback(async () => {
    if (!returnSessionId || stripeOutcome !== 'success') return;
    setFulfillmentStatus('processing');
    try {
      for (let i = 0; i < 8; i++) {
        const res = await api.get(`/api/user/topup/stripe/orders/${encodeURIComponent(returnSessionId)}`);
        const order = res.data?.data;
        if (res.data?.success && order?.status === 'paid') {
          setFulfillmentStatus('paid');
          loadUserData();
          loadHistory();
          return;
        }
        if (order?.status === 'manual_review' || order?.status === 'failed') {
          setFulfillmentStatus('failed');
          return;
        }
        await new Promise((r) => setTimeout(r, 1500));
      }
      setFulfillmentStatus('processing');
    } catch (error) {
      console.error(`Error polling payment order: ${errorMessage(error)}`);
      setFulfillmentStatus('processing');
    }
  }, [returnSessionId, stripeOutcome]);

  useEffect(() => {
    loadUserData();
    loadSystemStatus();
    loadHistory();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    pollReturnSession();
  }, [pollReturnSession]);

  /** onStripeSubmit creates a Checkout Session and redirects the browser to Stripe. */
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
        message: errorMessage(error) || tr('stripe.failed', 'Failed to create checkout session'),
      });
    } finally {
      setIsStripeSubmitting(false);
    }
  };

  /** onCodeSubmit redeems a top-up code and refreshes balance/history. */
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
            value: (added || 0).toLocaleString(i18n.language || 'en'),
          }),
        });
        loadUserData();
        loadHistory();
      } else {
        codeForm.setError('root', { message: message || tr('redeem.failed', 'Redemption failed') });
      }
    } catch (error) {
      codeForm.setError('root', {
        message: errorMessage(error) || tr('redeem.failed', 'Redemption failed'),
      });
    } finally {
      setIsCodeSubmitting(false);
    }
  };

  /** openTopUpLink opens the configured external top-up portal in a new tab. */
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
      console.error(`Error opening top-up link: ${errorMessage(error)}`);
    }
  };

  const balanceUSD = formatUSD(userQuota);
  const tokensLabel = `${userQuota.toLocaleString(i18n.language || 'en')} ${tr('tokens', 'tokens')}`;
  const watchedAmount = stripeForm.watch('amount_usd');
  const userEmail = (userData?.email as string | undefined) || (user as any)?.email || '';

  /** setPreset writes a preset amount into the Stripe amount field. */
  const setPreset = (value: number) => {
    stripeForm.setValue('amount_usd', value, { shouldValidate: true, shouldDirty: true });
  };

  /** sourceLabel returns a translated label for a history source category. */
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

  /** statusLabel returns a translated label for a payment order status. */
  const statusLabel = (s: string): string => {
    switch (s) {
      case 'paid':
        return tr('history.status.paid', 'Paid');
      case 'pending':
        return tr('history.status.pending', 'Pending');
      case 'manual_review':
        return tr('history.status.manual_review', 'Under review');
      case 'failed':
        return tr('history.status.failed', 'Failed');
      case 'canceled':
        return tr('history.status.canceled', 'Canceled');
      default:
        return s;
    }
  };

  return (
    <ResponsivePageContainer
      title={tr('title', 'Billing')}
      description={tr('description', 'Manage your balance and add credits')}
      className="max-w-4xl"
    >
      <div className="space-y-6">
        {stripeOutcome === 'success' && (
          <div className="flex items-start gap-3 rounded-lg border border-success-border bg-success-muted px-4 py-3 text-sm">
            <CheckCircle2 className="h-4 w-4 mt-0.5 text-success flex-shrink-0" />
            <div className="text-success-foreground">
              <p className="font-medium mb-0.5">
                {fulfillmentStatus === 'paid'
                  ? tr('stripe.outcome_credited_title', 'Credits added')
                  : fulfillmentStatus === 'failed'
                    ? tr('stripe.outcome_failed_title', 'Payment needs review')
                    : tr('stripe.outcome_success_title', 'Payment received')}
              </p>
              <p className="text-success-foreground/80">
                {fulfillmentStatus === 'paid'
                  ? tr('stripe.outcome_credited', 'Your balance has been updated from the server order status.')
                  : fulfillmentStatus === 'failed'
                    ? tr('stripe.outcome_failed', 'Payment was recorded but needs operator review. Contact support if balance is wrong.')
                    : tr(
                        'stripe.outcome_success',
                        'Confirming with the server… your balance updates after the webhook settles the order.'
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

        <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
          <Card className="lg:col-span-2">
            <CardContent className="p-6 h-full flex flex-col">
              <div className="flex items-start justify-between gap-2">
                <p className="text-sm text-muted-foreground">{tr('balance.title', 'Current balance')}</p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={loadUserData}
                  disabled={isRefreshing}
                  data-label={tr('balance.refresh', 'Refresh balance')}
                  aria-label={tr('balance.refresh', 'Refresh balance')}
                  className="-mr-2 -mt-2 min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"
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

          {stripeEnabled && (
            <Card className="lg:col-span-3">
              <CardHeader>
                <div className="flex items-center gap-2">
                  <CreditCard className="h-4 w-4 text-muted-foreground" />
                  <CardTitle>{tr('stripe.title', 'Add credits')}</CardTitle>
                </div>
                <CardDescription>
                  {tr(
                    'stripe.description',
                    'Pay by card via Stripe. USD only, ${{min}} minimum. A receipt is emailed to your registered address.',
                    { min: minTopUpUSD }
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Form {...stripeForm}>
                  <form onSubmit={stripeForm.handleSubmit(onStripeSubmit)} className="space-y-5">
                    <div className="flex flex-wrap gap-2">
                      {presets.map((amount) => {
                        const active = Number(watchedAmount) === amount;
                        return (
                          <button
                            type="button"
                            key={amount}
                            onClick={() => setPreset(amount)}
                            data-label={`$${amount}`}
                            aria-pressed={active}
                            className={cn(
                              'inline-flex min-h-11 items-center justify-center rounded-md border px-3 py-2 text-sm font-medium tabular-nums transition-colors',
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
                                min={minTopUpUSD}
                                step="0.01"
                                placeholder={String(minTopUpUSD)}
                                className="min-h-11 pl-7 tabular-nums"
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
                      <Button type="submit" disabled={isStripeSubmitting} className="min-h-11 sm:w-auto">
                        {isStripeSubmitting
                          ? tr('stripe.processing', 'Redirecting…')
                          : tr('stripe.button', 'Continue to Stripe')}
                      </Button>
                    </div>
                  </form>
                </Form>
              </CardContent>
            </Card>
          )}
        </div>

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
                          className="min-h-11"
                          placeholder={tr('redeem.placeholder', 'Enter your redemption code')}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button type="submit" variant="outline" disabled={isCodeSubmitting} className="min-h-11 sm:w-auto">
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
                data-label={tr('history.refresh', 'Refresh history')}
                aria-label={tr('history.refresh', 'Refresh history')}
                className="min-h-11 min-w-11 h-11 text-muted-foreground hover:text-foreground"
              >
                <RefreshCw className={cn('h-4 w-4', isHistoryLoading && 'animate-spin')} />
              </Button>
            </div>
            <CardDescription>
              {tr('history.description', 'Recent card top-ups with original charged amounts (immutable cents).')}
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {history.length === 0 && !isHistoryLoading ? (
              <p className="px-6 pb-6 text-sm text-muted-foreground">
                {tr('history.empty', 'No card top-ups yet. Add credits to get started.')}
              </p>
            ) : (
              <>
                {/* Mobile cards */}
                <div className="border-t md:hidden divide-y">
                  {history.map((entry) => (
                    <div key={entry.id} className="px-6 py-3 space-y-1 text-sm">
                      <div className="flex justify-between gap-2">
                        <span className="text-muted-foreground" data-label={tr('history.col.date', 'Date')}>
                          {formatHistoryDate(entry.created_at)}
                        </span>
                        <span className="font-medium tabular-nums" data-label={tr('history.col.amount', 'Amount')}>
                          {formatCents(entry.amount_cents, entry.currency)}
                        </span>
                      </div>
                      <div className="text-foreground/80" data-label={tr('history.col.source', 'Source')}>
                        {sourceLabel(entry.source)} · {statusLabel(entry.status)}
                      </div>
                    </div>
                  ))}
                </div>
                {/* Desktop table */}
                <div className="border-t hidden md:block">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-xs uppercase tracking-wider text-muted-foreground">
                        <th className="text-left font-medium px-6 py-2.5">{tr('history.col.date', 'Date')}</th>
                        <th className="text-left font-medium px-6 py-2.5">{tr('history.col.source', 'Source')}</th>
                        <th className="text-left font-medium px-6 py-2.5">{tr('history.col.status', 'Status')}</th>
                        <th className="text-right font-medium px-6 py-2.5">{tr('history.col.amount', 'Amount')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {history.map((entry) => (
                        <tr key={entry.id} className="border-t hover:bg-muted/30">
                          <td className="px-6 py-3 tabular-nums text-foreground/90 whitespace-nowrap" data-label={tr('history.col.date', 'Date')}>
                            {formatHistoryDate(entry.created_at)}
                          </td>
                          <td className="px-6 py-3 text-foreground/80" data-label={tr('history.col.source', 'Source')}>
                            <div className="flex items-center gap-2">
                              <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary" />
                              <span>{sourceLabel(entry.source)}</span>
                            </div>
                          </td>
                          <td className="px-6 py-3 text-foreground/80" data-label={tr('history.col.status', 'Status')}>
                            {statusLabel(entry.status)}
                          </td>
                          <td className="px-6 py-3 text-right tabular-nums text-foreground font-medium" data-label={tr('history.col.amount', 'Amount')}>
                            {formatCents(entry.amount_cents, entry.currency)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            )}
          </CardContent>
        </Card>

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
              <Button
                variant="ghost"
                size="sm"
                onClick={openTopUpLink}
                className="min-h-11 flex-shrink-0"
                aria-label={tr('online.button', 'Open')}
              >
                {tr('online.button', 'Open')}
                <ArrowUpRight className="h-4 w-4 ml-1" />
              </Button>
            </CardContent>
          </Card>
        )}

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
                  'Credits never expire. Card payments are USD only with a ${{min}} minimum; redemption codes have no minimum.',
                  { min: minTopUpUSD }
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
