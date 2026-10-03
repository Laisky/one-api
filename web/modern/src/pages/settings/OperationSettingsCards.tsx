import { AlertCircle, Info } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { TFunction } from 'i18next';
import type { UseFormReturn } from 'react-hook-form';
import type { GroupRatioIssue, OperationForm, OperationFormInput } from './OperationSettings';

type OperationQuotaCardProps = {
  t: TFunction;
  form: UseFormReturn<OperationFormInput, unknown, OperationForm>;
  descriptions: Record<string, string>;
  onSave: () => void;
};

/**
 * OperationQuotaCard renders the editable signup quota options and save action.
 * It receives the shared operation form, translated descriptions, and save callback from OperationSettings.
 */
export function OperationQuotaCard({ t, form, descriptions, onSave }: OperationQuotaCardProps) {
  const quotaFields = [
    ['QuotaForNewUser', 'operation_settings.quota.quota_for_new_user'],
    ['QuotaForInviter', 'operation_settings.quota.quota_for_inviter'],
    ['QuotaForInvitee', 'operation_settings.quota.quota_for_invitee'],
    ['PreConsumedQuota', 'operation_settings.quota.pre_consumed_quota'],
  ] as const;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('operation_settings.quota.title')}</CardTitle>
        <CardDescription>{t('operation_settings.quota.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {quotaFields.map(([name, label]) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="flex items-center gap-2">
                      {t(label)}
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                            <Info className="h-4 w-4" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent side="top" align="start" className="max-w-[320px]">
                          {descriptions[name]}
                        </TooltipContent>
                      </Tooltip>
                    </FormLabel>
                    <FormControl>
                      <Input type="number" {...field} onChange={(event) => field.onChange(Number(event.target.value))} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
          <div className="mt-4">
            <Button onClick={onSave}>{t('operation_settings.quota.save')}</Button>
          </div>
        </Form>
      </CardContent>
    </Card>
  );
}

type OperationAdministrationCardsProps = {
  t: TFunction;
  groupRatioText: string;
  groupRatioIssue: GroupRatioIssue | null;
  groupRatioDirty: boolean;
  savingGroupRatio: boolean;
  onGroupRatioTextChange: (value: string) => void;
  onFormatGroupRatio: () => void;
  onSaveGroupRatio: () => void;
  historyTimestamp: string;
  onHistoryTimestampChange: (value: string) => void;
  onDeleteHistoryLogs: () => void;
};

/**
 * OperationAdministrationCards renders group-ratio validation and historical-log deletion controls.
 * It receives the current values, validation result, and mutation callbacks from OperationSettings.
 */
export function OperationAdministrationCards({
  t,
  groupRatioText,
  groupRatioIssue,
  groupRatioDirty,
  savingGroupRatio,
  onGroupRatioTextChange,
  onFormatGroupRatio,
  onSaveGroupRatio,
  historyTimestamp,
  onHistoryTimestampChange,
  onDeleteHistoryLogs,
}: OperationAdministrationCardsProps) {
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t('operation_settings.group_ratio.title')}</CardTitle>
          <CardDescription>{t('operation_settings.group_ratio.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <label htmlFor="group-ratio-textarea" className="text-sm font-medium flex items-center gap-2">
                {t('operation_settings.group_ratio.label')}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                      <Info className="h-4 w-4" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="top" align="start" className="max-w-[320px]">
                    {t('operation_settings.group_ratio.help')}
                  </TooltipContent>
                </Tooltip>
              </label>
              <Button type="button" variant="ghost" size="sm" className="h-6 text-xs self-start sm:self-auto" onClick={onFormatGroupRatio}>
                {t('operation_settings.group_ratio.format')}
              </Button>
            </div>
            <Textarea
              id="group-ratio-textarea"
              value={groupRatioText}
              onChange={(event) => onGroupRatioTextChange(event.target.value)}
              placeholder={'{\n  "default": 1,\n  "vip": 0.8,\n  "svip": 0.5\n}'}
              className={`font-mono text-xs min-h-[180px] ${groupRatioIssue ? 'border-destructive focus-visible:ring-destructive' : ''}`}
              spellCheck={false}
            />
            {groupRatioIssue && (
              <div className="flex items-start gap-2 rounded-lg border border-destructive bg-destructive/10 p-3">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                <span className="text-sm text-destructive">
                  {groupRatioIssue.type === 'parse'
                    ? t('operation_settings.group_ratio.invalid_json', { message: groupRatioIssue.message })
                    : groupRatioIssue.type === 'shape'
                      ? t('operation_settings.group_ratio.shape_invalid')
                      : t('operation_settings.group_ratio.invalid_entries', { keys: groupRatioIssue.entries.join(', ') })}
                </span>
              </div>
            )}
            <div className="flex items-center gap-2">
              <Button onClick={onSaveGroupRatio} disabled={savingGroupRatio || !!groupRatioIssue || !groupRatioDirty}>
                {savingGroupRatio ? t('operation_settings.group_ratio.saving') : t('operation_settings.group_ratio.save')}
              </Button>
              {groupRatioDirty && !groupRatioIssue && (
                <span className="text-xs text-muted-foreground">{t('operation_settings.group_ratio.unsaved')}</span>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('operation_settings.logs.title')}</CardTitle>
          <CardDescription>{t('operation_settings.logs.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center space-x-4">
            <Input
              type="date"
              value={historyTimestamp}
              onChange={(event) => onHistoryTimestampChange(event.target.value)}
              className="w-auto"
            />
            <Button variant="destructive" onClick={onDeleteHistoryLogs}>
              {t('operation_settings.logs.clear_button')}
            </Button>
          </div>
          <p className="text-sm text-muted-foreground mt-2">{t('operation_settings.logs.clear_warning')}</p>
        </CardContent>
      </Card>
    </>
  );
}
