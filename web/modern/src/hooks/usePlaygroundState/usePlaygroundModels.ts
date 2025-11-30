import { useNotifications } from "@/components/ui/notifications";
import { api } from "@/lib/api";
import { STORAGE_KEYS } from "@/lib/storage";
import { loadFromStorage } from "@/lib/utils";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { PlaygroundModel, SuggestionOption, Token } from "./types";

export function usePlaygroundModels(
  selectedToken: string,
  tokens: Token[],
  isLoadingTokens: boolean,
  selectedChannel: string,
  channelModelMap: Record<string, string[]>,
  channelLabelMap: Map<string, string>
) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [models, setModels] = useState<PlaygroundModel[]>([]);
  const [selectedModel, setSelectedModel] = useState("");
  const [isLoadingModels, setIsLoadingModels] = useState(true);
  const [modelInputValue, setModelInputValue] = useState("");
  const selectedModelRef = useRef(selectedModel);
  const userAvailableModelsCache = useRef<string[] | null>(null);

  selectedModelRef.current = selectedModel;

  const fetchUserAvailableModels = useCallback(async (): Promise<string[]> => {
    if (userAvailableModelsCache.current !== null) {
      return userAvailableModelsCache.current;
    }

    try {
      const response = await api.get("/api/user/available_models");
      const payload = response.data;

      if (payload?.success && Array.isArray(payload.data)) {
        const normalized = (payload.data as Array<unknown>)
          .map((model) => (typeof model === "string" ? model.trim() : ""))
          .filter((model): model is string => model.length > 0);
        const uniqueModels: string[] = Array.from(new Set(normalized));
        userAvailableModelsCache.current = uniqueModels;
        return uniqueModels;
      }
    } catch {
      // Swallow fetch errors; caller will surface a user-facing notification.
    }

    userAvailableModelsCache.current = [];
    return [];
  }, []);

  useEffect(() => {
    const loadModels = async () => {
      if (!selectedToken) {
        setModels([]);
        setSelectedModel("");
        setModelInputValue("");
        setIsLoadingModels(false);
        return;
      }

      setIsLoadingModels(true);
      try {
        const token = tokens.find((t) => t.key === selectedToken);
        if (!token) {
          throw new Error("Token not found");
        }

        let availableModels: string[] = [];

        if (token.models) {
          availableModels = token.models.split(",").map((m) => m.trim());
        } else {
          availableModels = await fetchUserAvailableModels();
        }

        if (selectedChannel) {
          const channelModels = channelModelMap[selectedChannel] || [];
          availableModels = availableModels.filter((m) =>
            channelModels.includes(m)
          );
        }

        const transformedModels: PlaygroundModel[] = availableModels.map(
          (modelId) => {
            const channels: string[] = [];
            Object.entries(channelModelMap).forEach(([channelName, models]) => {
              if (models.includes(modelId)) {
                const label = channelLabelMap.get(channelName) || channelName;
                channels.push(label);
              }
            });
            channels.sort((a, b) => a.localeCompare(b));

            return {
              id: modelId,
              object: "model",
              owned_by: "system",
              label: modelId,
              channels,
            };
          }
        );

        if (transformedModels.length === 0) {
          setModels([]);
          setSelectedModel("");
          setModelInputValue("");
          return;
        }

        setModels(transformedModels);

        const availableIds = new Set(
          transformedModels.map((model) => model.id)
        );

        const resolveLabel = (modelId: string): string => {
          const match = transformedModels.find((model) => model.id === modelId);
          return match?.label ?? modelId;
        };

        if (
          selectedModelRef.current &&
          availableIds.has(selectedModelRef.current)
        ) {
          setModelInputValue(resolveLabel(selectedModelRef.current));
          return;
        }

        const savedModel = loadFromStorage(STORAGE_KEYS.MODEL, "");
        if (savedModel && availableIds.has(savedModel)) {
          setSelectedModel(savedModel);
          setModelInputValue(resolveLabel(savedModel));
          return;
        }

        const fallbackModelId = transformedModels[0].id;
        setSelectedModel(fallbackModelId);
        setModelInputValue(resolveLabel(fallbackModelId));
      } catch {
        notify({
          title: t("playground.notifications.error_title"),
          message: t("playground.notifications.load_models_error"),
          type: "error",
        });
        setModels([]);
        setSelectedModel("");
        setModelInputValue("");
      } finally {
        setIsLoadingModels(false);
      }
    };

    if (tokens.length > 0) {
      void loadModels();
    } else if (!isLoadingTokens) {
      setIsLoadingModels(false);
      setModels([]);
      setSelectedModel("");
      setModelInputValue("");
      notify({
        title: t("playground.notifications.no_tokens_title"),
        message: t("playground.notifications.no_tokens_error"),
        type: "error",
      });
    }
  }, [
    selectedToken,
    tokens,
    isLoadingTokens,
    notify,
    fetchUserAvailableModels,
    selectedChannel,
    channelModelMap,
    channelLabelMap,
    t,
  ]);

  const modelSuggestions = useMemo<SuggestionOption[]>(() => {
    if (models.length === 0) {
      return [];
    }

    const options: SuggestionOption[] = models.map((model) => {
      const label = model.label ?? model.id;
      let description: string | undefined;

      if (!selectedChannel && model.channels && model.channels.length > 0) {
        const visibleChannels = model.channels.slice(0, 3);
        const remaining = model.channels.length - visibleChannels.length;
        if (visibleChannels.length > 0) {
          const base = visibleChannels.join(", ");
          const summary =
            remaining > 0
              ? `${base}, ${t("playground.parameters.model.more_channels", { count: remaining })}`
              : base;
          description = t("playground.parameters.model.channels_label", {
            channels: summary,
          });
        }
      }

      return {
        key: model.id,
        label,
        description,
      };
    });

    const sortedOptions = options
      .slice()
      .sort((a, b) => a.label.localeCompare(b.label));

    const query = modelInputValue.trim().toLowerCase();
    if (!query) {
      return sortedOptions;
    }

    const filtered = sortedOptions.filter((option) => {
      const labelLower = option.label.toLowerCase();
      const keyLower = option.key.toLowerCase();
      return labelLower.includes(query) || keyLower.includes(query);
    });

    return filtered;
  }, [models, modelInputValue, selectedChannel, t]);

  const handleModelQueryChange = useCallback((value: string) => {
    setModelInputValue(value);
    if (value.trim().length === 0) {
      setSelectedModel("");
    }
  }, []);

  const handleModelSelect = useCallback(
    (modelId: string) => {
      const match = models.find((model) => model.id === modelId);
      const label = match?.label ?? modelId;
      setSelectedModel(modelId);
      setModelInputValue(label);
    },
    [models]
  );

  const handleModelClear = useCallback(() => {
    setSelectedModel("");
    setModelInputValue("");
  }, []);

  return {
    models,
    selectedModel,
    isLoadingModels,
    modelInputValue,
    modelSuggestions,
    handleModelQueryChange,
    handleModelSelect,
    handleModelClear,
    setSelectedModel,
  };
}
