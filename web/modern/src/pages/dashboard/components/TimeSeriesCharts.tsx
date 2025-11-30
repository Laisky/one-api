import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from 'recharts'
import { CHART_CONFIG } from '../types'

interface TimeSeriesChartsProps {
  timeSeries: any[]
}

const GradientDefs = () => (
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
)

export function TimeSeriesCharts({ timeSeries }: TimeSeriesChartsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-indigo-500 dark:text-indigo-400">{t('dashboard.labels.requests')}</h3>
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
              stroke={CHART_CONFIG.colors.requests}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: CHART_CONFIG.colors.requests }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-sky-500 dark:text-sky-400">{t('dashboard.labels.quota')}</h3>
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
              stroke={CHART_CONFIG.colors.quota}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: CHART_CONFIG.colors.quota }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="bg-white dark:bg-gray-900 rounded-lg border p-4">
        <h3 className="font-medium mb-4 text-violet-500 dark:text-violet-400">{t('dashboard.labels.tokens')}</h3>
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
              stroke={CHART_CONFIG.colors.tokens}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: CHART_CONFIG.colors.tokens }}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
