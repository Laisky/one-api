import { api } from '@/lib/api';
import { buildChannelSubmitPayload, normalizeChannelType, parsePrivateChannelJSON, sanitizeJsonInput } from '../helpers';
import type { BuildChannelSubmitPayloadOptions } from '../helpers';
import type { ChannelForm } from '../schemas';

/** ChannelMappingTargetWarning is a Model Mapping entry whose target the channel does not recognize. */
export interface ChannelMappingTargetWarning {
  source: string;
  target: string;
}

/** ChannelMappingWarnings lists Model Mapping keys that cannot be routed and targets that look like typos. */
export interface ChannelMappingWarnings {
  unreachableKeys: string[];
  unknownTargets: ChannelMappingTargetWarning[];
}

/**
 * ChannelPricingGap is one supported model that request admission would refuse until the channel
 * prices it: `model` is the supported model, `upstream_model` the model after Model Mapping,
 * `requirement` the missing pricing code, and `message` the server's refusal text.
 */
export interface ChannelPricingGap {
  model: string;
  upstream_model: string;
  requirement: string;
  message: string;
}

/** CHANNEL_PRICING_READINESS_ENDPOINT is the admin API that evaluates a channel draft's pricing. */
export const CHANNEL_PRICING_READINESS_ENDPOINT = '/api/channel/pricing-readiness';

/** CHANNEL_PRICING_READINESS_TIMEOUT_MS bounds how long saving waits for the advisory check. */
export const CHANNEL_PRICING_READINESS_TIMEOUT_MS = 5000;

/**
 * getChannelMappingWarnings inspects Model Mapping entries against the channel's configuration to
 * surface silent misconfigurations before saving. It takes the submitted form data and the
 * complete per-channel-type provider catalog (/api/models), never the default suggestions: a
 * target that is valid but needs operator pricing is known, and its pricing is the readiness
 * check's concern. It returns:
 * - unreachableKeys: sources (keys) not in Supported Models, so requests to the alias cannot reach this channel;
 * - unknownTargets: targets (values) in neither the supported list nor the channel-type catalog, likely typos.
 */
export const getChannelMappingWarnings = (data: ChannelForm, modelsCatalog: Record<number, string[]>): ChannelMappingWarnings => {
  const empty: ChannelMappingWarnings = { unreachableKeys: [], unknownTargets: [] };
  const mappingRaw = (data.model_mapping || '').trim();
  if (!mappingRaw) {
    return empty;
  }
  let parsed: unknown;
  try {
    parsed = parsePrivateChannelJSON(sanitizeJsonInput(mappingRaw));
  } catch {
    return empty;
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    return empty;
  }
  const supported = new Set((data.models || []).map((model) => model.trim()).filter((model) => model.length > 0));
  const channelType = normalizeChannelType(data.type);
  const catalog = channelType !== null ? (modelsCatalog[channelType] ?? []) : [];
  const catalogSet = new Set(catalog.map((model) => model.trim()).filter((model) => model.length > 0));
  const knownTargets = new Set<string>();
  supported.forEach((entry) => knownTargets.add(entry));
  catalogSet.forEach((entry) => knownTargets.add(entry));

  const unreachableKeys: string[] = [];
  const unknownTargets: ChannelMappingTargetWarning[] = [];
  for (const [rawKey, rawValue] of Object.entries(parsed as Record<string, unknown>)) {
    const key = rawKey.trim();
    if (key.length === 0) continue;
    if (!supported.has(key)) {
      unreachableKeys.push(key);
    }
    if (catalogSet.size === 0) continue; // Cannot judge target validity without a catalog.
    if (typeof rawValue !== 'string') continue;
    const target = rawValue.trim();
    if (target.length === 0) continue;
    if (!knownTargets.has(target)) {
      unknownTargets.push({ source: key, target });
    }
  }
  return { unreachableKeys, unknownTargets };
};

