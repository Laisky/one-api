import { useCallback, useMemo, useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import {
  cloneNormalizedToolingConfig,
  clonePricingMap,
  findPricingEntryCaseInsensitive,
  normalizeToolingConfigShape,
  prepareToolingConfigForSet,
  stringifyToolingConfig,
} from "../helpers";
import type {
  ChannelForm,
  NormalizedToolingConfig,
  ToolPricingEntry,
} from "../schemas";

export const useChannelTooling = (
  form: UseFormReturn<ChannelForm>,
  defaultTooling: string,
  notify: (options: any) => void,
  tr: (
    key: string,
    defaultValue: string,
    options?: Record<string, unknown>
  ) => string
) => {
  const [customTool, setCustomTool] = useState("");
  const watchTooling = form.watch("tooling") ?? "";

  const showToolingJSONError = useCallback(() => {
    notify({
      type: "error",
      title: tr("tooling.errors.invalid_json_title", "Invalid JSON"),
      message: tr(
        "tooling.errors.invalid_json_message",
        "Fix tooling JSON before editing the whitelist."
      ),
    });
  }, [notify, tr]);

  const parsedToolingConfig = useMemo<NormalizedToolingConfig | null>(() => {
    const raw = (watchTooling ?? "").trim();
    if (raw === "") {
      return normalizeToolingConfigShape({});
    }
    try {
      const parsed = JSON.parse(raw);
      return normalizeToolingConfigShape(parsed);
    } catch (_error) {
      return null;
    }
  }, [watchTooling]);

  const parsedDefaultTooling = useMemo<NormalizedToolingConfig | null>(() => {
    if (!defaultTooling || defaultTooling.trim() === "") {
      return null;
    }
    try {
      const parsed = JSON.parse(defaultTooling);
      return normalizeToolingConfigShape(parsed);
    } catch (_error) {
      return null;
    }
  }, [defaultTooling]);

  const currentToolWhitelist = useMemo(() => {
    return parsedToolingConfig?.whitelist ?? [];
  }, [parsedToolingConfig]);

  const pricedToolSet = useMemo(() => {
    const result = new Set<string>();
    const collectPricing = (pricing?: Record<string, ToolPricingEntry>) => {
      if (!pricing || typeof pricing !== "object") {
        return;
      }
      Object.keys(pricing).forEach((tool) => {
        const canonical = tool.trim().toLowerCase();
        if (canonical) {
          result.add(canonical);
        }
      });
    };

    if (parsedToolingConfig) {
      collectPricing(parsedToolingConfig.pricing);
    }
    if (parsedDefaultTooling) {
      collectPricing(parsedDefaultTooling.pricing);
    }

    return result;
  }, [parsedDefaultTooling, parsedToolingConfig]);

  const availableDefaultTools = useMemo(() => {
    const defaults = new Set<string>();
    const collectWhitelist = (list?: string[]) => {
      if (!Array.isArray(list)) {
        return;
      }
      list.forEach((tool) => {
        const trimmed = tool.trim();
        if (trimmed) {
          defaults.add(trimmed);
        }
      });
    };
    const collectPricingKeys = (pricing?: Record<string, ToolPricingEntry>) => {
      if (!pricing || typeof pricing !== "object") {
        return;
      }
      Object.keys(pricing).forEach((tool) => {
        const trimmed = tool.trim();
        if (trimmed) {
          defaults.add(trimmed);
        }
      });
    };

    if (parsedDefaultTooling) {
      collectWhitelist(parsedDefaultTooling.whitelist);
      collectPricingKeys(parsedDefaultTooling.pricing);
    }
    if (parsedToolingConfig && parsedToolingConfig !== null) {
      collectWhitelist(parsedToolingConfig.whitelist);
      collectPricingKeys(parsedToolingConfig.pricing);
    }

    return Array.from(defaults).sort((a, b) => a.localeCompare(b));
  }, [parsedDefaultTooling, parsedToolingConfig]);

  const toolEditorDisabled = parsedToolingConfig === null;

  const mutateToolWhitelist = useCallback(
    (
      transform: (
        config: NormalizedToolingConfig
      ) => NormalizedToolingConfig | null
    ) => {
      if (parsedToolingConfig === null) {
        showToolingJSONError();
        return;
      }
      const raw = watchTooling ?? "";
      let configs: NormalizedToolingConfig;
      try {
        if (!raw || raw.trim() === "") {
          configs = normalizeToolingConfigShape({});
        } else {
          const parsed = JSON.parse(raw);
          configs = normalizeToolingConfigShape(parsed);
        }
      } catch (_error) {
        showToolingJSONError();
        return;
      }

      const workingConfig = cloneNormalizedToolingConfig(configs);
      const updatedConfig = transform(workingConfig);
      if (!updatedConfig) {
        return;
      }

      const normalizedResult = normalizeToolingConfigShape(updatedConfig);
      const prepared = prepareToolingConfigForSet(normalizedResult);

      form.setValue("tooling", stringifyToolingConfig(prepared), {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form, parsedToolingConfig, showToolingJSONError, watchTooling]
  );

  const addToolToWhitelist = useCallback(
    (toolName: string, options?: { isCustom?: boolean }) => {
      if (!toolName || parsedToolingConfig === null) {
        return;
      }
      const trimmed = toolName.trim();
      if (!trimmed) {
        return;
      }
      const canonical = trimmed.toLowerCase();
      const isCustomTool = options?.isCustom ?? false;

      mutateToolWhitelist((config) => {
        if (config.whitelist.some((item) => item.toLowerCase() === canonical)) {
          return null;
        }

        const updatedWhitelist = [...config.whitelist, trimmed];
        const nextPricing = clonePricingMap(config.pricing);

        // Remove any entries using a different casing for the same tool
        Object.keys(nextPricing).forEach((key) => {
          if (key.toLowerCase() === canonical && key !== trimmed) {
            nextPricing[trimmed] = { ...nextPricing[key] };
            delete nextPricing[key];
          }
        });

        if (!Object.hasOwn(nextPricing, trimmed)) {
          const { entry: existingEntry } = findPricingEntryCaseInsensitive(
            config.pricing,
            trimmed
          );
          const { entry: defaultEntry } = findPricingEntryCaseInsensitive(
            parsedDefaultTooling?.pricing,
            trimmed
          );
          const pricingEntry = existingEntry
            ? { ...existingEntry }
            : defaultEntry
              ? { ...defaultEntry }
              : { usd_per_call: 0.1 };

          // Ensure custom tools always have a sensible default even without prior pricing
          nextPricing[trimmed] =
            isCustomTool && !existingEntry && !defaultEntry
              ? { usd_per_call: 0.1 }
              : pricingEntry;
        }

        const hasPricingEntries = Object.keys(nextPricing).length > 0;

        return {
          ...config,
          whitelist: updatedWhitelist,
          ...(hasPricingEntries ? { pricing: nextPricing } : {}),
        };
      });
      setCustomTool("");
    },
    [mutateToolWhitelist, parsedDefaultTooling, parsedToolingConfig]
  );

  const removeToolFromWhitelist = useCallback(
    (toolName: string) => {
      if (!toolName || parsedToolingConfig === null) {
        return;
      }
      const canonical = toolName.toLowerCase();
      mutateToolWhitelist((config) => {
        const filtered = config.whitelist.filter(
          (item) => item.toLowerCase() !== canonical
        );
        if (filtered.length === config.whitelist.length) {
          return null;
        }

        const nextPricing = clonePricingMap(config.pricing);
        Object.keys(nextPricing).forEach((key) => {
          if (key.toLowerCase() === canonical) {
            delete nextPricing[key];
          }
        });

        const hasPricingEntries = Object.keys(nextPricing).length > 0;

        return {
          ...config,
          whitelist: filtered,
          ...(hasPricingEntries ? { pricing: nextPricing } : {}),
        };
      });
    },
    [mutateToolWhitelist, parsedToolingConfig]
  );

  const formatToolingConfig = () => {
    const value = form.getValues("tooling");
    if (!value || value.trim() === "") {
      form.setValue(
        "tooling",
        stringifyToolingConfig({ whitelist: [], pricing: {} }),
        {
          shouldDirty: true,
          shouldValidate: true,
        }
      );
      return;
    }
    try {
      const parsed = JSON.parse(value);
      form.setValue("tooling", stringifyToolingConfig(parsed), {
        shouldDirty: true,
        shouldValidate: true,
      });
    } catch (error) {
      notify({
        type: "error",
        title: tr("validation.invalid_json_title", "Invalid JSON"),
        message: tr(
          "tooling.format_error",
          "Unable to format tooling config: {{error}}",
          { error: (error as Error).message }
        ),
      });
    }
  };

  return {
    customTool,
    setCustomTool,
    parsedToolingConfig,
    parsedDefaultTooling,
    currentToolWhitelist,
    pricedToolSet,
    availableDefaultTools,
    toolEditorDisabled,
    addToolToWhitelist,
    removeToolFromWhitelist,
    formatToolingConfig,
  };
};
