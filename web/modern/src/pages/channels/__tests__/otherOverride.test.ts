import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

import { buildChannelSubmitPayload, CONFIG_OVERRIDES_OF_OTHER } from '../helpers';
import {
  applyOtherFieldEdit,
  effectiveOtherValue,
  OTHER_OVERRIDE_KEY_BY_TYPE,
  otherOverrideKeyFor,
  readOtherOverride,
  snapshotStoredOther,
} from '../otherOverride';
import type { ChannelForm } from '../schemas';

/** OtherOverrideCase is one entry of the shared Go/frontend contract fixture. */
interface OtherOverrideCase {
  name: string;
  type: number;
  override_key: string;
  stored: { other: string; override: string | null };
  edit: string | null;
  displayed: string;
  payload: { other: string; override: string | null };
  effective: string;
}

const repoRoot = resolve(__dirname, '../../../../../..');
const fixture = JSON.parse(readFileSync(resolve(repoRoot, 'test/testdata/channel_other_override_cases.json'), 'utf8')) as {
  cases: OtherOverrideCase[];
};

/** storedConfig builds the stored config object of a fixture case. */
const storedConfig = (testCase: OtherOverrideCase): Record<string, unknown> =>
  testCase.stored.override === null ? { kept: 1 } : { kept: 1, [testCase.override_key]: testCase.stored.override };

/** baseForm returns a validated channel form for the payload builder. */
const baseForm = (overrides: Partial<ChannelForm>): ChannelForm =>
  ({
    name: 'Channel',
    type: 1,
    key: '',
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
    config: { auth_type: 'personal_access_token', api_format: 'chat_completion' },
    inference_profile_arn_map: '',
    ...overrides,
  }) as ChannelForm;

describe('OTHER_OVERRIDE_KEY_BY_TYPE', () => {
  it('mirrors the relay precedence table, including Gemini which has no editor field', () => {
    expect(OTHER_OVERRIDE_KEY_BY_TYPE).toEqual({ 3: 'api_version', 17: 'plugin', 18: 'api_version', 21: 'library_id', 24: 'api_version' });
    expect(otherOverrideKeyFor(1)).toBeNull();
    expect(otherOverrideKeyFor(null)).toBeNull();
    expect([...CONFIG_OVERRIDES_OF_OTHER].sort()).toEqual(['api_version', 'library_id', 'plugin']);
  });

  it('agrees with the shared fixture about each case override key', () => {
    for (const testCase of fixture.cases) {
      expect(otherOverrideKeyFor(testCase.type)).toBe(testCase.override_key);
    }
  });
});

describe('effectiveOtherValue and readOtherOverride', () => {
  it('prefer a non-empty override and fall back to other like the relay', () => {
    expect(effectiveOtherValue(3, '2024-02-01', { api_version: '2024-10-21' })).toBe('2024-10-21');
    expect(effectiveOtherValue(3, '2024-02-01', { api_version: '' })).toBe('2024-02-01');
    expect(effectiveOtherValue(3, '2024-02-01', { library_id: 'ignored' })).toBe('2024-02-01');
    expect(effectiveOtherValue(1, 'legacy', { api_version: 'not-for-openai' })).toBe('legacy');
    expect(effectiveOtherValue(24, '', { api_version: 'v1beta' })).toBe('v1beta');
    expect(effectiveOtherValue(3, undefined, null)).toBe('');
    expect(readOtherOverride(17, { plugin: 7 })).toBe('');
  });

  it.each(fixture.cases.map((testCase) => [testCase.name, testCase] as const))('%s: displays the fixture value', (_name, testCase) => {
    expect(effectiveOtherValue(testCase.type, testCase.stored.other, storedConfig(testCase))).toBe(testCase.displayed);
  });
});

