import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { translateText } from '@/test/dom-translation';
import { PricingContent } from './ModelPricingContent';
import type { ModelDisplayData } from './ModelPricingModal';

const data: ModelDisplayData = {
  input_price: 0,
  output_price: 0,
  image_pricing: { price_per_image_usd: 0.1, quality_size_multipliers: { standard: { '1024x1024': 1 } } },
};

/** englishLabel returns the supplied English fallback for a pricing translation key. */
function englishLabel(_key: string, fallback: string): string {
  return fallback;
}

/** changedLabel returns visibly changed English labels for a language-change regression. */
function changedLabel(_key: string, fallback: string): string {
  return `Updated ${fallback}`;
}

describe('image pricing matrix with browser-translated headers', () => {
  it('updates both reused matrix labels when the active language changes', () => {
    const props = { modelName: 'image-model', channelName: 'Example provider', data, locale: 'en' };
    const { rerender } = render(<PricingContent {...props} tr={englishLabel} />);
    const header = screen.getByRole('columnheader', { name: 'Quality \\ Size' });
    expect(translateText(header)).toBeGreaterThan(0);

    rerender(<PricingContent {...props} tr={changedLabel} locale="en-US" />);

    expect(header).toHaveTextContent('Updated Quality \\ Updated Size');
    expect(screen.getByRole('cell', { name: '$0.10' })).toBeInTheDocument();
  });
});
