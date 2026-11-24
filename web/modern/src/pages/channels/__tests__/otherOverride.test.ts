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
  removeOtherOverrides,
  scanConfigMembers,
  snapshotStoredOther,
} from '../otherOverride';
import type { ChannelForm } from '../schemas';

/** OverrideState is the stored or submitted `other` value, relay-effective override, and optional stored spellings. */
interface OverrideState {
  other: string;
  override: string | null;
  spellings?: [string, string][];
}

/** OtherOverrideCase is one entry of the shared Go/frontend contract fixture. */
interface OtherOverrideCase {
  name: string;
  type: number;
  override_key: string;
  config_key?: string;
  stored: OverrideState;
  edit: string | null;
  displayed: string;
  payload: OverrideState;
  effective: string;
}

const repoRoot = resolve(__dirname, '../../../../../..');
const fixture = JSON.parse(readFileSync(resolve(repoRoot, 'test/testdata/channel_other_override_cases.json'), 'utf8')) as {
  cases: OtherOverrideCase[];
};

/** stateEntries lists the override members a fixture state places in config, in document order, possibly repeating a key. */
const stateEntries = (testCase: OtherOverrideCase, state: OverrideState): [string, string][] =>
  state.spellings ?? (state.override === null ? [] : [[testCase.config_key ?? testCase.override_key, state.override]]);

/** overrideEntries lists, in config order, every key of config that spells the override key in any letter case. */
const overrideEntries = (config: Record<string, unknown>, key: string): [string, unknown][] =>
  Object.entries(config).filter(([name]) => name.toLowerCase() === key);

/** storedConfigText renders the raw stored config text of a fixture case: an unrelated key, then the override members, repeated keys kept. */
const storedConfigText = (testCase: OtherOverrideCase): string =>
  `{${['"kept":1', ...stateEntries(testCase, testCase.stored).map(([key, value]) => `${JSON.stringify(key)}:${JSON.stringify(value)}`)].join(',')}}`;

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
  it('match override keys the way Go encoding/json decodes the relay config', () => {
    // Pinned against model.Channel.LoadConfig on Go 1.27: keys match case-insensitively with
    // Unicode simple folding (U+017F long s folds to s), underscores stay significant, keys apply
    // in document order so the last string wins, and null or non-string values are skipped.
    expect(readOtherOverride(3, { API_VERSION: 'upper' })).toBe('upper');
    expect(readOtherOverride(3, { Api_Version: 'mixed' })).toBe('mixed');
    expect(readOtherOverride(3, { ['api_ver\u017fion']: 'long-s' })).toBe('long-s');
    expect(readOtherOverride(3, { APIVERSION: 'x', 'api-version': 'y', apiversion: 'z' })).toBe('');
    expect(readOtherOverride(3, { api_version: 'a', API_VERSION: 'b' })).toBe('b');
    expect(readOtherOverride(3, { API_VERSION: 'b', api_version: 'a' })).toBe('a');
    expect(readOtherOverride(3, { api_version: 'a', API_VERSION: '' })).toBe('');
    expect(readOtherOverride(3, { api_version: 'a', API_VERSION: null })).toBe('a');
    expect(readOtherOverride(3, { api_version: 'a', API_VERSION: 5 })).toBe('a');
    expect(readOtherOverride(17, { PLUGIN: 'p' })).toBe('p');
    expect(readOtherOverride(21, { LIBRARY_ID: 'l', library_id: '' })).toBe('');
    expect(effectiveOtherValue(3, 'legacy', { API_VERSION: '2024-10-21' })).toBe('2024-10-21');
    // Raw stored text keeps the repeated keys JSON.parse collapses into the first position.
    const repeated = '{"api_version":"a","API_VERSION":"b","api_version":"c"}';
    expect(readOtherOverride(3, repeated)).toBe('c');
    expect(readOtherOverride(3, JSON.parse(repeated))).toBe('b');
    expect(readOtherOverride(3, '{"API_VERSION":"b","api_version":"a","api_version":""}')).toBe('');
    expect(readOtherOverride(3, '{"api_version":"a","API_VERSION":5}')).toBe('a');
    expect(readOtherOverride(3, 'not json')).toBe('');
  });

  it('scans the top-level members of a JSON object text in document order', () => {
    expect(scanConfigMembers('{"a":1,"A":"x","a":2}')).toEqual([
      ['a', 1],
      ['A', 'x'],
      ['a', 2],
    ]);
    expect(scanConfigMembers(' {\n "\\u0061pi_version" : "v" ,\t"n": {"api_version": ["}", "\\\\"]}, "z": [1, {"k": null}] } ')).toEqual([
      ['api_version', 'v'],
      ['n', { api_version: ['}', '\\'] }],
      ['z', [1, { k: null }]],
    ]);
    expect(scanConfigMembers('{}')).toEqual([]);
    expect(scanConfigMembers('[1]')).toBeNull();
    expect(scanConfigMembers('null')).toBeNull();
    expect(scanConfigMembers('{"a":')).toBeNull();
    expect(scanConfigMembers('')).toBeNull();
  });

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
    expect(effectiveOtherValue(testCase.type, testCase.stored.other, storedConfigText(testCase))).toBe(testCase.displayed);
  });
});