describe('applyOtherFieldEdit', () => {
  it.each(fixture.cases.map((testCase) => [testCase.name, testCase] as const))(
    '%s: produces the fixture payload and effective value',
    (_name, testCase) => {
      const config = storedConfig(testCase);
      const stored = snapshotStoredOther(testCase.type, testCase.stored.other, config);
      const formOther = testCase.edit ?? testCase.displayed;

      const result = applyOtherFieldEdit(stored, testCase.type, formOther, config);

      expect(result.other).toBe(testCase.payload.other);
      if (testCase.payload.override === null) {
        expect(result.config).not.toHaveProperty(testCase.override_key);
      } else {
        expect(result.config[testCase.override_key]).toBe(testCase.payload.override);
      }
      expect(result.config.kept).toBe(1);
      expect(effectiveOtherValue(testCase.type, result.other, result.config)).toBe(testCase.effective);
      // The input config is never mutated.
      expect(config).toEqual(storedConfig(testCase));
    }
  );

  it('passes form values through when creating or after a channel type change', () => {
    expect(applyOtherFieldEdit(null, 3, 'typed', { api_version: 'x' })).toEqual({ other: 'typed', config: { api_version: 'x' } });
    const stored = snapshotStoredOther(3, 'legacy', { api_version: 'override' });
    expect(applyOtherFieldEdit(stored, 17, '', {})).toEqual({ other: '', config: {} });
  });

  it('restores a stored override the form lost while the field still shows it', () => {
    const stored = snapshotStoredOther(3, 'legacy', { api_version: 'override' });
    expect(applyOtherFieldEdit(stored, 3, 'override', {})).toEqual({ other: 'legacy', config: { api_version: 'override' } });
  });

  it('never creates an override for a channel that stored none', () => {
    const stored = snapshotStoredOther(18, 'v3.1', {});
    expect(applyOtherFieldEdit(stored, 18, 'v3.5', {})).toEqual({ other: 'v3.5', config: {} });
    expect(applyOtherFieldEdit(stored, 18, 'v3.5', { api_version: '' })).toEqual({ other: 'v3.5', config: { api_version: '' } });
  });

  it('resends a Gemini override untouched because no editor field changes other', () => {
    const stored = snapshotStoredOther(24, 'v1', { api_version: 'v1beta' });
    expect(applyOtherFieldEdit(stored, 24, 'v1beta', { api_version: 'v1beta' })).toEqual({
      other: 'v1',
      config: { api_version: 'v1beta' },
    });
  });
});

describe('buildChannelSubmitPayload other binding', () => {
  const opts = { isEdit: true, watchConfig: { auth_type: 'personal_access_token', api_format: 'chat_completion' as const } };

  it('does not inject the Azure default when a typed api_version override supplies the version', () => {
    const config = { auth_type: 'personal_access_token', api_format: 'chat_completion' as const, api_version: '2024-10-21' };
    const payload = buildChannelSubmitPayload(baseForm({ type: 3, other: '2024-10-21', config }), {
      ...opts,
      watchType: 3,
      storedOther: snapshotStoredOther(3, '', config),
    });
    expect(payload.other).toBe('');
    expect(JSON.parse(payload.config).api_version).toBe('2024-10-21');
  });

  it('injects the Azure default once an edit clears the override', () => {
    const config = { auth_type: 'personal_access_token', api_format: 'chat_completion' as const, api_version: '2024-10-21' };
    const payload = buildChannelSubmitPayload(baseForm({ type: 3, other: '', config }), {
      ...opts,
      watchType: 3,
      storedOther: snapshotStoredOther(3, '', config),
    });
    expect(payload.other).toBe('2024-03-01-preview');
    expect(JSON.parse(payload.config)).not.toHaveProperty('api_version');
  });

  it('keeps the legacy behavior when no binding is supplied', () => {
    const payload = buildChannelSubmitPayload(baseForm({ type: 17, other: 'plugin-x' }), { ...opts, watchType: 17 });
    expect(payload.other).toBe('plugin-x');
    expect(JSON.parse(payload.config)).not.toHaveProperty('plugin');
  });
});
