import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';

const themeMirrors = {
  air: 'web/air/src/constants/channel.constants.js',
  berry: 'web/berry/src/constants/ChannelConstants.js',
};

const repoRoot = resolve(__dirname, '../../../../../..');

describe('MuAPI channel metadata', () => {
  it('offers native multi-model video routing with the append-only backend ID', () => {
    const channels = CHANNEL_TYPES.filter((channel) => channel.value === 61);
    expect(channels).toHaveLength(1);
    expect(channels[0].key).toBe(61);
    expect(channels[0].text).toBe('MuAPI');
    expect(channels[0].description).toContain('any catalog model slug');
    expect(channels[0].description).toContain('request-specific pricing');
    expect(CHANNEL_TYPE_LABELS[61].name).toBe('MuAPI');
    expect(CHANNEL_TYPE_LABELS[60].name).toBe('TypeSafe');
  });

  it.each(Object.entries(themeMirrors))('is selectable in the %s theme', (_theme, file) => {
    const source = readFileSync(resolve(repoRoot, file), 'utf8');
    expect(source).toMatch(/key:\s*61\b/);
    expect(source).toContain('MuAPI');
    expect(source).toMatch(/key:\s*60\b/);
    expect(source).toContain('TypeSafe');
  });
});
