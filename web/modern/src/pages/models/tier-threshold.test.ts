import { describe, expect, it } from 'vitest';

import { formatTierThreshold } from './tier-threshold';

describe('formatTierThreshold', () => {
  it('formats input-only thresholds', () => {
    expect(formatTierThreshold({ input_token_threshold: 32_000 }, 'Input', 'Output')).toBe('Input ≥ 32K');
  });

  it('formats output-only thresholds', () => {
    expect(formatTierThreshold({ input_token_threshold: 0, output_token_threshold: 200 }, 'Input', 'Output')).toBe('Output ≥ 200');
  });

  it('formats combined thresholds', () => {
    expect(formatTierThreshold({ input_token_threshold: 32_000, output_token_threshold: 200 }, 'Input', 'Output')).toBe(
      'Input ≥ 32K · Output ≥ 200'
    );
  });
});
