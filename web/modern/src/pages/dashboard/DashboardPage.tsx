import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useAuthStore } from '@/lib/stores/auth'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api } from '@/lib/api'
import { ResponsivePageContainer } from '@/components/ui/responsive-container'
import { formatNumber, cn } from '@/lib/utils'

// Quota conversion utility
const getQuotaPerUnit = () => parseFloat(localStorage.getItem('quota_per_unit') || '500000')
const getDisplayInCurrency = () => localStorage.getItem('display_in_currency') === 'true'

const renderQuota = (quota: number, precision: number = 2): string => {
  const displayInCurrency = getDisplayInCurrency()
  const quotaPerUnit = getQuotaPerUnit()

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(precision)
    return `$${amount}`
  }

  return formatNumber(quota)
}
import {
  ResponsiveContainer,
  LineChart,
  Line,
  CartesianGrid,
  XAxis,
  YAxis,
  Tooltip,
  BarChart,
  Bar,
  Legend,
} from 'recharts'

// Gradient definitions component for charts
const GradientDefs = () => (
  <defs>
    <linearGradient id="requestsGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#4318FF" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#4318FF" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="quotaGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#00B5D8" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#00B5D8" stopOpacity={0.1} />
    </linearGradient>
    <linearGradient id="tokensGradient" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%" stopColor="#FF5E7D" stopOpacity={0.8} />
      <stop offset="100%" stopColor="#FF5E7D" stopOpacity={0.1} />
    </linearGradient>
  </defs>
)

