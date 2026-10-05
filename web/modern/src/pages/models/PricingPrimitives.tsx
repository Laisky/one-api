import type { ReactNode } from 'react';
import { formatUsdForTokens, formatUsdRaw, type TrFn } from './pricing-format';

const sectionIcons: Record<string, string> = {
  profile: '\u{1F9ED}',
  text: '\u{1F4DD}',
  cache: '\u{1F4BE}',
  tiers: '\u{1F4CA}',
  time: '\u{23F1}',
  image: '\u{1F5BC}',
  video: '\u{1F3AC}',
  audio: '\u{1F3B5}',
  embedding: '\u{1F9E9}',
};

/**
 * PricingSection renders a titled card that groups one pricing dimension.
 * @param title - Localized section heading.
 * @param icon - Key into the section icon table.
 * @param children - Section body.
 * @returns The section element.
 */
export function PricingSection({ title, icon, children }: { title: string; icon: string; children: ReactNode }) {
  return (
    <div>
      <div className="flex items-center gap-2 mb-3">
        <span className="text-base" role="img">
          {sectionIcons[icon] || ''}
        </span>
        <h3 className="text-sm font-semibold tracking-wide text-foreground">{title}</h3>
      </div>
      <div className="rounded-xl border bg-card p-4">{children}</div>
    </div>
  );
}

/**
 * PriceGrid lays out price cells in a responsive grid.
 * @param children - Price cells.
 * @returns The grid element.
 */
export function PriceGrid({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">{children}</div>;
}

/**
 * PriceCell renders one labeled price.
 * @param label - Localized price label.
 * @param sublabel - Optional localized unit description.
 * @param value - USD amount.
 * @param tr - Translation function for the model-detail namespace.
 * @param raw - When true the value is a flat per-unit price and zero renders as "$0" instead of "Free".
 * @returns The cell element.
 */
export function PriceCell({ label, sublabel, value, tr, raw }: { label: string; sublabel?: string; value: number; tr: TrFn; raw?: boolean }) {
  return (
    <div className="rounded-lg border bg-muted/30 p-3">
      <div className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</div>
      {sublabel && <div className="text-[10px] text-muted-foreground/70">{sublabel}</div>}
      <div className="mt-1 text-lg font-semibold tabular-nums">{raw ? formatUsdRaw(value) : formatUsdForTokens(value, tr)}</div>
    </div>
  );
}
