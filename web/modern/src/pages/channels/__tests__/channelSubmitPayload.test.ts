import { describe, expect, it } from 'vitest';

import { buildChannelSubmitPayload, type BuildChannelSubmitPayloadOptions } from '../helpers';
import type { ChannelConfigForm, ChannelForm } from '../schemas';

/**
 * baseConfig returns a minimal but realistic ChannelConfigForm. Tests can
 * override individual fields without having to spell out every option.
 */
const baseConfig = (overrides: Partial<ChannelConfigForm> = {}): ChannelConfigForm => ({
  region: '',
  ak: '',
  sk: '',
  user_id: '',
  vertex_ai_project_id: '',
  vertex_ai_adc: '',
  auth_type: 'personal_access_token',
  api_format: 'chat_completion',
  supported_endpoints: [],
  mcp_tool_blacklist: [],
  spark_app_id: '',
  spark_api_secret: '',
  spark_api_key: '',
  tencent_app_id: '',
  tencent_secret_id: '',
  tencent_secret_key: '',
  ...overrides,
});

/**
 * baseForm returns a fully-populated ChannelForm so each test can override
 * just the fields it cares about. Defaults match the form's `defaultValues`.
 */
const baseForm = (overrides: Partial<ChannelForm> = {}): ChannelForm => ({
  name: 'Test Channel',
  type: 1,
  key: 'sk-test-key',
  base_url: '',
  other: '',
  models: ['gpt-4o'],
  hidden_models: [],
  model_mapping: '',
  model_configs: '',
  tooling: '',
  system_prompt: '',
  groups: ['default'],
  priority: 0,
  weight: 0,
  ratelimit: 0,
  config: baseConfig(),
  inference_profile_arn_map: '',
  ...overrides,
});

const editOpts = (overrides: Partial<BuildChannelSubmitPayloadOptions> = {}): BuildChannelSubmitPayloadOptions => ({
  isEdit: true,
  watchType: 1,
  watchConfig: baseConfig(),
  ...overrides,
});

const createOpts = (overrides: Partial<BuildChannelSubmitPayloadOptions> = {}): BuildChannelSubmitPayloadOptions => ({
  isEdit: false,
  watchType: 1,
  watchConfig: baseConfig(),
  ...overrides,
});

/**
 * Cleared nullable fields must be sent as JSON `null` (key present, value
 * null) so the backend can distinguish "user wants to clear me" from
 * "user did not touch me". Sending `undefined` would let GORM's
 * `Updates(struct)` silently skip the column. Sending `""` would mean
 * "set me to empty string", not "clear me". This regression test locks in
 * the contract.
 */
describe('buildChannelSubmitPayload nullable empty fields', () => {
  const nullableFields: Array<keyof ChannelForm> = ['model_mapping', 'model_configs', 'inference_profile_arn_map', 'system_prompt'];

  for (const field of nullableFields) {
    it(`sends "${field}" as JSON null when the user clears it (empty string)`, () => {
      const data = baseForm({ [field]: '' } as Partial<ChannelForm>);

      const payload = buildChannelSubmitPayload(data, editOpts());

      // Key MUST be present.
      expect(Object.hasOwn(payload, field)).toBe(true);
      // Value MUST be null, not undefined and not empty string.
      expect(payload[field]).toBeNull();
      expect(payload[field]).not.toBe('');
      expect(payload[field]).not.toBeUndefined();

      // Cross-check via JSON serialisation: backend will see literal "null".
      const serialised = JSON.stringify(payload);
      const reparsed = JSON.parse(serialised) as Record<string, unknown>;
      expect(Object.hasOwn(reparsed, field)).toBe(true);
      expect(reparsed[field]).toBeNull();
    });

    it(`sends "${field}" as JSON null when the user enters whitespace-only input`, () => {
      const data = baseForm({ [field]: '   ' } as Partial<ChannelForm>);

      const payload = buildChannelSubmitPayload(data, editOpts());

      expect(Object.hasOwn(payload, field)).toBe(true);
      expect(payload[field]).toBeNull();
    });
  }
});

