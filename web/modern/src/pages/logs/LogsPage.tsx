import { useEffect, useMemo, useState } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table'
import { SearchableDropdown, type SearchOption } from '@/components/ui/searchable-dropdown'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { formatTimestamp, fromDateTimeLocal, toDateTimeLocal, renderQuota, cn } from '@/lib/utils'
import { useAuthStore } from '@/lib/stores/auth'
import { RefreshCw, Eye, EyeOff, Copy, FileDown, Calendar, Filter } from 'lucide-react'
import { TracingModal } from '@/components/TracingModal'

interface LogRow {
  id: number
  type: number
  created_at: number
  model_name: string
  token_name?: string
  username?: string
  channel?: number
  quota: number
  prompt_tokens?: number
  completion_tokens?: number
  cached_prompt_tokens?: number
  cached_completion_tokens?: number
  cache_write_5m_tokens?: number
  cache_write_1h_tokens?: number
  elapsed_time?: number
  request_id?: string
  trace_id?: string
  content?: string
  is_stream?: boolean
  system_prompt_reset?: boolean
}

interface LogStatistics {
  quota: number
  token_count?: number
  request_count?: number
}

// Log type constants
const LOG_TYPES = {
  ALL: 0,
  TOPUP: 1,
  CONSUME: 2,
  MANAGE: 3,
  SYSTEM: 4,
  TEST: 5,
} as const

const LOG_TYPE_OPTIONS = [
  { value: '0', label: 'All Types' },
  { value: '1', label: 'Topup' },
  { value: '2', label: 'Consume' },
  { value: '3', label: 'Management' },
  { value: '4', label: 'System' },
  { value: '5', label: 'Test' },
]

const getLogTypeBadge = (type: number) => {
  switch (type) {
    case LOG_TYPES.TOPUP:
      return <Badge className="bg-green-100 text-green-800">Topup</Badge>
    case LOG_TYPES.CONSUME:
      return <Badge className="bg-blue-100 text-blue-800">Consume</Badge>
    case LOG_TYPES.MANAGE:
      return <Badge className="bg-purple-100 text-purple-800">Management</Badge>
    case LOG_TYPES.SYSTEM:
      return <Badge className="bg-gray-100 text-gray-800">System</Badge>
    case LOG_TYPES.TEST:
      return <Badge className="bg-yellow-100 text-yellow-800">Test</Badge>
    default:
      return <Badge variant="outline">Unknown</Badge>
  }
}