// Chart configuration
const chartConfig = {
  colors: {
    requests: '#4318FF',
    quota: '#00B5D8',
    tokens: '#FF5E7D',
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
  barColors: [
    '#4318FF', // Deep purple
    '#00B5D8', // Cyan
    '#6C63FF', // Purple
    '#05CD99', // Green
    '#FFB547', // Orange
    '#FF5E7D', // Pink
    '#41B883', // Emerald
    '#7983FF', // Light Purple
    '#FF8F6B', // Coral
    '#49BEFF', // Sky Blue
    '#8B5CF6', // Violet
    '#F59E0B', // Amber
    '#EF4444', // Red
    '#10B981', // Emerald
    '#3B82F6', // Blue
  ],
}

export function DashboardPage() {
  const { user } = useAuthStore()
  const isAdmin = useMemo(() => (user?.role ?? 0) >= 10, [user])
  const [filtersReady, setFiltersReady] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)

  useLayoutEffect(() => {
    if (typeof document === 'undefined') {
      return
    }

    const active = document.activeElement as HTMLElement | null
    if (active && ['INPUT', 'SELECT', 'TEXTAREA'].includes(active.tagName)) {
      active.blur()
    }

    // Defer mounting interactive filter controls until after layout settles
    if (!filtersReady) {
      requestAnimationFrame(() => setFiltersReady(true))
    }
  }, [])

  // date range defaults: last 7 days (inclusive)
  const fmt = (d: Date) => d.toISOString().slice(0, 10)
  const today = new Date()
  const last7 = new Date(today)
  last7.setDate(today.getDate() - 6)

  const [fromDate, setFromDate] = useState(fmt(last7))
  const [toDate, setToDate] = useState(fmt(today))
  const [dashUser, setDashUser] = useState<string>('all')
  const [userOptions, setUserOptions] = useState<Array<{ id: number; username: string; display_name: string }>>([])
  const [loading, setLoading] = useState(false)
  const [lastUpdated, setLastUpdated] = useState<string | null>(null)
  const [statisticsMetric, setStatisticsMetric] = useState<'tokens' | 'requests' | 'expenses'>('tokens')
  const [dateError, setDateError] = useState<string>('')

  type BaseMetricRow = {
    day: string
    request_count: number
    quota: number
    prompt_tokens: number
    completion_tokens: number
  }

  type ModelRow = BaseMetricRow & { model_name: string }
  type UserRow = BaseMetricRow & { username: string; user_id: number }
  type TokenRow = BaseMetricRow & { token_name: string; username: string; user_id: number }

  const [rows, setRows] = useState<ModelRow[]>([])
  const [userRows, setUserRows] = useState<UserRow[]>([])
  const [tokenRows, setTokenRows] = useState<TokenRow[]>([])



  // Date validation functions
  const getMaxDate = () => {
    const today = new Date()
    return today.toISOString().split('T')[0]
  }

  const getMinDate = () => {
    if (isAdmin) {
      // Admin users can go back 1 year
      const oneYearAgo = new Date()
      oneYearAgo.setFullYear(oneYearAgo.getFullYear() - 1)
      return oneYearAgo.toISOString().split('T')[0]
    } else {
      // Regular users can only go back 7 days from today
      const sevenDaysAgo = new Date()
      sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7)
      return sevenDaysAgo.toISOString().split('T')[0]
    }
  }

  // Date validation
  const validateDateRange = (from: string, to: string): string => {
    if (!from || !to) return ''

    const fromDate = new Date(from)
    const toDate = new Date(to)
    const today = new Date()
    const minDate = new Date(getMinDate())

    if (fromDate > toDate) {
      return 'From date must be before or equal to To date'
    }

    if (toDate > today) {
      return 'To date cannot be in the future'
    }

    if (fromDate < minDate) {
      return isAdmin
        ? 'From date cannot be more than 1 year ago'
        : 'From date cannot be more than 7 days ago'
    }

    const daysDiff = Math.ceil((toDate.getTime() - fromDate.getTime()) / (1000 * 60 * 60 * 24))
    const maxDays = isAdmin ? 365 : 7

    if (daysDiff > maxDays) {
      return isAdmin
        ? 'Date range cannot exceed 1 year'
        : 'Date range cannot exceed 7 days'
    }

    return ''
  }

  const loadUsers = async () => {
    if (!isAdmin) return
    // Unified API call - complete URL with /api prefix
    const res = await api.get('/api/user/dashboard/users')
    if (res.data?.success) {
      setUserOptions(res.data.data || [])
    }
  }

  const loadStats = async () => {
    // Validate date range before making API call
    const validationError = validateDateRange(fromDate, toDate)
    if (validationError) {
      setDateError(validationError)
      return
    }

    // Cancel any pending request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }

    // Create new AbortController for this request
    const abortController = new AbortController()
    abortControllerRef.current = abortController

    setLoading(true)
    setDateError('')
    try {
      const params = new URLSearchParams()
      params.set('from_date', fromDate)
      params.set('to_date', toDate)
      if (isAdmin) {
        params.set('user_id', dashUser || 'all')
      }
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/user/dashboard?' + params.toString(), {
        signal: abortController.signal
      })

      // Check if this request was aborted
      if (abortController.signal.aborted) {
        return
      }

      const { success, data, message } = res.data
      if (success) {
        // Handle new API response structure
        const logs = data?.logs || data || []
        const userLogs = data?.user_logs || []
        const tokenLogs = data?.token_logs || []
        setRows(
          logs.map((row: any) => ({
            day: row.Day,
            model_name: row.ModelName,
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )
        setUserRows(
          userLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            user_id: Number(row.UserId ?? 0),
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )
        setTokenRows(
          tokenLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            token_name: row.TokenName,
            user_id: Number(row.UserId ?? 0),
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )

        setLastUpdated(new Date().toLocaleString())
        setDateError('')
      } else {
        setDateError(message || 'Failed to fetch dashboard data')
        setRows([])
        setUserRows([])
        setTokenRows([])
      }
    } catch (error: any) {
      // Ignore abort errors
      if (error.name === 'AbortError' || error.name === 'CanceledError') {
        return
      }
      console.error('Failed to fetch dashboard data:', error)
      setDateError('Failed to fetch dashboard data')
      setRows([])
      setUserRows([])
      setTokenRows([])
    } finally {
      // Only clear loading if this request wasn't aborted
      if (!abortController.signal.aborted) {
        setLoading(false)
      }
    }
  }

  useEffect(() => {
    if (isAdmin) loadUsers()
    // load initial stats
    loadStats()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAdmin])

  // Fetch quota/status when dashUser changes (but not on initial load)
  // Presets
  const applyPreset = async (preset: 'today' | '7d' | '30d') => {
    const today = new Date()
    const start = new Date(today)
    if (preset === 'today') start.setDate(today.getDate())
    if (preset === '7d') start.setDate(today.getDate() - 6)
    if (preset === '30d') start.setDate(today.getDate() - 29)

    const newFromDate = fmt(start)
    const newToDate = fmt(today)

    setFromDate(newFromDate)
    setToDate(newToDate)

    // Validate and fetch data immediately
    const validationError = validateDateRange(newFromDate, newToDate)
    if (validationError) {
      setDateError(validationError)
      return
    }

    // Cancel any pending request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }

    // Create new AbortController for this request
    const abortController = new AbortController()
    abortControllerRef.current = abortController

    setLoading(true)
    setDateError('')
    try {
      const params = new URLSearchParams()
      params.set('from_date', newFromDate)
      params.set('to_date', newToDate)
      if (isAdmin) {
        params.set('user_id', dashUser || 'all')
      }
      const res = await api.get('/api/user/dashboard?' + params.toString(), {
        signal: abortController.signal
      })

      // Check if this request was aborted
      if (abortController.signal.aborted) {
        return
      }

      const { success, data, message } = res.data
      if (success) {
        const logs = data?.logs || data || []
        const userLogs = data?.user_logs || []
        const tokenLogs = data?.token_logs || []
        setRows(
          logs.map((row: any) => ({
            day: row.Day,
            model_name: row.ModelName,
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )
        setUserRows(
          userLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            user_id: Number(row.UserId ?? 0),
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )
        setTokenRows(
          tokenLogs.map((row: any) => ({
            day: row.Day,
            username: row.Username,
            token_name: row.TokenName,
            user_id: Number(row.UserId ?? 0),
            request_count: row.RequestCount,
            quota: row.Quota,
            prompt_tokens: row.PromptTokens,
            completion_tokens: row.CompletionTokens,
          }))
        )

        setLastUpdated(new Date().toLocaleString())
        setDateError('')
      } else {
        setDateError(message || 'Failed to fetch dashboard data')
        setRows([])
        setUserRows([])
        setTokenRows([])
      }
    } catch (error: any) {
      // Ignore abort errors
      if (error.name === 'AbortError' || error.name === 'CanceledError') {
        return
      }
      console.error('Failed to fetch dashboard data:', error)
      setDateError('Failed to fetch dashboard data')
      setRows([])
      setUserRows([])
      setTokenRows([])
    } finally {
      // Only clear loading if this request wasn't aborted
      if (!abortController.signal.aborted) {
        setLoading(false)
      }
    }
  }

  // Aggregate daily metrics (quota kept in raw units to avoid double conversion)
  const dailyAgg = useMemo(() => {
    const map: Record<string, { date: string; requests: number; quota: number; tokens: number }> = {}
    for (const r of rows) {
      if (!map[r.day]) {
        map[r.day] = { date: r.day, requests: 0, quota: 0, tokens: 0 }
      }
      map[r.day].requests += r.request_count || 0
      map[r.day].quota += r.quota || 0
      map[r.day].tokens += (r.prompt_tokens || 0) + (r.completion_tokens || 0)
    }
    return Object.values(map).sort((a, b) => a.date.localeCompare(b.date))
  }, [rows])

  const xAxisDays = useMemo(() => {
    const values = new Set<string>()
    for (const row of rows) {
      if (row.day) {
        values.add(row.day)
      }
    }
    for (const row of userRows) {
      if (row.day) {
        values.add(row.day)
      }
    }
    for (const row of tokenRows) {
      if (row.day) {
        values.add(row.day)
      }
    }
    return Array.from(values).sort((a, b) => a.localeCompare(b))
  }, [rows, userRows, tokenRows])

  const timeSeries = useMemo(() => {
    const quotaPerUnit = getQuotaPerUnit()
    const displayInCurrency = getDisplayInCurrency()
    return dailyAgg.map(day => ({
      date: day.date,
      requests: day.requests,
      quota: displayInCurrency ? day.quota / quotaPerUnit : day.quota,
      tokens: day.tokens,
    }))
  }, [dailyAgg])

  const computeStackedSeries = <T extends BaseMetricRow>(rowsSource: T[], daysList: string[], labelFn: (row: T) => string | null) => {
    const quotaPerUnit = getQuotaPerUnit()
    const displayInCurrency = getDisplayInCurrency()
    const dayToValues: Record<string, Record<string, number>> = {}
    for (const day of daysList) {
      dayToValues[day] = {}
    }

    const uniqueKeys: string[] = []
    const seen = new Set<string>()

    for (const row of rowsSource) {
      const label = labelFn(row)
      if (!label) {
        continue
      }
      if (!seen.has(label)) {
        uniqueKeys.push(label)
        seen.add(label)
      }

      const day = row.day
      if (!dayToValues[day]) {
        dayToValues[day] = {}
      }

      let value: number
      switch (statisticsMetric) {
        case 'requests':
          value = row.request_count || 0
          break
        case 'expenses':
          value = row.quota || 0
          if (displayInCurrency) {
            value = value / quotaPerUnit
          }
          break
        case 'tokens':
        default:
          value = (row.prompt_tokens || 0) + (row.completion_tokens || 0)
          break
      }

      dayToValues[day][label] = (dayToValues[day][label] || 0) + value
    }

    const stackedData = daysList.map(day => ({ date: day, ...(dayToValues[day] || {}) }))

    return { uniqueKeys, stackedData }
  }

  const { uniqueKeys: modelKeys, stackedData: modelStackedData } = useMemo(
    () => computeStackedSeries(rows, xAxisDays, (row) => (row.model_name ? row.model_name : 'Unknown model')),
    [rows, xAxisDays, statisticsMetric]
  )

  const { uniqueKeys: userKeys, stackedData: userStackedData } = useMemo(
    () => computeStackedSeries(userRows, xAxisDays, (row) => (row.username ? row.username : 'Unknown user')),
    [userRows, xAxisDays, statisticsMetric]
  )

  const { uniqueKeys: tokenKeys, stackedData: tokenStackedData } = useMemo(
    () =>
      computeStackedSeries(tokenRows, xAxisDays, (row) => {
        const token = row.token_name && row.token_name.trim().length > 0 ? row.token_name : 'unnamed token'
        const owner = row.username && row.username.trim().length > 0 ? row.username : 'unknown'
        return `${token}(${owner})`
      }),
    [tokenRows, xAxisDays, statisticsMetric]
  )

  const metricLabel = useMemo(() => {
    switch (statisticsMetric) {
      case 'requests':
        return 'Requests'
      case 'expenses':
        return 'Expenses'
      default:
        return 'Tokens'
    }
  }, [statisticsMetric])

  const formatStackedTick = useCallback((value: number) => {
    switch (statisticsMetric) {
      case 'requests':
        return formatNumber(value)
      case 'expenses':
        return getDisplayInCurrency()
          ? `$${Number(value).toFixed(2)}`
          : formatNumber(value)
      case 'tokens':
      default:
        return formatNumber(value)
    }
  }, [statisticsMetric])

  const stackedTooltip = useMemo(() => {
    return ({ active, payload, label }: any) => {
      if (active && payload && payload.length) {
        const filtered = payload
          .filter((entry: any) => entry.value && typeof entry.value === 'number' && entry.value > 0)
          .sort((a: any, b: any) => (b.value as number) - (a.value as number))

        if (!filtered.length) {
          return null
        }

        const formatValue = (value: number) => {
          switch (statisticsMetric) {
            case 'requests':
              return formatNumber(value)
            case 'expenses':
              return getDisplayInCurrency()
                ? `$${value.toFixed(6)}`
                : formatNumber(value)
            case 'tokens':
            default:
              return formatNumber(value)
          }
        }

        const isDark =
          typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
        const tooltipBg = isDark ? 'rgba(17,24,39,1)' : 'rgba(255,255,255,1)'
        const tooltipText = isDark ? 'rgba(255,255,255,0.95)' : 'rgba(17,24,39,0.9)'

        return (
          <div
            style={{
              backgroundColor: tooltipBg,
              border: '1px solid var(--border)',
              borderRadius: '8px',
              padding: '12px 16px',
              fontSize: '12px',
              color: tooltipText,
              boxShadow: '0 8px 32px rgba(0, 0, 0, 0.12)',
            }}
          >
            <div style={{ fontWeight: '600', marginBottom: '8px', color: 'var(--foreground)' }}>
              {label}
            </div>
            {filtered.map((entry: any, index: number) => (
              <div
                key={`${entry.name ?? 'series'}-${index}`}
                style={{ marginBottom: '4px', display: 'flex', alignItems: 'center' }}
              >
                <span
                  style={{
                    display: 'inline-block',
                    width: '12px',
                    height: '12px',
                    backgroundColor: entry.color,
                    borderRadius: '50%',
                    marginRight: '8px',
                  }}
                ></span>
                <span style={{ fontWeight: '600', color: 'var(--foreground)' }}>
                  {entry.name}: {formatValue(entry.value as number)}
                </span>
              </div>
            ))}
          </div>
        )
      }

      return null
    }
  }, [statisticsMetric])

  const rangeTotals = useMemo(() => {
    let requests = 0
    let quota = 0
    let tokens = 0
    const modelSet = new Set<string>()

    for (const row of rows) {
      requests += row.request_count || 0
      quota += row.quota || 0
      tokens += (row.prompt_tokens || 0) + (row.completion_tokens || 0)
      if (row.model_name) {
        modelSet.add(row.model_name)
      }
    }

    const dayCount = dailyAgg.length
    const avgCostPerRequestRaw = requests ? quota / requests : 0
    const avgTokensPerRequest = requests ? tokens / requests : 0
    const avgDailyRequests = dayCount ? requests / dayCount : 0
    const avgDailyQuotaRaw = dayCount ? quota / dayCount : 0
    const avgDailyTokens = dayCount ? tokens / dayCount : 0

    return {
      requests,
      quota,
      tokens,
      avgCostPerRequestRaw,
      avgTokensPerRequest,
      avgDailyRequests,
      avgDailyQuotaRaw,
      avgDailyTokens,
      dayCount,
      uniqueModels: modelSet.size,
    }
  }, [rows, dailyAgg])

  const {
    requests: totalRequests,
    quota: totalQuota,
    tokens: totalTokens,
    avgCostPerRequestRaw,
    avgTokensPerRequest,
    avgDailyRequests,
    avgDailyQuotaRaw,
    avgDailyTokens,
    uniqueModels: totalModels,
  } = rangeTotals

  const byModel = useMemo(() => {
    const mm: Record<string, { model: string; requests: number; quota: number; tokens: number }> = {}
    for (const r of rows) {
      const key = r.model_name
      if (!mm[key]) mm[key] = { model: key, requests: 0, quota: 0, tokens: 0 }
      mm[key].requests += r.request_count || 0
      mm[key].quota += r.quota || 0
      mm[key].tokens += (r.prompt_tokens || 0) + (r.completion_tokens || 0)
    }
    return Object.values(mm)
  }, [rows])

  const modelLeaders = useMemo(() => {
    if (!byModel.length) {
      return {
        mostRequested: null,
        mostTokens: null,
        mostQuota: null,
      }
    }

    const mostRequested = [...byModel].sort((a, b) => b.requests - a.requests)[0]
    const mostTokens = [...byModel].sort((a, b) => b.tokens - a.tokens)[0]
    const mostQuota = [...byModel].sort((a, b) => b.quota - a.quota)[0]

    return { mostRequested, mostTokens, mostQuota }
  }, [byModel])

  const rangeInsights = useMemo(() => {
    if (!dailyAgg.length) {
      return {
        busiestDay: null as { date: string; requests: number; quota: number; tokens: number } | null,
        tokenHeavyDay: null as { date: string; requests: number; quota: number; tokens: number } | null,
      }
    }

    let busiestDay = dailyAgg[0]
    let tokenHeavyDay = dailyAgg[0]

    for (const day of dailyAgg) {
      if (day.requests > busiestDay.requests) {
        busiestDay = day
      }
      if (day.tokens > tokenHeavyDay.tokens) {
        tokenHeavyDay = day
      }
    }

    return { busiestDay, tokenHeavyDay }
  }, [dailyAgg])

  if (!user) {
    return <div>Please log in to access the dashboard.</div>
  }

  // --- UI ---
  return (
    <ResponsivePageContainer
      title="Dashboard"
      description="Monitor your API usage and account statistics"
    >
      {loading && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
          <div className="flex flex-col items-center gap-3" role="status" aria-live="polite">
            <span className="h-10 w-10 animate-spin rounded-full border-4 border-primary/20 border-t-primary" />
            <p className="text-sm text-muted-foreground">Refreshing dashboard…</p>
          </div>
        </div>
      )}

      {/* Filter bar - Date Range Controls */}
      {filtersReady ? (
        <div className="bg-white dark:bg-gray-900 rounded-lg border p-4 mb-6">
          <div className="flex flex-col lg:flex-row gap-4 items-start lg:items-end w-full">
            <div className="flex flex-col sm:flex-row gap-3 flex-1 w-full">
              <div className="flex-1 min-w-0">
                <label className="text-sm font-medium mb-2 block">From</label>
                <Input
                  type="date"
                  value={fromDate}
                  min={getMinDate()}
                  max={getMaxDate()}
                  onChange={(e) => setFromDate(e.target.value)}
                  className={cn("h-10", dateError ? "border-red-500" : "")}
                  aria-label="From date"
                />
              </div>
              <div className="flex-1 min-w-0">
                <label className="text-sm font-medium mb-2 block">To</label>
                <Input
                  type="date"
                  value={toDate}
                  min={getMinDate()}
                  max={getMaxDate()}
                  onChange={(e) => setToDate(e.target.value)}
                  className={cn("h-10", dateError ? "border-red-500" : "")}
                  aria-label="To date"
                />
              </div>
              {isAdmin && (
                <div className="flex-1 min-w-0">
                  <label className="text-sm font-medium mb-2 block">User</label>
                  <select
                    className="h-11 sm:h-10 w-full border rounded-md px-3 py-2 text-base sm:text-sm bg-background"
                    value={dashUser}
                    onChange={(e) => setDashUser(e.target.value)}
                    aria-label="Select user"
                  >
                    <option value="all">All Users</option>
                    {userOptions.map(u => (
                      <option key={u.id} value={String(u.id)}>{u.display_name || u.username}</option>
                    ))}
                  </select>
                </div>
              )}
            </div>

            <div className="flex flex-wrap sm:flex-nowrap gap-2 w-full sm:w-auto sm:justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => applyPreset('today')}
                className="h-10 flex-1 min-w-[6rem] sm:flex-none"
              >
                Today
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => applyPreset('7d')}
                className="h-10 flex-1 min-w-[6rem] sm:flex-none"
              >
                7D
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => applyPreset('30d')}
                className="h-10 flex-1 min-w-[6rem] sm:flex-none"
              >
                30D
              </Button>
              <Button
                onClick={loadStats}
                disabled={loading}
                className="h-10 flex-1 min-w-[6rem] sm:flex-none sm:px-6"
              >
                {loading ? 'Loading...' : 'Apply'}
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-900 rounded-lg border p-4 mb-6">
          <div className="flex flex-col gap-3 animate-pulse">
            <div className="h-4 bg-muted/30 rounded w-24" />
            <div className="h-11 bg-muted/30 rounded" />
            <div className="h-11 bg-muted/30 rounded" />
            <div className="h-11 bg-muted/30 rounded" />
          </div>
        </div>
      )}

      {/* Error Message */}
      {dateError && (
        <div
          id="date-error"
          className="mb-4 p-3 bg-red-50 border border-red-200 rounded-md dark:bg-red-950/20 dark:border-red-800"
          role="alert"
          aria-live="polite"
        >
          <div className="flex items-center gap-2">
            <svg className="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span className="text-sm font-medium text-red-800 dark:text-red-200">Error</span>
          </div>
          <p className="text-sm text-red-700 dark:text-red-300 mt-1">{dateError}</p>
        </div>
      )}
      {/* Usage Overview */}
      <div className="mb-6">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mb-6">
          <div>
            <h2 className="text-xl font-semibold">Usage Overview</h2>
            <p className="text-sm text-muted-foreground">Totals and leaders for the selected time range</p>
          </div>
          {lastUpdated && (
            <span className="text-xs text-muted-foreground">Updated: {lastUpdated}</span>
          )}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Total Requests</div>
            <div className="text-2xl font-bold mt-1">{formatNumber(totalRequests)}</div>
            <div className="text-xs text-muted-foreground mt-2">Avg daily: {formatNumber(Math.round(avgDailyRequests || 0))}</div>
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Quota Used</div>
            <div className="text-2xl font-bold mt-1">{renderQuota(totalQuota)}</div>
            <div className="text-xs text-muted-foreground mt-2">Avg daily: {renderQuota(avgDailyQuotaRaw)}</div>
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Tokens Consumed</div>
            <div className="text-2xl font-bold mt-1">{formatNumber(totalTokens)}</div>
            <div className="text-xs text-muted-foreground mt-2">Avg daily: {formatNumber(Math.round(avgDailyTokens || 0))}</div>
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Avg Cost / Request</div>
            <div className="text-2xl font-bold mt-1">{renderQuota(avgCostPerRequestRaw, 4)}</div>
            <div className="text-xs text-muted-foreground mt-2">{Math.round(avgTokensPerRequest || 0)} tokens per request</div>
          </div>
        </div>

        <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
          <h3 className="text-lg font-semibold mb-4">Top Models This Period</h3>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
              <div className="text-sm text-muted-foreground">Most Requests</div>
              <div className="text-xl font-semibold mt-1">
                {modelLeaders.mostRequested ? modelLeaders.mostRequested.model : 'No data'}
              </div>
              {modelLeaders.mostRequested && (
                <div className="text-xs text-muted-foreground mt-2">
                  {formatNumber(modelLeaders.mostRequested.requests)} requests
                </div>
              )}
            </div>
            <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
              <div className="text-sm text-muted-foreground">Most Tokens</div>
              <div className="text-xl font-semibold mt-1">
                {modelLeaders.mostTokens ? modelLeaders.mostTokens.model : 'No data'}
              </div>
              {modelLeaders.mostTokens && (
                <div className="text-xs text-muted-foreground mt-2">
                  {formatNumber(modelLeaders.mostTokens.tokens)} tokens
                </div>
              )}
            </div>
            <div className="rounded-lg border bg-white dark:bg-gray-900/70 p-4">
              <div className="text-sm text-muted-foreground">Highest Cost</div>
              <div className="text-xl font-semibold mt-1">
                {modelLeaders.mostQuota ? modelLeaders.mostQuota.model : 'No data'}
              </div>
              {modelLeaders.mostQuota && (
                <div className="text-xs text-muted-foreground mt-2">
                  {renderQuota(modelLeaders.mostQuota.quota)} consumed
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Busiest Day</div>
            <div className="text-lg font-semibold mt-1">
              {rangeInsights.busiestDay ? rangeInsights.busiestDay.date : 'No data'}
            </div>
            {rangeInsights.busiestDay && (
              <div className="text-xs text-muted-foreground mt-2">
                {formatNumber(rangeInsights.busiestDay.requests)} requests
              </div>
            )}
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Peak Token Day</div>
            <div className="text-lg font-semibold mt-1">
              {rangeInsights.tokenHeavyDay ? rangeInsights.tokenHeavyDay.date : 'No data'}
            </div>
            {rangeInsights.tokenHeavyDay && (
              <div className="text-xs text-muted-foreground mt-2">
                {formatNumber(rangeInsights.tokenHeavyDay.tokens)} tokens
              </div>
            )}
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <div className="text-sm text-muted-foreground">Models in Use</div>
            <div className="text-lg font-semibold mt-1">{formatNumber(totalModels)}</div>
            <div className="text-xs text-muted-foreground mt-2">
              {totalModels ? `${formatNumber(Math.round(totalRequests / totalModels))} requests per model` : '—'}
            </div>
          </div>
        </div>

        {/* Time Series */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <h3 className="font-medium mb-4 text-blue-600">Requests</h3>
            <ResponsiveContainer width="100%" height={140}>
              <LineChart data={timeSeries}>
                <GradientDefs />
                <CartesianGrid strokeOpacity={0.1} vertical={false} />
                <XAxis dataKey="date" hide />
                <YAxis hide />
                <Tooltip
                  contentStyle={{
                    backgroundColor: 'var(--background)',
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    fontSize: '12px'
                  }}
                />
                <Line
                  type="monotone"
                  dataKey="requests"
                  stroke={chartConfig.colors.requests}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, fill: chartConfig.colors.requests }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <h3 className="font-medium mb-4 text-cyan-600">Quota</h3>
            <ResponsiveContainer width="100%" height={140}>
              <LineChart data={timeSeries}>
                <GradientDefs />
                <CartesianGrid strokeOpacity={0.1} vertical={false} />
                <XAxis dataKey="date" hide />
                <YAxis hide />
                <Tooltip
                  contentStyle={{
                    backgroundColor: 'var(--background)',
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    fontSize: '12px'
                  }}
                />
                <Line
                  type="monotone"
                  dataKey="quota"
                  stroke={chartConfig.colors.quota}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, fill: chartConfig.colors.quota }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>

          <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
            <h3 className="font-medium mb-4 text-pink-600">Tokens</h3>
            <ResponsiveContainer width="100%" height={140}>
              <LineChart data={timeSeries}>
                <GradientDefs />
                <CartesianGrid strokeOpacity={0.1} vertical={false} />
                <XAxis dataKey="date" hide />
                <YAxis hide />
                <Tooltip
                  contentStyle={{
                    backgroundColor: 'var(--background)',
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    fontSize: '12px'
                  }}
                />
                <Line
                  type="monotone"
                  dataKey="tokens"
                  stroke={chartConfig.colors.tokens}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, fill: chartConfig.colors.tokens }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Model Usage Statistics */}
        <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
          <div className="flex items-center justify-between mb-6">
            <h3 className="text-lg font-semibold">Model Usage</h3>
            <Select
              value={statisticsMetric}
              onValueChange={(value) => setStatisticsMetric(value as 'tokens' | 'requests' | 'expenses')}
            >
              <SelectTrigger className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tokens">Tokens</SelectItem>
                <SelectItem value="requests">Requests</SelectItem>
                <SelectItem value="expenses">Expenses</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={modelStackedData}>
              <CartesianGrid strokeOpacity={0.1} vertical={false} />
              <XAxis
                dataKey="date"
                tickLine={false}
                axisLine={false}
                fontSize={12}
              />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={60}
                fontSize={12}
                tickFormatter={formatStackedTick}
              />
              <Tooltip content={stackedTooltip} />
              <Legend />
              {modelKeys.map((m, idx) => (
                <Bar key={m} dataKey={m} stackId="statistics-models" fill={barColor(idx)} radius={[2, 2, 0, 0]} />
              ))}
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
          <div className="flex items-center justify-between mb-6">
            <h3 className="text-lg font-semibold">User Usage</h3>
            <span className="text-xs text-muted-foreground">Metric: {metricLabel}</span>
          </div>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={userStackedData}>
              <CartesianGrid strokeOpacity={0.1} vertical={false} />
              <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={60}
                fontSize={12}
                tickFormatter={formatStackedTick}
              />
              <Tooltip content={stackedTooltip} />
              <Legend />
              {userKeys.map((userKey, idx) => (
                <Bar
                  key={userKey}
                  dataKey={userKey}
                  stackId="statistics-users"
                  fill={barColor(idx)}
                  radius={[2, 2, 0, 0]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="bg-white dark:bg-gray-900 rounded-lg border p-6 mb-6">
          <div className="flex items-center justify-between mb-6">
            <h3 className="text-lg font-semibold">Token Usage</h3>
            <span className="text-xs text-muted-foreground">Metric: {metricLabel}</span>
          </div>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={tokenStackedData}>
              <CartesianGrid strokeOpacity={0.1} vertical={false} />
              <XAxis dataKey="date" tickLine={false} axisLine={false} fontSize={12} />
              <YAxis
                tickLine={false}
                axisLine={false}
                width={60}
                fontSize={12}
                tickFormatter={formatStackedTick}
              />
              <Tooltip content={stackedTooltip} />
              <Legend />
              {tokenKeys.map((tokenKey, idx) => (
                <Bar
                  key={tokenKey}
                  dataKey={tokenKey}
                  stackId="statistics-tokens"
                  fill={barColor(idx)}
                  radius={[2, 2, 0, 0]}
                />
              ))}
            </BarChart>
          </ResponsiveContainer>
        </div>

        {/* Additional sections removed per optimization request */}
      </div>
    </ResponsivePageContainer>
  )
}

export function barColor(i: number) {
  return chartConfig.barColors[i % chartConfig.barColors.length]
}
