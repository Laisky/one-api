export type BaseMetricRow = {
  day: string;
  request_count: number;
  quota: number;
  prompt_tokens: number;
  completion_tokens: number;
};

export type ModelRow = BaseMetricRow & { model_name: string };
export type UserRow = BaseMetricRow & { username: string; user_id: number };
export type TokenRow = BaseMetricRow & {
  token_name: string;
  username: string;
  user_id: number;
};

export type DashboardData = {
  rows: ModelRow[];
  userRows: UserRow[];
  tokenRows: TokenRow[];
};

export type UserOption = {
  id: number;
  username: string;
  display_name: string;
};

/**
 * Static fallback palette used when CSS variables are unavailable (SSR, tests).
 * Matches the HSL values defined in index.css :root at default (light) theme.
 */
const FALLBACK_COLORS: string[] = [
  '#22a392', // chart-1  teal
  '#d47a1e', // chart-2  orange
  '#4272c4', // chart-3  blue
  '#d04a5e', // chart-4  rose
  '#b89418', // chart-5  gold
  '#7e5daa', // chart-6  purple-ish
  '#1e8fa8', // chart-7  cyan
  '#c85530', // chart-8  burnt orange
  '#3a9e5c', // chart-9  green
  '#3a8ab8', // chart-10 steel blue
  '#c44e80', // chart-11 pink
  '#6a9a28', // chart-12 lime
  '#8868b0', // chart-13 muted violet
  '#b87a20', // chart-14 amber
  '#349e78', // chart-15 sea green
];

/**
 * Resolve a CSS custom property (e.g. "--chart-1") to a computed HSL color
 * string usable in SVG attributes and Recharts props.
 * Must be called at render time to respect the current theme.
 */
export function resolveChartVar(name: string): string {
  if (typeof document === 'undefined') return '#888';
  const hsl = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return hsl ? `hsl(${hsl})` : '';
}

/** Return the resolved color for --chart-{index} (1-based). */
export function getChartColor(index: number): string {
  const resolved = resolveChartVar(`--chart-${index}`);
  return resolved || FALLBACK_COLORS[(index - 1) % FALLBACK_COLORS.length];
}

/** Palette size — matches --chart-1 … --chart-15 in index.css */
const CHART_PALETTE_SIZE = 15;

export const CHART_CONFIG = {
  /** Resolved at render time from CSS variables --chart-1/2/3 */
  get colors() {
    return {
      requests: getChartColor(1),
      quota: getChartColor(2),
      tokens: getChartColor(3),
    };
  },
  gradients: {
    requests: 'url(#requestsGradient)',
    quota: 'url(#quotaGradient)',
    tokens: 'url(#tokensGradient)',
  },
  lineChart: {
    strokeWidth: 3,
    dot: false,
    activeDot: {
      r: 6,
      strokeWidth: 2,
      filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.1))',
    },
    grid: {
      vertical: false,
      horizontal: true,
      opacity: 0.2,
    },
  },
  paletteSize: CHART_PALETTE_SIZE,
};

export const getQuotaPerUnit = () => parseFloat(localStorage.getItem('quota_per_unit') || '500000');
export const getDisplayInCurrency = () => localStorage.getItem('display_in_currency') === 'true';

/** Return the theme-aware bar color for index i (wraps around the palette). */
export const barColor = (i: number): string => {
  return getChartColor((i % CHART_PALETTE_SIZE) + 1);
};
