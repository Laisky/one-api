import { formatNumber } from "@/lib/utils";

export const getQuotaPerUnit = () =>
  parseFloat(localStorage.getItem("quota_per_unit") || "500000");
export const getDisplayInCurrency = () =>
  localStorage.getItem("display_in_currency") === "true";

export const renderQuota = (quota: number, precision: number = 2): string => {
  const displayInCurrency = getDisplayInCurrency();
  const quotaPerUnit = getQuotaPerUnit();

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(precision);
    return `$${amount}`;
  }

  return formatNumber(quota);
};

export const chartConfig = {
  colors: {
    requests: "#2A7A82", // Deep teal — primary
    quota: "#C27530", // Warm amber — secondary
    tokens: "#4A7A5C", // Sage green — tertiary
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

export const barColor = (index: number) =>
  chartConfig.barColors[index % chartConfig.barColors.length];

export const GradientDefs = () => (
  <defs>
    <linearGradient id="requestsGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#2A7A82" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#2A7A82" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="quotaGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#C27530" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#C27530" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="tokensGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#4A7A5C" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#4A7A5C" stopOpacity={0.1} />
    </linearGradient>
  </defs>
);
