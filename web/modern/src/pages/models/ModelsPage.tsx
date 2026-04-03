import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { ChevronRight } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { ModelDisplayData, ModelPricingModal } from './ModelPricingModal';

interface ChannelInfo {
  models: Record<string, ModelDisplayData>;
}

interface ModelsData {
  [channelName: string]: ChannelInfo;
}

export function ModelsPage() {
  const { isMobile } = useResponsive();
  const [searchParams, setSearchParams] = useSearchParams();
  const [modelsData, setModelsData] = useState<ModelsData>({});
  const [filteredData, setFilteredData] = useState<ModelsData>({});
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState<{ name: string; data: ModelDisplayData; channel: string } | null>(null);
  const modalOpen = searchParams.get('model') !== null && selectedModel !== null;
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`models.${key}`, { defaultValue, ...options }),
    [t]
  );

  const fetchModelsData = async () => {
    try {
      setLoading(true);
      const res = await api.get('/api/models/display');
      const { success, message, data } = res.data;
      if (success) {
        setModelsData(data || {});
        setFilteredData(data || {});
      } else {
        console.error('Failed to fetch models:', message);
      }
    } catch (error) {
      console.error('Error fetching models:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchModelsData();
  }, []);

  // Auto-populate selectedModel from URL param when data loads
  useEffect(() => {
    const modelParam = searchParams.get('model');
    if (!modelParam || Object.keys(modelsData).length === 0) return;
    // Already resolved
    if (selectedModel?.name === modelParam) return;
    for (const channelName of Object.keys(modelsData)) {
      const channelInfo = modelsData[channelName];
      if (channelInfo.models[modelParam]) {
        setSelectedModel({ name: modelParam, data: channelInfo.models[modelParam], channel: formatChannelName(channelName) });
        return;
      }
    }
    // Model not found — clean up URL
    setSearchParams((prev) => { prev.delete('model'); return prev; }, { replace: true });
  }, [searchParams, modelsData]);

  useEffect(() => {
    let filtered = { ...modelsData };

    if (selectedChannels.length > 0) {
      const channelFiltered: ModelsData = {};
      selectedChannels.forEach((channelName) => {
        if (filtered[channelName]) {
          channelFiltered[channelName] = filtered[channelName];
        }
      });
      filtered = channelFiltered;
    }

    if (searchTerm) {
      const searchFiltered: ModelsData = {};
      Object.keys(filtered).forEach((channelName) => {
        const channelData = filtered[channelName];
        const filteredModels: Record<string, ModelDisplayData> = {};

        Object.keys(channelData.models).forEach((modelName) => {
          if (modelName.toLowerCase().includes(searchTerm.toLowerCase())) {
            filteredModels[modelName] = channelData.models[modelName];
          }
        });

        if (Object.keys(filteredModels).length > 0) {
          searchFiltered[channelName] = {
            ...channelData,
            models: filteredModels,
          };
        }
      });
      filtered = searchFiltered;
    }

    setFilteredData(filtered);
  }, [searchTerm, selectedChannels, modelsData]);

  const formatPrice = (price: number): string => {
    if (price === 0) return tr('labels.free', 'Free');
    if (price < 0.001) return `$${price.toFixed(6)}`;
    if (price < 1) return `$${price.toFixed(4)}`;
    return `$${price.toFixed(2)}`;
  };

  const formatChannelName = (channelName: string): string => {
    const colonIndex = channelName.indexOf(':');
    if (colonIndex !== -1) {
      return channelName.substring(colonIndex + 1);
    }
    return channelName;
  };

  const toggleChannelFilter = (channelName: string) => {
    if (selectedChannels.includes(channelName)) {
      setSelectedChannels(selectedChannels.filter((ch) => ch !== channelName));
    } else {
      setSelectedChannels([...selectedChannels, channelName]);
    }
  };

  const clearFilters = () => {
    setSearchTerm('');
    setSelectedChannels([]);
  };

  const openModelDetail = (modelName: string, data: ModelDisplayData, channelName: string) => {
    setSelectedModel({ name: modelName, data, channel: formatChannelName(channelName) });
    setSearchParams((prev) => { prev.set('model', modelName); return prev; });
  };

  const handleModalClose = (open: boolean) => {
    if (!open) {
      setSearchParams((prev) => { prev.delete('model'); return prev; });
      setSelectedModel(null);
    }
  };

  /** Check if a model has rich pricing data beyond basic text tokens */
  const hasRichPricing = (data: ModelDisplayData): boolean => {
    return !!(
      data.tiers?.length ||
      data.video_pricing ||
      data.audio_pricing ||
      data.image_pricing ||
      data.embedding_pricing ||
      (data.cache_write_5m_price && data.cache_write_5m_price > 0) ||
      (data.cache_write_1h_price && data.cache_write_1h_price > 0) ||
      (data.cached_input_price !== undefined && data.cached_input_price !== data.input_price)
    );
  };

  const renderChannelModels = (channelName: string, channelInfo: ChannelInfo) => {
    const models = Object.keys(channelInfo.models)
      .sort()
      .map((modelName) => ({
        model: modelName,
        data: channelInfo.models[modelName],
        inputPrice: channelInfo.models[modelName].input_price,
        cachedInputPrice: channelInfo.models[modelName].cached_input_price ?? channelInfo.models[modelName].input_price,
        outputPrice: channelInfo.models[modelName].output_price,
        imagePrice: channelInfo.models[modelName].image_price,
      }));

    return (
      <Card key={channelName} className="mb-6 border-0 shadow-none md:border md:shadow-sm">
        <CardHeader>
          <CardTitle className="text-lg">
            {formatChannelName(channelName)} ({models.length} models)
          </CardTitle>
        </CardHeader>
        <CardContent>
          {isMobile ? (
            <div className="space-y-3">
              {models.map((model) => (
                <div
                  key={model.model}
                  className="rounded-xl border bg-card p-4 shadow-sm space-y-3 cursor-pointer transition-colors hover:bg-muted/50 active:bg-muted/70"
                  onClick={() => openModelDetail(model.model, model.data, channelName)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') openModelDetail(model.model, model.data, channelName); }}
                >
                  <div className="flex items-center justify-between">
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.model', 'Model')}
                      </div>
                      <div className="font-mono text-sm break-all">{model.model}</div>
                    </div>
                    <ChevronRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.input_short', 'Input')}
                      </div>
                      <div className="text-sm">{formatPrice(model.inputPrice)}</div>
                    </div>
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.output_short', 'Output')}
                      </div>
                      <div className="text-sm">{formatPrice(model.outputPrice)}</div>
                    </div>
                  </div>
                  {hasRichPricing(model.data) && (
                    <div className="flex flex-wrap gap-1">
                      {model.data.image_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Image</Badge>}
                      {model.data.video_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Video</Badge>}
                      {model.data.audio_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Audio</Badge>}
                      {model.data.tiers && model.data.tiers.length > 0 && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Tiered</Badge>}
                      {model.data.embedding_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Embedding</Badge>}
                    </div>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b">
                    <th className="text-left py-2 px-3 font-medium">{tr('table.model', 'Model')}</th>
                    <th className="text-left py-2 px-3 font-medium">{tr('table.input_price', 'Input Price (per 1M tokens)')}</th>
                    <th className="text-left py-2 px-3 font-medium">{tr('table.cached_input_price', 'Cached Input Price')}</th>
                    <th className="text-left py-2 px-3 font-medium">{tr('table.output_price', 'Output Price')}</th>
                    <th className="text-left py-2 px-3 font-medium">{tr('table.image_price', 'Image Price (per image)')}</th>
                    <th className="w-8"></th>
                  </tr>
                </thead>
                <tbody>
                  {models.map((model) => (
                    <tr
                      key={model.model}
                      className="border-b cursor-pointer transition-colors hover:bg-muted/50"
                      onClick={() => openModelDetail(model.model, model.data, channelName)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') openModelDetail(model.model, model.data, channelName); }}
                    >
                      <td className="py-2 px-3 font-mono text-sm">
                        <span className="inline-flex items-center gap-2">
                          {model.model}
                          {hasRichPricing(model.data) && (
                            <span className="inline-flex gap-1">
                              {model.data.image_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Image</Badge>}
                              {model.data.video_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Video</Badge>}
                              {model.data.audio_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Audio</Badge>}
                              {model.data.tiers && model.data.tiers.length > 0 && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Tiered</Badge>}
                              {model.data.embedding_pricing && <Badge variant="secondary" className="text-[10px] px-1.5 py-0">Embedding</Badge>}
                            </span>
                          )}
                        </span>
                      </td>
                      <td className="py-2 px-3">{formatPrice(model.inputPrice)}</td>
                      <td className="py-2 px-3">{formatPrice(model.cachedInputPrice)}</td>
                      <td className="py-2 px-3">{formatPrice(model.outputPrice)}</td>
                      <td className="py-2 px-3">{model.imagePrice && model.imagePrice > 0 ? formatPrice(model.imagePrice) : '-'}</td>
                      <td className="py-2 px-1">
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    );
  };

  if (loading) {
    return (
      <ResponsivePageContainer
        title={tr('title', 'Supported Models')}
        description={tr('description', 'Browse all models supported by the server.')}
      >
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr('loading', 'Loading models...')}</span>
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  const totalModels = Object.values(filteredData).reduce((total, channelInfo) => total + Object.keys(channelInfo.models).length, 0);

  const channelOptions = Object.keys(modelsData).sort();

  return (
    <>
      <ResponsivePageContainer
        title={tr('title', 'Supported Models')}
        description={tr('description', 'Browse all models supported by the server, grouped by channel/adaptor with pricing information.')}
      >
        <Card className="mb-6 border-0 shadow-none md:border md:shadow-sm">
          <CardHeader>
            <CardTitle className="text-lg">{tr('filters.title', 'Filter Models')}</CardTitle>
            <CardDescription>{tr('filters.description', 'Search by model name or narrow the list by channel.')}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3 mb-6">
              <div className="md:col-span-1">
                <Input placeholder={tr('search', 'Search models...')} value={searchTerm} onChange={(e) => setSearchTerm(e.target.value)} />
              </div>
              <div className="md:col-span-1">
                <div className="flex flex-wrap gap-2">
                  {channelOptions.map((channelName) => (
                    <Badge
                      key={channelName}
                      variant={selectedChannels.includes(channelName) ? 'default' : 'outline'}
                      className="cursor-pointer break-all"
                      onClick={() => toggleChannelFilter(channelName)}
                    >
                      {formatChannelName(channelName)} ({Object.keys(modelsData[channelName].models).length})
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

            {totalModels === 0 ? (
              <div className="text-center py-8">
                <h3 className="text-lg font-medium mb-2">{tr('no_models', 'No models found')}</h3>
                <p className="text-muted-foreground">{tr('no_models_desc', 'Try adjusting your search terms or filters.')}</p>
              </div>
            ) : (
              <>
                <div className="mb-6">
                  <h3 className="text-lg font-medium">
                    {tr('found', 'Found {{count}} models in {{channels}} channels', {
                      count: totalModels,
                      channels: Object.keys(filteredData).length,
                    })}
                  </h3>
                </div>
                {Object.keys(filteredData)
                  .sort()
                  .map((channelName) => renderChannelModels(channelName, filteredData[channelName]))}
              </>
            )}
          </CardContent>
        </Card>
      </ResponsivePageContainer>

      {selectedModel && (
        <ModelPricingModal
          open={modalOpen}
          onOpenChange={handleModalClose}
          modelName={selectedModel.name}
          data={selectedModel.data}
          channelName={selectedModel.channel}
        />
      )}
    </>
  );
}

export default ModelsPage;
