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

export const CHART_CONFIG = {
  colors: {
    requests: "#2A7A82", // Deep teal — primary
    quota: "#C27530", // Warm amber — secondary
    tokens: "#4A7A5C", // Sage green — tertiary
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
  // Professional earthy palette — no purple, no neon
  // Designed for good contrast in both light and dark modes
  barColors: [
    "#2A7A82", // Teal — primary
    "#C27530", // Amber — warm accent
    "#4A7A5C", // Sage — muted green
    "#5A7A9A", // Steel blue
    "#B85C4A", // Terracotta
    "#64748B", // Slate — neutral
    "#2E8B8B", // Dark teal
    "#8B7355", // Bronze
    "#3D8B6E", // Forest
    "#7A8D5C", // Olive
    "#6A8FA0", // Dusty blue
    "#A0785A", // Sienna
    "#4A9090", // Cyan-teal
    "#7A6B5A", // Warm gray
    "#5A8A6A", // Muted green
  ],
};

export const getQuotaPerUnit = () =>
  parseFloat(localStorage.getItem("quota_per_unit") || "500000");
export const getDisplayInCurrency = () =>
  localStorage.getItem("display_in_currency") === "true";

export const barColor = (i: number) => {
  return CHART_CONFIG.barColors[i % CHART_CONFIG.barColors.length];
};