/** isChannelPricingGap reports whether value has the string fields of a ChannelPricingGap. */
const isChannelPricingGap = (value: unknown): value is ChannelPricingGap => {
  if (!value || typeof value !== 'object') return false;
  const gap = value as Record<string, unknown>;
  return (
    typeof gap.model === 'string' &&
    gap.model !== '' &&
    typeof gap.upstream_model === 'string' &&
    typeof gap.requirement === 'string' &&
    typeof gap.message === 'string'
  );
};

/** skipPricingReadiness logs why one advisory check was skipped (as a string) and returns null. */
const skipPricingReadiness = (reason: string): null => {
  console.warn(`[channel-pricing-readiness] check skipped, saving without pricing warnings: ${reason}`);
  return null;
};

/** ChannelPricingReadinessRequest is the body of one CHANNEL_PRICING_READINESS_ENDPOINT request. */
export interface ChannelPricingReadinessRequest {
  type: number;
  models: string;
  model_mapping: string | null;
  model_configs: string | null;
}

/**
 * buildPricingReadinessRequest encodes a channel form for the readiness endpoint exactly like the
 * save payload (sanitized JSONC, cleared fields as null, no credentials). It takes the form data
 * and the payload options, and returns the request body, or null when the form has no channel
 * type or no supported models (nothing to check).
 */
export const buildPricingReadinessRequest = (
  data: ChannelForm,
  opts: BuildChannelSubmitPayloadOptions
): ChannelPricingReadinessRequest | null => {
  const channelType = normalizeChannelType(data.type);
  const payload = buildChannelSubmitPayload(data, opts);
  if (channelType === null || typeof payload.models !== 'string' || payload.models.trim() === '') {
    return null;
  }
  return {
    type: channelType,
    models: payload.models,
    model_mapping: typeof payload.model_mapping === 'string' ? payload.model_mapping : null,
    model_configs: typeof payload.model_configs === 'string' ? payload.model_configs : null,
  };
};

/**
 * requestChannelPricingGaps posts one readiness request. It takes the request body and resolves to
 * the well-formed gaps, or to null (after a string-only console warning) when the check is
 * unavailable: a network error, timeout, unsuccessful response, or malformed body.
 */
const requestChannelPricingGaps = async (request: ChannelPricingReadinessRequest): Promise<ChannelPricingGap[] | null> => {
  try {
    const response = await api.post(CHANNEL_PRICING_READINESS_ENDPOINT, request, { timeout: CHANNEL_PRICING_READINESS_TIMEOUT_MS });
    const body = response?.data;
    if (!body || body.success !== true) {
      const message = typeof body?.message === 'string' && body.message !== '' ? body.message : 'unsuccessful response';
      return skipPricingReadiness(message);
    }
    const gaps: unknown = body.data?.gaps;
    if (!Array.isArray(gaps)) {
      return skipPricingReadiness('response has no gaps list');
    }
    return gaps.filter(isChannelPricingGap);
  } catch (error) {
    return skipPricingReadiness(error instanceof Error ? error.message : String(error));
  }
};

/**
 * fetchChannelPricingGaps asks the server which supported models request admission would refuse
 * for missing channel pricing, sending the draft encoded exactly like the save payload. It takes
 * the submitted form data and the payload options, and resolves to the gaps. The check is advisory
 * and fails open: any error, timeout, or unexpected response resolves to an empty list after a
 * string-only console warning, so saving is never blocked by the check itself.
 */
export const fetchChannelPricingGaps = async (data: ChannelForm, opts: BuildChannelSubmitPayloadOptions): Promise<ChannelPricingGap[]> => {
  let request: ChannelPricingReadinessRequest | null;
  try {
    request = buildPricingReadinessRequest(data, opts);
  } catch (error) {
    return skipPricingReadiness(error instanceof Error ? error.message : String(error)) ?? [];
  }
  if (request === null) {
    return [];
  }
  return (await requestChannelPricingGaps(request)) ?? [];
};

