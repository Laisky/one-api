import { api } from '@/lib/api';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { BuildChannelSubmitPayloadOptions } from '../helpers';
import type { ChannelForm } from '../schemas';
import {
  CHANNEL_PRICING_READINESS_ENDPOINT,
  CHANNEL_PRICING_READINESS_TIMEOUT_MS,
  collectChannelSaveWarnings,
  fetchChannelPricingGaps,
  getChannelMappingWarnings,
  type ChannelPricingGap,
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

/** gapFor builds a token_tariff gap for a supported model and its mapped upstream model. */
const gapFor = (model: string, upstream = model): ChannelPricingGap => ({
  model,
  upstream_model: upstream,
  requirement: 'token_tariff',
  message: `model "${upstream}" requires explicit positive input and output pricing in channel model_configs`,
});

/** respondByModels answers each readiness request from its comma-separated models: listed gaps, or a rejection. */
const respondByModels = (responses: Record<string, ChannelPricingGap[] | Error>) => {
  mockApiPost.mockImplementation(async (_url: string, body: unknown) => {
    const models = (body as { models: string }).models;
    const response = responses[models];
    if (response === undefined) throw new Error(`unexpected readiness request for ${models}`);
    if (response instanceof Error) throw response;
    return { data: { success: true, message: '', data: { gaps: response } } };
  });
};

/** requestedModels lists the models field of every readiness request sent so far. */
const requestedModels = (): string[] => mockApiPost.mock.calls.map((call) => (call[1] as { models: string }).models);

describe('collectChannelSaveWarnings', () => {
  let warn: ReturnType<typeof vi.spyOn>;
  let debug: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    debug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  });

  afterEach(() => {
    warn.mockRestore();
    debug.mockRestore();
  });

  const catalog = { 3: ['gpt-4o', 'gpt-6-cyber'] };
  const loaded = buildForm({ models: ['gpt-4o', 'cyber'], model_mapping: '{"cyber": "gpt-6-cyber", "ghost": "retired-model"}' });

  it('creating a channel shows every warning', async () => {
    respondByModels({ 'gpt-4o,cyber': [cyberGap] });

    const warnings = await collectChannelSaveWarnings(loaded, { baseline: null, modelsCatalog: catalog, payloadOptions });

    expect(warnings.pricingGaps).toEqual([cyberGap]);
    expect(warnings.unreachableKeys).toEqual(['ghost']);
    expect(warnings.unknownTargets).toEqual([{ source: 'ghost', target: 'retired-model' }]);
    expect(requestedModels()).toEqual(['gpt-4o,cyber']);
  });

  it('suppresses pre-existing gaps and mapping warnings without any request when pricing inputs are unchanged', async () => {
    const draft = buildForm({ ...loaded, name: 'Renamed channel', priority: 5 });

    const warnings = await collectChannelSaveWarnings(draft, { baseline: loaded, modelsCatalog: catalog, payloadOptions });

    expect(warnings).toEqual({ unreachableKeys: [], unknownTargets: [], pricingGaps: [] });
    expect(mockApiPost).not.toHaveBeenCalled();
    expect(warn).not.toHaveBeenCalled();
    for (const call of debug.mock.calls) {
      expect(call).toHaveLength(1);
      expect(typeof call[0]).toBe('string');
    }
  });

  it('shows only the gap of a newly added unpriced model and runs both checks in parallel', async () => {
    const draft = buildForm({ ...loaded, models: ['gpt-4o', 'cyber', 'gpt-6-new'] });
    const newGap = gapFor('gpt-6-new');
    const pending: Array<() => void> = [];
    mockApiPost.mockImplementation((_url: string, body: unknown) => {
      const models = (body as { models: string }).models;
      const gaps = models === 'gpt-4o,cyber' ? [cyberGap] : [cyberGap, newGap];
      return new Promise((resolve) => pending.push(() => resolve({ data: { success: true, data: { gaps } } })));
    });

    const result = collectChannelSaveWarnings(draft, { baseline: loaded, modelsCatalog: catalog, payloadOptions });
    // Both requests are in flight before either answers.
    await vi.waitFor(() => expect(mockApiPost).toHaveBeenCalledTimes(2));
    expect(requestedModels().sort()).toEqual(['gpt-4o,cyber', 'gpt-4o,cyber,gpt-6-new']);
    pending.forEach((resolve) => resolve());

    const warnings = await result;
    expect(warnings.pricingGaps).toEqual([newGap]);
    // The stored mapping's problems predate the edit.
    expect(warnings.unreachableKeys).toEqual([]);
    expect(warnings.unknownTargets).toEqual([]);
  });

  it('shows mapping warnings the edit introduced while hiding the loaded ones', async () => {
    const draft = buildForm({ ...loaded, model_mapping: '{"cyber": "gpt-6-cyber", "ghost": "retired-model", "alias": "typo-model"}' });
    respondByModels({ 'gpt-4o,cyber': [cyberGap] });

    const warnings = await collectChannelSaveWarnings(draft, { baseline: loaded, modelsCatalog: catalog, payloadOptions });

    expect(warnings.unreachableKeys).toEqual(['alias']);
    expect(warnings.unknownTargets).toEqual([{ source: 'alias', target: 'typo-model' }]);
    expect(warnings.pricingGaps).toEqual([]);
  });

  it('changing the channel type shows every warning', async () => {
    const draft = buildForm({ ...loaded, type: 1 });
    respondByModels({ 'gpt-4o,cyber': [cyberGap] });

    const warnings = await collectChannelSaveWarnings(draft, {
      baseline: loaded,
      modelsCatalog: { 1: ['gpt-4o', 'gpt-6-cyber'] },
      payloadOptions: { ...payloadOptions, watchType: 1 },
    });

    expect(warnings.pricingGaps).toEqual([cyberGap]);
    expect(warnings.unreachableKeys).toEqual(['ghost']);
    expect(warnings.unknownTargets).toEqual([{ source: 'ghost', target: 'retired-model' }]);
    expect(requestedModels()).toEqual(['gpt-4o,cyber']);
  });

  it('when only the baseline check fails, hides gaps of unchanged models and shows those the edit touched', async () => {
    const baseline = buildForm({ ...loaded, models: ['gpt-4o', 'cyber', 'plain'], model_configs: '{"plain": {"ratio": 0}}' });
    // The draft adds a model and edits the configs entry of "plain", but leaves "cyber" untouched.
    const draft = buildForm({
      ...baseline,
      models: ['gpt-4o', 'cyber', 'plain', 'gpt-6-new'],
      model_configs: '{"plain": {"ratio": 0, "completion_ratio": 0}}',
    });
    const plainGap = gapFor('plain');
    const newGap = gapFor('gpt-6-new');
    respondByModels({
      'gpt-4o,cyber,plain': new Error('timeout of 5000ms exceeded'),
      'gpt-4o,cyber,plain,gpt-6-new': [cyberGap, plainGap, newGap],
    });

    const warnings = await collectChannelSaveWarnings(draft, { baseline, modelsCatalog: catalog, payloadOptions });

    expect(warnings.pricingGaps).toEqual([plainGap, newGap]);
    expect(mockApiPost).toHaveBeenCalledTimes(2);
    expect(warn.mock.calls.length).toBeGreaterThan(0);
    for (const call of warn.mock.calls) {
      expect(call).toHaveLength(1);
      expect(typeof call[0]).toBe('string');
    }
  });

  it('when only the baseline check fails, treats a remapped model as changed', async () => {
    const draft = buildForm({ ...loaded, models: ['gpt-4o', 'cyber', 'extra'], model_mapping: '{"cyber": "gpt-6-cyber-v2"}' });
    const remappedGap = gapFor('cyber', 'gpt-6-cyber-v2');
    respondByModels({
      'gpt-4o,cyber': new Error('Network Error'),
      'gpt-4o,cyber,extra': [remappedGap],
    });

    const warnings = await collectChannelSaveWarnings(draft, { baseline: loaded, modelsCatalog: catalog, payloadOptions });

    expect(warnings.pricingGaps).toEqual([remappedGap]);
  });

  it('fails open with no pricing gaps when the draft check fails', async () => {
    const draft = buildForm({ ...loaded, models: ['gpt-4o', 'cyber', 'gpt-6-new'] });
    respondByModels({ 'gpt-4o,cyber': [], 'gpt-4o,cyber,gpt-6-new': new Error('Request failed with status code 500') });

    const warnings = await collectChannelSaveWarnings(draft, { baseline: loaded, modelsCatalog: catalog, payloadOptions });

    expect(warnings.pricingGaps).toEqual([]);
  });

  it('treats every draft gap as new when the loaded channel had no supported models', async () => {
    const baseline = buildForm({ ...loaded, models: [] });
    respondByModels({ 'gpt-4o,cyber': [cyberGap] });

    const warnings = await collectChannelSaveWarnings(loaded, { baseline, modelsCatalog: catalog, payloadOptions });

    expect(warnings.pricingGaps).toEqual([cyberGap]);
    expect(requestedModels()).toEqual(['gpt-4o,cyber']);
  });
});
