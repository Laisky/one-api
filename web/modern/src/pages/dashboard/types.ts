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
  "#3d8a8f", // chart-1
  "#d49235", // chart-2
  "#4b8b6e", // chart-3
  "#5b7a9a", // chart-4
  "#c06a54", // chart-5
  "#6b7a8b", // chart-6
  "#2d8b8b", // chart-7
  "#8b7355", // chart-8
  "#3d8b6e", // chart-9
  "#7a8d5c", // chart-10
  "#6a8fa0", // chart-11
  "#a0785a", // chart-12
  "#4a9090", // chart-13
  "#7a6b5a", // chart-14
  "#5a8a6a", // chart-15
];

/**
 * Resolve a CSS custom property (e.g. "--chart-1") to a computed HSL color
 * string usable in SVG attributes and Recharts props.
 * Must be called at render time to respect the current theme.
 */
export function resolveChartVar(name: string): string {
  if (typeof document === "undefined") return "#888";
  const hsl = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  return hsl ? `hsl(${hsl})` : "";
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
    requests: "url(#requestsGradient)",
    quota: "url(#quotaGradient)",
    tokens: "url(#tokensGradient)",
  },
  lineChart: {
    strokeWidth: 3,
    dot: false,
    activeDot: {
      r: 6,
      strokeWidth: 2,
      filter: "drop-shadow(0 2px 4px rgba(0,0,0,0.1))",
    },
    grid: {
      vertical: false,
      horizontal: true,
      opacity: 0.2,
    },
  },
  paletteSize: CHART_PALETTE_SIZE,
};

export const getQuotaPerUnit = () =>
  parseFloat(localStorage.getItem("quota_per_unit") || "500000");
export const getDisplayInCurrency = () =>
  localStorage.getItem("display_in_currency") === "true";

/** Return the theme-aware bar color for index i (wraps around the palette). */
export const barColor = (i: number): string => {
  return getChartColor((i % CHART_PALETTE_SIZE) + 1);
};
