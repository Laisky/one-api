export type BaseMetricRow = {
  day: string
  request_count: number
  quota: number
  prompt_tokens: number
  completion_tokens: number
}

export type ModelRow = BaseMetricRow & { model_name: string }
export type UserRow = BaseMetricRow & { username: string; user_id: number }
export type TokenRow = BaseMetricRow & { token_name: string; username: string; user_id: number }

export type DashboardData = {
  rows: ModelRow[]
  userRows: UserRow[]
  tokenRows: TokenRow[]
}

export type UserOption = {
  id: number
  username: string
  display_name: string
}

export const CHART_CONFIG = {
  colors: {
    requests: '#6366F1',  // Soft indigo - modern, cool
    quota: '#0EA5E9',     // Sky blue - tech-forward
    tokens: '#8B5CF6',    // Soft violet - cool accent
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
      filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.1))'
    },
    grid: {
      vertical: false,
      horizontal: true,
      opacity: 0.2,
    },
  },
  // Modern high-tech palette with cooler, less saturated tones
  // Designed for good contrast in both light and dark modes
  barColors: [
    '#6366F1', // Indigo - primary cool tone
    '#0EA5E9', // Sky blue - tech accent
    '#8B5CF6', // Violet - soft purple
    '#06B6D4', // Cyan - aqua tech
    '#14B8A6', // Teal - balanced green-blue
    '#64748B', // Slate - neutral gray-blue
    '#7C3AED', // Purple - deeper accent
    '#0284C7', // Dark sky - deeper blue
    '#6D28D9', // Violet dark - rich purple
    '#0891B2', // Dark cyan - ocean tone
    '#4F46E5', // Indigo dark - primary variant
    '#059669', // Emerald muted - cool green
    '#7DD3FC', // Light sky - bright accent (dark mode friendly)
    '#A78BFA', // Light violet - soft accent
    '#22D3EE', // Light cyan - vibrant cool
  ],
}

export const getQuotaPerUnit = () => parseFloat(localStorage.getItem('quota_per_unit') || '500000')
export const getDisplayInCurrency = () => localStorage.getItem('display_in_currency') === 'true'

export const barColor = (i: number) => {
  return CHART_CONFIG.barColors[i % CHART_CONFIG.barColors.length]
}