describe('buildChannelSubmitPayload preserves non-empty values', () => {
  it('keeps a present model_mapping in the payload (sanitised to strict JSON)', () => {
    const mapping = '{"gpt-4o":"gpt-4o-2024-05-13"}';
    const data = baseForm({ model_mapping: mapping });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.model_mapping).toBe(mapping);
    expect(payload.model_mapping).not.toBeNull();
  });

  it('sanitises JSONC model_configs to strict JSON without changing semantics', () => {
    const jsonc = '{ /* gpt-4o pricing */ "gpt-4o": { "ratio": 0.03, } }';
    const data = baseForm({ model_configs: jsonc });

    const payload = buildChannelSubmitPayload(data, editOpts());

    // No comments, no trailing commas, valid strict JSON.
    expect(payload.model_configs).toBe('{"gpt-4o":{"ratio":0.03}}');
    expect(() => JSON.parse(payload.model_configs as string)).not.toThrow();
  });

  it('keeps a present inference_profile_arn_map in the payload', () => {
    const arnMap = '{"foo":"arn:aws:bedrock:us-east-1:123:inference-profile/foo"}';
    const data = baseForm({ inference_profile_arn_map: arnMap });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.inference_profile_arn_map).toBe(arnMap);
  });

  it('keeps a present system_prompt in the payload (no sanitisation applied)', () => {
    const prompt = 'You are a helpful assistant.';
    const data = baseForm({ system_prompt: prompt });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.system_prompt).toBe(prompt);
  });
});

describe('buildChannelSubmitPayload key handling on edit', () => {
  it('removes the key field entirely when editing with an empty key', () => {
    const data = baseForm({ key: '' });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(Object.hasOwn(payload, 'key')).toBe(false);
    expect(payload.key).toBeUndefined();
  });

  it('removes the key field when editing with a whitespace-only key', () => {
    const data = baseForm({ key: '   ' });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(Object.hasOwn(payload, 'key')).toBe(false);
  });

  it('keeps a non-empty key when editing', () => {
    const data = baseForm({ key: 'sk-new-rotation' });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.key).toBe('sk-new-rotation');
  });

  it('keeps an empty key on create so downstream validation can reject it', () => {
    // The hook enforces "API key is required" on create BEFORE building the
    // payload. The helper itself must not silently strip the field on create
    // — that would mask validation bugs.
    const data = baseForm({ key: '' });

    const payload = buildChannelSubmitPayload(data, createOpts());

    expect(Object.hasOwn(payload, 'key')).toBe(true);
    expect(payload.key).toBe('');
  });
});

describe('buildChannelSubmitPayload misc shape guarantees', () => {
  it('joins models[] into a comma-separated string', () => {
    const data = baseForm({ models: ['gpt-4o', 'gpt-4o-mini'] });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.models).toBe('gpt-4o,gpt-4o-mini');
  });

  it('serialises hidden_models as JSON array string when non-empty', () => {
    const data = baseForm({ hidden_models: ['gpt-4o', '  gpt-4o-mini  ', '', 'gpt-4o'] });

    const payload = buildChannelSubmitPayload(data, editOpts());

    // Trims, removes empties, dedupes.
    expect(payload.hidden_models).toBe('["gpt-4o","gpt-4o-mini"]');
  });

  it('sets hidden_models to null when the trimmed list is empty', () => {
    const data = baseForm({ hidden_models: ['', '   '] });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(Object.hasOwn(payload, 'hidden_models')).toBe(true);
    expect(payload.hidden_models).toBeNull();
  });

  it('joins groups[] into a comma-separated `group` field and removes `groups`', () => {
    const data = baseForm({ groups: ['default', 'vip'] });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.group).toBe('default,vip');
    expect(Object.hasOwn(payload, 'groups')).toBe(false);
  });

  it('stringifies the config object', () => {
    const config = baseConfig({ region: 'us-east-1', ak: 'AKIA...' });
    const data = baseForm({ config });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(typeof payload.config).toBe('string');
    expect(JSON.parse(payload.config as string)).toEqual(config);
  });

  it('strips a trailing slash from base_url', () => {
    const data = baseForm({ base_url: 'https://api.example.com/' });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.base_url).toBe('https://api.example.com');
  });

  it('coerces priority/weight/ratelimit to integers', () => {
    const data = baseForm({
      priority: '5' as unknown as number,
      weight: 3.7 as unknown as number,
      ratelimit: '12' as unknown as number,
    });

    const payload = buildChannelSubmitPayload(data, editOpts());

    expect(payload.priority).toBe(5);
    expect(payload.weight).toBe(3);
    expect(payload.ratelimit).toBe(12);
  });

  it('applies the default Azure api_version when type is 3 and `other` is empty', () => {
    const data = baseForm({ type: 3, other: '' });

    const payload = buildChannelSubmitPayload(data, editOpts({ watchType: 3 }));

    expect(payload.other).toBe('2024-03-01-preview');
  });

  it('builds composite AWS key from ak/sk/region for type 33', () => {
    const config = baseConfig({ ak: 'AK', sk: 'SK', region: 'us-east-1' });
    const data = baseForm({ type: 33, config });

    const payload = buildChannelSubmitPayload(data, editOpts({ watchType: 33, watchConfig: config }));

    expect(payload.key).toBe('AK|SK|us-east-1');
  });
});
