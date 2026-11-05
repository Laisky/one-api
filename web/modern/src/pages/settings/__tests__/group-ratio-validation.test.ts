import { describe, expect, it } from 'vitest';
import { groupRatioKeys, validateGroupRatioJSON } from '../group-ratio-validation';

/** existing builds the grandfathered key set of a stored GroupRatio value. */
const existing = (...keys: string[]) => new Set(keys);

describe('groupRatioKeys', () => {
  it('returns the exact keys of a stored value', () => {
    expect(groupRatioKeys('{"default":1," vip":2,"legacy group":1}')).toEqual(existing('default', ' vip', 'legacy group'));
  });

  it('returns an empty set for empty or unparsable text', () => {
    expect(groupRatioKeys('')).toEqual(existing());
    expect(groupRatioKeys('{oops')).toEqual(existing());
    expect(groupRatioKeys('[1]')).toEqual(existing());
  });
});

describe('validateGroupRatioJSON', () => {
  it('accepts valid new names and JSONC input', () => {
    expect(validateGroupRatioJSON('{\n  "default": 1, // base\n  "team_a.v-2": 0,\n}', existing('default'))).toBeNull();
  });

  it('reports syntax and shape problems first', () => {
    expect(validateGroupRatioJSON('{"default":', existing())).toEqual({ type: 'parse', message: expect.any(String) });
    expect(validateGroupRatioJSON('   ', existing())).toEqual({ type: 'shape' });
    expect(validateGroupRatioJSON('[]', existing())).toEqual({ type: 'shape' });
    expect(validateGroupRatioJSON('null', existing())).toEqual({ type: 'shape' });
  });

  it('keeps rejecting ratios that are not finite numbers >= 0, for new and legacy keys', () => {
    expect(validateGroupRatioJSON('{"default":-1,"vip":"0.5","svip":0.5}', existing('default'))).toEqual({
      type: 'invalid-entries',
      entries: ['default', 'vip'],
    });
  });

  it('reports every new key that breaks the group-name rule with its reason', () => {
    const raw = JSON.stringify({ default: 1, 'my group': 1, ' vip': 1, '': 1, ['a'.repeat(33)]: 1 });
    expect(validateGroupRatioJSON(raw, existing('default'))).toEqual({
      type: 'invalid-names',
      entries: [
        { key: 'my group', reason: 'invalid_chars' },
        { key: ' vip', reason: 'whitespace' },
        { key: '', reason: 'empty' },
        { key: 'a'.repeat(33), reason: 'too_long' },
      ],
    });
  });

  it('grandfathers legacy keys that break the rule', () => {
    expect(
      validateGroupRatioJSON('{"default":1,"legacy group":0.5," old":1,"":1}', existing('default', 'legacy group', ' old', ''))
    ).toBeNull();
  });

  it('treats a renamed legacy key as new', () => {
    expect(validateGroupRatioJSON('{"default":1,"legacy group 2":1}', existing('default', 'legacy group'))).toEqual({
      type: 'invalid-names',
      entries: [{ key: 'legacy group 2', reason: 'invalid_chars' }],
    });
  });

  it('rejects keys that differ only by case when at least one of them is new', () => {
    expect(validateGroupRatioJSON('{"gold":1,"Gold":0.5}', existing('gold'))).toEqual({
      type: 'case-conflicts',
      groups: [['gold', 'Gold']],
    });
    expect(validateGroupRatioJSON('{"Gold":1,"gold":0.5}', existing('gold'))).toEqual({
      type: 'case-conflicts',
      groups: [['Gold', 'gold']],
    });
    expect(validateGroupRatioJSON('{"Gold":1,"gold":0.5,"GOLD":1}', existing())).toEqual({
      type: 'case-conflicts',
      groups: [['Gold', 'gold', 'GOLD']],
    });
  });

  it('tolerates a legacy pair that already differs only by case', () => {
    expect(validateGroupRatioJSON('{"Gold":0.9,"gold":1}', existing('Gold', 'gold'))).toBeNull();
  });

  it('flags a new spelling added next to a legacy case pair', () => {
    expect(validateGroupRatioJSON('{"Gold":1,"gold":1,"GOLD":1}', existing('Gold', 'gold'))).toEqual({
      type: 'case-conflicts',
      groups: [['Gold', 'gold', 'GOLD']],
    });
  });
});
