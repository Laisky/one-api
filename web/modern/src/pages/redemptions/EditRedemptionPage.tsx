import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { api } from '@/lib/api';
import { zodResolver } from '@hookform/resolvers/zod';
import { Check, Copy, Download } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import * as z from 'zod';

// Helper function to render quota with USD conversion (USD only)
const renderQuotaWithPrompt = (quota: number): string => {
  const quotaPerUnitRaw = localStorage.getItem('quota_per_unit');
  const quotaPerUnit = parseFloat(quotaPerUnitRaw || '500000');
  const usd = Number.isFinite(quota) && quotaPerUnit > 0 ? quota / quotaPerUnit : NaN;
  const usdValue = Number.isFinite(usd) ? usd.toFixed(2) : '0.00';
  return `$${usdValue}`;
};

// sanitizeFilenameSegment keeps only filesystem-safe characters so the download
// filename remains predictable across platforms.
const sanitizeFilenameSegment = (raw: string): string => {
  const trimmed = (raw || '')
    .trim()
    .replace(/[^a-zA-Z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return trimmed || 'codes';
};

// triggerTextDownload writes the given text content to a .txt file using the
// standard Blob + temporary anchor pattern. Mirrors Berry's downloadTextAsFile
// behavior without introducing a new dependency.
const triggerTextDownload = (content: string, filename: string): void => {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
};

// copyTextToClipboard copies a string to the clipboard, falling back to a
// hidden textarea when the async clipboard API is unavailable.
const copyTextToClipboard = async (text: string): Promise<boolean> => {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-999999px';
    textArea.style.top = '-999999px';
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textArea);
    return ok;
  } catch (err) {
    console.error('Failed to copy redemption codes:', err);
    return false;
  }
};

