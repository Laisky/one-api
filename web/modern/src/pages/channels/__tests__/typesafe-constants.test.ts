import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';

// Each theme keeps its own copy of relay/channeltype/define.go. A type missing
// from one of them still relays correctly but is unselectable in that theme's
// channel editor and renders as an unknown type in its list, so all three
// mirrors are checked together rather than only the one this suite imports.
const themeMirrors = {
  air: 'web/air/src/constants/channel.constants.js',
  berry: 'web/berry/src/constants/ChannelConstants.js',
};

const repoRoot = resolve(__dirname, '../../../../../..');

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

  it.each(Object.entries(themeMirrors))('is selectable in the %s theme', (_theme, file) => {
    const source = readFileSync(resolve(repoRoot, file), 'utf8');
    expect(source).toMatch(/key:\s*60\b/);
    expect(source).toContain('TypeSafe');
    // Type 59 shipped before 60 and was missing from a mirror, which is how
    // this gap is normally discovered; assert the predecessor too.
    expect(source).toMatch(/key:\s*59\b/);
    expect(source).toContain('Jina AI');
  });
});
