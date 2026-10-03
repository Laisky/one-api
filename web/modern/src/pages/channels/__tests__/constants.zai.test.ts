import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';
import { LEGACY_COLOR_MAP } from '../utils/colorGenerator';

describe('Z.ai channel metadata', () => {
  it('registers channel type 58 in the create dropdown and label map', () => {
    const channel = CHANNEL_TYPES.find((candidate) => candidate.value === 58);

    expect(channel).toMatchObject({
      key: 58,
      text: 'Z.ai',
      value: 58,
    });
    expect(CHANNEL_TYPE_LABELS[58]).toEqual({ name: 'Z.ai', color: 'teal' });
  });

  it('uses a color the legacy map can resolve', () => {
    const channel = CHANNEL_TYPES.find((candidate) => candidate.value === 58);
    expect(channel?.color).toBeDefined();
    expect(LEGACY_COLOR_MAP[channel!.color!]).toBeDefined();
  });

  it('is distinct from Zhipu, which remains its own channel', () => {
    const zai = CHANNEL_TYPES.find((candidate) => candidate.value === 58);
    const zhipu = CHANNEL_TYPES.find((candidate) => candidate.value === 16);

    expect(zai).toBeDefined();
    expect(zhipu).toBeDefined();
    expect(zai!.text).not.toEqual(zhipu!.text);
    expect(zai!.color).not.toEqual(zhipu!.color);
  });
});
