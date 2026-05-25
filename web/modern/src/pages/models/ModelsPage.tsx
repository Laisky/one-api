import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { ChevronRight } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { ModelDisplayData, ModelPricingModal } from './ModelPricingModal';

interface ChannelInfo {
  models: Record<string, ModelDisplayData>;
}

interface ModelsData {
  [channelName: string]: ChannelInfo;
}

interface ModelFilters {
  keyword: string;
  selectedChannels: string[];
  inputModalities: string[];
  outputModalities: string[];
  features: string[];
  hasImage: boolean;
  hasVideo: boolean;
  hasAudio: boolean;
  hasEmbedding: boolean;
  minContextLength: number;
  maxInputPrice: number;
}

const INPUT_MODALITY_OPTIONS = ['text', 'image', 'audio', 'video', 'file'] as const;
const OUTPUT_MODALITY_OPTIONS = ['text', 'image', 'audio', 'video'] as const;
const FEATURE_OPTIONS = ['tools', 'json_mode', 'structured_outputs', 'web_search', 'reasoning', 'logprobs'] as const;

const CONTEXT_PRESETS: Array<{ label: string; value: number }> = [
  { label: 'context_any', value: 0 },
  { label: '≥32k', value: 32_000 },
  { label: '≥128k', value: 128_000 },
  { label: '≥200k', value: 200_000 },
  { label: '≥1M', value: 1_000_000 },
];

const PRICE_PRESETS: Array<{ label: string; value: number }> = [
  { label: 'price_any', value: 0 },
  { label: '≤$1', value: 1 },
  { label: '≤$3', value: 3 },
  { label: '≤$10', value: 10 },
  { label: '≤$30', value: 30 },
];

const DEFAULT_FILTERS: ModelFilters = {
  keyword: '',
  selectedChannels: [],
  inputModalities: [],
  outputModalities: [],
  features: [],
  hasImage: false,
  hasVideo: false,
  hasAudio: false,
  hasEmbedding: false,
  minContextLength: 0,
  maxInputPrice: 0,
};

