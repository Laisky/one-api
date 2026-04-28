import type { ChannelConfigForm, ChannelForm, NormalizedToolingConfig, ToolPricingEntry } from './schemas';

export const normalizeChannelType = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (value === null || value === undefined) {
    return null;
  }
  if (typeof value === 'string' && value.trim() === '') {
    return null;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
};

// Coercion helpers to ensure numbers are numbers (avoid Zod "expected number, received string")
export const toInt = (v: unknown, def = 0): number => {
  if (typeof v === 'number' && Number.isFinite(v)) return Math.trunc(v);
  const n = Number(v as any);
  return Number.isFinite(n) ? Math.trunc(n) : def;
};

export const normalizeToolingConfigShape = (value: unknown): NormalizedToolingConfig => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { whitelist: [] };
  }

  const record = value as Record<string, unknown>;
  const normalized: Record<string, unknown> = { ...record };
  const whitelistValue = (record as any).whitelist;

  normalized.whitelist = Array.isArray(whitelistValue) ? whitelistValue : [];

  return normalized as NormalizedToolingConfig;
};

export const stringifyToolingConfig = (value: unknown): string => JSON.stringify(normalizeToolingConfigShape(value), null, 2);

export const clonePricingMap = (pricing?: Record<string, ToolPricingEntry>): Record<string, ToolPricingEntry> => {
  if (!pricing) {
    return {};
  }
  const entries = Object.entries(pricing).map(([key, entry]) => [key, { ...(entry ?? {}) } as ToolPricingEntry]);
  return Object.fromEntries(entries);
};

export const cloneNormalizedToolingConfig = (config: NormalizedToolingConfig): NormalizedToolingConfig => {
  const cloned: NormalizedToolingConfig = {
    ...config,
    whitelist: [...config.whitelist],
  };
  if (config.pricing) {
    cloned.pricing = clonePricingMap(config.pricing);
  }
  return cloned;
};

