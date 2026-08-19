import { Badge } from '@/components/ui/badge';
import { FormField, FormItem, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import type { UseFormReturn } from 'react-hook-form';
import type { ChannelForm } from '../schemas';
import { LabelWithHelp } from './LabelWithHelp';

interface ChannelGroupsProps {
  form: UseFormReturn<ChannelForm>;
  groups: string[];
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

// ChannelGroups renders the user-group selector for a channel. It is placed
// directly above the Supported Models section so access scope reads before the
// model catalog.
export const ChannelGroups = ({ form, groups, tr }: ChannelGroupsProps) => {
  const toggleGroup = (groupValue: string) => {
    const currentGroups = form.getValues('groups');
    if (currentGroups.includes(groupValue)) {
      form.setValue(
        'groups',
        currentGroups.filter((g) => g !== groupValue)
      );
    } else {
      form.setValue('groups', [...currentGroups, groupValue]);
    }
  };

  const addGroup = (groupName: string) => {
    const currentGroups = form.getValues('groups');
    if (!currentGroups.includes(groupName)) {
      form.setValue('groups', [...currentGroups, groupName]);
    }
  };

  const removeGroup = (groupToRemove: string) => {
    const currentGroups = form.getValues('groups');
    const newGroups = currentGroups.filter((g) => g !== groupToRemove);
    // Ensure at least 'default' group remains
    if (newGroups.length === 0) {
      newGroups.push('default');
    }
    form.setValue('groups', newGroups);
  };

  return (
    <FormField
      control={form.control}
      name="groups"
      render={() => (
        <FormItem>
          <LabelWithHelp
            label={tr('groups.label', 'Groups *')}
            help={tr('groups.help', 'User groups that can access this channel. "default" is standard for normal users.')}
          />
          <div className="flex flex-wrap gap-2 mb-2">
            {groups.map((group) => {
              const isSelected = form.watch('groups').includes(group);
              return (
                <Badge
                  key={group}
                  variant={isSelected ? 'default' : 'outline'}
                  className="cursor-pointer hover:bg-primary/90"
                  onClick={() => toggleGroup(group)}
                >
                  {group}
                </Badge>
              );
            })}
          </div>
          <div className="flex gap-2">
            <Input
              placeholder={tr('groups.add_placeholder', 'Add custom group...')}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                if (e.key === 'Enter') {
                  e.preventDefault();
                  const val = (e.target as HTMLInputElement).value.trim();
                  if (val) {
                    addGroup(val);
                    (e.target as HTMLInputElement).value = '';
                  }
                }
              }}
            />
          </div>
          <div className="flex flex-wrap gap-2 mt-2">
            {form.watch('groups').map((group) => (
              <Badge key={group} variant="secondary" className="gap-1 max-w-full">
                <span className="truncate min-w-0" title={group}>
                  {group}
                </span>
                <span className="cursor-pointer ml-1 hover:text-destructive shrink-0" onClick={() => removeGroup(group)}>
                  ×
                </span>
              </Badge>
            ))}
          </div>
          <FormMessage />
        </FormItem>
      )}
    />
  );
};
