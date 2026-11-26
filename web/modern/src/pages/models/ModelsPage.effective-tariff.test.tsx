import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }));

vi.mock('@/lib/api', () => ({ api: { get: getMock } }));
vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false, isDesktop: true, isLarge: true }),
}));

import type { ModelDisplayData, ModelDisplayTariffData } from './ModelPricingModal';
import { ModelsPage } from './ModelsPage';

const promoBase: ModelDisplayTariffData = { input_price: 4, cached_input_price: 0.4, cache_write_5m_price: 5, output_price: 20 };
const standardTariff: ModelDisplayTariffData = { input_price: 5, cached_input_price: 0.5, cache_write_5m_price: 6.25, output_price: 30 };

// Fetched before the cutoff: the server top-level is still the promo base.
const solFetchedBeforeCutoff: ModelDisplayData = {
  ...promoBase,
  time_windows: [
    {
      name: 'gpt-5.6-sol-standard-from-december-2026',
      timezone: 'UTC',
      ranges: [{ start: '00:00', end: '00:00' }],
      date_from: '2026-12-01',
      overlay: { ...standardTariff },
      effective_pricing: standardTariff,
    },
  ],
};

/** rowCells returns the price cells of the table row for a model. */
async function rowCells(modelName: string): Promise<string[]> {
  const name = await screen.findByText(modelName);
  const row = name.closest('tr')!;
  return within(row)
    .getAllByRole('cell')
    .map((cell) => cell.textContent ?? '');
}

describe('ModelsPage price columns', () => {
  beforeEach(() => {
    // Only Date is faked so the fetch promise and findBy polling keep running.
    vi.useFakeTimers({ toFake: ['Date'] });
    getMock.mockResolvedValue({
      data: {
        success: true,
        data: {
          openai: { models: { 'gpt-5.6-sol': solFetchedBeforeCutoff, 'operator-model': { input_price: 3, output_price: 15 } } },
        },
      },
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    getMock.mockReset();
  });

  it('lists the promo base before the cutoff', async () => {
    vi.setSystemTime(new Date('2026-11-30T23:59:00Z'));
    render(
      <MemoryRouter>
        <ModelsPage />
      </MemoryRouter>
    );
    expect((await rowCells('gpt-5.6-sol')).slice(1, 4)).toEqual(['$4.00', '$0.4000', '$20.00']);
  });

  it('lists the window tariff in effect at render time even when the payload predates the cutoff', async () => {
    vi.setSystemTime(new Date('2026-12-01T00:00:30Z'));
    render(
      <MemoryRouter>
        <ModelsPage />
      </MemoryRouter>
    );
    expect((await rowCells('gpt-5.6-sol')).slice(1, 4)).toEqual(['$5.00', '$0.5000', '$30.00']);
    expect((await rowCells('operator-model')).slice(1, 4)).toEqual(['$3.00', '$3.00', '$15.00']);
  });
});
