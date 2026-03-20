import type { TFunction } from 'i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { NotificationOptions } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { STORAGE_KEYS } from '@/lib/storage';
import { loadFromStorage, saveToStorage } from '@/lib/utils';
import { formatChannelName, type PlaygroundModel, type SuggestionOption, TOKEN_STATUS, type Token } from '../types';

interface UseModelAndTokenBrowserArgs {
  t: TFunction;
  notify: (args: NotificationOptions) => string;
}

export interface UseModelAndTokenBrowserResult {
  tokens: Token[];
  isLoadingTokens: boolean;
  models: PlaygroundModel[];
  isLoadingModels: boolean;
  isLoadingChannels: boolean;
  selectedToken: string;
  setSelectedToken: React.Dispatch<React.SetStateAction<string>>;
  selectedModel: string;
  setSelectedModel: React.Dispatch<React.SetStateAction<string>>;
  selectedChannel: string;
  channelInputValue: string;
  modelInputValue: string;
  channelSuggestions: SuggestionOption[];
  modelSuggestions: SuggestionOption[];
  handleChannelQueryChange: (value: string) => void;
  handleChannelSelect: (key: string) => void;
  handleChannelClear: () => void;
  handleModelQueryChange: (value: string) => void;
  handleModelSelect: (key: string) => void;
  handleModelClear: () => void;
}

