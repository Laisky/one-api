import { describe, expect, it } from 'vitest';

import { CHANNEL_TYPES, CHANNEL_TYPE_LABELS } from '../constants';

describe('DeepInfra channel metadata', () => {
  it('registers channel type 57 in the create dropdown and label map', () => {
    const channel = CHANNEL_TYPES.find((candidate) => candidate.value === 57);

    expect(channel).toMatchObject({
      key: 57,
      text: 'DeepInfra',
      value: 57,
    });
    expect(CHANNEL_TYPE_LABELS[57]).toEqual({ name: 'DeepInfra', color: 'purple' });
  });
});
