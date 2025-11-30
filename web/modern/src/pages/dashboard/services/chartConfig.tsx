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
    requests: "#6366F1", // Soft indigo - modern, cool
    quota: "#0EA5E9", // Sky blue - tech-forward
    tokens: "#8B5CF6", // Soft violet - cool accent
  },
  // Modern high-tech palette with cooler, less saturated tones
  // Designed for good contrast in both light and dark modes
  barColors: [
    "#6366F1", // Indigo - primary cool tone
    "#0EA5E9", // Sky blue - tech accent
    "#8B5CF6", // Violet - soft purple
    "#06B6D4", // Cyan - aqua tech
    "#14B8A6", // Teal - balanced green-blue
    "#64748B", // Slate - neutral gray-blue
    "#7C3AED", // Purple - deeper accent
    "#0284C7", // Dark sky - deeper blue
    "#6D28D9", // Violet dark - rich purple
    "#0891B2", // Dark cyan - ocean tone
    "#4F46E5", // Indigo dark - primary variant
    "#059669", // Emerald muted - cool green
    "#7DD3FC", // Light sky - bright accent (dark mode friendly)
    "#A78BFA", // Light violet - soft accent
    "#22D3EE", // Light cyan - vibrant cool
  ],
};

export const barColor = (index: number) =>
  chartConfig.barColors[index % chartConfig.barColors.length];

export const GradientDefs = () => (
  <defs>
    <linearGradient id="requestsGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#6366F1" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#6366F1" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="quotaGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#0EA5E9" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#0EA5E9" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="tokensGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#8B5CF6" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#8B5CF6" stopOpacity={0.1} />
    </linearGradient>
  </defs>
);
