import { describe, expect, it } from 'vitest';
import { getGroupNameIssue, isValidGroupName, normalizeGroupName, parseGroupList } from './group-name';
import cases from './testdata/group-names.json';

describe('group-name rule', () => {
  it.each(cases)('matches the shared backend fixture $name', ({ group, valid }) => {
    expect(isValidGroupName(group)).toBe(valid);
  });

  it('allows exactly ASCII letters, digits, ".", "_" and "-"', () => {
    for (let code = 0; code < 128; code += 1) {
      const char = String.fromCharCode(code);
      const allowed = /[A-Za-z0-9._-]/.test(char);
      expect(isValidGroupName(`vip${char}`)).toBe(allowed);
    }
  });

  it('reports the specific issue', () => {
    expect(getGroupNameIssue('')).toBe('empty');
    expect(getGroupNameIssue(' vip')).toBe('whitespace');
    expect(getGroupNameIssue('a'.repeat(33))).toBe('too_long');
    expect(getGroupNameIssue('my group')).toBe('invalid_chars');
    expect(getGroupNameIssue('VIP')).toBeNull();
  });

  it('normalizes by trimming only', () => {
    expect(normalizeGroupName(' \tVIP\n')).toBe('VIP');
  });

  it('parses channel group lists like the backend', () => {
    expect(parseGroupList('')).toEqual([]);
    expect(parseGroupList('default, vip')).toEqual(['default', 'vip']);
    expect(parseGroupList(' Default,default,VIP,vip ')).toEqual(['Default', 'VIP']);
  });
});
