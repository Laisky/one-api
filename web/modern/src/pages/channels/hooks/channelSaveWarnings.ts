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
 * per-channel-type model catalog, and returns:
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

/** skipPricingReadiness logs why the advisory check was skipped (as a string) and returns no gaps. */
const skipPricingReadiness = (reason: string): ChannelPricingGap[] => {
  console.warn(`[channel-pricing-readiness] check skipped, saving without pricing warnings: ${reason}`);
  return [];
};

/**
 * fetchChannelPricingGaps asks the server which supported models request admission would refuse
 * for missing channel pricing, sending the draft encoded exactly like the save payload. It takes
 * the submitted form data and the payload options, and resolves to the gaps. The check is advisory
 * and fails open: any error, timeout, or unexpected response resolves to an empty list after a
 * string-only console warning, so saving is never blocked by the check itself.
 */
export const fetchChannelPricingGaps = async (data: ChannelForm, opts: BuildChannelSubmitPayloadOptions): Promise<ChannelPricingGap[]> => {
  try {
    const channelType = normalizeChannelType(data.type);
    const payload = buildChannelSubmitPayload(data, opts);
    if (channelType === null || typeof payload.models !== 'string' || payload.models.trim() === '') {
      return [];
    }
    const response = await api.post(
      CHANNEL_PRICING_READINESS_ENDPOINT,
      {
        type: channelType,
        models: payload.models,
        model_mapping: typeof payload.model_mapping === 'string' ? payload.model_mapping : null,
        model_configs: typeof payload.model_configs === 'string' ? payload.model_configs : null,
      },
      { timeout: CHANNEL_PRICING_READINESS_TIMEOUT_MS }
    );
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