const formatLatency = (ms?: number) => {
  if (!ms) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

const getLatencyColor = (ms?: number) => {
  if (!ms) return ''
  if (ms < 1000) return 'text-green-600'
  if (ms < 3000) return 'text-yellow-600'
  return 'text-red-600'
}

export function LogsPage() {
  const { user } = useAuthStore()
  const [data, setData] = useState<LogRow[]>([])
  const [loading, setLoading] = useState(false)
  const [pageIndex, setPageIndex] = useState(0)
  const [pageSize, setPageSize] = useState(10)
  const [total, setTotal] = useState(0)

  // Determine if user is admin/root
  // Use strict equality for admin (10) and root (100)
  const isAdmin = useMemo(() => (user?.role ?? 0) === 10, [user])
  const isRoot = useMemo(() => (user?.role ?? 0) === 100, [user])
  const isAdminOrRoot = isAdmin || isRoot

  // Filters: for admin/root, username is '', for others, username is self
  const [filters, setFilters] = useState(() => ({
    type: '0',
    model_name: '',
    token_name: '',
    username: (user && (user.role === 10 || user.role === 100)) ? '' : (user?.username || ''),
    channel: '',
    start_timestamp: toDateTimeLocal(Math.floor((Date.now() - 7 * 24 * 3600 * 1000) / 1000)),
    end_timestamp: toDateTimeLocal(Math.floor((Date.now() + 3600 * 1000) / 1000)),
  }))

  // Statistics
  const [stat, setStat] = useState<LogStatistics>({ quota: 0 })
  const [showStat, setShowStat] = useState(false)
  const [statLoading, setStatLoading] = useState(false)

  // Search
  const [searchKeyword, setSearchKeyword] = useState('')
  const [searchOptions, setSearchOptions] = useState<SearchOption[]>([])
  const [searchLoading, setSearchLoading] = useState(false)

  // Sorting
  const [sortBy, setSortBy] = useState('created_at')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc')

  // Tracing modal
  const [tracingModalOpen, setTracingModalOpen] = useState(false)
  const [selectedLogId, setSelectedLogId] = useState<number | null>(null)
  const [selectedTraceId, setSelectedTraceId] = useState<string | null>(null)

  // (removed duplicate isAdmin declaration)

  const load = async (p = 0, size = pageSize) => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      params.set('p', String(p))
      params.set('size', String(size))

      if (filters.type !== '0') params.set('type', filters.type)
      if (filters.model_name) params.set('model_name', filters.model_name)
      if (filters.token_name) params.set('token_name', filters.token_name)
      if (isAdminOrRoot && filters.username) params.set('username', filters.username)
      if (filters.channel && isAdminOrRoot) params.set('channel', filters.channel)
      if (filters.start_timestamp) params.set('start_timestamp', String(fromDateTimeLocal(filters.start_timestamp)))
      if (filters.end_timestamp) params.set('end_timestamp', String(fromDateTimeLocal(filters.end_timestamp)))
      if (sortBy) {
        params.set('sort', sortBy)
        params.set('order', sortOrder)
      }

      // Unified API call - complete URL with /api prefix
      const path = isAdminOrRoot ? `/api/log/?${params}` : `/api/log/self?${params}`
      const res = await api.get(path)
      const { success, data: responseData, total: responseTotal } = res.data

      if (success) {
        setData(responseData || [])
        setTotal(responseTotal || 0)
        setPageIndex(p)
        setPageSize(size)
      }
    } catch (error) {
      console.error('Failed to load logs:', error)
      setData([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }

  const loadStatistics = async () => {
    setStatLoading(true)
    try {
      const params = new URLSearchParams()
      if (filters.type !== '0') params.set('type', filters.type)
      if (filters.model_name) params.set('model_name', filters.model_name)
      if (filters.token_name) params.set('token_name', filters.token_name)
      if (isAdminOrRoot && filters.username) params.set('username', filters.username)
      if (filters.channel && isAdminOrRoot) params.set('channel', filters.channel)
      if (filters.start_timestamp) params.set('start_timestamp', String(fromDateTimeLocal(filters.start_timestamp)))
      if (filters.end_timestamp) params.set('end_timestamp', String(fromDateTimeLocal(filters.end_timestamp)))

      // Unified API call - complete URL with /api prefix
      const statPath = isAdminOrRoot ? '/api/log/stat' : '/api/log/self/stat'
      const res = await api.get(statPath + '?' + params.toString())

      if (res.data?.success) {
        setStat(res.data.data || { quota: 0 })
      }
    } catch (error) {
      console.error('Failed to load statistics:', error)
    } finally {
      setStatLoading(false)
    }
  }

  // Search functionality
  const searchLogs = async (query: string) => {
    if (!query.trim()) {
      setSearchOptions([])
      return
    }

    setSearchLoading(true)
    try {
      // Unified API call - complete URL with /api prefix
      const url = isAdminOrRoot ? '/api/log/search' : '/api/log/self/search'
      const res = await api.get(url + '?keyword=' + encodeURIComponent(query))
      const { success, data: responseData } = res.data

      if (success && Array.isArray(responseData)) {
        const options: SearchOption[] = responseData.slice(0, 10).map((log: LogRow) => ({
          key: log.id.toString(),
          value: log.content || log.model_name || 'Log Entry',
          text: log.content || log.model_name || 'Log Entry',
          content: (
            <div className="flex flex-col">
              <div className="font-medium">{log.model_name}</div>
              <div className="text-sm text-muted-foreground">
                {formatTimestamp(log.created_at)} • {getLogTypeBadge(log.type)} • Quota: {renderQuota(log.quota)}
              </div>
            </div>
          )
        }))
        setSearchOptions(options)
      }
    } catch (error) {
      console.error('Search failed:', error)
      setSearchOptions([])
    } finally {
      setSearchLoading(false)
    }
  }

  const performSearch = async () => {
    if (!searchKeyword.trim()) {
      return load(0, pageSize)
    }

    setLoading(true)
    try {
      // Unified API call - complete URL with /api prefix
      const url = isAdminOrRoot ? '/api/log/search' : '/api/log/self/search'
      const res = await api.get(url + '?keyword=' + encodeURIComponent(searchKeyword))
      const { success, data: responseData } = res.data

      if (success) {
        setData(responseData || [])
        setPageIndex(0)
        setTotal(responseData?.length || 0)
      }
    } catch (error) {
      console.error('Search failed:', error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(0, pageSize)
  }, [pageSize])

  useEffect(() => {
    if (showStat) {
      loadStatistics()
    }
  }, [showStat, filters])

  const toggleStatVisibility = () => {
    setShowStat(!showStat)
  }

  const handleFilterSubmit = () => {
    load(0, pageSize)
  }

  const handleClearLogs = async () => {
    const ts = fromDateTimeLocal(filters.end_timestamp)
    const confirmed = window.confirm('Delete logs before ' + filters.end_timestamp + ' ?')
    if (!confirmed) return

    try {
      // Unified API call - complete URL with /api prefix
      await api.delete('/api/log?target_timestamp=' + ts)
      load(0, pageSize)
    } catch (error) {
      console.error('Failed to clear logs:', error)
    }
  }

  const handleExportLogs = () => {
    // Implementation for exporting logs to CSV
    const csvHeaders = ['Time', 'Type', 'Model', 'Token', 'Username', 'Quota', 'Prompt Tokens', 'Completion Tokens', 'Cached Prompt Tokens', 'Cached Completion Tokens', 'Cache Write 5m Tokens', 'Cache Write 1h Tokens', 'Latency', 'Content']
    const csvData = data.map(log => [
      formatTimestamp(log.created_at),
      log.type,
      log.model_name,
      log.token_name || '',
      log.username || '',
      log.quota,
      log.prompt_tokens || 0,
      log.completion_tokens || 0,
      log.cached_prompt_tokens || 0,
      log.cached_completion_tokens || 0,
      log.cache_write_5m_tokens || 0,
      log.cache_write_1h_tokens || 0,
      log.elapsed_time || 0,
      (log.content || '').replace(/,/g, ';').replace(/\n/g, ' ')
    ])

    const csv = [csvHeaders, ...csvData].map(row => row.join(',')).join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `logs_${new Date().toISOString().split('T')[0]}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const CopyButton = ({ text }: { text: string }) => (
    <Button
      size="sm"
      variant="ghost"
      className="h-6 w-6 p-0"
      onClick={() => navigator.clipboard.writeText(text)}
    >
      <Copy className="h-3 w-3" />
    </Button>
  )

  const ExpandableCell = ({ content, isStream, systemPromptReset }: {
    content?: string,
    isStream?: boolean,
    systemPromptReset?: boolean
  }) => {
    const [expanded, setExpanded] = useState(false)
    const maxLength = 100
    const truncated = (content || '').length > maxLength

    return (
      <div className="max-w-[300px]">
        <div className={expanded ? 'whitespace-pre-wrap' : 'truncate'} title={content}>
          {expanded ? content : ((content || '').slice(0, truncated ? maxLength : undefined))}
          {truncated && (
            <Button
              size="sm"
              variant="ghost"
              className="ml-1 px-2 h-6 text-xs"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? 'Less' : 'More'}
            </Button>
          )}
        </div>
        {(isStream || systemPromptReset) && (
          <div className="mt-1 flex gap-1 flex-wrap">
            {isStream && (
              <Badge variant="secondary" className="text-xs">
                Stream
              </Badge>
            )}
            {systemPromptReset && (
              <Badge variant="destructive" className="text-xs">
                System Reset
              </Badge>
            )}
          </div>
        )}
      </div>
    )
  }

  const columns: ColumnDef<LogRow>[] = [
    {
      accessorKey: 'created_at',
      header: 'Time',
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs" title={row.original.request_id || ''}>
            {formatTimestamp(row.original.created_at)}
          </span>
          {row.original.request_id && <CopyButton text={row.original.request_id} />}
        </div>
      ),
    },
    ...(isAdminOrRoot ? [{
      accessorKey: 'channel',
      header: 'Channel',
      cell: ({ row }: { row: any }) => (
        <span className="font-mono text-sm">{row.original.channel || '-'}</span>
      ),
    } as ColumnDef<LogRow>] : []),
    {
      accessorKey: 'type',
      header: 'Type',
      cell: ({ row }) => getLogTypeBadge(row.original.type),
    },
    {
      accessorKey: 'model_name',
      header: 'Model',
      cell: ({ row }) => (
        <span className="font-medium">{row.original.model_name}</span>
      ),
    },
    ...(Number(filters.type) !== LOG_TYPES.TEST ? [
      // Always show user column; for non-admins fall back to current user if username missing
      {
        accessorKey: 'username',
        header: 'User',
        cell: ({ row }) => (
          <span className="text-sm">{row.original.username || user?.username || '-'}</span>
        ),
      } as ColumnDef<LogRow>,
      {
        accessorKey: 'token_name',
        header: 'Token',
        cell: ({ row }) => (
          <span className="text-sm">{row.original.token_name || '-'}</span>
        ),
      },
      {
        accessorKey: 'prompt_tokens',
        header: 'Prompt',
        cell: ({ row }) => (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="font-mono text-sm cursor-help">
                  {row.original.prompt_tokens || 0}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <div className="flex flex-col gap-1">
                  <div>Input tokens: {row.original.prompt_tokens ?? 0}</div>
                  <div>Cached tokens: {row.original.cached_prompt_tokens ?? 0}</div>
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ),
      },
      {
        accessorKey: 'completion_tokens',
        header: 'Completion',
        cell: ({ row }) => (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="font-mono text-sm cursor-help">{row.original.completion_tokens || 0}</span>
              </TooltipTrigger>
              <TooltipContent>
                <div className="flex flex-col gap-1">
                  <div>Output tokens: {row.original.completion_tokens ?? 0}</div>
                  <div>Cached tokens: {row.original.cached_completion_tokens ?? 0}</div>
                  <div>Cache write 5m: {row.original.cache_write_5m_tokens ?? 0}</div>
                  <div>Cache write 1h: {row.original.cache_write_1h_tokens ?? 0}</div>
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ),
      },
      {
        accessorKey: 'quota',
        header: 'Cost',
        cell: ({ row }) => (
          <span className="font-mono text-sm" title={row.original.content || ''}>
            {renderQuota(row.original.quota)}
          </span>
        ),
      },
      {
        accessorKey: 'elapsed_time',
        header: 'Latency',
        cell: ({ row }) => (
          <span className={cn('font-mono text-sm', getLatencyColor(row.original.elapsed_time))}>
            {formatLatency(row.original.elapsed_time)}
          </span>
        ),
      },
    ] : [] as ColumnDef<LogRow>[])
  ]

  const handlePageChange = (newPageIndex: number, newPageSize: number) => {
    if (searchKeyword.trim()) {
      setPageIndex(newPageIndex)
    } else {
      load(newPageIndex, newPageSize)
    }
  }

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize)
    if (searchKeyword.trim()) {
      performSearch()
    } else {
      load(0, newPageSize)
    }
  }

  const handleSortChange = (newSortBy: string, newSortOrder: 'asc' | 'desc') => {
    setSortBy(newSortBy)
    setSortOrder(newSortOrder)
    load(0, pageSize)
  }

  const handleRowClick = (log: LogRow) => {
    if (log.trace_id && log.trace_id.trim() !== '') {
      setSelectedLogId(log.id)
      setSelectedTraceId(log.trace_id)
      setTracingModalOpen(true)
    }
  }

  const handleTracingModalClose = () => {
    setTracingModalOpen(false)
    setSelectedLogId(null)
  }

  const refresh = () => {
    if (searchKeyword.trim()) {
      performSearch()
    } else {
      load(pageIndex, pageSize)
    }
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                Logs
                {showStat && (
                  <div className="text-sm font-normal text-muted-foreground">
                    (Total Quota: {renderQuota(stat.quota)}
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={loadStatistics}
                      disabled={statLoading}
                      className="ml-2 h-6 w-6 p-0"
                    >
                      <RefreshCw className={cn('h-3 w-3', statLoading && 'animate-spin')} />
                    </Button>
                    )
                  </div>
                )}
              </CardTitle>
              <CardDescription>
                View and analyze API request logs with advanced filtering
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                onClick={toggleStatVisibility}
                className="gap-2"
              >
                {showStat ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                {showStat ? 'Hide' : 'Show'} Stats
              </Button>
              <Button variant="outline" onClick={handleExportLogs} className="gap-2">
                <FileDown className="h-4 w-4" />
                Export
              </Button>
              {isAdmin && (
                <Button variant="destructive" onClick={handleClearLogs}>
                  Clear Logs
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Filters */}
          <div className="grid grid-cols-1 md:grid-cols-7 gap-3 md:gap-4 mb-6 p-4 border rounded-lg bg-muted/10">
            <div className="md:col-span-7 flex items-center gap-2 mb-1">
              <Filter className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">Filters</span>
            </div>
            <div>
              <Label className="text-xs">Type</Label>
              <Select value={filters.type} onValueChange={(value) => setFilters({ ...filters, type: value })}>
                <SelectTrigger className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LOG_TYPE_OPTIONS.map(option => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="text-xs">Model</Label>
              <SearchableDropdown
                value={filters.model_name}
                placeholder="Model"
                searchPlaceholder="Model"
                options={[]}
                searchEndpoint="/api/models/display" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) => {
                  // /api/models/display returns a map; flatten to model names
                  const options: SearchOption[] = []
                  if (data && typeof data === 'object') {
                    Object.values<any>(data).forEach((entry: any) => {
                      if (entry?.models && typeof entry.models === 'object') {
                        Object.keys(entry.models).forEach((modelName: string) => {
                          options.push({ key: modelName, value: modelName, text: modelName })
                        })
                      }
                    })
                  }
                  return options
                }}
                onChange={(value) => setFilters({ ...filters, model_name: value })}
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">Token</Label>
              <SearchableDropdown
                value={filters.token_name}
                placeholder="Token"
                searchPlaceholder="Token"
                options={[]}
                searchEndpoint="/api/token/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) => (Array.isArray(data) ? data.map((t: any) => ({ key: String(t.id), value: t.name, text: t.name })) : [])}
                onChange={(value) => setFilters({ ...filters, token_name: value })}
                clearable
              />
            </div>
            <div>
              <Label className="text-xs">Username</Label>
              <SearchableDropdown
                value={filters.username}
                placeholder="Username"
                searchPlaceholder="Username"
                options={[]}
                searchEndpoint="/api/user/search" // SearchableDropdown uses fetch() directly, needs /api prefix
                transformResponse={(data) => (Array.isArray(data) ? data.map((u: any) => ({ key: String(u.id), value: u.username, text: u.username })) : [])}
                onChange={(value) => setFilters({ ...filters, username: value })}
                clearable
              />
            </div>
            {isAdmin && (
              <>
                <div>
                  <Label className="text-xs">Channel ID</Label>
                  <Input
                    value={filters.channel}
                    onChange={(e) => setFilters({ ...filters, channel: e.target.value })}
                    placeholder="Channel ID"
                    className="h-9"
                  />
                </div>
              </>
            )}
            <div className="md:col-span-2 grid grid-cols-2 gap-3">
              <div>
                <Label className="text-xs">Start</Label>
                <Input
                  type="datetime-local"
                  value={filters.start_timestamp}
                  onChange={(e) => setFilters({ ...filters, start_timestamp: e.target.value })}
                  className="h-9"
                />
              </div>
              <div>
                <Label className="text-xs">End</Label>
                <Input
                  type="datetime-local"
                  value={filters.end_timestamp}
                  onChange={(e) => setFilters({ ...filters, end_timestamp: e.target.value })}
                  className="h-9"
                />
              </div>
            </div>
            <div className="flex items-end md:justify-end md:col-span-1">
              <Button onClick={handleFilterSubmit} disabled={loading} className="w-full md:w-auto gap-2 px-4">
                <Filter className="h-4 w-4" />
                Apply
              </Button>
            </div>
          </div>

          <EnhancedDataTable
            columns={columns}
            data={data}
            pageIndex={pageIndex}
            pageSize={pageSize}
            total={total}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
            sortBy={sortBy}
            sortOrder={sortOrder}
            onSortChange={handleSortChange}
            onRowClick={handleRowClick}
            onRefresh={refresh}
            loading={loading}
            emptyMessage="No logs found. Adjust your filters or search criteria."
          />
        </CardContent>
      </Card>

      <TracingModal
        open={tracingModalOpen}
        onOpenChange={handleTracingModalClose}
        logId={selectedLogId}
        traceId={selectedTraceId}
      />
    </div>
  )
}