export const prepareToolingConfigForSet = (config: NormalizedToolingConfig): NormalizedToolingConfig => {
  const cloned = cloneNormalizedToolingConfig(config);
  if (cloned.pricing) {
    const cleanedPricing = Object.fromEntries(
      Object.entries(cloned.pricing).map(([key, entry]) => [key, { ...(entry ?? {}) } as ToolPricingEntry])
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
  const matchedKey = Object.keys(pricing).find((key) => key.toLowerCase() === canonical);
  if (!matchedKey) {
    return { key: null, entry: undefined };
  }
  return { key: matchedKey, entry: pricing[matchedKey] };
};

// Admin-entered JSON inputs are JSONC-flavoured: they may include `//` and
// `/* */` comments and trailing commas for better authoring ergonomics. The
// helpers below normalise the text into strict JSON before `JSON.parse`, and
// are also used at submit time so the backend only ever sees canonical JSON.

const stripJsonComments = (input: string): string => {
  let output = '';
  let i = 0;
  const n = input.length;
  let inString = false;
  while (i < n) {
    const ch = input[i];
    const next = i + 1 < n ? input[i + 1] : '';
    if (inString) {
      output += ch;
      if (ch === '\\' && i + 1 < n) {
        output += input[i + 1];
        i += 2;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      i++;
      continue;
    }
    if (ch === '"') {
      inString = true;
      output += ch;
      i++;
      continue;
    }
    if (ch === '/' && next === '/') {
      i += 2;
      while (i < n && input[i] !== '\n' && input[i] !== '\r') i++;
      continue;
    }
    if (ch === '/' && next === '*') {
      i += 2;
      while (i < n && !(input[i] === '*' && input[i + 1] === '/')) i++;
      if (i < n) i += 2;
      continue;
    }
    output += ch;
    i++;
  }
  return output;
};

const stripTrailingCommas = (input: string): string => {
  let output = '';
  let i = 0;
  const n = input.length;
  let inString = false;
  while (i < n) {
    const ch = input[i];
    if (inString) {
      output += ch;
      if (ch === '\\' && i + 1 < n) {
        output += input[i + 1];
        i += 2;
        continue;
      }
      if (ch === '"') {
        inString = false;
      }
      i++;
      continue;
    }
    if (ch === '"') {
      inString = true;
      output += ch;
      i++;
      continue;
    }
    if (ch === ',') {
      let j = i + 1;
      while (j < n && /\s/.test(input[j])) j++;
      if (j < n && (input[j] === '}' || input[j] === ']')) {
        i++;
        continue;
      }
    }
    output += ch;
    i++;
  }
  return output;
};

/**
 * sanitizeJsonInput converts admin-entered JSONC (JSON with line and block
 * comments and trailing commas) into strict JSON. Safe to call on empty or
 * already-clean input.
 */
export const sanitizeJsonInput = (jsonString: string): string => {
  if (!jsonString) return jsonString;
  return stripTrailingCommas(stripJsonComments(jsonString));
};

// JSON validation functions (accept JSONC-flavoured input)
export const isValidJSON = (jsonString: string) => {
  if (!jsonString || jsonString.trim() === '') return true;
  try {
    JSON.parse(sanitizeJsonInput(jsonString));
    return true;
  } catch (_e) {
    return false;
  }
};

export const formatJSON = (jsonString: string) => {
  if (!jsonString || jsonString.trim() === '') return '';
  try {
    const parsed = JSON.parse(sanitizeJsonInput(jsonString));
    return JSON.stringify(parsed, null, 2);
  } catch (_e) {
    return jsonString;
  }
};

/**
 * sanitizeJsonField returns canonical JSON if the input parses as JSONC,
 * otherwise returns the original string so the backend can surface the
 * validation error. Empty/whitespace inputs pass through unchanged.
 */
export const sanitizeJsonField = (jsonString: string): string => {
  if (!jsonString || jsonString.trim() === '') return jsonString;
  try {
    const parsed = JSON.parse(sanitizeJsonInput(jsonString));
    return JSON.stringify(parsed);
  } catch (_e) {
    return jsonString;
  }
};

// Enhanced model configs validation
export const validateModelConfigs = (configStr: string) => {
  if (!configStr || configStr.trim() === '') {
    return { valid: true };
  }

  try {
    const configs = JSON.parse(sanitizeJsonInput(configStr));

    if (typeof configs !== 'object' || configs === null || Array.isArray(configs)) {
      return { valid: false, error: 'Model configs must be a JSON object' };
    }

    for (const [modelName, config] of Object.entries(configs)) {
      if (!modelName || modelName.trim() === '') {
        return { valid: false, error: 'Model name cannot be empty' };
      }

      if (typeof config !== 'object' || config === null || Array.isArray(config)) {
        return {
          valid: false,
          error: `Configuration for model "${modelName}" must be an object`,
        };
      }

      const configObj = config as any;
      // Validate ratio
      if (configObj.ratio !== undefined) {
        if (typeof configObj.ratio !== 'number' || configObj.ratio < 0) {
          return {
            valid: false,
            error: `Invalid ratio for model "${modelName}": must be a non-negative number`,
          };
        }
      }

      // Validate completion_ratio
      if (configObj.completion_ratio !== undefined) {
        if (typeof configObj.completion_ratio !== 'number' || configObj.completion_ratio < 0) {
          return {
            valid: false,
            error: `Invalid completion_ratio for model "${modelName}": must be a non-negative number`,
          };
        }
      }

      // Validate max_tokens
      if (configObj.max_tokens !== undefined) {
        if (!Number.isInteger(configObj.max_tokens) || configObj.max_tokens < 0) {
          return {
            valid: false,
            error: `Invalid max_tokens for model "${modelName}": must be a non-negative integer`,
          };
        }
      }

      const hasPricingField =
        configObj.ratio !== undefined || configObj.completion_ratio !== undefined || configObj.max_tokens !== undefined;
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
  if (!configStr || configStr.trim() === '') {
    return { valid: true };
  }

  try {
    const config = JSON.parse(sanitizeJsonInput(configStr));
    if (typeof config !== 'object' || config === null || Array.isArray(config)) {
      return { valid: false, error: 'Tooling config must be a JSON object' };
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
        if (typeof entry !== 'string' || entry.trim() === '') {
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
      if (typeof value !== 'object' || value === null || Array.isArray(value)) {
        return { valid: false, error: `${scope} pricing must be an object` };
      }
      for (const [toolName, entry] of Object.entries(value as Record<string, any>)) {
        if (!toolName || toolName.trim() === '') {
          return {
            valid: false,
            error: `${scope} pricing has an empty tool name`,
          };
        }
        if (typeof entry !== 'object' || entry === null || Array.isArray(entry)) {
          return {
            valid: false,
            error: `${scope} pricing for tool "${toolName}" must be an object`,
          };
        }
        const { usd_per_call, quota_per_call } = entry as Record<string, any>;
        if (usd_per_call !== undefined && (typeof usd_per_call !== 'number' || usd_per_call < 0)) {
          return {
            valid: false,
            error: `${scope} pricing usd_per_call for "${toolName}" must be a non-negative number`,
          };
        }
        if (quota_per_call !== undefined && (typeof quota_per_call !== 'number' || quota_per_call < 0)) {
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

    const whitelistResult = validateWhitelist((config as any).whitelist, 'Default');
    if (!whitelistResult.valid) {
      return whitelistResult;
    }

    const pricingResult = validatePricing((config as any).pricing, 'Default');
    if (!pricingResult.valid) {
      return pricingResult;
    }

    if ((config as any).model_overrides !== undefined) {
      return {
        valid: false,
        error: 'model_overrides is no longer supported. Configure tooling at the channel level.',
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

/**
 * BuildChannelSubmitPayloadOptions captures all the contextual flags that
 * influence the shape of the payload but are not part of the form data
 * itself. Keeping this separate from `ChannelForm` lets us unit-test the
 * transformation without spinning up the React hook.
 */
export interface BuildChannelSubmitPayloadOptions {
  /** True when editing an existing channel (PUT). False on create (POST). */
  isEdit: boolean;
  /** Currently selected channel type (mirrors `form.watch('type')`). */
  watchType: number | null | undefined;
  /** Live config values used to derive composite key fields. */
  watchConfig: ChannelConfigForm;
}

/**
 * buildChannelSubmitPayload transforms a validated `ChannelForm` plus
 * contextual flags into the exact payload object that should be sent to
 * the backend. It is intentionally pure: no network, no validation, no
 * error throwing for user-visible issues. Validation is performed by the
 * caller before invoking this helper.
 *
 * Key behaviours that are locked-in by tests:
 *  - On edit, an empty `key` is REMOVED from the payload so the backend
 *    treats it as "do not change".
 *  - For nullable JSON-ish fields (`model_mapping`, `model_configs`,
 *    `inference_profile_arn_map`, `system_prompt`), an empty user input is
 *    serialised as JSON `null` (key present, value `null`) so the backend
 *    can distinguish "clear me" from "leave me alone".
 *  - JSONC inputs are sanitised to strict JSON.
 */
export const buildChannelSubmitPayload = (data: ChannelForm, opts: BuildChannelSubmitPayloadOptions): Record<string, any> => {
  const { isEdit, watchType, watchConfig } = opts;
  const payload: Record<string, any> = { ...data };

  // Composite key handling for channel types that pack multiple credentials.
  if (watchType === 33 && watchConfig?.ak && watchConfig?.sk && watchConfig?.region) {
    payload.key = `${watchConfig.ak}|${watchConfig.sk}|${watchConfig.region}`;
  } else if (watchType === 42 && watchConfig?.region && watchConfig?.vertex_ai_project_id && watchConfig?.vertex_ai_adc) {
    payload.key = `${watchConfig.region}|${watchConfig.vertex_ai_project_id}|${watchConfig.vertex_ai_adc}`;
  } else if (watchType === 18 && (watchConfig?.spark_app_id || watchConfig?.spark_api_secret || watchConfig?.spark_api_key)) {
    payload.key = `${watchConfig.spark_app_id || ''}|${watchConfig.spark_api_secret || ''}|${watchConfig.spark_api_key || ''}`;
  } else if (watchType === 23 && (watchConfig?.tencent_app_id || watchConfig?.tencent_secret_id || watchConfig?.tencent_secret_key)) {
    payload.key = `${watchConfig.tencent_app_id || ''}|${watchConfig.tencent_secret_id || ''}|${watchConfig.tencent_secret_key || ''}`;
  }

  payload.priority = toInt(payload.priority, 0);
  payload.weight = toInt(payload.weight, 0);
  payload.ratelimit = toInt(payload.ratelimit, 0);

  payload.models = Array.isArray(payload.models) ? payload.models.join(',') : '';

  const normalizedHiddenModels = [...new Set((data.hidden_models || []).map((model) => model.trim()).filter((model) => model !== ''))];
  payload.hidden_models = normalizedHiddenModels.length > 0 ? JSON.stringify(normalizedHiddenModels) : null;

  payload.group = Array.isArray(payload.groups) ? payload.groups.join(',') : '';
  delete payload.groups;

  payload.config = JSON.stringify(data.config);

  // On edit, an empty key signals "leave the existing key alone". Drop the
  // field entirely so the backend's partial-update logic skips it.
  if (isEdit && (typeof payload.key !== 'string' || payload.key.trim() === '')) {
    delete payload.key;
  }

  const baseURLRawValue = typeof payload.base_url === 'string' ? payload.base_url : '';
  let trimmedBaseURL = baseURLRawValue.trim();
  if (trimmedBaseURL.endsWith('/')) {
    trimmedBaseURL = trimmedBaseURL.slice(0, -1);
  }
  payload.base_url = trimmedBaseURL;

  // Azure (type 3) requires an api_version-like value in `other`. Apply a
  // reasonable default so existing callers keep working without manual entry.
  if (watchType === 3 && (typeof payload.other !== 'string' || payload.other.trim() === '')) {
    payload.other = '2024-03-01-preview';
  }

  const jsoncFields = ['model_mapping', 'model_configs', 'inference_profile_arn_map', 'tooling'];
  jsoncFields.forEach((field) => {
    const v = payload[field];
    if (typeof v === 'string' && v.trim() !== '') {
      payload[field] = sanitizeJsonField(v);
    }
  });

  if (watchType === 34 && watchConfig?.auth_type === 'oauth_jwt' && typeof payload.key === 'string' && payload.key.trim() !== '') {
    payload.key = sanitizeJsonField(payload.key);
  }

  // CRITICAL: when the user clears any of these fields, send JSON `null`
  // (key present, value null) — NOT undefined, NOT missing. The backend
  // relies on the field key being present in the JSON body to know the
  // user wants to clear it; omitting the key would leave the existing
  // value untouched.
  const nullableEmptyFields = ['model_mapping', 'model_configs', 'inference_profile_arn_map', 'system_prompt'];
  nullableEmptyFields.forEach((field) => {
    const v = payload[field];
    if (typeof v === 'string' && v.trim() === '') {
      payload[field] = null;
    }
  });

  return payload;
};

// Helper function to get key prompt based on channel type
export const getKeyPrompt = (type: number) => {
  switch (type) {
    case 15:
      return 'APIKey|SecretKey';
    case 18:
      return 'APPID|APISecret|APIKey';
    case 22:
      return 'APIKey-AppId (e.g., fastgpt-0sp2gtvfdgyi4k30jwlgwf1i-64f335d84283f05518e9e041)';
    case 23:
      return 'AppId|SecretId|SecretKey';
    case 26:
      return 'APIKey|SecretKey';
    case 53:
      return 'Please enter a GitHub access token (PAT or OAuth token) with an active Copilot subscription';
    default:
      return 'Please enter your API key';
  }
};
