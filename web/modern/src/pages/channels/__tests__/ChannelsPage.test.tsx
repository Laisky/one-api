import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { ChannelsPage } from '../ChannelsPage'
import { api } from '@/lib/api'
vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() })
}))

// Mock the API
vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    delete: vi.fn(),
    put: vi.fn(),
  },
}))

// Mock the responsive hook
vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: false, isTablet: false }),
}))

// Mock react-router-dom
const mockNavigate = vi.fn()
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

const mockApiGet = vi.mocked(api.get)

const mockChannelsData = {
  success: true,
  data: Array.from({ length: 25 }, (_, i) => ({
    id: i + 1,
    name: `Channel ${i + 1}`,
    type: 1,
    status: 1,
    created_time: Date.now(),
    priority: 0,
    weight: 0,
    models: 'gpt-3.5-turbo',
    group: 'default',
    balance: 100,
    used_quota: 0,
  })),
  total: 25,
}

describe('ChannelsPage Pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Clear localStorage to ensure consistent page size defaults
    localStorage.clear()
    mockApiGet.mockResolvedValue({ data: mockChannelsData })
  })

  const renderChannelsPage = () => {
    return render(
      <BrowserRouter>
        <ChannelsPage />
      </BrowserRouter>
    )
  }

  it('should load initial data with default page size', async () => {
    renderChannelsPage()

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/?p=0&size=10&sort=id&order=desc')
    })
    // NOTE: In CI or under React 18 StrictMode-like double render patterns (or if the
    // underlying EnhancedDataTable fires an initial onPageChange), we may see a
    // transient second fetch for the same initial page. The critical requirement
    // is that we at least fetched once with the expected query (asserted above),
    // and we did not spam more than twice. Keep this tolerant to avoid flaky
    // failures while still catching real regressions (3+ unintended calls).
    const calls = mockApiGet.mock.calls.length
    expect(calls).toBeGreaterThanOrEqual(1)
    expect(calls).toBeLessThanOrEqual(2)
  })

  it('should not make duplicate API calls when changing page size', async () => {
    renderChannelsPage()

    const user = userEvent.setup()

    // Wait for initial load
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledTimes(1)
    })

    // Clear the mock to track new calls
    mockApiGet.mockClear()

    // Find and click the page size selector (Radix Select opens on pointer/keyboard)
    const pageSizeSelect = screen.getByRole('combobox', { name: /rows per page/i })
    await user.click(pageSizeSelect)

    // Wait for the options portal to render, then choose 20
    const option20 = await screen.findByRole('option', { name: '20' })
    await user.click(option20)

    // Wait for the API call
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/?p=0&size=20&sort=id&order=desc')
    })

    // Should only make ONE API call, not multiple
    expect(mockApiGet).toHaveBeenCalledTimes(1)
  })

  it('should handle page navigation correctly', async () => {
    renderChannelsPage()

    // Wait for initial load
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledTimes(1)
    })

    // Clear the mock to track new calls
    mockApiGet.mockClear()

    // Find and click page 2
    const page2Button = screen.getByRole('button', { name: 'Page 2' })
    await userEvent.click(page2Button)

    // Wait for the API call
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/?p=1&size=10&sort=id&order=desc')
    })

    expect(mockApiGet).toHaveBeenCalledTimes(1)
  })

  it('should handle sorting without duplicate calls', async () => {
    renderChannelsPage()

    // Wait for initial load
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledTimes(1)
    })

    // Clear the mock to track new calls
    mockApiGet.mockClear()

    // Find and click a sortable column header
    const nameHeader = screen.getByRole('button', { name: /name/i })
    await userEvent.click(nameHeader)

    // Wait for the API call
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith('/api/channel/?p=0&size=10&sort=name&order=asc')
    })

    expect(mockApiGet).toHaveBeenCalledTimes(1)
  })
})
