import { api } from '@/lib/api';

/** ChannelModelCatalog maps a channel type ID to model names. */
export type ChannelModelCatalog = Record<number, string[]>;

/**
 * CHANNEL_MODELS_CATALOG_ENDPOINT returns the complete provider model catalog per channel type:
 * every model an adaptor serves, including models that need operator pricing before they are
 * admitted. It is the reference for model selector options and known Model Mapping targets.
 */
export const CHANNEL_MODELS_CATALOG_ENDPOINT = '/api/models';

/**
 * CHANNEL_DEFAULT_MODELS_ENDPOINT returns the zero-config default suggestions per channel type:
 * only models a channel can serve without operator pricing. It feeds the Fill actions only; it
 * never decides whether a model name is valid, and pricing readiness is checked separately.
 */
export const CHANNEL_DEFAULT_MODELS_ENDPOINT = '/api/models/defaults';

/**
 * ChannelModelCatalogs holds both catalogs the channel editor uses: `full` (the complete provider
 * catalog) and `defaults` (the default suggestions, or the complete catalog when the suggestions
 * could not be loaded).
 */
export interface ChannelModelCatalogs {
  full: ChannelModelCatalog;
  defaults: ChannelModelCatalog;
}

/**
 * parseChannelModelCatalog decodes the `data` object of a catalog response. It takes the raw value
 * and returns the catalog with non-numeric type keys, non-array entries, and blank model names
 * dropped, or null when the value is not an object.
 */
export const parseChannelModelCatalog = (data: unknown): ChannelModelCatalog | null => {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return null;
  const catalog: ChannelModelCatalog = {};
  for (const [typeKey, models] of Object.entries(data as Record<string, unknown>)) {
    if (!Array.isArray(models)) continue;
    const typeId = Number(typeKey);
    if (!Number.isFinite(typeId)) continue;
    catalog[typeId] = models.filter((model): model is string => typeof model === 'string' && model.trim() !== '');
  }
  return catalog;
};

/**
 * requestChannelModelCatalog fetches one catalog endpoint. It takes the endpoint path and resolves
 * to the parsed catalog, or rejects with an Error describing a failed request, an unsuccessful
 * response, or a malformed body.
 */
const requestChannelModelCatalog = async (endpoint: string): Promise<ChannelModelCatalog> => {
  const response = await api.get(endpoint);
  const body = response?.data;
  if (!body || body.success !== true) {
    const message = typeof body?.message === 'string' && body.message !== '' ? body.message : 'unsuccessful response';
    throw new Error(message);
  }
  const catalog = parseChannelModelCatalog(body.data);
  if (catalog === null) {
    throw new Error('response has no catalog object');
  }
  return catalog;
};

/** describeError renders an unknown rejection reason as a plain string for console output. */
const describeError = (reason: unknown): string => (reason instanceof Error ? reason.message : String(reason));

/**
 * loadChannelModelCatalogs requests the complete provider catalog and the default suggestions in
 * parallel and resolves to both; it never rejects. A failed complete catalog resolves to an empty
 * catalog (so mapping target checks are skipped rather than guessed) after a string-only console
 * error. Failed default suggestions fall back to the complete catalog, after a string-only console
 * warning, so the Fill actions keep working against an older server.
 */
export const loadChannelModelCatalogs = async (): Promise<ChannelModelCatalogs> => {
  const [fullResult, defaultsResult] = await Promise.allSettled([
    requestChannelModelCatalog(CHANNEL_MODELS_CATALOG_ENDPOINT),
    requestChannelModelCatalog(CHANNEL_DEFAULT_MODELS_ENDPOINT),
  ]);
  let full: ChannelModelCatalog = {};
  if (fullResult.status === 'fulfilled') {
    full = fullResult.value;
  } else {
    console.error(`Error loading models catalog: ${describeError(fullResult.reason)}`);
  }
  if (defaultsResult.status === 'fulfilled') {
    return { full, defaults: defaultsResult.value };
  }
  console.warn(
    `[channel-models] default model suggestions unavailable, Fill uses the complete catalog: ${describeError(defaultsResult.reason)}`
  );
  return { full, defaults: full };
};
