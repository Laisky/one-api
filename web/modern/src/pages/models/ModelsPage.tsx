import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { api } from "@/lib/api";
import { Info } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

interface ModelData {
  input_price: number;
  cached_input_price?: number;
  output_price: number;
  max_tokens: number;
  image_price?: number;
}

interface ChannelInfo {
  models: Record<string, ModelData>;
}

interface ModelsData {
  [channelName: string]: ChannelInfo;
}

export function ModelsPage() {
  const [modelsData, setModelsData] = useState<ModelsData>({});
  const [filteredData, setFilteredData] = useState<ModelsData>({});
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedChannels, setSelectedChannels] = useState<string[]>([]);
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`models.${key}`, { defaultValue, ...options }),
    [t]
  );

  const fetchModelsData = async () => {
    try {
      setLoading(true);
      // Unified API call - complete URL with /api prefix
      const res = await api.get("/api/models/display");
      const { success, message, data } = res.data;
      if (success) {
        setModelsData(data || {});
        setFilteredData(data || {});
      } else {
        console.error("Failed to fetch models:", message);
      }
    } catch (error) {
      console.error("Error fetching models:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchModelsData();
  }, []);

  useEffect(() => {
    let filtered = { ...modelsData };

    // Filter by selected channels
    if (selectedChannels.length > 0) {
      const channelFiltered: ModelsData = {};
      selectedChannels.forEach((channelName) => {
        if (filtered[channelName]) {
          channelFiltered[channelName] = filtered[channelName];
        }
      });
      filtered = channelFiltered;
    }

    // Filter by search term
    if (searchTerm) {
      const searchFiltered: ModelsData = {};
      Object.keys(filtered).forEach((channelName) => {
        const channelData = filtered[channelName];
        const filteredModels: Record<string, ModelData> = {};

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
    if (price === 0) return tr("labels.free", "Free");
    if (price < 0.001) return `$${price.toFixed(6)}`;
    if (price < 1) return `$${price.toFixed(4)}`;
    return `$${price.toFixed(2)}`;
  };

  const formatMaxTokens = (maxTokens: number): string => {
    if (maxTokens === 0) return tr("labels.unlimited", "Unlimited");
    if (maxTokens >= 1000000) return `${(maxTokens / 1000000).toFixed(1)}M`;
    if (maxTokens >= 1000) return `${(maxTokens / 1000).toFixed(0)}K`;
    return maxTokens.toString();
  };

  const formatChannelName = (channelName: string): string => {
    const colonIndex = channelName.indexOf(":");
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
    setSearchTerm("");
    setSelectedChannels([]);
  };

  const renderChannelModels = (
    channelName: string,
    channelInfo: ChannelInfo
  ) => {
    const models = Object.keys(channelInfo.models)
      .sort()
      .map((modelName) => ({
        model: modelName,
        inputPrice: channelInfo.models[modelName].input_price,
        cachedInputPrice:
          channelInfo.models[modelName].cached_input_price ??
          channelInfo.models[modelName].input_price,
        outputPrice: channelInfo.models[modelName].output_price,
        maxTokens: channelInfo.models[modelName].max_tokens,
        imagePrice: channelInfo.models[modelName].image_price,
      }));

    return (
      <Card key={channelName} className="mb-6">
        <CardHeader>
          <CardTitle className="text-lg">
            {formatChannelName(channelName)} ({models.length} models)
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b">
                  <th className="text-left py-2 px-3 font-medium">
                    {tr("table.model", "Model")}
                  </th>
                  <th className="text-left py-2 px-3 font-medium">
                    {tr("table.input_price", "Input Price (per 1M tokens)")}
                  </th>
                  <th className="text-left py-2 px-3 font-medium">
                    {tr("table.cached_input_price", "Cached Input Price")}
                  </th>
                  <th className="text-left py-2 px-3 font-medium">
                    {tr("table.output_price", "Output Price")}
                  </th>
                  <th className="text-left py-2 px-3 font-medium">
                    {tr("table.image_price", "Image Price (per image)")}
                  </th>
                  <th className="text-left py-2 px-3 font-medium">
                    <span className="inline-flex items-center gap-1">
                      {tr("table.max_tokens", "Max Tokens")}
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type="button"
                            className="inline-flex items-center text-muted-foreground hover:text-foreground focus:outline-none"
                            aria-label="What does max tokens mean?"
                          >
                            <Info className="h-4 w-4" aria-hidden="true" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent
                          side="top"
                          align="start"
                          className="max-w-xs text-sm"
                        >
                          {tr(
                            "table.max_tokens_tooltip",
                            "Maximum total tokens this channel allows per request for the model, including prompt and completion tokens. A value of 0 means the provider does not advertise a fixed limit."
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {models.map((model) => (
                  <tr key={model.model} className="border-b hover:bg-muted/50">
                    <td
                      className="py-2 px-3 font-mono text-sm"
                      data-label="Model"
                    >
                      {model.model}
                    </td>
                    <td className="py-2 px-3" data-label="Input Price">
                      {formatPrice(model.inputPrice)}
                    </td>
                    <td className="py-2 px-3" data-label="Cached Input Price">
                      {formatPrice(model.cachedInputPrice)}
                    </td>
                    <td className="py-2 px-3" data-label="Output Price">
                      {formatPrice(model.outputPrice)}
                    </td>
                    <td className="py-2 px-3" data-label="Image Price">
                      {model.imagePrice && model.imagePrice > 0
                        ? formatPrice(model.imagePrice)
                        : "-"}
                    </td>
                    <td className="py-2 px-3" data-label="Max Tokens">
                      {formatMaxTokens(model.maxTokens)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    );
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr("loading", "Loading models...")}</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  const totalModels = Object.values(filteredData).reduce(
    (total, channelInfo) => total + Object.keys(channelInfo.models).length,
    0
  );

  const channelOptions = Object.keys(modelsData).sort();

  return (
    <TooltipProvider delayDuration={150}>
      <div className="container mx-auto px-4 py-8">
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>{tr("title", "Supported Models")}</CardTitle>
            <CardDescription>
              {tr(
                "description",
                "Browse all models supported by the server, grouped by channel/adaptor with pricing information."
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
              <div className="md:col-span-1">
                <Input
                  placeholder={tr("search", "Search models...")}
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                />
              </div>
              <div className="md:col-span-1">
                <div className="flex flex-wrap gap-2">
                  {channelOptions.map((channelName) => (
                    <Badge
                      key={channelName}
                      variant={
                        selectedChannels.includes(channelName)
                          ? "default"
                          : "outline"
                      }
                      className="cursor-pointer"
                      onClick={() => toggleChannelFilter(channelName)}
                    >
                      {formatChannelName(channelName)} (
                      {Object.keys(modelsData[channelName].models).length})
                    </Badge>
                  ))}
                </div>
              </div>
              <div className="md:col-span-1">
                <Button
                  variant="outline"
                  onClick={clearFilters}
                  className="w-full"
                >
                  {tr("clear_filters", "Clear Filters")}
                </Button>
              </div>
            </div>

            {totalModels === 0 ? (
              <div className="text-center py-8">
                <h3 className="text-lg font-medium mb-2">
                  {tr("no_models", "No models found")}
                </h3>
                <p className="text-muted-foreground">
                  {tr(
                    "no_models_desc",
                    "Try adjusting your search terms or filters."
                  )}
                </p>
              </div>
            ) : (
              <>
                <div className="mb-6">
                  <h3 className="text-lg font-medium">
                    {tr(
                      "found",
                      "Found {{count}} models in {{channels}} channels",
                      {
                        count: totalModels,
                        channels: Object.keys(filteredData).length,
                      }
                    )}
                  </h3>
                </div>
                {Object.keys(filteredData)
                  .sort()
                  .map((channelName) =>
                    renderChannelModels(channelName, filteredData[channelName])
                  )}
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </TooltipProvider>
  );
}

export default ModelsPage;
