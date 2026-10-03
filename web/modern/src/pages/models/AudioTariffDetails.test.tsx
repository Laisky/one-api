import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { AudioTariffDetails, type AudioInputTariff } from './AudioTariffDetails';

afterEach(cleanup);

describe('exact audio tariff display', () => {
  it.each([
    ['characters', 22, 1000000, '$22 per 1000000 characters'],
    ['utf8_bytes', 7.15, 1000000, '$7.15 per 1000000 UTF-8 bytes'],
    ['seconds', 0.04, 3600, '$0.04 per 3600 seconds'],
  ])('does not disguise %s as token pricing', (input_unit, input_price_usd, input_price_quantity, expected) => {
    render(<AudioTariffDetails pricing={{ input_unit, input_price_usd, input_price_quantity } as AudioInputTariff} />);
    expect(screen.getByLabelText('Input audio tariff')).toHaveTextContent(expected);
    expect(screen.queryByText(/token/i)).not.toBeInTheDocument();
  });

  it('shows an explicitly free tariff even when zero is omitted in JSON', () => {
    render(<AudioTariffDetails pricing={{ input_unit: 'characters', input_price_quantity: 1000000 }} />);
    expect(screen.getByLabelText('Input audio tariff')).toHaveTextContent('$0 per 1000000 characters');
  });

  it('shows the minimum duration and billing increment', () => {
    render(<AudioTariffDetails pricing={{ input_unit: 'seconds', input_price_usd: 0.04, input_price_quantity: 3600, minimum_billable_seconds: 10, billing_increment_seconds: 1 }} />);
    expect(screen.getByText('Minimum billable duration: 10 seconds')).toBeInTheDocument();
    expect(screen.getByText('Billing increment: 1 seconds')).toBeInTheDocument();
  });

  it.each([undefined, {}, { input_unit: 'bogus' }, { input_unit: 'characters', input_price_usd: -1 }, { input_unit: 'characters', input_price_quantity: 0 }])('does not manufacture rates for incomplete or invalid metadata: %s', (pricing) => {
    const view = render(<AudioTariffDetails pricing={pricing} />);
    expect(view.container).toBeEmptyDOMElement();
  });
});
