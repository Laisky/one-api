import { LOG_TYPES } from '@/lib/constants/logs';

/**
 * Types shared by the log list page and its export hook.
 */

export const LOG_TYPE_TRANSLATION_KEYS: Record<number, string> = {
  [LOG_TYPES.ALL]: 'all',
  [LOG_TYPES.TOPUP]: 'topup',
  [LOG_TYPES.CONSUME]: 'consume',
  [LOG_TYPES.MANAGE]: 'manage',
  [LOG_TYPES.SYSTEM]: 'system',
  [LOG_TYPES.TEST]: 'test',
  [LOG_TYPES.TOOL]: 'tool',
};

/** ExportTracePayload describes trace data embedded in exported log CSV rows. */
export interface ExportTracePayload {
  id: number;
  trace_id: string;
  url: string;
  method: string;
  body_size: number;
  status: number;
  created_at: number;
  updated_at: number;
  timestamps?: Record<string, unknown>;
  durations?: Record<string, unknown>;
  log?: Record<string, unknown>;
}

/**
 * logTypeTranslationKey maps a log type onto its translation key suffix.
 *
 * @param typeValue - the numeric log type.
 * @returns the key suffix, 'unknown' for an unrecognized type.
 */
export function logTypeTranslationKey(typeValue: number): string {
  return LOG_TYPE_TRANSLATION_KEYS[typeValue] ?? 'unknown';
}
