export type CacheWriteTokensMetadata = {
  ephemeral_5m?: number;
  ephemeral_1h?: number;
};

export type LogMetadata = {
  cache_write_tokens?: CacheWriteTokensMetadata;
  user_api_format?: string;
  upstream_api_format?: string;
  upstream_endpoint?: string;
  [key: string]: unknown;
};

export interface LogEntry {
  id?: number;
  uuid?: string;
  type: number;
  created_at: number;
  model_name: string;
  origin_model_name?: string;
  token_name?: string;
  username?: string;
  user_id?: number;
  user_uuid?: string | null;
  channel?: number | string;
  channel_uuid?: string | null;
  token_uuid?: string | null;
  quota: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  cached_prompt_tokens?: number;
  elapsed_time?: number;
  request_id?: string;
  trace_id?: string;
  content?: string;
  is_stream?: boolean;
  system_prompt_reset?: boolean;
  metadata?: LogMetadata;
}
