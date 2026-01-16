import { ToolListEditor } from '@/components/mcp/ToolListEditor';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useNotifications } from '@/components/ui/notifications';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { api } from '@/lib/api';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import * as z from 'zod';

interface MCPTool {
  id: number;
  name: string;
  description?: string;
}

const serverSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional(),
  status: z.coerce.number().int().default(1),
  base_url: z.string().min(1, 'Base URL is required'),
  protocol: z.string().default('streamable_http'),
  auth_type: z.string().default('none'),
  api_key: z.string().optional(),
  headers: z.string().optional(),
  tool_whitelist: z.array(z.string()).default([]),
  tool_blacklist: z.array(z.string()).default([]),
  tool_pricing: z.string().optional(),
  auto_sync_enabled: z.boolean().default(true),
  auto_sync_interval_minutes: z.coerce.number().int().min(5).max(1440).default(60),
});

type ServerForm = z.infer<typeof serverSchema>;

export function EditMCPServerPage() {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const navigate = useNavigate();
  const params = useParams();
  const serverId = params.id;
  const isEdit = Boolean(serverId);
  const [loading, setLoading] = useState(isEdit);
  const [tools, setTools] = useState<MCPTool[]>([]);

  const form = useForm<ServerForm>({
    resolver: zodResolver(serverSchema),
    defaultValues: {
      name: '',
      description: '',
      status: 1,
      base_url: '',
      protocol: 'streamable_http',
      auth_type: 'none',
      api_key: '',
      headers: '',
      tool_whitelist: [],
      tool_blacklist: [],
      tool_pricing: '',
      auto_sync_enabled: true,
      auto_sync_interval_minutes: 60,
    },
  });

  const loadServer = async () => {
    if (!serverId) return;
    setLoading(true);
    try {
      const response = await api.get(`/api/mcp_servers/${serverId}`);
      const { success, data, message } = response.data;
      if (!success) {
        notify({
          type: 'error',
          title: t('mcp.edit.notifications.load_failed', 'Failed to load MCP server'),
          message: message || '',
        });
        return;
      }
      form.reset({
        name: data.name || '',
        description: data.description || '',
        status: data.status ?? 1,
        base_url: data.base_url || '',
        protocol: data.protocol || 'streamable_http',
        auth_type: data.auth_type || 'none',
        api_key: data.api_key || '',
        headers: data.headers ? JSON.stringify(data.headers, null, 2) : '',
        tool_whitelist: Array.isArray(data.tool_whitelist) ? data.tool_whitelist : [],
        tool_blacklist: Array.isArray(data.tool_blacklist) ? data.tool_blacklist : [],
        tool_pricing: data.tool_pricing ? JSON.stringify(data.tool_pricing, null, 2) : '',
        auto_sync_enabled: Boolean(data.auto_sync_enabled ?? true),
        auto_sync_interval_minutes: data.auto_sync_interval_minutes ?? 60,
      });
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.edit.notifications.load_failed', 'Failed to load MCP server'),
        message: error instanceof Error ? error.message : String(error),
      });
    } finally {
      setLoading(false);
    }
  };

  const loadTools = async () => {
    if (!serverId) return;
    try {
      const response = await api.get(`/api/mcp_servers/${serverId}/tools`);
      const { success, data } = response.data;
      if (success) {
        setTools(data || []);
      }
    } catch (error) {
      console.error('Failed to load MCP tools', error);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadServer();
      loadTools();
    }
  }, [serverId]);

  const parseJSON = (value?: string) => {
    if (!value || value.trim() === '') return undefined;
    return JSON.parse(value);
  };

  const onSubmit = async (values: ServerForm) => {
    try {
      let headers: Record<string, any> = {};
      let pricing: Record<string, any> = {};
      try {
        headers = parseJSON(values.headers) || {};
        pricing = parseJSON(values.tool_pricing) || {};
      } catch (error) {
        notify({
          type: 'error',
          title: t('mcp.edit.notifications.save_failed', 'Save failed'),
          message: error instanceof Error ? error.message : String(error),
        });
        return;
      }
      const payload: Record<string, any> = {
        name: values.name,
        description: values.description,
        status: values.status,
        base_url: values.base_url,
        protocol: values.protocol,
        auth_type: values.auth_type,
        api_key: values.api_key,
        headers,
        tool_whitelist: values.tool_whitelist,
        tool_blacklist: values.tool_blacklist,
        tool_pricing: pricing,
        auto_sync_enabled: values.auto_sync_enabled,
        auto_sync_interval_minutes: values.auto_sync_interval_minutes,
      };
      const response = isEdit
        ? await api.put(`/api/mcp_servers/${serverId}`, payload)
        : await api.post('/api/mcp_servers', payload);
      const { success, message } = response.data;
      if (!success) {
        notify({
          type: 'error',
          title: t('mcp.edit.notifications.save_failed', 'Save failed'),
          message: message || '',
        });
        return;
      }
      notify({
        type: 'success',
        title: t('mcp.edit.notifications.save_success', 'Saved'),
        message: '',
      });
      navigate('/mcps');
    } catch (error) {
      notify({
        type: 'error',
        title: t('mcp.edit.notifications.save_failed', 'Save failed'),
        message: error instanceof Error ? error.message : String(error),
      });
    }
  };

  const toolPricingWarning = () => {
    const whitelist = form.watch('tool_whitelist') || [];
    let pricing: Record<string, any> = {};
    try {
      pricing = parseJSON(form.watch('tool_pricing')) || {};
    } catch {
      return t('mcp.edit.pricing.invalid', 'Tool pricing JSON is invalid');
    }
    const missing = whitelist.filter((tool) => !pricing[tool]);
    if (missing.length === 0) return '';
    return t('mcp.edit.pricing.missing', 'Missing pricing for: {{tools}}', {
      tools: missing.join(', '),
    });
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>
            {isEdit
              ? t('mcp.edit.title_edit', 'Edit MCP Server')
              : t('mcp.edit.title_add', 'Add MCP Server')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('mcp.edit.fields.name', 'Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={loading} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('mcp.edit.fields.description', 'Description')}</FormLabel>
                    <FormControl>
                      <Textarea {...field} disabled={loading} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="status"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('mcp.edit.fields.status', 'Status')}</FormLabel>
                      <Select onValueChange={field.onChange} value={String(field.value)}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="1">{t('mcp.status.enabled', 'Enabled')}</SelectItem>
                          <SelectItem value="0">{t('mcp.status.disabled', 'Disabled')}</SelectItem>
                        </SelectContent>
                      </Select>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="protocol"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('mcp.edit.fields.protocol', 'Protocol')}</FormLabel>
                      <Select onValueChange={field.onChange} value={field.value}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="streamable_http">
                            {t('mcp.edit.fields.protocol_streamable', 'Streamable HTTP')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="base_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('mcp.edit.fields.base_url', 'Base URL')}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={loading} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="auth_type"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('mcp.edit.fields.auth_type', 'Auth type')}</FormLabel>
                      <Select onValueChange={field.onChange} value={field.value}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="none">
                            {t('mcp.edit.fields.auth_type_none', 'None')}
                          </SelectItem>
                          <SelectItem value="bearer">
                            {t('mcp.edit.fields.auth_type_bearer', 'Bearer')}
                          </SelectItem>
                          <SelectItem value="api_key">
                            {t('mcp.edit.fields.auth_type_api_key', 'API Key')}
                          </SelectItem>
                          <SelectItem value="custom_headers">
                            {t('mcp.edit.fields.auth_type_custom_headers', 'Custom headers')}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="api_key"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('mcp.edit.fields.api_key', 'API key')}</FormLabel>
                      <FormControl>
                        <Input type="password" {...field} disabled={loading} />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="headers"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('mcp.edit.fields.headers', 'Custom headers (JSON)')}</FormLabel>
                    <FormControl>
                      <Textarea {...field} className="font-mono text-xs" rows={4} disabled={loading} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <ToolListEditor
                label={t('mcp.edit.fields.tool_whitelist', 'Tool whitelist')}
                description={t('mcp.edit.fields.tool_whitelist_help', 'Only tools listed here will be enabled.')}
                value={form.watch('tool_whitelist')}
                onChange={(value) => form.setValue('tool_whitelist', value)}
                placeholder={t('mcp.edit.fields.tool_whitelist_placeholder', 'tool_name')}
                addLabel={t('mcp.edit.actions.add', 'Add')}
              />

              <ToolListEditor
                label={t('mcp.edit.fields.tool_blacklist', 'Tool blacklist')}
                description={t('mcp.edit.fields.tool_blacklist_help', 'Blocked tools will never be exposed.')}
                value={form.watch('tool_blacklist')}
                onChange={(value) => form.setValue('tool_blacklist', value)}
                placeholder={t('mcp.edit.fields.tool_blacklist_placeholder', 'tool_name')}
                addLabel={t('mcp.edit.actions.add', 'Add')}
              />

              <FormField
                control={form.control}
                name="tool_pricing"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('mcp.edit.fields.tool_pricing', 'Tool pricing (JSON)')}</FormLabel>
                    <FormControl>
                      <Textarea {...field} className="font-mono text-xs" rows={5} disabled={loading} />
                    </FormControl>
                    <FormMessage />
                    {toolPricingWarning() && (
                      <p className="text-xs text-yellow-600">{toolPricingWarning()}</p>
                    )}
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="auto_sync_enabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center justify-between rounded-lg border p-3">
                      <div>
                        <FormLabel>{t('mcp.edit.fields.auto_sync', 'Auto sync')}</FormLabel>
                        <p className="text-xs text-muted-foreground">
                          {t('mcp.edit.fields.auto_sync_help', 'Sync tools on a schedule.')}
                        </p>
                      </div>
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => field.onChange(Boolean(checked))}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="auto_sync_interval_minutes"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('mcp.edit.fields.auto_sync_interval', 'Sync interval (minutes)')}</FormLabel>
                      <FormControl>
                        <Input type="number" min={5} max={1440} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {isEdit && tools.length > 0 && (
                <Card>
                  <CardHeader>
                    <CardTitle>{t('mcp.edit.tools.title', 'Synced tools')}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <ul className="space-y-2">
                      {tools.map((tool) => (
                        <li key={tool.id} className="border rounded-md p-3">
                          <div className="font-medium">{tool.name}</div>
                          {tool.description && (
                            <p className="text-xs text-muted-foreground">{tool.description}</p>
                          )}
                        </li>
                      ))}
                    </ul>
                  </CardContent>
                </Card>
              )}

              <div className="flex gap-2">
                <Button type="submit">
                  {isEdit
                    ? t('mcp.edit.actions.update', 'Update Server')
                    : t('mcp.edit.actions.create', 'Create Server')}
                </Button>
                <Button type="button" variant="outline" onClick={() => navigate('/mcps')}>
                  {t('mcp.edit.actions.cancel', 'Cancel')}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
