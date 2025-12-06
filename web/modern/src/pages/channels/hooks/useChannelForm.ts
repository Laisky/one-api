import { useNotifications } from "@/components/ui/notifications";
import { api } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { CHANNEL_TYPES_WITH_DEDICATED_BASE_URL } from "../constants";
import {
  isValidJSON,
  normalizeChannelType,
  stringifyToolingConfig,
  toInt,
  validateModelConfigs,
} from "../helpers";
import {
  type ChannelConfigForm,
  type ChannelForm,
  type EndpointInfo,
  channelSchema,
} from "../schemas";

export const useChannelForm = () => {
  const params = useParams();
  const channelId = params.id;
  const isEdit = channelId !== undefined;
  const navigate = useNavigate();
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`channels.edit.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [modelsCatalog, setModelsCatalog] = useState<Record<number, string[]>>(
    {}
  );
  const [groups, setGroups] = useState<string[]>([]);
  const [defaultPricing, setDefaultPricing] = useState<string>("");
  const [defaultTooling, setDefaultTooling] = useState<string>("");
  const [defaultBaseURL, setDefaultBaseURL] = useState<string>("");
  const [baseURLEditable, setBaseURLEditable] = useState<boolean>(true);
  const [defaultEndpoints, setDefaultEndpoints] = useState<string[]>([]);
  const [allEndpoints, setAllEndpoints] = useState<EndpointInfo[]>([]);
  const [formInitialized, setFormInitialized] = useState(!isEdit);
  const [loadedChannelType, setLoadedChannelType] = useState<number | null>(
    null
  );
  // State for channel type change confirmation dialog
  const [pendingTypeChange, setPendingTypeChange] = useState<{
    fromType: number;
    toType: number;
  } | null>(null);

  const form = useForm<ChannelForm>({
    resolver: zodResolver(channelSchema),
    defaultValues: {
      name: "",
      type: isEdit ? 1 : (undefined as unknown as number),
      key: "",
      base_url: "",
      other: "",
      models: [],
      model_mapping: "",
      model_configs: "",
      tooling: "",
      system_prompt: "",
      groups: ["default"],
      priority: 0,
      weight: 0,
      ratelimit: 0,
      config: {
        region: "",
        ak: "",
        sk: "",
        user_id: "",
        vertex_ai_project_id: "",
        vertex_ai_adc: "",
        auth_type: "personal_access_token",
        api_format: "chat_completion",
        supported_endpoints: [],
      },
      inference_profile_arn_map: "",
    },
  });

  const watchType = form.watch("type");
  const watchConfig = form.watch("config");
  const watchTooling = form.watch("tooling") ?? "";

  const normalizedChannelType = useMemo(
    () => normalizeChannelType(watchType),
    [watchType]
  );

  const loadDefaultPricing = useCallback(async (channelType: number) => {
    try {
      setDefaultPricing("");
      setDefaultTooling("");
      const response = await api.get(
        `/api/channel/default-pricing?type=${channelType}`
      );
      const { success, data } = response.data;
      if (success) {
        if (data?.model_configs) {
          try {
            const parsed = JSON.parse(data.model_configs);
            const formatted = JSON.stringify(parsed, null, 2);
            setDefaultPricing(formatted);
          } catch (_e) {
            setDefaultPricing(data.model_configs);
          }
        } else {
          setDefaultPricing("");
        }

        if (typeof data?.tooling === "string" && data.tooling.trim() !== "") {
          try {
            const parsedTooling = JSON.parse(data.tooling);
            setDefaultTooling(stringifyToolingConfig(parsedTooling));
          } catch (_e) {
            setDefaultTooling(data.tooling);
          }
        } else {
          setDefaultTooling(
            stringifyToolingConfig({ whitelist: [], pricing: {} })
          );
        }
      }
    } catch (error) {
      console.error("Error loading default pricing:", error);
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
        if (
          data.models &&
          typeof data.models === "string" &&
          data.models.trim() !== ""
        ) {
          models = data.models
            .split(",")
            .map((model: string) => model.trim())
            .filter((model: string) => model !== "");
        }

        let groups: string[] = ["default"];
        if (
          data.group &&
          typeof data.group === "string" &&
          data.group.trim() !== ""
        ) {
          groups = data.group
            .split(",")
            .map((group: string) => group.trim())
            .filter((group: string) => group !== "");
        }

        let config: ChannelConfigForm = {
          region: "",
          ak: "",
          sk: "",
          user_id: "",
          vertex_ai_project_id: "",
          vertex_ai_adc: "",
          auth_type: "personal_access_token",
          api_format: "chat_completion",
          supported_endpoints: [],
        };
        if (
          data.config &&
          typeof data.config === "string" &&
          data.config.trim() !== ""
        ) {
          try {
            const parsed = JSON.parse(
              data.config
            ) as Partial<ChannelConfigForm>;
            config = {
              ...config,
              ...parsed,
              api_format:
                parsed.api_format === "response"
                  ? "response"
                  : "chat_completion",
              supported_endpoints: Array.isArray(parsed.supported_endpoints)
                ? parsed.supported_endpoints
                : [],
            };
          } catch (e) {
            console.error("Failed to parse config JSON:", e);
          }
        }

        const formatJsonField = (field: string) => {
          if (field && typeof field === "string" && field.trim() !== "") {
            try {
              return JSON.stringify(JSON.parse(field), null, 2);
            } catch (_e) {
              return field;
            }
          }
          return "";
        };

        const channelType = toInt(data.type, 1);
        let toolingField = "";
        if (
          data.tooling &&
          typeof data.tooling === "string" &&
          data.tooling.trim() !== ""
        ) {
          try {
            const parsedTooling = JSON.parse(data.tooling);
            toolingField = stringifyToolingConfig(parsedTooling);
          } catch (_e) {
            toolingField = data.tooling;
          }
        }

        const formData: ChannelForm = {
          name: data.name || "",
          type: channelType,
          key: data.key || "",
          base_url: data.base_url || "",
          other: data.other || "",
          models,
          model_mapping: formatJsonField(data.model_mapping),
          model_configs: formatJsonField(data.model_configs),
          tooling: toolingField,
          system_prompt: data.system_prompt || "",
          groups,
          priority: toInt(data.priority, 0),
          weight: toInt(data.weight, 0),
          ratelimit: toInt(data.ratelimit, 0),
          config,
          inference_profile_arn_map: formatJsonField(
            data.inference_profile_arn_map
          ),
        };

        console.debug("[EditChannel] Loaded channel payload", {
          channelId: data.id ?? channelId,
          channelType,
          hasModelMapping: Boolean(data.model_mapping),
          modelMappingLength:
            typeof data.model_mapping === "string"
              ? data.model_mapping.length
              : 0,
          hasModelConfigs: Boolean(data.model_configs),
          hasSystemPrompt: Boolean(data.system_prompt),
        });

        if (channelType) {
          await loadDefaultPricing(channelType);
        }

        setLoadedChannelType(channelType);
        reset(formData);
        await new Promise((resolve) => setTimeout(resolve, 0));

        const currentTypeValue = getValues("type");
        if (currentTypeValue !== channelType) {
          setValue("type", channelType, {
            shouldValidate: true,
            shouldDirty: false,
          });
          await new Promise((resolve) => setTimeout(resolve, 0));
        }

        setFormInitialized(true);
      } else {
        throw new Error(message || "Failed to load channel");
      }
    } catch (error) {
      console.error("Error loading channel:", error);
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, loadDefaultPricing]);

  const loadModelsCatalog = useCallback(async () => {
    try {
      const response = await api.get("/api/models");
      const { success, data } = response.data;

      if (success && data) {
        const catalog: Record<number, string[]> = {};
        Object.entries(data).forEach(([typeKey, models]) => {
          if (!Array.isArray(models)) return;
          const typeId = Number(typeKey);
          if (!Number.isFinite(typeId)) return;
          catalog[typeId] = (models as string[]).filter(
            (model) => typeof model === "string" && model.trim() !== ""
          );
        });
        setModelsCatalog(catalog);
      }
    } catch (error) {
      console.error("Error loading models catalog:", error);
    }
  }, []);

  const loadGroups = useCallback(async () => {
    try {
      const response = await api.get("/api/option/");
      const { success, data } = response.data;

      if (success && data) {
        const groupsOption = data.find(
          (option: any) => option.key === "AvailableGroups"
        );
        if (groupsOption?.value) {
          const availableGroups = groupsOption.value
            .split(",")
            .map((g: string) => g.trim())
            .filter((g: string) => g !== "");
          setGroups(["default", ...availableGroups]);
        } else {
          setGroups(["default"]);
        }
      }
    } catch (error) {
      console.error("Error loading groups:", error);
      setGroups(["default"]);
    }
  }, []);

  useEffect(() => {
    loadModelsCatalog();
    loadGroups();
  }, [loadModelsCatalog, loadGroups]);

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
        setDefaultBaseURL("");
        setBaseURLEditable(true);
        setDefaultEndpoints([]);
        setAllEndpoints([]);
        if (normalizedChannelType === null) return;
        const res = await api.get(
          `/api/channel/metadata?type=${normalizedChannelType}`
        );
        const base = (res.data?.data?.default_base_url as string) || "";
        const editable = res.data?.data?.base_url_editable !== false;
        const defEndpoints =
          (res.data?.data?.default_endpoints as string[]) || [];
        const allEndpointsData =
          (res.data?.data?.all_endpoints as EndpointInfo[]) || [];
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

  const onSubmit = async (data: ChannelForm) => {
    setIsSubmitting(true);
    try {
      const payload: any = { ...data };

      if (
        watchType === 33 &&
        watchConfig.ak &&
        watchConfig.sk &&
        watchConfig.region
      ) {
        payload.key = `${watchConfig.ak}|${watchConfig.sk}|${watchConfig.region}`;
      } else if (
        watchType === 42 &&
        watchConfig.region &&
        watchConfig.vertex_ai_project_id &&
        watchConfig.vertex_ai_adc
      ) {
        payload.key = `${watchConfig.region}|${watchConfig.vertex_ai_project_id}|${watchConfig.vertex_ai_adc}`;
      }

      if (!isEdit && (!payload.key || payload.key.trim() === "")) {
        form.setError("key", { message: "API key is required" });
        notify({
          type: "error",
          title: tr("validation.error_title", "Validation error"),
          message: tr("validation.api_key_required", "API key is required."),
        });
        return;
      }

      if (data.model_mapping && !isValidJSON(data.model_mapping)) {
        form.setError("model_mapping", {
          message: "Invalid JSON format in model mapping",
        });
        notify({
          type: "error",
          title: tr("validation.invalid_json_title", "Invalid JSON"),
          message: tr(
            "validation.model_mapping_invalid",
            "Model Mapping has invalid JSON."
          ),
        });
        return;
      }

      if (data.model_configs) {
        const validation = validateModelConfigs(data.model_configs);
        if (!validation.valid) {
          form.setError("model_configs", {
            message: validation.error || "Invalid model configs format",
          });
          notify({
            type: "error",
            title: tr("validation.model_configs_title", "Invalid configs"),
            message:
              validation.error ||
              tr(
                "validation.model_configs_message",
                "Model Configs are invalid."
              ),
          });
          return;
        }
      }

      if (
        data.inference_profile_arn_map &&
        !isValidJSON(data.inference_profile_arn_map)
      ) {
        form.setError("inference_profile_arn_map", {
          message: "Invalid JSON format in inference profile ARN map",
        });
        notify({
          type: "error",
          title: tr("validation.invalid_json_title", "Invalid JSON"),
          message: tr(
            "validation.inference_profile_invalid",
            "Inference Profile ARN Map has invalid JSON."
          ),
        });
        return;
      }

      if (watchType === 34 && watchConfig.auth_type === "oauth_jwt") {
        if (!isValidJSON(data.key)) {
          form.setError("key", {
            message: "Invalid JSON format for OAuth JWT configuration",
          });
          notify({
            type: "error",
            title: tr("validation.invalid_json_title", "Invalid JSON"),
            message: tr(
              "validation.oauth_invalid_json",
              "OAuth JWT configuration JSON is invalid."
            ),
          });
          return;
        }

        try {
          const oauthConfig = JSON.parse(data.key);
          const requiredFields = [
            "client_type",
            "client_id",
            "coze_www_base",
            "coze_api_base",
            "private_key",
            "public_key_id",
          ];

          for (const field of requiredFields) {
            if (!Object.hasOwn(oauthConfig, field)) {
              form.setError("key", {
                message: `Missing required field: ${field}`,
              });
              notify({
                type: "error",
                title: tr(
                  "validation.oauth_missing_field_title",
                  "Missing field"
                ),
                message: tr(
                  "validation.oauth_missing_field_message",
                  "OAuth JWT configuration missing: {{field}}",
                  { field }
                ),
              });
              return;
            }

            const selectedEndpoints = data.config.supported_endpoints || [];
            const effectiveEndpoints =
              selectedEndpoints.length === 0
                ? defaultEndpoints
                : selectedEndpoints;

            if (effectiveEndpoints.length === 0) {
              form.setError("config.supported_endpoints", {
                message: "Select at least one endpoint",
              });
              notify({
                type: "error",
                title: tr("validation.error_title", "Validation error"),
                message: tr(
                  "validation.endpoints_required",
                  "Enable at least one endpoint before saving."
                ),
              });
              return;
            }
          }
        } catch (error) {
          form.setError("key", {
            message: `OAuth config parse error: ${(error as Error).message}`,
          });
          notify({
            type: "error",
            title: tr("validation.oauth_parse_title", "Parse error"),
            message: tr(
              "validation.oauth_parse_message",
              "OAuth JWT parse error: {{error}}",
              { error: (error as Error).message }
            ),
          });
          return;
        }
      }

      payload.priority = toInt(payload.priority, 0);
      payload.weight = toInt(payload.weight, 0);
      payload.ratelimit = toInt(payload.ratelimit, 0);

      payload.models = payload.models.join(",");
      payload.group = payload.groups.join(",");
      delete payload.groups;

      payload.config = JSON.stringify(data.config);

      if (isEdit && (!payload.key || payload.key.trim() === "")) {
        delete payload.key;
      }

      const normalizedSubmitType = normalizeChannelType(payload.type);
      const baseURLRawValue =
        typeof payload.base_url === "string" ? payload.base_url : "";
      const trimmedBaseURL = baseURLRawValue.trim();
      const baseURLRequired =
        normalizedSubmitType !== null &&
        CHANNEL_TYPES_WITH_DEDICATED_BASE_URL.has(normalizedSubmitType);

      if (baseURLRequired && !trimmedBaseURL) {
        form.setError("base_url", {
          message: "Base URL is required for this channel type",
        });
        notify({
          type: "error",
          title: tr("validation.error_title", "Validation error"),
          message: tr(
            "validation.base_url_required",
            "Base URL is required for this channel type."
          ),
        });
        return;
      }

      payload.base_url = trimmedBaseURL;
      form.clearErrors("base_url");

      if (payload.base_url?.endsWith("/")) {
        payload.base_url = payload.base_url.slice(0, -1);
      }

      if (watchType === 3 && (!payload.other || payload.other.trim() === "")) {
        payload.other = "2024-03-01-preview";
      }

      const jsonFields = [
        "model_mapping",
        "model_configs",
        "inference_profile_arn_map",
        "system_prompt",
      ];
      jsonFields.forEach((field) => {
        const v = payload[field];
        if (typeof v === "string" && v.trim() === "") {
          payload[field] = null;
        }
      });

      let response;
      if (isEdit && channelId) {
        response = await api.put("/api/channel/", {
          ...payload,
          id: parseInt(channelId, 10),
        });
      } else {
        response = await api.post("/api/channel/", payload);
      }

      const { success, message } = response.data;
      if (success) {
        navigate("/channels", {
          state: {
            message: isEdit
              ? "Channel updated successfully"
              : "Channel created successfully",
          },
        });
      } else {
        form.setError("root", { message: message || "Operation failed" });
        notify({
          type: "error",
          title: tr("errors.request_failed_title", "Request failed"),
          message: message || tr("errors.operation_failed", "Operation failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message: error instanceof Error ? error.message : "Operation failed",
      });
      notify({
        type: "error",
        title: tr("errors.unexpected_title", "Unexpected error"),
        message:
          error instanceof Error
            ? error.message
            : tr("errors.operation_failed", "Operation failed"),
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
      const currentType = getValues("type");
      if (isEdit && loadedChannelType !== null && currentType !== newType) {
        // In edit mode, show confirmation dialog
        setPendingTypeChange({
          fromType: currentType,
          toType: newType,
        });
      } else {
        // In create mode or same type, just set the value
        setValue("type", newType, {
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
      setValue("type", pendingTypeChange.toType, {
        shouldValidate: true,
        shouldDirty: true,
      });
      // Clear related fields that may not be compatible with the new type
      setValue("base_url", "");
      setValue("other", "");
      setPendingTypeChange(null);
    }
  }, [pendingTypeChange, setValue]);

  /**
   * Cancels a pending type change and reverts the selection.
   */
  const cancelTypeChange = useCallback(() => {
    setPendingTypeChange(null);
  }, []);

  const testChannel = async () => {
    if (!channelId) return;

    try {
      setIsSubmitting(true);
      const response = await api.get(`/api/channel/test/${channelId}`);
      const { success, message } = response.data;

      if (success) {
        notify({
          type: "success",
          title: tr("test.success_title", "Success"),
          message: tr("test.success_message", "Channel test successful!"),
        });
      } else {
        notify({
          type: "error",
          title: tr("test.failed_title", "Failed"),
          message: tr(
            "test.failed_message",
            "Channel test failed: {{message}}",
            { message: message || "Unknown error" }
          ),
        });
      }
    } catch (error) {
      notify({
        type: "error",
        title: tr("test.error_title", "Error"),
        message: tr("test.error_message", "Channel test failed: {{error}}", {
          error: error instanceof Error ? error.message : "Network error",
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
    groups,
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
  };
};
