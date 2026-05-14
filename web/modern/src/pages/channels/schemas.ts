import * as z from 'zod';

type SchemaTranslationFn = (key: string, defaultValue: string) => string;

/**
 * createChannelSchema builds the channel form schema and injects localized
 * validation messages when a translator is provided.
 */
export const createChannelSchema = (tr?: SchemaTranslationFn) => {
  const message = (key: string, defaultValue: string) => tr?.(key, defaultValue) ?? defaultValue;

  return z.object({
    name: z.string().min(1, message('validation.name_required', 'Channel name is required')),
    // Coerce because Select returns string
    type: z.coerce.number().int().min(1, message('validation.type_required', 'Channel type is required')),
    // key optional on edit; we enforce presence only on create in submit handler
    key: z.string().optional(),
    base_url: z.string().optional(),
    other: z.string().optional(),
    models: z.array(z.string()).default([]),
    hidden_models: z.array(z.string()).default([]),
    model_mapping: z.string().optional(),
    model_configs: z.string().optional(),
    tooling: z.string().optional(),
    system_prompt: z.string().optional(),
    groups: z.array(z.string()).default(['default']),
    // Coerce because inputs emit strings; enforce integers for these numeric fields
    priority: z.coerce.number().int().default(0),
    weight: z.coerce.number().int().default(0),
    ratelimit: z.coerce.number().int().min(0).default(0),
    // AWS and Vertex AI specific config
    config: z
      .object({
        region: z.string().optional(),
        ak: z.string().optional(),
        sk: z.string().optional(),
        user_id: z.string().optional(),
        vertex_ai_project_id: z.string().optional(),
        vertex_ai_adc: z.string().optional(),
        auth_type: z.string().default('personal_access_token'),
        api_format: z.enum(['chat_completion', 'response']).default('chat_completion'),
        // Supported endpoints for this channel (empty means use defaults)
        supported_endpoints: z.array(z.string()).optional(),
        mcp_tool_blacklist: z.array(z.string()).optional(),
        // iFlytek Spark (type 18): APPID|APISecret|APIKey
        spark_app_id: z.string().optional(),
        spark_api_secret: z.string().optional(),
        spark_api_key: z.string().optional(),
        // Tencent Hunyuan (type 23): AppId|SecretId|SecretKey
        tencent_app_id: z.string().optional(),
        tencent_secret_id: z.string().optional(),
        tencent_secret_key: z.string().optional(),
      })
      .default({}),
    inference_profile_arn_map: z.string().optional(),
  });
};

type ChannelSchema = ReturnType<typeof createChannelSchema>;

// Enhanced channel schema with comprehensive validation
export const channelSchema: ChannelSchema = createChannelSchema();

export type ChannelForm = z.infer<ChannelSchema>;
export type ChannelConfigForm = NonNullable<ChannelForm['config']>;

export type ToolPricingEntry = {
  usd_per_call?: number;
  quota_per_call?: number;
};

export type ParsedToolingConfig = {
  whitelist?: string[];
  pricing?: Record<string, ToolPricingEntry>;
};

export type NormalizedToolingConfig = ParsedToolingConfig & {
  whitelist: string[];
};

// Endpoint info returned from the API
export type EndpointInfo = {
  id: number;
  name: string;
  description: string;
  path: string;
};
