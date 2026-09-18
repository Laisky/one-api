import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';

describe('Jina channel registration', () => {
  it('offers the appended backend ID exactly once and labels stored channels', () => {
    expect(CHANNEL_TYPES.filter((type) => type.value === 59)).toHaveLength(1);
    expect(CHANNEL_TYPES.find((type) => type.value === 59)?.text).toBe('Jina AI');
    expect(CHANNEL_TYPE_LABELS[59]?.name).toBe('Jina AI');
    expect(CHANNEL_TYPE_LABELS[58]?.name).toBe('Z.ai');
  });
});
