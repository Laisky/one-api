export type CacheWriteTokensMetadata = {
  ephemeral_5m?: number;
  ephemeral_1h?: number;
};

export type LogMetadata = {
  cache_write_tokens?: CacheWriteTokensMetadata;
  [key: string]: unknown;
};

export interface LogEntry {
  id: number;
  type: number;
  created_at: number;
  model_name: string;
  token_name?: string;
  username?: string;
  channel?: number;
  quota: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  cached_prompt_tokens?: number;
  cached_completion_tokens?: number;
  elapsed_time?: number;
  request_id?: string;
  trace_id?: string;
  content?: string;
  is_stream?: boolean;
  system_prompt_reset?: boolean;
  metadata?: LogMetadata;
}
