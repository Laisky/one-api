import type { NormalizedToolingConfig, ToolPricingEntry } from "./schemas";

export const normalizeChannelType = (value: unknown): number | null => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === "string" && value.trim() === "") {
    return null;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
};

// Coercion helpers to ensure numbers are numbers (avoid Zod "expected number, received string")
export const toInt = (v: unknown, def = 0): number => {
  if (typeof v === "number" && Number.isFinite(v)) return Math.trunc(v);
  const n = Number(v as any);
  return Number.isFinite(n) ? Math.trunc(n) : def;
};

export const normalizeToolingConfigShape = (
  value: unknown
): NormalizedToolingConfig => {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { whitelist: [] };
  }

  const record = value as Record<string, unknown>;
  const normalized: Record<string, unknown> = { ...record };
  const whitelistValue = (record as any).whitelist;

  normalized.whitelist = Array.isArray(whitelistValue) ? whitelistValue : [];

  return normalized as NormalizedToolingConfig;
};

export const stringifyToolingConfig = (value: unknown): string =>
  JSON.stringify(normalizeToolingConfigShape(value), null, 2);

export const clonePricingMap = (
  pricing?: Record<string, ToolPricingEntry>
): Record<string, ToolPricingEntry> => {
  if (!pricing) {
    return {};
  }
  const entries = Object.entries(pricing).map(([key, entry]) => [
    key,
    { ...(entry ?? {}) } as ToolPricingEntry,
  ]);
  return Object.fromEntries(entries);
};

export const cloneNormalizedToolingConfig = (
  config: NormalizedToolingConfig
): NormalizedToolingConfig => {
  const cloned: NormalizedToolingConfig = {
    ...config,
    whitelist: [...config.whitelist],
  };
  if (config.pricing) {
    cloned.pricing = clonePricingMap(config.pricing);
  }
  return cloned;
};

export const prepareToolingConfigForSet = (
  config: NormalizedToolingConfig
): NormalizedToolingConfig => {
  const cloned = cloneNormalizedToolingConfig(config);
  if (cloned.pricing) {
    const cleanedPricing = Object.fromEntries(
      Object.entries(cloned.pricing).map(([key, entry]) => [
        key,
        { ...(entry ?? {}) } as ToolPricingEntry,
      ])
    );
    if (Object.keys(cleanedPricing).length > 0) {
      cloned.pricing = cleanedPricing;
    } else {
      delete (cloned as any).pricing;
    }
  }
  delete (cloned as any).model_overrides;
  return cloned;
};

export const findPricingEntryCaseInsensitive = (
  pricing: Record<string, ToolPricingEntry> | undefined,
  toolName: string
): { key: string | null; entry?: ToolPricingEntry } => {
  if (!pricing) {
    return { key: null, entry: undefined };
  }
  if (Object.hasOwn(pricing, toolName)) {
    return { key: toolName, entry: pricing[toolName] };
  }
  const canonical = toolName.toLowerCase();
  const matchedKey = Object.keys(pricing).find(
    (key) => key.toLowerCase() === canonical
  );
  if (!matchedKey) {
    return { key: null, entry: undefined };
  }
  return { key: matchedKey, entry: pricing[matchedKey] };
};

// JSON validation functions
export const isValidJSON = (jsonString: string) => {
  if (!jsonString || jsonString.trim() === "") return true;
  try {
    JSON.parse(jsonString);
    return true;
  } catch (_e) {
    return false;
  }
};

export const formatJSON = (jsonString: string) => {
  if (!jsonString || jsonString.trim() === "") return "";
  try {
    const parsed = JSON.parse(jsonString);
    return JSON.stringify(parsed, null, 2);
  } catch (_e) {
    return jsonString;
  }
};

// Enhanced model configs validation
export const validateModelConfigs = (configStr: string) => {
  if (!configStr || configStr.trim() === "") {
    return { valid: true };
  }

  try {
    const configs = JSON.parse(configStr);

    if (
      typeof configs !== "object" ||
      configs === null ||
      Array.isArray(configs)
    ) {
      return { valid: false, error: "Model configs must be a JSON object" };
    }

    for (const [modelName, config] of Object.entries(configs)) {
      if (!modelName || modelName.trim() === "") {
        return { valid: false, error: "Model name cannot be empty" };
      }

      if (
        typeof config !== "object" ||
        config === null ||
        Array.isArray(config)
      ) {
        return {
          valid: false,
          error: `Configuration for model "${modelName}" must be an object`,
        };
      }

      const configObj = config as any;
      // Validate ratio
      if (configObj.ratio !== undefined) {
        if (typeof configObj.ratio !== "number" || configObj.ratio < 0) {
          return {
            valid: false,
            error: `Invalid ratio for model "${modelName}": must be a non-negative number`,
          };
        }
      }

      // Validate completion_ratio
      if (configObj.completion_ratio !== undefined) {
        if (
          typeof configObj.completion_ratio !== "number" ||
          configObj.completion_ratio < 0
        ) {
          return {
            valid: false,
            error: `Invalid completion_ratio for model "${modelName}": must be a non-negative number`,
          };
        }
      }

      // Validate max_tokens
      if (configObj.max_tokens !== undefined) {
        if (
          !Number.isInteger(configObj.max_tokens) ||
          configObj.max_tokens < 0
        ) {
          return {
            valid: false,
            error: `Invalid max_tokens for model "${modelName}": must be a non-negative integer`,
          };
        }
      }

      const hasPricingField =
        configObj.ratio !== undefined ||
        configObj.completion_ratio !== undefined ||
        configObj.max_tokens !== undefined;
      if (!hasPricingField) {
        return {
          valid: false,
          error: `Model "${modelName}" must include pricing configuration`,
        };
      }
    }

    return { valid: true };
  } catch (error) {
    return {
      valid: false,
      error: `Invalid JSON format: ${(error as Error).message}`,
    };
  }
};

