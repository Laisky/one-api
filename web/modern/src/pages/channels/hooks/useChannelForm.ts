import { useNotifications } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { parseGroupList } from '@/lib/group-name';
import { zodResolver } from '@/lib/zod-resolver';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { CHANNEL_TYPES_WITH_DEDICATED_BASE_URL } from '../constants';
import {
  buildChannelSubmitPayload,
  CONFIG_OVERRIDES_OF_OTHER,
  isValidJSON,
  normalizeChannelType,
  parsePrivateChannelJSON,
  formatPrivateChannelJSONField,
  logPrivateChannelJSONError,
  sanitizeJsonInput,
  stringifyToolingConfig,
  toInt,
  validateModelConfigs,
} from '../helpers';
import { createChannelSchema, type ChannelConfigForm, type ChannelForm, type ChannelFormInput, type EndpointInfo } from '../schemas';
import { collectChannelSaveWarnings, type ChannelPricingGap, type ChannelSaveWarnings } from './channelSaveWarnings';
/** useChannelForm manages channel loading, validation, submission, and related UI state, returning the complete page model. */
export const useChannelForm = () => {
  const params = useParams();
  const channelId = params.id;
  const isEdit = channelId !== undefined;
  const navigate = useNavigate();
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`channels.edit.${key}`, { defaultValue, ...options }),
    [t]
  );
  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [modelsCatalog, setModelsCatalog] = useState<Record<number, string[]>>({});
  const [defaultPricing, setDefaultPricing] = useState<string>('');
  const [defaultTooling, setDefaultTooling] = useState<string>('');
  const [defaultBaseURL, setDefaultBaseURL] = useState<string>('');
  const [baseURLEditable, setBaseURLEditable] = useState<boolean>(true);
  const [defaultEndpoints, setDefaultEndpoints] = useState<string[]>([]);
  const [allEndpoints, setAllEndpoints] = useState<EndpointInfo[]>([]);
  const [formInitialized, setFormInitialized] = useState(!isEdit);
  const [loadedChannelType, setLoadedChannelType] = useState<number | null>(null);
  // The channel exactly as loaded for editing; save warnings report only what the edit introduces relative to it.
  const loadedChannelRef = useRef<ChannelForm | null>(null);
  // State for channel type change confirmation dialog
  const [pendingTypeChange, setPendingTypeChange] = useState<{
    fromType: number;
    toType: number;
  } | null>(null);
  // State for the save warning dialog when Model Mapping entries look suspicious or models lack pricing.
  const [pendingSaveConfirmation, setPendingSaveConfirmation] = useState<{
    data: ChannelForm;
    unreachableMappingKeys: string[];
    unknownMappingTargets: { source: string; target: string }[];
    pricingGaps: ChannelPricingGap[];
  } | null>(null);
  const schema = useMemo(() => createChannelSchema((key, defaultValue) => tr(key, defaultValue)), [tr]);
  const form = useForm<ChannelFormInput, unknown, ChannelForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      type: isEdit ? 1 : (undefined as unknown as number),
      key: '',
      base_url: '',
      other: '',
      models: [],
      hidden_models: [],
      model_mapping: '',
      model_configs: '',
      tooling: '',
      system_prompt: '',
      groups: ['default'],
      priority: 0,
      weight: 0,
      ratelimit: 0,
      config: {
        region: '',
        ak: '',
        sk: '',
        user_id: '',
        vertex_ai_project_id: '',
        vertex_ai_adc: '',
        auth_type: 'personal_access_token',
        api_format: 'chat_completion',
        supported_endpoints: [],
        endpoint_urls: {},
        mcp_tool_blacklist: [],
        custom_headers: {},
        spark_app_id: '',
        spark_api_secret: '',
        spark_api_key: '',
        tencent_app_id: '',
        tencent_secret_id: '',
        tencent_secret_key: '',
      },
      inference_profile_arn_map: '',
    },
  });
  const watchType = form.watch('type');
  const watchConfig: ChannelConfigForm = {
    auth_type: 'personal_access_token',
    api_format: 'chat_completion',
    ...(form.watch('config') ?? {}),
  };
  const watchTooling = form.watch('tooling') ?? '';
  const normalizedChannelType = useMemo(() => normalizeChannelType(watchType), [watchType]);

  const loadDefaultPricing = useCallback(async (channelType: number) => {
    try {
      setDefaultPricing('');
      setDefaultTooling('');
      const response = await api.get(`/api/channel/default-pricing?type=${channelType}`);
      const { success, data } = response.data;
      if (success) {
        if (data?.model_configs) {
          try {
            const parsed = parsePrivateChannelJSON(data.model_configs);
            const formatted = JSON.stringify(parsed, null, 2);
            setDefaultPricing(formatted);
          } catch (_e) {
            setDefaultPricing(data.model_configs);
          }
        } else {
          setDefaultPricing('');
        }

        if (typeof data?.tooling === 'string' && data.tooling.trim() !== '') {
          try {
            const parsedTooling = parsePrivateChannelJSON(data.tooling);
            setDefaultTooling(stringifyToolingConfig(parsedTooling));
          } catch (_e) {
            setDefaultTooling(data.tooling);
          }
        } else {
          setDefaultTooling(stringifyToolingConfig({ whitelist: [], pricing: {} }));
        }
      }
    } catch (error) {
      console.error(`Error loading default pricing: ${error instanceof Error ? error.message : String(error)}`);
    }
  }, []);

  const { reset, setValue, getValues } = form;

  const loadChannel = useCallback(async () => {
    if (!channelId) return;

    try {
      const response = await api.get(`/api/channel/${channelId}`);
      const { success, message, data } = response.data;

      if (success && data) {
        let models: string[] = [];
        if (data.models && typeof data.models === 'string' && data.models.trim() !== '') {
          models = data.models
            .split(',')
            .map((model: string) => model.trim())
            .filter((model: string) => model !== '');
        }

        // Parse like the backend (trim, drop empties, case-insensitive de-dup) so the form shows the effective groups.
        const storedGroups = typeof data.group === 'string' ? parseGroupList(data.group) : [];
        const groups: string[] = storedGroups.length > 0 ? storedGroups : ['default'];

        let config: ChannelConfigForm = {
          region: '',
          ak: '',
          sk: '',
          user_id: '',
          vertex_ai_project_id: '',
          vertex_ai_adc: '',
          auth_type: 'personal_access_token',
          api_format: 'chat_completion',
          supported_endpoints: [],
          endpoint_urls: {},
          mcp_tool_blacklist: [],
          custom_headers: {},
          spark_app_id: '',
          spark_api_secret: '',
          spark_api_key: '',
          tencent_app_id: '',
          tencent_secret_id: '',
          tencent_secret_key: '',
        };
        if (data.config && typeof data.config === 'string' && data.config.trim() !== '') {
          try {
            const parsed = parsePrivateChannelJSON(data.config) as Partial<ChannelConfigForm>;
            config = {
              ...config,
              ...parsed,
              api_format: parsed.api_format === 'response' ? 'response' : 'chat_completion',
              supported_endpoints: Array.isArray(parsed.supported_endpoints) ? parsed.supported_endpoints : [],
              endpoint_urls:
                parsed.endpoint_urls && typeof parsed.endpoint_urls === 'object' && !Array.isArray(parsed.endpoint_urls)
                  ? Object.fromEntries(
                      Object.entries(parsed.endpoint_urls)
                        .map(([key, value]) => [key, typeof value === 'string' ? value.trim() : String(value ?? '').trim()])
                        .filter(([, value]) => value !== '')
                    )
                  : {},
              mcp_tool_blacklist: Array.isArray(parsed.mcp_tool_blacklist) ? parsed.mcp_tool_blacklist : [],
              custom_headers:
                parsed.custom_headers && typeof parsed.custom_headers === 'object' && !Array.isArray(parsed.custom_headers)
                  ? Object.fromEntries(
                      Object.entries(parsed.custom_headers).map(([key, value]) => [
                        key,
                        typeof value === 'string' ? value : String(value ?? ''),
                      ])
                    )
                  : {},
            };
          } catch (e) {
            logPrivateChannelJSONError(e);
          }
        }

        const parseStringArrayField = (field: unknown): string[] => {
          if (Array.isArray(field)) {
            return field
              .filter((item): item is string => typeof item === 'string')
              .map((item) => item.trim())
              .filter((item) => item !== '');
          }
          if (typeof field !== 'string' || field.trim() === '') {
            return [];
          }
          try {
            const parsed = parsePrivateChannelJSON(field);
            if (!Array.isArray(parsed)) {
              return [];
            }
            return parsed
              .filter((item): item is string => typeof item === 'string')
              .map((item) => item.trim())
              .filter((item) => item !== '');
          } catch (_e) {
            return [];
          }
        };

        const channelType = toInt(data.type, 1);

        // Spark (18): key is APPID|APISecret|APIKey. Hydrate config inputs from
        // existing key so admins can edit each part individually.
        if (channelType === 18 && typeof data.key === 'string' && data.key.includes('|')) {
          const [appId = '', apiSecret = '', apiKey = ''] = data.key.split('|');
          if (!config.spark_app_id) config.spark_app_id = appId;
          if (!config.spark_api_secret) config.spark_api_secret = apiSecret;
          if (!config.spark_api_key) config.spark_api_key = apiKey;
        }
        // Tencent (23): key is AppId|SecretId|SecretKey.
        if (channelType === 23 && typeof data.key === 'string' && data.key.includes('|')) {
          const [appId = '', secretId = '', secretKey = ''] = data.key.split('|');
          if (!config.tencent_app_id) config.tencent_app_id = appId;
          if (!config.tencent_secret_id) config.tencent_secret_id = secretId;
          if (!config.tencent_secret_key) config.tencent_secret_key = secretKey;
        }
        let toolingField = '';
        if (data.tooling && typeof data.tooling === 'string' && data.tooling.trim() !== '') {
          try {
            const parsedTooling = parsePrivateChannelJSON(data.tooling);
            toolingField = stringifyToolingConfig(parsedTooling);
          } catch (_e) {
            toolingField = data.tooling;
          }
        }

        const formData: ChannelForm = {
          name: data.name || '',
          type: channelType,
          key: data.key || '',
          base_url: data.base_url || '',
          other: data.other || '',
          models,
          hidden_models: parseStringArrayField(data.hidden_models),
          model_mapping: formatPrivateChannelJSONField(data.model_mapping),
          model_configs: formatPrivateChannelJSONField(data.model_configs),
          tooling: toolingField,
          system_prompt: data.system_prompt || '',
          groups,
          priority: toInt(data.priority, 0),
          weight: toInt(data.weight, 0),
          ratelimit: toInt(data.ratelimit, 0),
          config,
          inference_profile_arn_map: formatPrivateChannelJSONField(data.inference_profile_arn_map),
        };

        console.debug(
          `[EditChannel] Loaded channel payload channelId=${data.id ?? channelId} channelType=${channelType} ` +
            `hasModelMapping=${Boolean(data.model_mapping)} ` +
            `modelMappingLength=${typeof data.model_mapping === 'string' ? data.model_mapping.length : 0} ` +
            `hasModelConfigs=${Boolean(data.model_configs)} hasSystemPrompt=${Boolean(data.system_prompt)}`
        );

        if (channelType) {
          await loadDefaultPricing(channelType);
        }

        setLoadedChannelType(channelType);
        loadedChannelRef.current = formData;
        reset(formData);
        await new Promise((resolve) => setTimeout(resolve, 0));

        const currentTypeValue = getValues('type');
        if (currentTypeValue !== channelType) {
          setValue('type', channelType, {
            shouldValidate: true,
            shouldDirty: false,
          });
          await new Promise((resolve) => setTimeout(resolve, 0));
        }

        setFormInitialized(true);
      } else {
        throw new Error(message || 'Failed to load channel');
      }
    } catch (error) {
      console.error(`Error loading channel: ${error instanceof Error ? error.message : String(error)}`);
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, loadDefaultPricing]);

  const loadModelsCatalog = useCallback(async () => {
    try {
      const response = await api.get('/api/models');
      const { success, data } = response.data;

      if (success && data) {
        const catalog: Record<number, string[]> = {};
        Object.entries(data).forEach(([typeKey, models]) => {
          if (!Array.isArray(models)) return;
          const typeId = Number(typeKey);
          if (!Number.isFinite(typeId)) return;
          catalog[typeId] = (models as string[]).filter((model) => typeof model === 'string' && model.trim() !== '');
        });
        setModelsCatalog(catalog);
      }
    } catch (error) {
      console.error(`Error loading models catalog: ${error instanceof Error ? error.message : String(error)}`);
    }
  }, []);

  // The group catalog is loaded by ChannelGroups itself through useGroupCatalog.
  useEffect(() => {
    loadModelsCatalog();
  }, [loadModelsCatalog]);

  useEffect(() => {
    if (isEdit) {
      loadChannel();
    } else {
      setLoading(false);
    }
  }, [isEdit, loadChannel]);

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      try {
        setDefaultBaseURL('');
        setBaseURLEditable(true);
        setDefaultEndpoints([]);
        setAllEndpoints([]);
        if (normalizedChannelType === null) return;
        const res = await api.get(`/api/channel/metadata?type=${normalizedChannelType}`);
        const base = (res.data?.data?.default_base_url as string) || '';
        const editable = res.data?.data?.base_url_editable !== false;
        const defEndpoints = (res.data?.data?.default_endpoints as string[]) || [];
        const allEndpointsData = (res.data?.data?.all_endpoints as EndpointInfo[]) || [];
        if (!cancelled) {
          setDefaultBaseURL(base);
          setBaseURLEditable(editable);
          setDefaultEndpoints(defEndpoints);
          setAllEndpoints(allEndpointsData);
        }
      } catch (_) {
        // ignore
      }
    };
    run();
    return () => {
      cancelled = true;
    };
  }, [normalizedChannelType]);

  // Removed: useEffect that prevented channel type changes when editing
  // Channel type changes are now allowed with a confirmation dialog

  useEffect(() => {
    if (normalizedChannelType !== null) {
      loadDefaultPricing(normalizedChannelType);
    }
  }, [normalizedChannelType, loadDefaultPricing]);

  /**
   * onSubmit intercepts a validated save so admins can review Model Mapping entries that look wrong
   * and supported models that request admission would refuse for missing channel pricing; it takes
   * the form data and either opens the save warning dialog or submits. When editing a channel of an
   * unchanged type, only warnings the edit introduced are shown. The pricing check is advisory and
   * fails open, so it never blocks saving by itself.
   */
  const onSubmit = async (data: ChannelForm) => {
    setIsSubmitting(true);
    let warnings: ChannelSaveWarnings;
    try {
      warnings = await collectChannelSaveWarnings(data, {
        baseline: isEdit ? loadedChannelRef.current : null,
        modelsCatalog,
        payloadOptions: { isEdit, watchType: data.type, watchConfig },
      });
    } finally {
      setIsSubmitting(false);
    }
    const { unreachableKeys, unknownTargets, pricingGaps } = warnings;
    if (unreachableKeys.length > 0 || unknownTargets.length > 0 || pricingGaps.length > 0) {
      setPendingSaveConfirmation({
        data,
        unreachableMappingKeys: unreachableKeys,
        unknownMappingTargets: unknownTargets,
        pricingGaps,
      });
      return;
    }
    await performSubmit(data);
  };

  const performSubmit = async (data: ChannelForm) => {
    setIsSubmitting(true);
    try {
      // API Key is optional for every channel type: an empty key is accepted on
      // both create and edit, so no presence validation is enforced here.

      if (data.model_mapping && !isValidJSON(data.model_mapping)) {
        form.setError('model_mapping', {
          message: tr('validation.model_mapping_invalid', 'Model Mapping has invalid JSON.'),
        });
        notify({
          type: 'error',
          title: tr('validation.invalid_json_title', 'Invalid JSON'),
          message: tr('validation.model_mapping_invalid', 'Model Mapping has invalid JSON.'),
        });
        return;
      }

      if (data.model_configs) {
        const validation = validateModelConfigs(data.model_configs);
        if (!validation.valid) {
          const validationError =
            validation.errorKey && validation.error ? tr(validation.errorKey, validation.error, validation.errorParams) : validation.error;
          form.setError('model_configs', {
            message: validationError || tr('model_configs.invalid', 'Invalid model configs format'),
          });
          notify({
            type: 'error',
            title: tr('validation.model_configs_title', 'Invalid configs'),
            message: validationError || tr('validation.model_configs_message', 'Model Configs are invalid.'),
          });
          return;
        }
      }

      if (data.inference_profile_arn_map && !isValidJSON(data.inference_profile_arn_map)) {
        form.setError('inference_profile_arn_map', {
          message: tr('validation.inference_profile_invalid', 'Inference Profile ARN Map has invalid JSON.'),
        });
        notify({
          type: 'error',
          title: tr('validation.invalid_json_title', 'Invalid JSON'),
          message: tr('validation.inference_profile_invalid', 'Inference Profile ARN Map has invalid JSON.'),
        });
        return;
      }

      // Only validate the Coze OAuth JWT payload when a key was actually
      // provided; an empty key is permitted like every other channel type.
      if (data.type === 34 && watchConfig.auth_type === 'oauth_jwt' && data.key && data.key.trim() !== '') {
        if (!isValidJSON(data.key)) {
          form.setError('key', {
            message: tr('validation.oauth_invalid_json', 'OAuth JWT configuration JSON is invalid.'),
          });
          notify({
            type: 'error',
            title: tr('validation.invalid_json_title', 'Invalid JSON'),
            message: tr('validation.oauth_invalid_json', 'OAuth JWT configuration JSON is invalid.'),
          });
          return;
        }

        try {
          const oauthConfig = parsePrivateChannelJSON(sanitizeJsonInput(data.key));
          const requiredFields = ['client_type', 'client_id', 'coze_www_base', 'coze_api_base', 'private_key', 'public_key_id'];

          for (const field of requiredFields) {
            if (!Object.hasOwn(oauthConfig, field)) {
              form.setError('key', {
                message: tr('validation.oauth_missing_field_message', 'OAuth JWT configuration missing: {{field}}', { field }),
              });
              notify({
                type: 'error',
                title: tr('validation.oauth_missing_field_title', 'Missing field'),
                message: tr('validation.oauth_missing_field_message', 'OAuth JWT configuration missing: {{field}}', { field }),
              });
              return;
            }

            const selectedEndpoints = data.config.supported_endpoints || [];
            const effectiveEndpoints = selectedEndpoints.length === 0 ? defaultEndpoints : selectedEndpoints;

            if (effectiveEndpoints.length === 0) {
              form.setError('config.supported_endpoints', {
                message: tr('validation.endpoints_required', 'Enable at least one endpoint before saving.'),
              });
              notify({
                type: 'error',
                title: tr('validation.error_title', 'Validation error'),
                message: tr('validation.endpoints_required', 'Enable at least one endpoint before saving.'),
              });
              return;
            }
          }
        } catch (error) {
          form.setError('key', {
            message: tr('validation.oauth_parse_message', 'OAuth JWT parse error: {{error}}', { error: (error as Error).message }),
          });
          notify({
            type: 'error',
            title: tr('validation.oauth_parse_title', 'Parse error'),
            message: tr('validation.oauth_parse_message', 'OAuth JWT parse error: {{error}}', { error: (error as Error).message }),
          });
          return;
        }
      }

      const normalizedSubmitType = normalizeChannelType(data.type);
      const baseURLRawValue = typeof data.base_url === 'string' ? data.base_url : '';
      const trimmedBaseURL = baseURLRawValue.trim();
      const baseURLRequired = normalizedSubmitType !== null && CHANNEL_TYPES_WITH_DEDICATED_BASE_URL.has(normalizedSubmitType);

      if (baseURLRequired && !trimmedBaseURL) {
        form.setError('base_url', {
          message: tr('validation.base_url_required', 'Base URL is required for this channel type.'),
        });
        notify({
          type: 'error',
          title: tr('validation.error_title', 'Validation error'),
          message: tr('validation.base_url_required', 'Base URL is required for this channel type.'),
        });
        return;
      }
      form.clearErrors('base_url');

      const payload = buildChannelSubmitPayload(data, {
        isEdit,
        watchType: data.type,
        watchConfig,
      });

      let response;
      if (isEdit && channelId) {
        response = await api.put('/api/channel/', {
          ...payload,
          uuid: channelId,
        });
      } else {
        response = await api.post('/api/channel/', payload);
      }

      const { success, message } = response.data;
      if (success) {
        navigate('/channels', {
          state: {
            message: isEdit ? 'Channel updated successfully' : 'Channel created successfully',
          },
        });
      } else {
        form.setError('root', { message: message || tr('errors.operation_failed', 'Operation failed') });
        notify({
          type: 'error',
          title: tr('errors.request_failed_title', 'Request failed'),
          message: message || tr('errors.operation_failed', 'Operation failed'),
        });
      }
    } catch (error) {
      form.setError('root', {
        message: error instanceof Error ? error.message : tr('errors.operation_failed', 'Operation failed'),
      });
      notify({
        type: 'error',
        title: tr('errors.unexpected_title', 'Unexpected error'),
        message: error instanceof Error ? error.message : tr('errors.operation_failed', 'Operation failed'),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  /**
   * Initiates a channel type change request.
   * When in edit mode, this sets up a pending change that requires confirmation.
   * When creating a new channel, the change is applied immediately.
   */
  const requestTypeChange = useCallback(
    (newType: number) => {
      const currentType = normalizeChannelType(getValues('type')) ?? 1;
      if (isEdit && loadedChannelType !== null && currentType !== newType) {
        // In edit mode, show confirmation dialog
        setPendingTypeChange({
          fromType: currentType,
          toType: newType,
        });
      } else {
        // In create mode or same type, just set the value
        setValue('type', newType, {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
    },
    [isEdit, loadedChannelType, getValues, setValue]
  );

  /**
   * Confirms a pending type change and applies it to the form.
   */
  const confirmTypeChange = useCallback(() => {
    if (pendingTypeChange) {
      setValue('type', pendingTypeChange.toType, {
        shouldValidate: true,
        shouldDirty: true,
      });
      // Clear related fields that may not be compatible with the new type, including the typed
      // config overrides of `other`, which the relay would otherwise apply to the new type.
      setValue('base_url', '');
      setValue('other', '');
      const config: Record<string, unknown> = { ...(getValues('config') ?? {}) };
      CONFIG_OVERRIDES_OF_OTHER.forEach((key) => delete config[key]);
      setValue('config', config as ChannelFormInput['config']);
      setPendingTypeChange(null);
    }
  }, [pendingTypeChange, setValue, getValues]);

  /**
   * Cancels a pending type change and reverts the selection.
   */
  const cancelTypeChange = useCallback(() => {
    setPendingTypeChange(null);
  }, []);

  /**
   * Confirms the pending save (dismissing the Model Mapping and pricing warnings) and submits.
   */
  const confirmSave = useCallback(async () => {
    const pending = pendingSaveConfirmation;
    if (!pending) return;
    setPendingSaveConfirmation(null);
    await performSubmit(pending.data);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingSaveConfirmation]);

  /**
   * Dismisses the pending save confirmation without submitting.
   */
  const cancelSave = useCallback(() => {
    setPendingSaveConfirmation(null);
  }, []);

  const testChannel = async () => {
    if (!channelId) return;

    try {
      setIsSubmitting(true);
      const response = await api.post(`/api/channel/test/${channelId}`);
      const { success, message, skipped } = response.data;

      if (success) {
        notify({
          type: 'success',
          title: tr('test.success_title', 'Success'),
          message: tr('test.success_message', 'Channel test successful!'),
        });
      } else if (skipped) {
        // The channel exposes no chat-capable endpoint, so no representative
        // probe exists. That is not a failure and must not read like one.
        notify({
          type: 'info',
          title: tr('test.skipped_title', 'Skipped'),
          message: tr('test.skipped_message', 'Channel test skipped: {{message}}', {
            message: message || 'this channel serves no chat-capable endpoint',
          }),
        });
      } else {
        notify({
          type: 'error',
          title: tr('test.failed_title', 'Failed'),
          message: tr('test.failed_message', 'Channel test failed: {{message}}', { message: message || 'Unknown error' }),
        });
      }
    } catch (error) {
      notify({
        type: 'error',
        title: tr('test.error_title', 'Error'),
        message: tr('test.error_message', 'Channel test failed: {{error}}', {
          error: error instanceof Error ? error.message : 'Network error',
        }),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  return {
    form,
    isEdit,
    channelId,
    loading,
    isSubmitting,
    modelsCatalog,
    defaultPricing,
    defaultTooling,
    defaultBaseURL,
    baseURLEditable,
    defaultEndpoints,
    allEndpoints,
    formInitialized,
    loadedChannelType,
    normalizedChannelType,
    watchType,
    watchConfig,
    watchTooling,
    onSubmit,
    testChannel,
    tr,
    notify,
    // Type change handling
    pendingTypeChange,
    requestTypeChange,
    confirmTypeChange,
    cancelTypeChange,
    // Save warning handling (suspicious Model Mapping entries and models lacking channel pricing)
    pendingSaveConfirmation,
    confirmSave,
    cancelSave,
  };
};
