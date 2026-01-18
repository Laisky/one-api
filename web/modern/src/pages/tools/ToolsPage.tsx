import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { api } from '@/lib/api';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface MCPToolPricing {
  usd_per_call?: number;
  quota_per_call?: number;
}

interface MCPTool {
  id: number;
  server_id: number;
  name: string;
  display_name?: string;
  description?: string;
  input_schema?: string;
  default_pricing?: MCPToolPricing;
  status?: number;
}

interface MCPServer {
  id: number;
  name: string;
  status: number;
  priority: number;
  base_url: string;
  protocol: string;
  auth_type: string;
}

interface MCPServerListItem {
  server: MCPServer;
  tool_count: number;
}

interface ToolsByServer {
  server: MCPServer;
  tools: MCPTool[];
}

type ToolsData = Record<string, ToolsByServer>;

export function ToolsPage() {
  const { t } = useTranslation();
  const [toolsData, setToolsData] = useState<ToolsData>({});
  const [filteredData, setFilteredData] = useState<ToolsData>({});
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedServers, setSelectedServers] = useState<string[]>([]);

  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`tools.${key}`, { defaultValue, ...options }),
    [t]
  );

  const fetchToolsData = async () => {
    try {
      setLoading(true);
      const res = await api.get('/api/mcp_servers?p=0&size=200&sort=id&order=asc');
      const { success, message, data } = res.data;
      if (!success) {
        console.error('Failed to fetch MCP servers:', message);
        return;
      }
      const servers = (data as MCPServerListItem[]).map((item) => item.server);
      const toolsByServer = await Promise.all(
        servers.map(async (server) => {
          const response = await api.get(`/api/mcp_servers/${server.id}/tools`);
          const payload = response.data;
          const tools = payload?.success ? (payload.data as MCPTool[]) : [];
          return { server, tools };
        })
      );

      const aggregated: ToolsData = {};
      toolsByServer.forEach(({ server, tools }) => {
        aggregated[server.name] = {
          server,
          tools,
        };
      });

      setToolsData(aggregated);
      setFilteredData(aggregated);
    } catch (error) {
      console.error('Error fetching MCP tools:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchToolsData();
  }, []);

  useEffect(() => {
    let filtered = { ...toolsData };

    if (selectedServers.length > 0) {
      const serverFiltered: ToolsData = {};
      selectedServers.forEach((serverName) => {
        if (filtered[serverName]) {
          serverFiltered[serverName] = filtered[serverName];
        }
      });
      filtered = serverFiltered;
    }

    if (searchTerm) {
      const lowerTerm = searchTerm.toLowerCase();
      const searchFiltered: ToolsData = {};
      Object.keys(filtered).forEach((serverName) => {
        const entry = filtered[serverName];
        const tools = entry.tools.filter((tool) => {
          const nameMatch = tool.name?.toLowerCase().includes(lowerTerm);
          const displayMatch = tool.display_name ? tool.display_name.toLowerCase().includes(lowerTerm) : false;
          const descMatch = tool.description ? tool.description.toLowerCase().includes(lowerTerm) : false;
          return nameMatch || displayMatch || descMatch;
        });
        if (tools.length > 0) {
          searchFiltered[serverName] = {
            ...entry,
            tools,
          };
        }
      });
      filtered = searchFiltered;
    }

    setFilteredData(filtered);
  }, [searchTerm, selectedServers, toolsData]);

  const totalTools = useMemo(() => Object.values(filteredData).reduce((total, entry) => total + entry.tools.length, 0), [filteredData]);

  const serverOptions = useMemo(() => Object.keys(toolsData).sort(), [toolsData]);

  const toggleServerFilter = (serverName: string) => {
    if (selectedServers.includes(serverName)) {
      setSelectedServers(selectedServers.filter((name) => name !== serverName));
    } else {
      setSelectedServers([...selectedServers, serverName]);
    }
  };

  const clearFilters = () => {
    setSearchTerm('');
    setSelectedServers([]);
  };

  const formatPricing = (pricing?: MCPToolPricing): string => {
    if (!pricing) {
      return tr('labels.free', 'Free');
    }
    const usd = pricing.usd_per_call ?? 0;
    const quota = pricing.quota_per_call ?? 0;
    if (usd <= 0 && quota <= 0) {
      return tr('labels.free', 'Free');
    }
    const parts: string[] = [];
    if (quota > 0) {
      parts.push(`${quota} ${tr('labels.quota', 'quota')}`);
    }
    if (usd > 0) {
      const formatted = usd < 0.001 ? usd.toFixed(6) : usd < 1 ? usd.toFixed(4) : usd.toFixed(2);
      parts.push(`$${formatted}`);
    }
    return parts.join(' / ');
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr('loading', 'Loading tools...')}</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>{tr('title', 'MCP Tools')}</CardTitle>
          <CardDescription>
            {tr('description', 'Browse tools synced from MCP servers, grouped by server with pricing and schema details.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
            <div className="md:col-span-1">
              <Input placeholder={tr('search', 'Search tools...')} value={searchTerm} onChange={(e) => setSearchTerm(e.target.value)} />
            </div>
            <div className="md:col-span-1">
              <div className="flex flex-wrap gap-2">
                {serverOptions.map((serverName) => (
                  <Badge
                    key={serverName}
                    variant={selectedServers.includes(serverName) ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => toggleServerFilter(serverName)}
                  >
                    {serverName} ({toolsData[serverName].tools.length})
                  </Badge>
                ))}
              </div>
            </div>
            <div className="md:col-span-1">
              <Button variant="outline" onClick={clearFilters} className="w-full">
                {tr('clear_filters', 'Clear Filters')}
              </Button>
            </div>
          </div>

          {totalTools === 0 ? (
            <div className="text-center py-8">
              <h3 className="text-lg font-medium mb-2">{tr('no_tools', 'No tools found')}</h3>
              <p className="text-muted-foreground">{tr('no_tools_desc', 'Try adjusting your search terms or filters.')}</p>
            </div>
          ) : (
            <>
              <div className="mb-6">
                <h3 className="text-lg font-medium">
                  {tr('found', 'Found {{count}} tools in {{servers}} servers', {
                    count: totalTools,
                    servers: Object.keys(filteredData).length,
                  })}
                </h3>
              </div>

              {Object.keys(filteredData)
                .sort()
                .map((serverName) => {
                  const entry = filteredData[serverName];
                  const tools = [...entry.tools].sort((a, b) => a.name.localeCompare(b.name));
                  return (
                    <Card key={serverName} className="mb-6">
                      <CardHeader>
                        <CardTitle className="text-lg">
                          {serverName} ({tools.length} tools)
                        </CardTitle>
                      </CardHeader>
                      <CardContent>
                        <div className="overflow-x-auto">
                          <table className="w-full text-sm">
                            <thead>
                              <tr className="border-b">
                                <th className="text-left py-2 px-3 font-medium">{tr('table.tool', 'Tool')}</th>
                                <th className="text-left py-2 px-3 font-medium">{tr('table.description', 'Description')}</th>
                                <th className="text-left py-2 px-3 font-medium">{tr('table.status', 'Status')}</th>
                                <th className="text-left py-2 px-3 font-medium">{tr('table.pricing', 'Pricing')}</th>
                                <th className="text-left py-2 px-3 font-medium">{tr('table.schema', 'Input Schema')}</th>
                              </tr>
                            </thead>
                            <tbody>
                              {tools.map((tool) => {
                                const schema = tool.input_schema || '';
                                return (
                                  <tr key={`${serverName}-${tool.name}`} className="border-b hover:bg-muted/50">
                                    <td className="py-2 px-3 font-mono text-sm" data-label="Tool">
                                      {tool.name}
                                    </td>
                                    <td className="py-2 px-3" data-label="Description">
                                      {tool.description || '-'}
                                    </td>
                                    <td className="py-2 px-3" data-label="Status">
                                      {tool.status === 1 ? tr('status.enabled', 'Enabled') : tr('status.disabled', 'Disabled')}
                                    </td>
                                    <td className="py-2 px-3" data-label="Pricing">
                                      {formatPricing(tool.default_pricing)}
                                    </td>
                                    <td className="py-2 px-3" data-label="Input Schema">
                                      {schema ? (
                                        <span className="block max-w-xs truncate font-mono text-xs" title={schema}>
                                          {schema}
                                        </span>
                                      ) : (
                                        '-'
                                      )}
                                    </td>
                                  </tr>
                                );
                              })}
                            </tbody>
                          </table>
                        </div>
                      </CardContent>
                    </Card>
                  );
                })}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default ToolsPage;