export const validateToolingConfig = (configStr: string) => {
  if (!configStr || configStr.trim() === "") {
    return { valid: true };
  }

  try {
    const config = JSON.parse(configStr);
    if (
      typeof config !== "object" ||
      config === null ||
      Array.isArray(config)
    ) {
      return { valid: false, error: "Tooling config must be a JSON object" };
    }

    const validateWhitelist = (value: any, scope: string) => {
      if (value === undefined) {
        return { valid: true };
      }
      if (!Array.isArray(value)) {
        return {
          valid: false,
          error: `${scope} whitelist must be an array of strings`,
        };
      }
      for (const entry of value) {
        if (typeof entry !== "string" || entry.trim() === "") {
          return {
            valid: false,
            error: `${scope} whitelist contains an invalid entry`,
          };
        }
      }
      return { valid: true };
    };

    const validatePricing = (value: any, scope: string) => {
      if (value === undefined) {
        return { valid: true };
      }
      if (typeof value !== "object" || value === null || Array.isArray(value)) {
        return { valid: false, error: `${scope} pricing must be an object` };
      }
      for (const [toolName, entry] of Object.entries(
        value as Record<string, any>
      )) {
        if (!toolName || toolName.trim() === "") {
          return {
            valid: false,
            error: `${scope} pricing has an empty tool name`,
          };
        }
        if (
          typeof entry !== "object" ||
          entry === null ||
          Array.isArray(entry)
        ) {
          return {
            valid: false,
            error: `${scope} pricing for tool "${toolName}" must be an object`,
          };
        }
        const { usd_per_call, quota_per_call } = entry as Record<string, any>;
        if (
          usd_per_call !== undefined &&
          (typeof usd_per_call !== "number" || usd_per_call < 0)
        ) {
          return {
            valid: false,
            error: `${scope} pricing usd_per_call for "${toolName}" must be a non-negative number`,
          };
        }
        if (
          quota_per_call !== undefined &&
          (typeof quota_per_call !== "number" || quota_per_call < 0)
        ) {
          return {
            valid: false,
            error: `${scope} pricing quota_per_call for "${toolName}" must be a non-negative number`,
          };
        }
        if (usd_per_call === undefined && quota_per_call === undefined) {
          return {
            valid: false,
            error: `${scope} pricing for "${toolName}" must include usd_per_call or quota_per_call`,
          };
        }
      }
      return { valid: true };
    };

    const whitelistResult = validateWhitelist(
      (config as any).whitelist,
      "Default"
    );
    if (!whitelistResult.valid) {
      return whitelistResult;
    }

    const pricingResult = validatePricing((config as any).pricing, "Default");
    if (!pricingResult.valid) {
      return pricingResult;
    }

    if ((config as any).model_overrides !== undefined) {
      return {
        valid: false,
        error:
          "model_overrides is no longer supported. Configure tooling at the channel level.",
      };
    }

    return { valid: true };
  } catch (error) {
    return {
      valid: false,
      error: `Invalid JSON format: ${(error as Error).message}`,
    };
  }
};

// Helper function to get key prompt based on channel type
export const getKeyPrompt = (type: number) => {
  switch (type) {
    case 15:
      return "Please enter Baidu API Key and Secret Key in format: API_KEY|SECRET_KEY";
    case 18:
      return "Please enter iFlytek App ID, API Secret, and API Key in format: APPID|API_SECRET|API_KEY";
    case 22:
      return "Please enter FastGPT API Key";
    case 23:
      return "Please enter Tencent SecretId and SecretKey in format: SECRET_ID|SECRET_KEY";
    case 53:
      return "Please enter a GitHub access token (PAT or OAuth token) with an active Copilot subscription";
    default:
      return "Please enter your API key";
  }
};