export function ModelsPage() {
  const { isMobile } = useResponsive();
  const [searchParams, setSearchParams] = useSearchParams();
  const [modelsData, setModelsData] = useState<ModelsData>({});
  const [filteredData, setFilteredData] = useState<ModelsData>({});
  const [loading, setLoading] = useState(true);
  const [filters, setFilters] = useState<ModelFilters>(DEFAULT_FILTERS);
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
    setSearchParams(
      (prev) => {
        prev.delete('model');
        return prev;
      },
      { replace: true }
    );
  }, [searchParams, modelsData]);

  const modelMatchesFilters = useCallback(
    (model: ModelDisplayData, modelName: string): boolean => {
      const keyword = filters.keyword.trim().toLowerCase();
      if (keyword && !modelName.toLowerCase().includes(keyword)) return false;

      if (filters.inputModalities.length > 0) {
        const modalities = model.input_modalities ?? [];
        const has = filters.inputModalities.some((m) => modalities.includes(m));
        if (!has) return false;
      }

      if (filters.outputModalities.length > 0) {
        const modalities = model.output_modalities ?? [];
        const has = filters.outputModalities.some((m) => modalities.includes(m));
        if (!has) return false;
      }

      if (filters.features.length > 0) {
        const supported = model.supported_features ?? [];
        const allPresent = filters.features.every((f) => supported.includes(f));
        if (!allPresent) return false;
      }

      if (filters.hasImage) {
        const ok = !!model.image_pricing || (model.output_modalities ?? []).includes('image');
        if (!ok) return false;
      }

      if (filters.hasVideo) {
        const ok = !!model.video_pricing || (model.output_modalities ?? []).includes('video');
        if (!ok) return false;
      }

      if (filters.hasAudio) {
        const ok =
          !!model.audio_pricing ||
          (model.input_modalities ?? []).includes('audio') ||
          (model.output_modalities ?? []).includes('audio');
        if (!ok) return false;
      }

      if (filters.hasEmbedding) {
        if (!model.embedding_pricing) return false;
      }

      if (filters.minContextLength > 0) {
        const ctx = model.context_length ?? 0;
        if (ctx < filters.minContextLength) return false;
      }

      if (filters.maxInputPrice > 0) {
        if (model.input_price > filters.maxInputPrice) return false;
      }

      return true;
    },
    [filters]
  );

  useEffect(() => {
    let workingChannels = modelsData;

    if (filters.selectedChannels.length > 0) {
      const channelFiltered: ModelsData = {};
      filters.selectedChannels.forEach((channelName) => {
        if (workingChannels[channelName]) {
          channelFiltered[channelName] = workingChannels[channelName];
        }
      });
      workingChannels = channelFiltered;
    }

    const result: ModelsData = {};
    Object.keys(workingChannels).forEach((channelName) => {
      const channelData = workingChannels[channelName];
      const filteredModels: Record<string, ModelDisplayData> = {};
      Object.keys(channelData.models).forEach((modelName) => {
        const model = channelData.models[modelName];
        if (modelMatchesFilters(model, modelName)) {
          filteredModels[modelName] = model;
        }
      });
      if (Object.keys(filteredModels).length > 0) {
        result[channelName] = { ...channelData, models: filteredModels };
      }
    });

    setFilteredData(result);
  }, [filters, modelsData, modelMatchesFilters]);

  const formatPrice = (price: number): string => {
    if (price === 0) return tr('labels.free', 'Free');
    if (price < 0.001) return `$${price.toFixed(6)}`;
    if (price < 1) return `$${price.toFixed(4)}`;
    return `$${price.toFixed(2)}`;
  };

  /**
   * True when the model carries pricing that isn't priced per input/output token —
   * e.g. flat per-call, per-image, per-second, or per-document-page billing.
   * Used to distinguish "truly free" from "non-token billed" when token-price columns
   * would otherwise show $0 → "Free".
   */
  const hasNonTokenPricing = (data: ModelDisplayData): boolean => {
    if (data.per_call_pricing && ((data.per_call_pricing.usd_per_thousand_calls ?? 0) > 0 || (data.per_call_pricing.usd_per_call ?? 0) > 0)) return true;
    if ((data.image_price ?? 0) > 0) return true;
    if (data.image_pricing && (data.image_pricing.price_per_image_usd ?? 0) > 0) return true;
    if (data.video_pricing && (data.video_pricing.per_second_usd ?? 0) > 0) return true;
    if (data.audio_pricing && (data.audio_pricing.usd_per_second ?? 0) > 0) return true;
    if (data.embedding_pricing) {
      const e = data.embedding_pricing;
      if ((e.usd_per_image ?? 0) > 0) return true;
      if ((e.usd_per_audio_second ?? 0) > 0) return true;
      if ((e.usd_per_video_frame ?? 0) > 0) return true;
      if ((e.usd_per_document_page ?? 0) > 0) return true;
    }
    return false;
  };

  /**
   * Render a per-token price cell. A zero price means "Free" only when the model
   * has no non-token billing surface; otherwise "Free" would be a lie (the model
   * IS charged, just not by token) so we render an em dash to defer to the
   * pricing badges + detail modal for the real story.
   */
  const formatTokenPrice = (price: number, data: ModelDisplayData): string => {
    if (price > 0) return formatPrice(price);
    return hasNonTokenPricing(data) ? '—' : formatPrice(price);
  };

  const formatChannelName = (channelName: string): string => {
    const colonIndex = channelName.indexOf(':');
    if (colonIndex !== -1) {
      return channelName.substring(colonIndex + 1);
    }
    return channelName;
  };

  const toggleArrayValue = useCallback(<K extends keyof ModelFilters>(key: K, value: string) => {
    setFilters((prev) => {
      const current = prev[key] as unknown as string[];
      const next = current.includes(value) ? current.filter((v) => v !== value) : [...current, value];
      return { ...prev, [key]: next } as ModelFilters;
    });
  }, []);

  const toggleChannelFilter = (channelName: string) => toggleArrayValue('selectedChannels', channelName);

  const toggleBooleanFilter = (key: 'hasImage' | 'hasVideo' | 'hasAudio' | 'hasEmbedding') => {
    setFilters((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const setNumericFilter = (key: 'minContextLength' | 'maxInputPrice', value: number) => {
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const clearFilters = () => {
    setFilters(DEFAULT_FILTERS);
  };

  const openModelDetail = (modelName: string, data: ModelDisplayData, channelName: string) => {
    setSelectedModel({ name: modelName, data, channel: formatChannelName(channelName) });
    setSearchParams((prev) => {
      prev.set('model', modelName);
      return prev;
    });
  };

  const handleModalClose = (open: boolean) => {
    if (!open) {
      setSearchParams((prev) => {
        prev.delete('model');
        return prev;
      });
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
      data.per_call_pricing ||
      (data.cache_write_5m_price && data.cache_write_5m_price > 0) ||
      (data.cache_write_1h_price && data.cache_write_1h_price > 0) ||
      (data.cached_input_price !== undefined && data.cached_input_price !== data.input_price)
    );
  };

  const hasReasoning = (data: ModelDisplayData): boolean => {
    return !!(
      (data.supported_features && data.supported_features.includes('reasoning')) ||
      (data.supported_reasoning_efforts && data.supported_reasoning_efforts.length > 0)
    );
  };

  const renderPricingBadges = (data: ModelDisplayData) => (
    <>
      {data.image_pricing && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.image', 'Image')}
        </Badge>
      )}
      {data.video_pricing && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.video', 'Video')}
        </Badge>
      )}
      {data.audio_pricing && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.audio', 'Audio')}
        </Badge>
      )}
      {data.tiers && data.tiers.length > 0 && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.tiered', 'Tiered')}
        </Badge>
      )}
      {data.embedding_pricing && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.embedding', 'Embedding')}
        </Badge>
      )}
      {data.per_call_pricing && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.per_call', 'Per Call')}
        </Badge>
      )}
      {hasReasoning(data) && (
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          {tr('labels.reasoning', 'Reasoning')}
        </Badge>
      )}
    </>
  );

  const renderModalityBadges = (data: ModelDisplayData) => {
    const inMods = data.input_modalities ?? [];
    const outMods = data.output_modalities ?? [];
    if (inMods.length === 0 && outMods.length === 0) return null;
    return (
      <span className="inline-flex flex-wrap gap-1">
        {inMods.length > 0 && (
          <Badge variant="outline" className="text-[10px] px-1.5 py-0 font-normal">
            {tr('labels.in_modalities', 'IN')}: {inMods.join(', ')}
          </Badge>
        )}
        {outMods.length > 0 && (
          <Badge variant="outline" className="text-[10px] px-1.5 py-0 font-normal">
            {tr('labels.out_modalities', 'OUT')}: {outMods.join(', ')}
          </Badge>
        )}
      </span>
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
            {formatChannelName(channelName)} ({tr('channel_count', '{{count}} models', { count: models.length })})
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
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') openModelDetail(model.model, model.data, channelName);
                  }}
                >
                  <div className="flex items-center justify-between">
                    <div className="min-w-0 flex-1 pr-2">
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.model', 'Model')}
                      </div>
                      <div className="font-mono text-sm break-all">{model.model}</div>
                      {renderModalityBadges(model.data) && (
                        <div className="mt-1.5">{renderModalityBadges(model.data)}</div>
                      )}
                    </div>
                    <ChevronRight className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.input_short', 'Input')}
                      </div>
                      <div className="text-sm">{formatTokenPrice(model.inputPrice, model.data)}</div>
                    </div>
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                        {tr('table.output_short', 'Output')}
                      </div>
                      <div className="text-sm">{formatTokenPrice(model.outputPrice, model.data)}</div>
                    </div>
                  </div>
                  {hasRichPricing(model.data) && <div className="flex flex-wrap gap-1">{renderPricingBadges(model.data)}</div>}
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
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') openModelDetail(model.model, model.data, channelName);
                      }}
                    >
                      <td className="py-2 px-3 font-mono text-sm">
                        <div className="flex flex-wrap items-center gap-2">
                          <span>{model.model}</span>
                          {hasRichPricing(model.data) && <span className="inline-flex gap-1">{renderPricingBadges(model.data)}</span>}
                          {renderModalityBadges(model.data)}
                        </div>
                      </td>
                      <td className="py-2 px-3">{formatTokenPrice(model.inputPrice, model.data)}</td>
                      <td className="py-2 px-3">{formatTokenPrice(model.cachedInputPrice, model.data)}</td>
                      <td className="py-2 px-3">{formatTokenPrice(model.outputPrice, model.data)}</td>
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

  const modalityLabel = (modality: string) => {
    switch (modality) {
      case 'text':
        return tr('filters.modality_text', 'Text');
      case 'image':
        return tr('filters.modality_image', 'Image');
      case 'audio':
        return tr('filters.modality_audio', 'Audio');
      case 'video':
        return tr('filters.modality_video', 'Video');
      case 'file':
        return tr('filters.modality_file', 'File');
      default:
        return modality;
    }
  };

  const featureLabel = (feature: string) => {
    switch (feature) {
      case 'tools':
        return tr('filters.feature_tools', 'Tools');
      case 'json_mode':
        return tr('filters.feature_json_mode', 'JSON Mode');
      case 'structured_outputs':
        return tr('filters.feature_structured_outputs', 'Structured Outputs');
      case 'web_search':
        return tr('filters.feature_web_search', 'Web Search');
      case 'reasoning':
        return tr('filters.feature_reasoning', 'Reasoning');
      case 'logprobs':
        return tr('filters.feature_logprobs', 'Logprobs');
      default:
        return feature;
    }
  };

  const contextLabel = (preset: { label: string; value: number }) => {
    if (preset.value === 0) return tr('filters.context_any', 'Any');
    return preset.label;
  };

  const priceLabel = (preset: { label: string; value: number }) => {
    if (preset.value === 0) return tr('filters.price_any', 'Any');
    return preset.label;
  };

  const channelOptions = useMemo(() => Object.keys(modelsData).sort(), [modelsData]);

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
          <CardContent className="space-y-5">
            {/* Search & Channels */}
            <div className="space-y-2">
              <Input
                placeholder={tr('search', 'Search models...')}
                value={filters.keyword}
                onChange={(e) => setFilters((prev) => ({ ...prev, keyword: e.target.value }))}
              />
              {channelOptions.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {channelOptions.map((channelName) => (
                    <Badge
                      key={channelName}
                      variant={filters.selectedChannels.includes(channelName) ? 'default' : 'outline'}
                      className="cursor-pointer break-all"
                      onClick={() => toggleChannelFilter(channelName)}
                    >
                      {formatChannelName(channelName)} ({Object.keys(modelsData[channelName].models).length})
                    </Badge>
                  ))}
                </div>
              )}
            </div>

            {/* Input Modalities */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.input_modality_title', 'Input Modality')}
              </div>
              <div className="flex flex-wrap gap-2">
                {INPUT_MODALITY_OPTIONS.map((m) => (
                  <Badge
                    key={m}
                    variant={filters.inputModalities.includes(m) ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => toggleArrayValue('inputModalities', m)}
                  >
                    {modalityLabel(m)}
                  </Badge>
                ))}
              </div>
            </div>

            {/* Output Modalities */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.output_modality_title', 'Output Modality')}
              </div>
              <div className="flex flex-wrap gap-2">
                {OUTPUT_MODALITY_OPTIONS.map((m) => (
                  <Badge
                    key={m}
                    variant={filters.outputModalities.includes(m) ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => toggleArrayValue('outputModalities', m)}
                  >
                    {modalityLabel(m)}
                  </Badge>
                ))}
              </div>
            </div>

            {/* Features */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.features_title', 'Features')}
              </div>
              <div className="flex flex-wrap gap-2">
                {FEATURE_OPTIONS.map((f) => (
                  <Badge
                    key={f}
                    variant={filters.features.includes(f) ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => toggleArrayValue('features', f)}
                  >
                    {featureLabel(f)}
                  </Badge>
                ))}
              </div>
            </div>

            {/* Capabilities */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.capabilities_title', 'Capabilities')}
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge
                  variant={filters.hasImage ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleBooleanFilter('hasImage')}
                >
                  {tr('filters.has_image', 'Image Output')}
                </Badge>
                <Badge
                  variant={filters.hasVideo ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleBooleanFilter('hasVideo')}
                >
                  {tr('filters.has_video', 'Video Output')}
                </Badge>
                <Badge
                  variant={filters.hasAudio ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleBooleanFilter('hasAudio')}
                >
                  {tr('filters.has_audio', 'Audio Support')}
                </Badge>
                <Badge
                  variant={filters.hasEmbedding ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleBooleanFilter('hasEmbedding')}
                >
                  {tr('filters.has_embedding', 'Embedding')}
                </Badge>
              </div>
            </div>

            {/* Context Window */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.context_title', 'Context Window')}
              </div>
              <div className="flex flex-wrap gap-2">
                {CONTEXT_PRESETS.map((preset) => (
                  <Badge
                    key={preset.value}
                    variant={filters.minContextLength === preset.value ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => setNumericFilter('minContextLength', preset.value)}
                  >
                    {contextLabel(preset)}
                  </Badge>
                ))}
              </div>
            </div>

            {/* Max Input Price */}
            <div className="space-y-2">
              <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">
                {tr('filters.price_title', 'Max Input Price')}
              </div>
              <div className="flex flex-wrap gap-2">
                {PRICE_PRESETS.map((preset) => (
                  <Badge
                    key={preset.value}
                    variant={filters.maxInputPrice === preset.value ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => setNumericFilter('maxInputPrice', preset.value)}
                  >
                    {priceLabel(preset)}
                  </Badge>
                ))}
              </div>
            </div>

            <div className="flex justify-end">
              <Button variant="outline" onClick={clearFilters}>
                {tr('clear_filters', 'Clear Filters')}
              </Button>
            </div>
          </CardContent>
        </Card>

        {totalModels === 0 ? (
          <Card className="border-0 shadow-none md:border md:shadow-sm">
            <CardContent className="text-center py-8">
              <h3 className="text-lg font-medium mb-2">{tr('no_models', 'No models found')}</h3>
              <p className="text-muted-foreground">{tr('no_models_desc', 'Try adjusting your search terms or filters.')}</p>
            </CardContent>
          </Card>
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
