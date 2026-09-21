import { useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { api } from '@/lib/api';

// Nullable Go timestamp pointers become absent optional fields in the UI.
const optionalNumber = z.number().finite().nullish().transform((value) => value ?? undefined);
const optionalString = z.string().nullish().transform((value) => value ?? undefined);
const externalCallSchema = z.object({
  key: optionalString,
  source: optionalString,
  tool: optionalString,
  server_id: optionalNumber,
  server_label: optionalString,
  started_at: optionalNumber,
  ended_at: optionalNumber,
  duration_ms: optionalNumber,
  is_error: z.boolean().nullish().transform((value) => value ?? undefined),
});
const timestampsSchema = z.object({
  request_received: optionalNumber,
  request_forwarded: optionalNumber,
  first_upstream_response: optionalNumber,
  first_client_response: optionalNumber,
  upstream_completed: optionalNumber,
  request_completed: optionalNumber,
  external_calls: z.array(externalCallSchema).nullish().transform((value) => value ?? undefined),
});
const durationsSchema = z.object({
  processing_time: optionalNumber,
  upstream_response_time: optionalNumber,
  response_processing_time: optionalNumber,
  streaming_time: optionalNumber,
  total_time: optionalNumber,
});
const traceSchema = z.object({
  uuid: optionalString,
  trace_id: z.string(),
  url: z.string(),
  method: z.string(),
  body_size: z.number().finite(),
  status: z.number().int(),
  created_at: z.number().finite(),
  updated_at: z.number().finite(),
  timestamps: timestampsSchema,
  durations: durationsSchema.nullish().transform((value) => value ?? undefined),
});
const retentionSchema = z.object({
  availability: z.literal('not_retained_locally'),
  trace_id: z.string(),
});
const responseSchema = z.object({
  success: z.literal(true),
  data: z.union([retentionSchema, traceSchema]),
});

/** TraceData is a validated retained trace; it never represents an error or a retention miss. */
export type TraceData = z.infer<typeof traceSchema>;
/** TraceTimestamps contains nullable-normalized millisecond timestamps and external calls. */
export type TraceTimestamps = z.infer<typeof timestampsSchema>;

/** TraceLoadError contains only safe diagnostics, never database/provider response text. */
interface TraceLoadError {
  status?: number;
}

/** TraceState binds response state to the correlation, viewer and explicit retry that produced it. */
interface TraceState {
  key: string;
  loading: boolean;
  data: TraceData | null;
  error: TraceLoadError | null;
  notRetained: boolean;
}

/** httpFailureStatus extracts a valid HTTP error status without exposing arbitrary error messages. */
function httpFailureStatus(error: unknown): number | undefined {
  if (typeof error !== 'object' || error === null || !('response' in error)) return undefined;
  const response = error.response;
  if (typeof response !== 'object' || response === null || !('status' in response)) return undefined;
  const status = response.status;
  return typeof status === 'number' && Number.isInteger(status) && status >= 400 && status <= 599 ? status : undefined;
}

/**
 * useLogTrace reads a trace by its existing correlation, not by a second billing-log lookup.
 * The server remains the authorization boundary. No trace result is cached across viewers.
 * @param traceID - the opaque correlation already returned by the authorized log list.
 * @param enabled - whether the details dialog is open and a log is selected.
 * @param viewerScope - the authenticated viewer identity/role, used only to discard obsolete UI state.
 * @returns the current trace/error/retention state and an explicit retry action; no automatic polling.
 */
export function useLogTrace(traceID: string | undefined, enabled: boolean, viewerScope: string) {
  // Preserve the exact nonblank correlation. Only URI encoding, not normalization,
  // may change how an opaque legacy identifier is carried in the request path.
  const id = traceID?.trim() ? traceID : '';
  const [attempt, setAttempt] = useState(0);
  const key = enabled && id ? JSON.stringify([id, viewerScope, attempt]) : '';
  const [state, setState] = useState<TraceState>({ key: '', loading: false, data: null, error: null, notRetained: false });
  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  useEffect(() => {
    if (!key) return;
    const controller = new AbortController();
    setState({ key, loading: true, data: null, error: null, notRetained: false });

    /** load fetches and validates one response; aborted or obsolete requests never publish state. */
    const load = async () => {
      try {
        const response = await api.get<unknown>(`/api/trace/${encodeURIComponent(id)}`, { signal: controller.signal });
        const parsed = responseSchema.safeParse(response.data);
        if (!parsed.success || parsed.data.data.trace_id !== id) {
          throw new Error('Invalid trace response');
        }
        if (controller.signal.aborted) return;
        const data = parsed.data.data;
        const notRetained = 'availability' in data;
        setState({ key, loading: false, data: notRetained ? null : data, error: null, notRetained });
      } catch (error: unknown) {
        if (controller.signal.aborted) return;
        setState({ key, loading: false, data: null, error: { status: httpFailureStatus(error) }, notRetained: false });
      }
    };
    void load();
    return () => controller.abort();
  }, [key, id]);

  // Hide previous-log, previous-viewer and previous-attempt data immediately,
  // including the render before the new effect starts its request.
  const current = key && state.key === key ? state : null;
  return {
    traceData: current?.data ?? null,
    traceLoading: Boolean(key) && (current?.loading ?? true),
    traceError: current?.error ?? null,
    traceNotRetainedLocally: current?.notRetained ?? false,
    retry,
  };
}
