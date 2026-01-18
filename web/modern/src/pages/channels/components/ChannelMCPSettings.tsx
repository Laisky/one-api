import { ToolListEditor } from '@/components/mcp/ToolListEditor';
import { FormField, FormItem, FormMessage } from '@/components/ui/form';
import type { UseFormReturn } from 'react-hook-form';
import type { ChannelForm } from '../schemas';

interface ChannelMCPSettingsProps {
  form: UseFormReturn<ChannelForm>;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

export function ChannelMCPSettings({ form, tr }: ChannelMCPSettingsProps) {
  return (
    <FormField
      control={form.control}
      name="config.mcp_tool_blacklist"
      render={({ field }) => (
        <FormItem>
          <ToolListEditor
            label={tr('mcp.blacklist_label', 'MCP tool blacklist')}
            description={tr(
              'mcp.blacklist_description',
              'Block MCP tools for this channel. Use server.tool or tool name.'
            )}
            value={Array.isArray(field.value) ? field.value : []}
            onChange={field.onChange}
            placeholder={tr('mcp.blacklist_placeholder', 'server.tool_name')}
            addLabel={tr('mcp.blacklist_add', 'Add')}
          />
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