export function EditRedemptionPage() {
  const params = useParams();
  const redemptionId = params.id;
  const isEdit = redemptionId !== undefined;
  const navigate = useNavigate();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`redemptions.edit.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);
  // generatedCodes holds the freshly-created redemption keys returned by the
  // server. When non-null we render the post-success view instead of the form.
  const [generatedCodes, setGeneratedCodes] = useState<string[] | null>(null);
  // generatedName is captured at submit time so the download filename and
  // success message stay consistent even if the operator edits the form later.
  const [generatedName, setGeneratedName] = useState<string>('');
  const [copied, setCopied] = useState(false);

  const redemptionSchema = z.object({
    name: z.string().min(1, tr('validation.name_required', 'Name is required')).max(20, tr('validation.name_max', 'Max 20 chars')),
    // Coerce numeric fields so typing works and validation runs
    quota: z.coerce.number().int().min(0, tr('validation.quota_min', 'Quota cannot be negative')),
    count: z.coerce
      .number()
      .int()
      .min(1, tr('validation.count_min', 'Count must be positive'))
      .max(100, tr('validation.count_max', 'Count cannot exceed 100'))
      .default(1),
  });

  type RedemptionForm = z.infer<typeof redemptionSchema>;

  const form = useForm<RedemptionForm>({
    resolver: zodResolver(redemptionSchema),
    defaultValues: {
      name: '',
      quota: 0,
      count: 1,
    },
  });

  const watchQuota = form.watch('quota');

  const loadRedemption = async () => {
    if (!redemptionId) return;

    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get(`/api/redemption/${redemptionId}`);
      const { success, message, data } = response.data;

      if (success && data) {
        form.reset(data);
      } else {
        throw new Error(message || 'Failed to load redemption');
      }
    } catch (error) {
      console.error('Error loading redemption:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadRedemption();
    } else {
      setLoading(false);
    }
  }, [isEdit, redemptionId]);

  const onSubmit = async (data: RedemptionForm) => {
    setIsSubmitting(true);
    try {
      let response;
      if (isEdit && redemptionId) {
        // Unified API call - complete URL with /api prefix
        response = await api.put('/api/redemption/', {
          ...data,
          id: parseInt(redemptionId),
        });
      } else {
        response = await api.post('/api/redemption/', data);
      }

      const { success, message, data: payload } = response.data;
      if (success) {
        if (!isEdit) {
          // POST /api/redemption returns an array of generated key strings.
          // Surface them here so the operator can download or copy the batch
          // before navigating away — matches the Berry UX.
          const keys = Array.isArray(payload) ? (payload as string[]).filter((k) => typeof k === 'string') : [];
          if (keys.length > 0) {
            setGeneratedCodes(keys);
            setGeneratedName(data.name);
            return;
          }
        }
        navigate('/redemptions', {
          state: {
            message: isEdit
              ? tr('notifications.update_success', 'Redemption updated successfully')
              : tr('notifications.create_success', 'Redemption created successfully'),
          },
        });
      } else {
        form.setError('root', {
          message: message || tr('notifications.failed', 'Operation failed'),
        });
      }
    } catch (error) {
      form.setError('root', {
        message: error instanceof Error ? error.message : tr('notifications.failed', 'Operation failed'),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  // handleDownloadCodes saves the generated codes as a plain-text file. One
  // code per line keeps the file easy to feed back into batch-redeem tooling.
  const handleDownloadCodes = useCallback(() => {
    if (!generatedCodes || generatedCodes.length === 0) return;
    const prefix = tr('download_filename_prefix', 'redemption-codes');
    const stamp = new Date().toISOString().replace(/[:.]/g, '-');
    const filename = `${sanitizeFilenameSegment(prefix)}-${sanitizeFilenameSegment(generatedName)}-${stamp}.txt`;
    triggerTextDownload(generatedCodes.join('\n'), filename);
  }, [generatedCodes, generatedName, tr]);

  // handleCopyCodes copies all generated codes (newline-separated) to the
  // clipboard and briefly flips the button label to "Copied".
  const handleCopyCodes = useCallback(async () => {
    if (!generatedCodes || generatedCodes.length === 0) return;
    const ok = await copyTextToClipboard(generatedCodes.join('\n'));
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [generatedCodes]);

  if (loading) {
    return (
      <ResponsivePageContainer
        title={isEdit ? tr('title.edit', 'Edit Redemption') : tr('title.create', 'Create Redemption')}
        description={
          isEdit ? tr('description.edit', 'Update redemption code settings') : tr('description.create', 'Create a new redemption code')
        }
      >
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr('loading', 'Loading redemption...')}</span>
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  // Post-success view: render the freshly-generated codes alongside download
  // and copy CTAs. Single-code generation skips the download button (one code
  // does not need a file) but still exposes a Copy button for convenience.
  if (generatedCodes && generatedCodes.length > 0) {
    const isBatch = generatedCodes.length > 1;
    return (
      <ResponsivePageContainer
        title={tr('title.create', 'Create Redemption')}
        description={tr('description.create', 'Create a new redemption code')}
      >
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="p-4 sm:p-6 space-y-4">
            <div>
              <div className="text-base font-medium">
                {tr('codes_generated', '{{count}} codes generated', { count: generatedCodes.length })}
              </div>
              <CardDescription className="mt-1">
                {isBatch
                  ? tr('codes_generated_hint', 'Save the codes now — once you leave this page they cannot be retrieved as a batch.')
                  : tr('code_generated_hint', 'Copy the code below; it will not be shown together with its name again.')}
              </CardDescription>
            </div>

            <textarea
              className="w-full h-40 p-2 border rounded font-mono text-sm bg-muted/30"
              readOnly
              value={generatedCodes.join('\n')}
              aria-label={tr('codes_generated_aria', 'Generated redemption codes')}
            />

            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button type="button" variant="outline" onClick={() => navigate('/redemptions')} className="w-full sm:w-auto">
                {tr('actions.back_to_list', 'Back to list')}
              </Button>
              <Button type="button" variant="outline" onClick={handleCopyCodes} className="w-full sm:w-auto">
                {copied ? (
                  <>
                    <Check className="mr-2 h-4 w-4" />
                    {tr('actions.copied', 'Copied')}
                  </>
                ) : (
                  <>
                    <Copy className="mr-2 h-4 w-4" />
                    {isBatch ? tr('actions.copy_all', 'Copy all') : tr('actions.copy', 'Copy')}
                  </>
                )}
              </Button>
              {isBatch && (
                <Button type="button" onClick={handleDownloadCodes} className="w-full sm:w-auto">
                  <Download className="mr-2 h-4 w-4" />
                  {tr('actions.download_codes', 'Download codes')}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  return (
    <ResponsivePageContainer
      title={isEdit ? tr('title.edit', 'Edit Redemption') : tr('title.create', 'Create Redemption')}
      description={
        isEdit ? tr('description.edit', 'Update redemption code settings') : tr('description.create', 'Create a new redemption code')
      }
    >
      <Card className="border-0 shadow-none md:border md:shadow-sm">
        <CardContent className="p-4 sm:p-6">
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('redemptions.fields.name.label', 'Name')}</FormLabel>
                    <FormControl>
                      <Input placeholder={t('redemptions.fields.name.placeholder', 'Enter redemption name')} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 md:gap-6">
                <FormField
                  control={form.control}
                  name="quota"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {(() => {
                          const current = (watchQuota ?? field.value ?? 0) as any;
                          const numeric = Number(current);
                          const usdLabel = Number.isFinite(numeric) && numeric >= 0 ? renderQuotaWithPrompt(numeric) : '$0.00';
                          return `${t('redemptions.fields.quota.label', 'Quota')} (${usdLabel})`;
                        })()}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min="0"
                          step="1"
                          {...field}
                          onChange={(e) => {
                            // Pass original event to RHF to keep name & target intact
                            field.onChange(e);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="count"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('redemptions.fields.count.label', 'Count')}</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min="1"
                          step="1"
                          {...field}
                          onChange={(e) => {
                            // Pass original event for consistency with RHF expectations
                            field.onChange(e);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {form.formState.errors.root && <div className="text-sm text-destructive">{form.formState.errors.root.message}</div>}

              <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                <Button type="button" variant="outline" onClick={() => navigate('/redemptions')} className="w-full sm:w-auto">
                  {t('redemptions.actions.cancel', 'Cancel')}
                </Button>
                <Button type="submit" disabled={isSubmitting} className="w-full sm:w-auto">
                  {isSubmitting
                    ? isEdit
                      ? t('redemptions.actions.updating', 'Updating...')
                      : t('redemptions.actions.creating', 'Creating...')
                    : isEdit
                      ? t('redemptions.actions.update', 'Update Redemption')
                      : t('redemptions.actions.create', 'Create Redemption')}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </ResponsivePageContainer>
  );
}

export default EditRedemptionPage;
