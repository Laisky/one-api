import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';

describe('TypeSafe channel metadata', () => {
  it('offers one native evaluation channel with the append-only backend ID', () => {
    const channels = CHANNEL_TYPES.filter((channel) => channel.value === 60);
    expect(channels).toHaveLength(1);
    expect(channels[0].key).toBe(60);
    expect(channels[0].text).toBe('TypeSafe');
    expect(channels[0].description).toContain('/v1/systemone');
    expect(channels[0].description).toContain('Not a chat or streaming API');
    expect(CHANNEL_TYPE_LABELS[60].name).toBe('TypeSafe');
    expect(CHANNEL_TYPE_LABELS[59].name).toBe('Jina AI');
  });
});
