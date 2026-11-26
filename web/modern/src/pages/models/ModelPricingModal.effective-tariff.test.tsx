import { act, render, screen, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/dialog', () => {
  const Dialog = ({ children }: { children: ReactNode }) => <>{children}</>;
  const DialogContent = ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  );
  const DialogHeader = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogTitle = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogDescription = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription };
});

vi.mock('@/components/ui/copy-button', () => ({
  CopyButton: () => <button type="button">copy</button>,
}));

import { ModelPricingModal, type ModelDisplayData, type ModelDisplayTariffData, type TimeWindowData } from './ModelPricingModal';

const SOL_WINDOW_NAME = 'gpt-5.6-sol-standard-from-december-2026';
const CURRENT_LABEL = `Current price — ${SOL_WINDOW_NAME} pricing applies`;
const STANDARD_TITLE = 'Standard price (not in effect now)';

const promoBase: ModelDisplayTariffData = {
  input_price: 4,
  cached_input_price: 0.4,
  cache_write_5m_price: 5,
  output_price: 20,
  tiers: [{ input_price: 8, output_price: 30, input_token_threshold: 272001 }],
};

const standardTariff: ModelDisplayTariffData = {
  input_price: 5,
  cached_input_price: 0.5,
  cache_write_5m_price: 6.25,
  output_price: 30,
  tiers: [{ input_price: 10, output_price: 45, input_token_threshold: 272001 }],
};

/** solWindow builds the dated all-day window; withEffective=false mimics an older server. */
function solWindow(withEffective = true): TimeWindowData {
  return {
    name: SOL_WINDOW_NAME,
    timezone: 'UTC',
    ranges: [{ start: '00:00', end: '00:00' }],
    date_from: '2026-12-01',
    overlay: { ...standardTariff },
    ...(withEffective ? { effective_pricing: standardTariff } : {}),
  };
}

// The server rendered this payload before the cutoff: no window applied, top-level is the promo base.
const fetchedBeforeCutoff: ModelDisplayData = { ...promoBase, time_windows: [solWindow()] };

// The server rendered this payload after the cutoff: top-level is the window tariff.
const fetchedAfterCutoff: ModelDisplayData = {
  ...standardTariff,
  time_windows: [solWindow()],
  active_time_window: SOL_WINDOW_NAME,
  inactive_base_pricing: promoBase,
};

/** renderModel renders the pricing modal for one display payload. */
function renderModel(data: ModelDisplayData, modelName = 'gpt-5.6-sol') {
  return render(<ModelPricingModal open onOpenChange={() => {}} modelName={modelName} data={data} channelName="openai" />);
}

/** section returns the card of the pricing section with the given heading. */
function section(title: string): HTMLElement {
  return screen.getByRole('heading', { name: title }).parentElement!.parentElement!;
}

/** standardPrice returns the inactive standard price listed for a label. */
function standardPrice(label: string): string {
  const group = screen.getByRole('group', { name: STANDARD_TITLE });
  return within(group).getByText(label).nextElementSibling?.textContent ?? '';
}

/** expectTokenPrices asserts the prominent text-token, cache, and tier prices. */
function expectTokenPrices(tariff: { input: string; output: string; cacheRead: string; cacheWrite: string; tier: [string, string] }) {
  const text = section('Text Token Pricing');
  expect(within(text).getByText(tariff.input)).toBeInTheDocument();
  expect(within(text).getByText(tariff.output)).toBeInTheDocument();
  const cache = section('Cache Pricing');
  expect(within(cache).getByText(tariff.cacheRead)).toBeInTheDocument();
  expect(within(cache).getByText(tariff.cacheWrite)).toBeInTheDocument();
  const tiers = section('Tiered Pricing');
  expect(within(tiers).getByText(tariff.tier[0])).toBeInTheDocument();
  expect(within(tiers).getByText(tariff.tier[1])).toBeInTheDocument();
}

const promoPrices = {
  input: '$4.00',
  output: '$20.00',
  cacheRead: '$0.4',
  cacheWrite: '$5.00',
  tier: ['$8.00', '$30.00'] as [string, string],
};
const standardPrices = {
  input: '$5.00',
  output: '$30.00',
  cacheRead: '$0.5',
  cacheWrite: '$6.25',
  tier: ['$10.00', '$45.00'] as [string, string],
};

