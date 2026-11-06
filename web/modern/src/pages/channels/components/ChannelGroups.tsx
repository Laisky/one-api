import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormField, FormItem, FormMessage } from '@/components/ui/form';
import { MultiSelectCombobox } from '@/components/ui/multi-select-combobox';
import { useGroupCatalog } from '@/hooks/useGroupCatalog';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ChannelFormMethods } from '../schemas';
import { LabelWithHelp } from './LabelWithHelp';

/** ChannelGroupsProps are the channel form and the `channels.edit` translator used by ChannelGroups. */
interface ChannelGroupsProps {
  form: ChannelFormMethods;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

// ChannelGroups renders the user-group selector for a channel. It is placed
// directly above the Supported Models section so access scope reads before the
// model catalog. The input autocompletes against the configured billing groups,
// which it loads itself through useGroupCatalog: it lists every group as soon
// as it is focused and filters while typing. Groups are created in Settings
// together with their ratios, so only groups in the loaded catalog can be
// added here; a stale group already on the channel stays removable. While the
// catalog loads or after it failed, nothing can be added, the selected groups
// stay visible and removable, and a failure shows an inline error with Retry.
// At least one group always remains selected.
export const ChannelGroups = ({ form, tr }: ChannelGroupsProps) => {
  const { t } = useTranslation();
  const { groups, loading, error, retry } = useGroupCatalog();
  const selectedGroups = form.watch('groups') ?? [];

  /** addGroup assigns groupName to the channel when it is in the loaded catalog and not yet selected. */
  const addGroup = (groupName: string) => {
    // Only configured groups may be assigned; an unknown name would have no billing ratio.
    if (!groups.includes(groupName)) {
      return;
    }
    const currentGroups = form.getValues('groups') ?? ['default'];
    if (!currentGroups.includes(groupName)) {
      form.setValue('groups', [...currentGroups, groupName], { shouldDirty: true });
    }
  };

  /** removeGroup unassigns groupToRemove, keeping 'default' when it was the last group. */
  const removeGroup = (groupToRemove: string) => {
    const currentGroups = form.getValues('groups') ?? ['default'];
    const newGroups = currentGroups.filter((g) => g !== groupToRemove);
    // A channel without any group is unreachable, so fall back to 'default'.
    if (newGroups.length === 0) {
      newGroups.push('default');
    }
    form.setValue('groups', newGroups, { shouldDirty: true });
  };

  const label = tr('groups.label', 'Groups *');
  const emptyLabel = loading
    ? tr('groups.loading', 'Loading groups...')
    : error
      ? tr('groups.load_failed_empty', 'Groups are unavailable because the list could not be loaded')
      : tr('groups.no_match', 'No matching groups');

  return (
    <FormField
      control={form.control}
      name="groups"
      render={() => (
        <FormItem>
          <LabelWithHelp
            label={label}
            help={tr(
              'groups.help',
              'User groups allowed to use this channel. Only groups configured in Settings → Operation → Groups & Ratios can be selected. Normal users belong to the "default" group. At least one group is required.'
            )}
          />
          <MultiSelectCombobox
            aria-label={label}
            options={groups}
            selected={selectedGroups}
            onAdd={addGroup}
            onRemove={removeGroup}
            placeholder={tr('groups.add_placeholder', 'Search or select groups...')}
            emptyLabel={emptyLabel}
          />
          {error && (
            <Alert variant="destructive" className="mt-2">
              <AlertDescription className="flex flex-wrap items-center gap-x-3 gap-y-2">
                <span>
                  {tr(
                    'groups.load_failed',
                    'The configured groups could not be loaded, so no group can be added right now. The groups already selected are kept.'
                  )}
                </span>
                <Button type="button" variant="outline" size="sm" onClick={retry}>
                  {t('common.retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <div className="flex flex-wrap gap-2 mt-2">
            {selectedGroups.map((group) => (
              <Badge key={group} variant="secondary" className="gap-1 max-w-full">
                <span className="truncate min-w-0" title={group}>
                  {group}
                </span>
                <button
                  type="button"
                  className="ml-1 shrink-0 rounded-sm hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  aria-label={tr('groups.remove', 'Remove group {{group}}', { group })}
                  onClick={() => removeGroup(group)}
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            ))}
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
};
