import type { TrFn } from './pricing-format';
import { PriceCell, PriceGrid, PricingSection } from './PricingPrimitives';

/**
 * PerPagePricingData mirrors the backend per_page_pricing display payload for
 * document models billed by processed page (for example OCR). A present value
 * with zero prices is an explicit free tariff, not missing pricing.
 */
export interface PerPagePricingData {
  usd_per_thousand_pages?: number;
  usd_per_page?: number;
}

/**
 * PerPagePricingSection renders the base per-page tariff of a page-priced model.
 * @param pricing - The per_page_pricing payload, or undefined when the model is not page-priced.
 * @param tr - Translation function for the model-detail namespace.
 * @returns The pricing section, or null when no per-page value is present.
 */
export function PerPagePricingSection({ pricing, tr }: { pricing?: PerPagePricingData; tr: TrFn }) {
  if (!pricing || (pricing.usd_per_thousand_pages === undefined && pricing.usd_per_page === undefined)) {
    return null;
  }
  return (
    <PricingSection title={tr('per_page_pricing', 'Per-Page Pricing')} icon="text">
      <PriceGrid>
        {pricing.usd_per_thousand_pages !== undefined && (
          <PriceCell label={tr('base_rate', 'Base Rate')} sublabel={tr('per_1k_pages', 'per 1K pages')} value={pricing.usd_per_thousand_pages} tr={tr} raw />
        )}
        {pricing.usd_per_page !== undefined && (
          <PriceCell label={tr('per_page_label', 'Per Page')} sublabel={tr('per_page', 'per page')} value={pricing.usd_per_page} tr={tr} raw />
        )}
      </PriceGrid>
    </PricingSection>
  );
}

/**
 * PerPageWindowCell renders a time-window overlay's per-page tariff inside the window price grid.
 * @param pricing - The overlay per_page_pricing payload, or undefined when the window does not override it.
 * @param tr - Translation function for the model-detail namespace.
 * @returns The price cell, or null when the overlay carries no per-page value.
 */
export function PerPageWindowCell({ pricing, tr }: { pricing?: PerPagePricingData; tr: TrFn }) {
  if (pricing?.usd_per_thousand_pages === undefined) {
    return null;
  }
  return (
    <PriceCell
      label={tr('per_page_pricing', 'Per-Page Pricing')}
      sublabel={tr('per_1k_pages', 'per 1K pages')}
      value={pricing.usd_per_thousand_pages}
      tr={tr}
      raw
    />
  );
}
