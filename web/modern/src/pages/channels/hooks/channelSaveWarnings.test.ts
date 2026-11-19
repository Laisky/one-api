import { api } from '@/lib/api';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { BuildChannelSubmitPayloadOptions } from '../helpers';
import type { ChannelForm } from '../schemas';
import {
  CHANNEL_PRICING_READINESS_ENDPOINT,
  CHANNEL_PRICING_READINESS_TIMEOUT_MS,
  fetchChannelPricingGaps,
  getChannelMappingWarnings,
} from './channelSaveWarnings';

vi.mock('@/lib/api', () => ({
  api: {
    post: vi.fn(),
  },
}));

const mockApiPost = vi.mocked(api.post);

/** buildForm returns a minimal valid channel form, overridden by the given fields. */
const buildForm = (overrides: Partial<ChannelForm> = {}): ChannelForm =>
  ({
    name: 'Azure channel',
    type: 3,
    key: 'sk-secret-key-never-sent',
    base_url: 'https://example.openai.azure.com',
    other: '',
    models: ['gpt-4o', 'cyber'],
    hidden_models: [],
    model_mapping: '{"cyber": "gpt-6-cyber"}',
    model_configs: '',
    tooling: '',
    system_prompt: '',
    groups: ['default'],
    priority: 0,
    weight: 0,
    ratelimit: 0,
    config: { auth_type: 'personal_access_token', api_format: 'chat_completion' },
    inference_profile_arn_map: '',
    ...overrides,
  }) as ChannelForm;

const payloadOptions: BuildChannelSubmitPayloadOptions = {
  isEdit: true,
  watchType: 3,
  watchConfig: { auth_type: 'personal_access_token', api_format: 'chat_completion' },
};

const cyberGap = {
  model: 'cyber',
  upstream_model: 'gpt-6-cyber',
  requirement: 'token_tariff',
  message: 'model "gpt-6-cyber" requires explicit positive input and output pricing in channel model_configs',
};

describe('getChannelMappingWarnings', () => {
  it('reports unreachable keys and unknown targets with exact trimmed IDs', () => {
    const warnings = getChannelMappingWarnings(
      buildForm({ type: 1, models: ['Public-Alias'], model_mapping: '{"public-alias":"gpt-4o"," Public-Alias ":"GPT-4o"}' }),
      { 1: ['Public-Alias', 'GPT-4o'] }
    );
    expect(warnings.unreachableKeys).toEqual(['public-alias']);
    expect(warnings.unknownTargets).toEqual([{ source: 'public-alias', target: 'gpt-4o' }]);
  });

  it('skips target checks without a catalog and ignores empty or malformed mappings', () => {
    expect(getChannelMappingWarnings(buildForm({ models: ['a'], model_mapping: '{"a":"anything"}' }), {})).toEqual({
      unreachableKeys: [],
      unknownTargets: [],
    });
    expect(getChannelMappingWarnings(buildForm({ model_mapping: '' }), {})).toEqual({ unreachableKeys: [], unknownTargets: [] });
    expect(getChannelMappingWarnings(buildForm({ model_mapping: '{not json' }), {})).toEqual({ unreachableKeys: [], unknownTargets: [] });
    expect(getChannelMappingWarnings(buildForm({ model_mapping: '["a"]' }), {})).toEqual({ unreachableKeys: [], unknownTargets: [] });
  });
});

describe('fetchChannelPricingGaps', () => {
  let warn: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    warn.mockRestore();
  });

  it('posts the draft encoded like the save payload, without credentials, and returns the gaps', async () => {
    mockApiPost.mockResolvedValue({ data: { success: true, message: '', data: { gaps: [cyberGap] } } });

    const gaps = await fetchChannelPricingGaps(
      buildForm({ model_configs: '{\n  // JSONC comment\n  "gpt-4o": {"ratio": 1}\n}' }),
      payloadOptions
    );

    expect(gaps).toEqual([cyberGap]);
    expect(mockApiPost).toHaveBeenCalledTimes(1);
    const [url, body, config] = mockApiPost.mock.calls[0];
    expect(url).toBe(CHANNEL_PRICING_READINESS_ENDPOINT);
    expect(body).toEqual({
      type: 3,
      models: 'gpt-4o,cyber',
      model_mapping: '{"cyber":"gpt-6-cyber"}',
      model_configs: '{"gpt-4o":{"ratio":1}}',
    });
    expect(JSON.stringify(body)).not.toContain('sk-secret-key-never-sent');
    expect(config).toEqual({ timeout: CHANNEL_PRICING_READINESS_TIMEOUT_MS });
    expect(warn).not.toHaveBeenCalled();
  });

  it('sends null for cleared mapping and configs', async () => {
    mockApiPost.mockResolvedValue({ data: { success: true, data: { gaps: [] } } });

    await expect(fetchChannelPricingGaps(buildForm({ model_mapping: '', model_configs: '   ' }), payloadOptions)).resolves.toEqual([]);
    expect(mockApiPost.mock.calls[0][1]).toEqual({ type: 3, models: 'gpt-4o,cyber', model_mapping: null, model_configs: null });
  });

  it('skips the request when the draft has no supported models or channel type', async () => {
    await expect(fetchChannelPricingGaps(buildForm({ models: [] }), payloadOptions)).resolves.toEqual([]);
    await expect(fetchChannelPricingGaps(buildForm({ type: undefined as unknown as number }), payloadOptions)).resolves.toEqual([]);
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it('fails open with a string-only console warning when the request fails', async () => {
    mockApiPost.mockRejectedValue(new Error('Request failed with status code 500'));

    await expect(fetchChannelPricingGaps(buildForm(), payloadOptions)).resolves.toEqual([]);
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0]).toHaveLength(1);
    expect(typeof warn.mock.calls[0][0]).toBe('string');
    expect(warn.mock.calls[0][0]).toContain('Request failed with status code 500');
  });

  it('fails open on an unsuccessful or malformed response', async () => {
    for (const data of [
      undefined,
      { success: false, message: 'Unsupported channel type' },
      { success: true, data: {} },
      { success: true, data: { gaps: 'none' } },
    ]) {
      mockApiPost.mockResolvedValueOnce({ data });
      await expect(fetchChannelPricingGaps(buildForm(), payloadOptions)).resolves.toEqual([]);
    }
    mockApiPost.mockResolvedValueOnce(undefined as never);
    await expect(fetchChannelPricingGaps(buildForm(), payloadOptions)).resolves.toEqual([]);
    expect(warn).toHaveBeenCalledTimes(5);
    for (const call of warn.mock.calls) {
      expect(call).toHaveLength(1);
      expect(typeof call[0]).toBe('string');
    }
    expect(warn.mock.calls[1][0]).toContain('Unsupported channel type');
  });

  it('drops malformed gap entries but keeps well-formed ones', async () => {
    mockApiPost.mockResolvedValue({
      data: { success: true, data: { gaps: [null, { model: '' }, { model: 'x', requirement: 1 }, cyberGap] } },
    });

    await expect(fetchChannelPricingGaps(buildForm(), payloadOptions)).resolves.toEqual([cyberGap]);
  });
});
