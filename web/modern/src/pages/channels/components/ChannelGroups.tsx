import { Badge } from '@/components/ui/badge';
import { FormField, FormItem, FormMessage } from '@/components/ui/form';
import { MultiSelectCombobox } from '@/components/ui/multi-select-combobox';
import { X } from 'lucide-react';
import type { ChannelFormMethods } from '../schemas';
import { LabelWithHelp } from './LabelWithHelp';

interface ChannelGroupsProps {
  form: ChannelFormMethods;
  groups: string[];
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

// ChannelGroups renders the user-group selector for a channel. It is placed
// directly above the Supported Models section so access scope reads before the
// model catalog. The input autocompletes against the configured groups: it
// lists every group as soon as it is focused, filters while typing, and still
// accepts custom group names. At least one group always remains selected.
export const ChannelGroups = ({ form, groups, tr }: ChannelGroupsProps) => {
  const selectedGroups = form.watch('groups') ?? [];

  const addGroup = (groupName: string) => {
    const currentGroups = form.getValues('groups') ?? ['default'];
    if (!currentGroups.includes(groupName)) {
      form.setValue('groups', [...currentGroups, groupName], { shouldDirty: true });
    }
  };

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
              'User groups allowed to use this channel. Normal users belong to the "default" group. At least one group is required.'
            )}
          />
          <MultiSelectCombobox
            aria-label={label}
            options={groups}
            selected={selectedGroups}
            onAdd={addGroup}
            onRemove={removeGroup}
            allowCustom
            placeholder={tr('groups.add_placeholder', 'Select a group or type a custom one...')}
            emptyLabel={tr('groups.no_match', 'No matching groups')}
            createLabel={(group) => tr('groups.create', 'Add custom group "{{group}}"', { group })}
          />
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