/** ChannelSaveWarnings is everything the save warning dialog may show for one save attempt. */
export interface ChannelSaveWarnings {
  unreachableKeys: string[];
  unknownTargets: ChannelMappingTargetWarning[];
  pricingGaps: ChannelPricingGap[];
}

/**
 * CollectChannelSaveWarningsOptions configures collectChannelSaveWarnings: `baseline` is the
 * channel exactly as loaded for editing (null when creating), `modelsCatalog` the complete
 * per-type provider catalog (not the default suggestions), and `payloadOptions` the draft's
 * save-payload options.
 */
export interface CollectChannelSaveWarningsOptions {
  baseline: ChannelForm | null;
  modelsCatalog: Record<number, string[]>;
  payloadOptions: BuildChannelSubmitPayloadOptions;
}

/** pricingGapKey identifies one pricing gap across the baseline and draft checks. */
export const pricingGapKey = (gap: ChannelPricingGap): string => `${gap.model}:${gap.upstream_model}:${gap.requirement}`;

/** mappingTargetKey identifies one unknown Model Mapping target across the baseline and draft. */
const mappingTargetKey = (warning: ChannelMappingTargetWarning): string => `${warning.source}->${warning.target}`;

/** parseJSONObjectField parses a JSONC object field, returning {} when empty and null when malformed or not an object. */
const parseJSONObjectField = (raw: string | undefined): Record<string, unknown> | null => {
  const text = (raw || '').trim();
  if (text === '') {
    return {};
  }
  try {
    const parsed: unknown = parsePrivateChannelJSON(sanitizeJsonInput(text));
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : null;
  } catch {
    return null;
  }
};

/** canonicalJSON serializes a JSON value with sorted object keys so equal values compare equal as strings. */
const canonicalJSON = (value: unknown): string => {
  if (value === undefined) return 'undefined';
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`;
  const entries = Object.keys(value as Record<string, unknown>)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonicalJSON((value as Record<string, unknown>)[key])}`);
  return `{${entries.join(',')}}`;
};

/** mappedUpstream mirrors the backend's GetMappedModelName: a non-empty string mapping wins, otherwise the model itself. */
const mappedUpstream = (model: string, mapping: Record<string, unknown>): string => {
  const target = mapping[model];
  return typeof target === 'string' && target !== '' ? target : model;
};

/**
 * pricingInputsUnchangedFor returns a predicate reporting whether a supported model's pricing inputs
 * are identical in the baseline and the draft: the model was already supported, maps to the same
 * upstream model, and that upstream model's model_configs entry is unchanged. Readiness evaluates
 * each model from exactly these inputs, so such a model's gaps already existed before the edit.
 * Malformed mapping or configs on either side make every model count as changed.
 */
const pricingInputsUnchangedFor = (baseline: ChannelForm, draft: ChannelForm): ((model: string) => boolean) => {
  const baselineModels = new Set((baseline.models || []).map((model) => model.trim()).filter((model) => model !== ''));
  const baselineMapping = parseJSONObjectField(baseline.model_mapping);
  const draftMapping = parseJSONObjectField(draft.model_mapping);
  const baselineConfigs = parseJSONObjectField(baseline.model_configs);
  const draftConfigs = parseJSONObjectField(draft.model_configs);
  if (!baselineMapping || !draftMapping || !baselineConfigs || !draftConfigs) {
    return () => false;
  }
  return (model: string) => {
    const name = model.trim();
    if (!baselineModels.has(name)) return false;
    const upstream = mappedUpstream(name, draftMapping);
    if (mappedUpstream(name, baselineMapping) !== upstream) return false;
    return canonicalJSON(baselineConfigs[upstream]) === canonicalJSON(draftConfigs[upstream]);
  };
};