describe('applyOtherFieldEdit', () => {
  it.each(fixture.cases.map((testCase) => [testCase.name, testCase] as const))(
    '%s: produces the fixture payload and effective value',
    (_name, testCase) => {
      // The editor reads the binding from the raw text and edits the JSON.parse result.
      const text = storedConfigText(testCase);
      const config = JSON.parse(text) as Record<string, unknown>;
      const stored = snapshotStoredOther(testCase.type, testCase.stored.other, text);
      const formOther = testCase.edit ?? testCase.displayed;

      const result = applyOtherFieldEdit(stored, testCase.type, formOther, config);

      expect(result.other).toBe(testCase.payload.other);
      expect(overrideEntries(result.config, testCase.override_key)).toEqual(stateEntries(testCase, testCase.payload));
      expect(result.config.kept).toBe(1);
      expect(effectiveOtherValue(testCase.type, result.other, result.config)).toBe(testCase.effective);
      // The input config is never mutated.
      expect(config).toEqual(JSON.parse(text));
    }
  );

  it('collapses repeated spellings into one canonical member at the first position', () => {
    const text = '{"kept":1,"API_VERSION":"a","mid":true,"api_version":"b","Api_Version":"c"}';
    const form = JSON.parse(text) as Record<string, unknown>;
    const binding = snapshotStoredOther(3, 'legacy', text);
    expect(binding).toEqual({ type: 3, other: 'legacy', override: 'c', spellings: 3, key: 'api_version' });

    const untouched = applyOtherFieldEdit(binding, 3, 'c', form);
    expect(Object.entries(untouched.config)).toEqual([
      ['kept', 1],
      ['api_version', 'c'],
      ['mid', true],
    ]);
    expect(untouched.other).toBe('legacy');
    expect(applyOtherFieldEdit(binding, 3, 'next', form).config).toEqual({ kept: 1, api_version: 'next', mid: true });
    expect(applyOtherFieldEdit(binding, 3, '', form).config).toEqual({ kept: 1, mid: true });
  });

  it('keeps one empty member when the repeated spellings hold no override', () => {
    const text = '{"api_version":"a","API_VERSION":""}';
    const binding = snapshotStoredOther(3, 'legacy', text);
    expect(binding.override).toBe('');
    expect(applyOtherFieldEdit(binding, 3, 'legacy', JSON.parse(text)).config).toEqual({ api_version: '' });
    expect(applyOtherFieldEdit(binding, 3, 'typed', JSON.parse(text))).toEqual({ other: 'typed', config: { api_version: '' } });
  });

  it('keeps a single stored spelling in place', () => {
    const config = { API_VERSION: 'win', kept: 1, after: true };
    const edited = applyOtherFieldEdit(snapshotStoredOther(3, '', config), 3, 'next', config);
    expect(Object.keys(edited.config)).toEqual(['API_VERSION', 'kept', 'after']);
    expect(edited.config.API_VERSION).toBe('next');
    expect(edited.config).not.toHaveProperty('api_version');
    const untouched = { api_version: 5, kept: 1 };
    expect(applyOtherFieldEdit(snapshotStoredOther(3, 'legacy', untouched), 3, 'legacy', untouched).config).toEqual(untouched);
  });

  it('removes every spelling of every override key', () => {
    const config = { kept: 1, API_VERSION: 'a', Plugin: 'b', library_id: 'c', APIVERSION: 'unrelated' };
    expect(removeOtherOverrides(config)).toEqual({ kept: 1, APIVERSION: 'unrelated' });
    expect(config).toHaveProperty('API_VERSION', 'a');
  });

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