export const useModelAndTokenBrowser = (args: UseModelAndTokenBrowserArgs): UseModelAndTokenBrowserResult => {
  const { t, notify } = args;
  const [tokens, setTokens] = useState<Token[]>([]);
  const [selectedToken, setSelectedToken] = useState(() => loadFromStorage(STORAGE_KEYS.TOKEN, ''));
  const [isLoadingTokens, setIsLoadingTokens] = useState(true);

  const [models, setModels] = useState<PlaygroundModel[]>([]);
  const [selectedModel, setSelectedModel] = useState(() => loadFromStorage(STORAGE_KEYS.MODEL, ''));
  const [modelInputValue, setModelInputValue] = useState(() => loadFromStorage(STORAGE_KEYS.MODEL, ''));
  const [isLoadingModels, setIsLoadingModels] = useState(true);
  const selectedModelRef = useRef(selectedModel);

  const [channelModelMap, setChannelModelMap] = useState<Record<string, string[]>>({});
  const [isLoadingChannels, setIsLoadingChannels] = useState(true);
  const [channelInputValue, setChannelInputValue] = useState('');
  const [selectedChannel, setSelectedChannel] = useState('');
  const channelErrorRef = useRef<string | null>(null);
  const userAvailableModelsCache = useRef<string[] | null>(null);

  const fetchUserAvailableModels = useCallback(async (): Promise<string[]> => {
    if (userAvailableModelsCache.current !== null) {
      return userAvailableModelsCache.current;
    }

    try {
      const response = await api.get('/api/user/available_models');
      const payload = response.data;

      if (payload?.success && Array.isArray(payload.data)) {
        const unique = Array.from(
          new Set(
            (payload.data as Array<unknown>)
              .map((model) => (typeof model === 'string' ? model.trim() : ''))
              .filter((model): model is string => model.length > 0)
          )
        );
        userAvailableModelsCache.current = unique;
        return unique;
      }
    } catch {
      // noop; caller will surface a notification if needed.
    }

    userAvailableModelsCache.current = [];
    return [];
  }, []);

  selectedModelRef.current = selectedModel;

  useEffect(() => {
    const fetchChannelModels = async () => {
      setIsLoadingChannels(true);
      try {
        const response = await api.get('/api/models/display');
        const payload = response.data;

        if (payload?.success && payload.data && typeof payload.data === 'object') {
          const normalized: Record<string, string[]> = {};
          Object.entries(payload.data as Record<string, unknown>).forEach(([channelName, rawInfo]) => {
            if (rawInfo && typeof rawInfo === 'object' && 'models' in rawInfo) {
              const modelsInfo = (rawInfo as { models?: Record<string, unknown> }).models;
              if (modelsInfo && typeof modelsInfo === 'object') {
                normalized[channelName] = Object.keys(modelsInfo);
              }
            }
          });
          setChannelModelMap(normalized);
        } else {
          setChannelModelMap({});
          notify({
            title: t('playground.notifications.error_title'),
            message: t('playground.notifications.channel_metadata_error'),
            type: 'error',
          });
        }
      } catch {
        setChannelModelMap({});
        notify({
          title: t('playground.notifications.error_title'),
          message: t('playground.notifications.channel_metadata_error'),
          type: 'error',
        });
      } finally {
        setIsLoadingChannels(false);
      }
    };

    fetchChannelModels();
  }, [notify, t]);

  const channelLabelMap = useMemo(() => {
    const map = new Map<string, string>();
    Object.keys(channelModelMap).forEach((channelName) => {
      map.set(channelName, formatChannelName(channelName));
    });
    return map;
  }, [channelModelMap]);

  const channelOptions = useMemo<SuggestionOption[]>(() => {
    return Array.from(channelLabelMap.entries())
      .map(([key, label]) => ({ key, label }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [channelLabelMap]);

  const channelSuggestions = useMemo<SuggestionOption[]>(() => {
    if (channelOptions.length === 0) {
      return [];
    }

    const query = channelInputValue.trim().toLowerCase();
    if (!query) {
      return channelOptions.slice(0, 12);
    }

    const scored = channelOptions
      .map((option) => {
        const labelLower = option.label.toLowerCase();
        const keyLower = option.key.toLowerCase();
        const labelIndex = labelLower.indexOf(query);
        const keyIndex = keyLower.indexOf(query);
        if (labelIndex === -1 && keyIndex === -1) {
          return null;
        }
        const score = Math.min(
          labelIndex === -1 ? Number.POSITIVE_INFINITY : labelIndex,
          keyIndex === -1 ? Number.POSITIVE_INFINITY : keyIndex
        );
        return { option, score };
      })
      .filter((entry): entry is { option: SuggestionOption; score: number } => entry !== null);

    scored.sort((a, b) => {
      if (a.score !== b.score) {
        return a.score - b.score;
      }
      return a.option.label.localeCompare(b.option.label);
    });

    return scored.slice(0, 12).map((entry) => entry.option);
  }, [channelOptions, channelInputValue]);

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
          const base = visibleChannels.join(', ');
          const summary = remaining > 0 ? `${base}, ${t('playground.parameters.model.more_channels', { count: remaining })}` : base;
          description = t('playground.parameters.model.channels_label', {
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

    const sortedOptions = options.slice().sort((a, b) => a.label.localeCompare(b.label));

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
      setSelectedModel('');
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
    setSelectedModel('');
    setModelInputValue('');
  }, []);

  const handleChannelQueryChange = useCallback(
    (value: string) => {
      setChannelInputValue(value);
      if (selectedChannel) {
        setSelectedChannel('');
      }
      channelErrorRef.current = null;
    },
    [selectedChannel]
  );

  const handleChannelSelect = useCallback(
    (channelKey: string) => {
      if (!channelKey) {
        setSelectedChannel('');
        setChannelInputValue('');
        channelErrorRef.current = null;
        return;
      }
      setSelectedChannel(channelKey);
      setChannelInputValue(channelLabelMap.get(channelKey) ?? formatChannelName(channelKey));
      channelErrorRef.current = null;
    },
    [channelLabelMap]
  );

  const handleChannelClear = useCallback(() => {
    handleChannelSelect('');
  }, [handleChannelSelect]);

  useEffect(() => {
    if (selectedChannel && !channelModelMap[selectedChannel]) {
      setSelectedChannel('');
      setChannelInputValue('');
      channelErrorRef.current = null;
    }
  }, [selectedChannel, channelModelMap]);

  const loadTokens = useCallback(async () => {
    setIsLoadingTokens(true);
    try {
      const res = await api.get('/api/token/?p=0&size=5');
      const data = res.data;

      if (data.success && data.data) {
        const enabledTokens = data.data.filter((t: Token) => t.status === TOKEN_STATUS.ENABLED);
        setTokens(enabledTokens);

        if (enabledTokens.length > 0 && !selectedToken) {
          setSelectedToken(enabledTokens[0].key);
        }
      } else {
        setTokens([]);
      }
    } catch {
      notify({
        title: t('playground.notifications.error_title'),
        message: t('playground.notifications.load_tokens_error'),
        type: 'error',
      });
      setTokens([]);
    } finally {
      setIsLoadingTokens(false);
    }
  }, [notify, selectedToken, t]);

  useEffect(() => {
    loadTokens();
  }, [loadTokens]);

  useEffect(() => {
    const loadModels = async () => {
      setIsLoadingModels(true);
      try {
        if (!selectedToken) {
          setModels([]);
          setSelectedModel('');
          setModelInputValue('');
          return;
        }

        const token = tokens.find((t) => t.key === selectedToken);
        if (!token) {
          setModels([]);
          setSelectedModel('');
          setModelInputValue('');
          return;
        }

        const rawModels = typeof token.models === 'string' ? token.models : '';
        let modelNames = rawModels
          .split(',')
          .map((name) => name.trim())
          .filter((name) => name.length > 0);

        let ownedBy = 'token-restricted';
        if (modelNames.length === 0) {
          const fallback = await fetchUserAvailableModels();
          modelNames = fallback;
          ownedBy = 'user-entitlement';
        }

        const baseModels = new Set(modelNames.filter((name) => name.length > 0));
        if (baseModels.size === 0) {
          setModels([]);
          setSelectedModel('');
          setModelInputValue('');
          channelErrorRef.current = null;
          notify({
            title: t('playground.notifications.no_models_title'),
            message: t('playground.notifications.no_models_token_error'),
            type: 'error',
          });
          return;
        }

        const hasChannelData = Object.keys(channelModelMap).length > 0;
        let transformedModels: PlaygroundModel[] = [];

        if (hasChannelData) {
          const relevantChannelKeys = selectedChannel ? [selectedChannel] : Object.keys(channelModelMap);

          const modelChannelLabels = new Map<string, Set<string>>();

          for (const channelKey of relevantChannelKeys) {
            const channelModels = channelModelMap[channelKey] ?? [];
            if (!Array.isArray(channelModels) || channelModels.length === 0) {
              continue;
            }

            const channelLabel = channelLabelMap.get(channelKey) ?? formatChannelName(channelKey);
            for (const modelName of channelModels) {
              if (!baseModels.has(modelName)) {
                continue;
              }
              if (!modelChannelLabels.has(modelName)) {
                modelChannelLabels.set(modelName, new Set<string>());
              }
              modelChannelLabels.get(modelName)?.add(channelLabel);
            }
          }

          const buildLabel = (modelName: string, channelNames: string[]): string => {
            if (channelNames.length === 0) {
              return modelName;
            }

            if (selectedChannel || channelNames.length === 1) {
              return `${modelName} (${channelNames[0]})`;
            }

            if (channelNames.length === 2) {
              return `${modelName} (${channelNames[0]} · ${channelNames[1]})`;
            }

            const visible = channelNames.slice(0, 2).join(' · ');
            return `${modelName} (${visible} +${channelNames.length - 2} more)`;
          };

          transformedModels = Array.from(modelChannelLabels.entries()).map(([modelName, channelSet]) => {
            const channelNames = Array.from(channelSet).sort((a, b) => a.localeCompare(b));
            return {
              id: modelName,
              object: 'model',
              owned_by: ownedBy,
              label: buildLabel(modelName, channelNames),
              channels: channelNames,
            };
          });

          if (!selectedChannel) {
            for (const modelName of baseModels) {
              if (!modelChannelLabels.has(modelName)) {
                transformedModels.push({
                  id: modelName,
                  object: 'model',
                  owned_by: ownedBy,
                  label: modelName,
                });
              }
            }
          }

          transformedModels.sort((a, b) => (a.label ?? a.id).localeCompare(b.label ?? b.id));
        } else {
          transformedModels = Array.from(baseModels)
            .sort()
            .map((modelName) => ({
              id: modelName,
              object: 'model',
              owned_by: ownedBy,
              label: modelName,
            }));
        }

        if (selectedChannel && transformedModels.length === 0) {
          if (channelErrorRef.current !== selectedChannel) {
            const channelLabel = channelLabelMap.get(selectedChannel) ?? formatChannelName(selectedChannel);
            notify({
              title: t('playground.notifications.no_models_title'),
              message: t('playground.notifications.no_models_channel_error', {
                channel: channelLabel,
              }),
              type: 'error',
            });
            channelErrorRef.current = selectedChannel;
          }
          setModels([]);
          setSelectedModel('');
          setModelInputValue('');
          return;
        }

        channelErrorRef.current = null;

        if (transformedModels.length === 0) {
          setModels([]);
          setSelectedModel('');
          setModelInputValue('');
          return;
        }

        setModels(transformedModels);

        const availableIds = new Set(transformedModels.map((model) => model.id));

        const resolveLabel = (modelId: string): string => {
          const match = transformedModels.find((model) => model.id === modelId);
          return match?.label ?? modelId;
        };

        if (selectedModelRef.current && availableIds.has(selectedModelRef.current)) {
          setModelInputValue(resolveLabel(selectedModelRef.current));
          return;
        }

        const savedModel = loadFromStorage(STORAGE_KEYS.MODEL, '');
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
          title: t('playground.notifications.error_title'),
          message: t('playground.notifications.load_models_error'),
          type: 'error',
        });
        setModels([]);
        setSelectedModel('');
        setModelInputValue('');
      } finally {
        setIsLoadingModels(false);
      }
    };

    if (tokens.length > 0) {
      void loadModels();
    } else if (!isLoadingTokens) {
      setIsLoadingModels(false);
      setModels([]);
      setSelectedModel('');
      setModelInputValue('');
      notify({
        title: t('playground.notifications.no_tokens_title'),
        message: t('playground.notifications.no_tokens_error'),
        type: 'error',
      });
    }
  }, [selectedToken, tokens, isLoadingTokens, notify, fetchUserAvailableModels, selectedChannel, channelModelMap, channelLabelMap, t]);

  useEffect(() => {
    if (selectedModel) {
      saveToStorage(STORAGE_KEYS.MODEL, selectedModel);
    }
  }, [selectedModel]);

  useEffect(() => {
    if (selectedToken) {
      saveToStorage(STORAGE_KEYS.TOKEN, selectedToken);
    }
  }, [selectedToken]);

  return {
    tokens,
    isLoadingTokens,
    models,
    isLoadingModels,
    isLoadingChannels,
    selectedToken,
    setSelectedToken,
    selectedModel,
    setSelectedModel,
    selectedChannel,
    channelInputValue,
    modelInputValue,
    channelSuggestions,
    modelSuggestions,
    handleChannelQueryChange,
    handleChannelSelect,
    handleChannelClear,
    handleModelQueryChange,
    handleModelSelect,
    handleModelClear,
  };
};
