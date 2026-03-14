import * as React from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { RefreshCw, Eye, EyeOff, TrendingUp, TrendingDown } from 'lucide-react'
import { renderQuota } from '@/lib/utils'

interface StatisticsData {
  quota: number
  token?: number
  request_count?: number
  [key: string]: any
}

interface StatisticsProps {
  title?: string
  data: StatisticsData
  loading?: boolean
  onRefresh?: () => void
  visible?: boolean
  onToggleVisibility?: () => void
  additionalStats?: Array<{
    label: string
    value: string | number
    icon?: React.ReactNode
    trend?: 'up' | 'down' | 'neutral'
  }>
  className?: string
}

export function Statistics({
  title = 'Usage Statistics',
  data,
  loading = false,
  onRefresh,
  visible = true,
  onToggleVisibility,
  additionalStats = [],
  className,
}: StatisticsProps) {
  if (!visible) {
    return (
      <div className={className}>
        <Button
          size="sm"
          variant="ghost"
          onClick={onToggleVisibility}
          className="flex items-center gap-2 text-muted-foreground"
        >
          <Eye className="h-4 w-4" />
          Click to view {title.toLowerCase()}
        </Button>
      </div>
    )
  }

  const formatStatValue = (value: string | number): string => {
    if (typeof value === 'number') {
      if (value >= 1000000) {
        return (value / 1000000).toFixed(1) + 'M'
      } else if (value >= 1000) {
        return (value / 1000).toFixed(1) + 'K'
      }
      return value.toString()
    }
    return value
  }

  const getTrendIcon = (trend?: 'up' | 'down' | 'neutral') => {
    switch (trend) {
      case 'up':
        return <TrendingUp className="h-3 w-3 text-success" />
      case 'down':
        return <TrendingDown className="h-3 w-3 text-destructive" />
      default:
        return null
    }
  }

  return (
    <Card className={className}>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center justify-between text-base">
          <span>{title}</span>
          <div className="flex items-center gap-2">
            {onRefresh && (
              <Button
                size="sm"
                variant="ghost"
                onClick={onRefresh}
                disabled={loading}
                className="h-6 w-6 p-0"
                title="Refresh statistics"
              >
                <RefreshCw className={`h-3 w-3 ${loading ? 'animate-spin' : ''}`} />
              </Button>
            )}
            {onToggleVisibility && (
              <Button
                size="sm"
                variant="ghost"
                onClick={onToggleVisibility}
                className="h-6 w-6 p-0"
                title="Hide statistics"
              >
                <EyeOff className="h-3 w-3" />
              </Button>
            )}
          </div>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {/* Primary quota statistic */}
          <div className="flex flex-col">
            <span className="text-sm text-muted-foreground">Total Quota</span>
            <span className="text-2xl font-semibold text-primary">
              {renderQuota(data.quota)}
            </span>
          </div>

          {/* Token count if available */}
          {data.token !== undefined && (
            <div className="flex flex-col">
              <span className="text-sm text-muted-foreground">Total Tokens</span>
              <span className="text-2xl font-semibold">
                {formatStatValue(data.token)}
              </span>
            </div>
          )}

          {/* Request count if available */}
          {data.request_count !== undefined && (
            <div className="flex flex-col">
              <span className="text-sm text-muted-foreground">Total Requests</span>
              <span className="text-2xl font-semibold">
                {formatStatValue(data.request_count)}
              </span>
            </div>
          )}

          {/* Additional custom statistics */}
          {additionalStats.map((stat, index) => (
            <div key={index} className="flex flex-col">
              <span className="text-sm text-muted-foreground flex items-center gap-1">
                {stat.icon}
                {stat.label}
              </span>
              <span className="text-2xl font-semibold flex items-center gap-1">
                {formatStatValue(stat.value)}
                {getTrendIcon(stat.trend)}
              </span>
            </div>
          ))}
        </div>

        {/* Loading state */}
        {loading && (
          <div className="mt-4 p-2 bg-muted/50 rounded text-center text-sm text-muted-foreground">
            Refreshing statistics...
          </div>
        )}
      </CardContent>
    </Card>
  )
}
