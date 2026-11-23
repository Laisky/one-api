import { describe, expect, it } from 'vitest';

import { MODEL_CONFIG_RECOGNIZED_KEYS, validateModelConfigs } from '../helpers';

// The backend accepts any model_configs entry whose fields are a subset of model.ModelConfigLocal,
// including entries without ratio/completion_ratio. A stricter save-time rule blocks existing
// channels after upgrade, so these tests pin the accepted shapes.
describe('validateModelConfigs accepts every backend-valid entry shape', () => {
  it.each([
    ['image-only GPT Image reservation', { 'gpt-image-2.5-flare': { image: { price_per_image_usd: 0.2 } } }],
    ['video-only', { 'sora-2': { video: { per_second_usd: 0.1 } } }],
    ['tiers-only', { 'gemini-3-pro': { tiers: [{ ratio: 1.5, completion_ratio: 4, input_token_threshold: 200000 }] } }],
    ['per_call-only', { 'flux-pro': { per_call: { usd_per_thousand_calls: 50 } } }],
    ['empty entry', { 'dall-e-2': {} }],
    ['cache-only', { 'claude-opus-5': { cached_input_ratio: 0.1, cache_write_5m_ratio: 1.25 } }],
    ['context_length-only', { 'gpt-4o': { context_length: 128000 } }],
    ['recognized key next to an unknown key', { 'gpt-4o': { ratio: 1, note: 'kept for humans' } }],
  ])('accepts a %s entry', (_name, configs) => {
    expect(validateModelConfigs(JSON.stringify(configs))).toEqual({ valid: true });
  });

  it('accepts the legacy image-price migration output for positive and zero legacy prices', () => {
    // MigrateChannelLegacyImagePricing rewrites {"image_price_usd": 0.04} to {"image": {...}}
    // and drops a zero legacy price entirely, leaving {} when nothing else was configured.
    const migrated = '{"dall-e-3":{"image":{"price_per_image_usd":0.04}},"dall-e-2":{},"gpt-image-1":{"max_tokens":4096}}';
    expect(validateModelConfigs(migrated)).toEqual({ valid: true });
  });

  it('accepts every ModelConfigLocal key on its own', () => {
    const samples: Record<string, unknown> = {
      tiers: [],
      time_windows: [],
      max_tokens: 1,
      context_length: 1,
      ratio: 1,
      completion_ratio: 1,
    };
    for (const key of MODEL_CONFIG_RECOGNIZED_KEYS) {
      const value = key in samples ? samples[key] : key.endsWith('_ratio') ? 0.5 : {};
      expect(validateModelConfigs(JSON.stringify({ m: { [key]: value } })), key).toEqual({ valid: true });
    }
  });

  it('accepts JSONC with an image-only entry', () => {
    const input = `{
  // GPT Image reservation only
  "gpt-image-2.5-flare": { "image": { "price_per_image_usd": 0.2, }, },
}`;
    expect(validateModelConfigs(input)).toEqual({ valid: true });
  });
});

describe('validateModelConfigs rejects likely typos and invalid values', () => {
  it('rejects a non-empty entry whose keys are all unrecognized and names them', () => {
    const result = validateModelConfigs('{"gpt-4o":{"ratoi":1,"completion_raito":2}}');
    expect(result.valid).toBe(false);
    expect(result.error).toBe('Model "gpt-4o" has no recognized configuration keys (unknown: ratoi, completion_raito)');
    expect(result.errorKey).toBe('validation.model_configs_unrecognized_keys');
    expect(result.errorParams).toEqual({ model: 'gpt-4o', keys: 'ratoi, completion_raito' });
  });

  it('keeps the existing type checks for ratio, completion_ratio, max_tokens, and time_windows', () => {
    expect(validateModelConfigs('{"m":{"image":{"price_per_image_usd":0.2},"ratio":-1}}').valid).toBe(false);
    expect(validateModelConfigs('{"m":{"completion_ratio":"2"}}').valid).toBe(false);
    expect(validateModelConfigs('{"m":{"max_tokens":1.5}}').valid).toBe(false);
    expect(validateModelConfigs('{"m":{"time_windows":{}}}').valid).toBe(false);
  });

  it('still rejects non-object entries and non-object roots', () => {
    expect(validateModelConfigs('{"m":1}').valid).toBe(false);
    expect(validateModelConfigs('{"m":null}').valid).toBe(false);
    expect(validateModelConfigs('{"m":[]}').valid).toBe(false);
    expect(validateModelConfigs('[]').valid).toBe(false);
  });
});