/**
 * collectNewPricingGaps returns the draft's pricing gaps that the edit introduced. Identical baseline
 * and draft requests cannot differ, so no request is sent. Otherwise both checks run in parallel;
 * a failed draft check fails open (no gaps), and a successful baseline check removes every gap it
 * also reports. When only the baseline check fails, gaps are suppressed for models whose pricing
 * inputs the edit left unchanged (pricingInputsUnchangedFor), so a transient failure neither
 * re-reports old gaps nor hides gaps of models the admin added or re-priced.
 */
const collectNewPricingGaps = async (
  draft: ChannelForm,
  baseline: ChannelForm,
  payloadOptions: BuildChannelSubmitPayloadOptions
): Promise<ChannelPricingGap[]> => {
  let draftRequest: ChannelPricingReadinessRequest | null;
  let baselineRequest: ChannelPricingReadinessRequest | null;
  try {
    draftRequest = buildPricingReadinessRequest(draft, payloadOptions);
    baselineRequest = buildPricingReadinessRequest(baseline, {
      ...payloadOptions,
      watchType: baseline.type,
      watchConfig: baseline.config ?? payloadOptions.watchConfig,
    });
  } catch (error) {
    return skipPricingReadiness(error instanceof Error ? error.message : String(error)) ?? [];
  }
  if (draftRequest === null) {
    return [];
  }
  if (baselineRequest !== null && JSON.stringify(baselineRequest) === JSON.stringify(draftRequest)) {
    console.debug('[channel-pricing-readiness] pricing inputs unchanged since load, so the edit introduces no pricing gaps');
    return [];
  }
  const [draftGaps, baselineGaps] = await Promise.all([
    requestChannelPricingGaps(draftRequest),
    baselineRequest === null ? Promise.resolve<ChannelPricingGap[]>([]) : requestChannelPricingGaps(baselineRequest),
  ]);
  if (draftGaps === null) {
    return [];
  }
  if (baselineGaps !== null) {
    const existing = new Set(baselineGaps.map(pricingGapKey));
    return draftGaps.filter((gap) => !existing.has(pricingGapKey(gap)));
  }
  console.warn('[channel-pricing-readiness] baseline check failed, hiding gaps of models whose pricing inputs are unchanged');
  const unchanged = pricingInputsUnchangedFor(baseline, draft);
  return draftGaps.filter((gap) => !unchanged(gap.model));
};

/**
 * collectChannelSaveWarnings computes the Model Mapping warnings and pricing gaps to show before
 * saving a channel draft. It takes the validated draft and the options, and resolves to the
 * warnings. When editing a channel whose type is unchanged, only findings the edit introduced are
 * returned: warnings the loaded channel already had (for example after a catalog or pricing-policy
 * upgrade) are subtracted, because the admin did not cause them and saving does not change them.
 * Creating a channel, or changing its type, returns every finding. It never rejects.
 */
export const collectChannelSaveWarnings = async (
  draft: ChannelForm,
  { baseline, modelsCatalog, payloadOptions }: CollectChannelSaveWarningsOptions
): Promise<ChannelSaveWarnings> => {
  const draftMapping = getChannelMappingWarnings(draft, modelsCatalog);
  const draftType = normalizeChannelType(draft.type);
  if (baseline === null || draftType === null || normalizeChannelType(baseline.type) !== draftType) {
    return { ...draftMapping, pricingGaps: await fetchChannelPricingGaps(draft, payloadOptions) };
  }
  const baselineMapping = getChannelMappingWarnings(baseline, modelsCatalog);
  const knownUnreachable = new Set(baselineMapping.unreachableKeys);
  const knownTargets = new Set(baselineMapping.unknownTargets.map(mappingTargetKey));
  return {
    unreachableKeys: draftMapping.unreachableKeys.filter((key) => !knownUnreachable.has(key)),
    unknownTargets: draftMapping.unknownTargets.filter((warning) => !knownTargets.has(mappingTargetKey(warning))),
    pricingGaps: await collectNewPricingGaps(draft, baseline, payloadOptions),
  };
};
