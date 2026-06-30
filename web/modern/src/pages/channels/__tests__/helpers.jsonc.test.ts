import { describe, expect, it } from 'vitest';

import { formatJSON, isValidJSON, sanitizeJsonField, sanitizeJsonInput, validateModelConfigs, validateToolingConfig } from '../helpers';

describe('sanitizeJsonInput', () => {
  it('passes through clean JSON unchanged', () => {
    const input = '{"a":1,"b":[1,2,3]}';
    expect(sanitizeJsonInput(input)).toBe(input);
  });

  it('strips line and block comments outside strings', () => {
    const input = `{
  // leading note
  "a": 1, /* inline */
  "b": 2 /* trailing */
}`;
    const parsed = JSON.parse(sanitizeJsonInput(input));
    expect(parsed).toEqual({ a: 1, b: 2 });
  });

  it('preserves comment-like text inside string values', () => {
    const input = '{"url":"https://example.com/path","note":"a // b /* c */"}';
    expect(JSON.parse(sanitizeJsonInput(input))).toEqual({
      url: 'https://example.com/path',
      note: 'a // b /* c */',
    });
  });

  it('preserves escaped quotes inside strings', () => {
    const input = '{"msg":"she said \\"hi\\""} // tail';
    expect(JSON.parse(sanitizeJsonInput(input))).toEqual({ msg: 'she said "hi"' });
  });

  it('strips trailing commas in objects and arrays', () => {
    const input = `{
  "list": [1, 2, 3,],
  "map": {"a": 1, "b": 2,},
}`;
    expect(JSON.parse(sanitizeJsonInput(input))).toEqual({
      list: [1, 2, 3],
      map: { a: 1, b: 2 },
    });
  });

  it('keeps commas inside string literals', () => {
    const input = '{"s":"one, two, three,"}';
    expect(JSON.parse(sanitizeJsonInput(input))).toEqual({ s: 'one, two, three,' });
  });

  it('handles mix of comments and trailing commas together', () => {
    const input = `{
  // section
  "a": [1, 2,], /* keep */
  "b": 3,
}`;
    expect(JSON.parse(sanitizeJsonInput(input))).toEqual({ a: [1, 2], b: 3 });
  });
});

describe('isValidJSON with JSONC input', () => {
  it('accepts comments and trailing commas', () => {
    expect(isValidJSON('{ "a": 1, /* c */ "b": 2, }')).toBe(true);
  });

  it('rejects genuinely invalid JSON', () => {
    expect(isValidJSON('{ "a": }')).toBe(false);
  });

  it('treats empty input as valid', () => {
    expect(isValidJSON('')).toBe(true);
    expect(isValidJSON('   ')).toBe(true);
  });
});

describe('formatJSON with JSONC input', () => {
  it('produces canonical JSON from JSONC input', () => {
    const formatted = formatJSON('{ // note\n "a": 1, "b": [1,2,], }');
    expect(formatted).toBe('{\n  "a": 1,\n  "b": [\n    1,\n    2\n  ]\n}');
  });

  it('returns original text for unparseable input', () => {
    const broken = '{ "a": }';
    expect(formatJSON(broken)).toBe(broken);
  });
});

describe('sanitizeJsonField', () => {
  it('returns compact JSON when parseable', () => {
    expect(sanitizeJsonField('{ "a": 1, /* c */ "b": 2, }')).toBe('{"a":1,"b":2}');
  });

  it('returns original when unparseable so backend can surface the error', () => {
    const broken = '{ "a": }';
    expect(sanitizeJsonField(broken)).toBe(broken);
  });

  it('passes empty/whitespace input through unchanged', () => {
    expect(sanitizeJsonField('')).toBe('');
    expect(sanitizeJsonField('   ')).toBe('   ');
  });
});

describe('validateModelConfigs with JSONC input', () => {
  it('accepts valid JSONC', () => {
    const input = `{
  // gpt4 pricing
  "gpt-4": { "ratio": 0.03, "completion_ratio": 2.0, },
}`;
    expect(validateModelConfigs(input)).toEqual({ valid: true });
  });

  it('still reports shape errors after sanitising', () => {
    const input = '[1,2,3]';
    expect(validateModelConfigs(input).valid).toBe(false);
  });
});

describe('validateToolingConfig with JSONC input', () => {
  it('accepts valid JSONC', () => {
    const input = `{
  /* allowed tools */
  "whitelist": ["search",],
}`;
    expect(validateToolingConfig(input)).toEqual({ valid: true });
  });
});

describe('validateModelConfigs time_windows', () => {
  it('accepts a model_configs entry carrying a time_windows overlay', () => {
    const input = `{
  "deepseek-v4-pro": {
    "ratio": 0.0002175,
    "completion_ratio": 2.0,
    "cached_input_ratio": 0.0000018125,
    "time_windows": [
      {
        "name": "deepseek-offpeak",
        "timezone": "Asia/Shanghai",
        "ranges": [{ "start": "18:00", "end": "09:00" }, { "start": "12:00", "end": "14:00" }],
        "overlay": { "ratio": 0.00010875, "cached_input_ratio": 0.00000090625 }
      }
    ]
  }
}`;
    expect(validateModelConfigs(input)).toEqual({ valid: true });
  });

  it('treats a windows-only entry as a valid pricing config', () => {
    const input = `{
  "m": {
    "time_windows": [
      { "ranges": [{ "start": "00:00", "end": "00:00" }], "overlay": { "ratio": 0.5 } }
    ]
  }
}`;
    expect(validateModelConfigs(input)).toEqual({ valid: true });
  });

  it('rejects a window whose ranges are missing or empty', () => {
    const input = '{ "m": { "ratio": 1, "time_windows": [{ "overlay": { "ratio": 0.5 } }] } }';
    expect(validateModelConfigs(input).valid).toBe(false);
  });

  it('rejects a range that is not an HH:MM string', () => {
    const input = '{ "m": { "ratio": 1, "time_windows": [{ "ranges": [{ "start": "9", "end": "17:00" }], "overlay": { "ratio": 0.5 } }] } }';
    expect(validateModelConfigs(input).valid).toBe(false);
  });

  it('rejects impossible HH:MM clock values', () => {
    const input = '{ "m": { "ratio": 1, "time_windows": [{ "ranges": [{ "start": "24:00", "end": "99:99" }], "overlay": { "ratio": 0.5 } }] } }';
    expect(validateModelConfigs(input).valid).toBe(false);
  });

  it('rejects a window without an overlay object', () => {
    const input = '{ "m": { "ratio": 1, "time_windows": [{ "ranges": [{ "start": "09:00", "end": "17:00" }] }] } }';
    expect(validateModelConfigs(input).valid).toBe(false);
  });
});