/** expectInactivePromoBase asserts the promo base is listed as the standard price not in effect. */
function expectInactivePromoBase() {
  expect(screen.getByText(CURRENT_LABEL)).toBeInTheDocument();
  expect(standardPrice('Input')).toContain('$4.00');
  expect(standardPrice('Output')).toContain('$20.00');
  expect(standardPrice('Cache Read')).toContain('$0.4');
  expect(standardPrice('5-min Cache Write')).toContain('$5.00');
  expect(within(section('Text Token Pricing')).queryByText('$4.00')).not.toBeInTheDocument();
}

describe('ModelPricingModal current tariff', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('shows the promo base without a window label before the cutoff', () => {
    vi.setSystemTime(new Date('2026-11-30T23:59:00Z'));
    renderModel(fetchedBeforeCutoff);
    expectTokenPrices(promoPrices);
    expect(screen.queryByText(CURRENT_LABEL)).not.toBeInTheDocument();
    expect(screen.queryByRole('group', { name: STANDARD_TITLE })).not.toBeInTheDocument();
    expect(screen.queryByText('Active now')).not.toBeInTheDocument();
  });

  it('shows the window tariff and the inactive base after the cutoff', () => {
    vi.setSystemTime(new Date('2026-12-01T00:00:30Z'));
    renderModel(fetchedAfterCutoff);
    expectTokenPrices(standardPrices);
    expectInactivePromoBase();
    expect(screen.getByText('Active now')).toBeInTheDocument();
  });

  it('selects effective_pricing when the payload predates the cutoff but the clock is past it', () => {
    vi.setSystemTime(new Date('2026-12-01T08:00:00Z'));
    renderModel(fetchedBeforeCutoff);
    expectTokenPrices(standardPrices);
    expectInactivePromoBase();
  });

  it('switches the prominent prices live when the cutoff passes with one shared timer', () => {
    vi.setSystemTime(new Date('2026-11-30T23:59:59Z'));
    const idle = vi.getTimerCount();
    const view = renderModel(fetchedBeforeCutoff);
    expect(vi.getTimerCount()).toBe(idle + 1);
    expectTokenPrices(promoPrices);

    act(() => {
      vi.setSystemTime(new Date('2026-12-01T00:00:01Z'));
      vi.advanceTimersByTime(1000);
    });

    expectTokenPrices(standardPrices);
    expectInactivePromoBase();
    expect(vi.getTimerCount()).toBe(idle + 1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(idle);
  });

  it('renders the base schedule when the clock is before a window the server saw active', () => {
    vi.setSystemTime(new Date('2026-11-30T12:00:00Z'));
    renderModel(fetchedAfterCutoff);
    expectTokenPrices(promoPrices);
    expect(screen.queryByText(CURRENT_LABEL)).not.toBeInTheDocument();
  });

  it('keeps the old behavior for an older server without effective_pricing', () => {
    vi.setSystemTime(new Date('2026-12-01T08:00:00Z'));
    renderModel({ ...promoBase, time_windows: [solWindow(false)], active_time_window: SOL_WINDOW_NAME });
    expectTokenPrices(promoPrices);
    expect(screen.queryByText(CURRENT_LABEL)).not.toBeInTheDocument();
    expect(screen.getByText('Active now')).toBeInTheDocument();
  });

  it('renders an explicit operator tariff without windows unchanged', () => {
    vi.setSystemTime(new Date('2026-12-01T08:00:00Z'));
    renderModel({ input_price: 3, cached_input_price: 0.3, output_price: 15, description: 'Operator tariff' }, 'operator-model');
    const text = section('Text Token Pricing');
    expect(within(text).getByText('$3.00')).toBeInTheDocument();
    expect(within(text).getByText('$15.00')).toBeInTheDocument();
    expect(within(section('Cache Pricing')).getByText('$0.3')).toBeInTheDocument();
    expect(screen.getByText('Operator tariff')).toBeInTheDocument();
    expect(screen.queryByText(/Current price/)).not.toBeInTheDocument();
    expect(screen.queryByRole('group', { name: STANDARD_TITLE })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Time-of-day Pricing' })).not.toBeInTheDocument();
  });
});
