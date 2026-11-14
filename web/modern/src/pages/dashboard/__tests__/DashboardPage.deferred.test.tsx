import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 'u1', username: 'u1' } }) }));
vi.mock('../hooks/useDashboardData', () => ({
  useDashboardData: () => ({
    isAdmin: false,
    loading: false,
    lastUpdated: null,
    dateError: '',
    rows: [],
    userRows: [],
    tokenRows: [],
    toolRows: [],
    toolUserRows: [],
    toolTokenRows: [],
  }),
}));
vi.mock('../hooks/useDashboardCharts', () => ({
  useDashboardCharts: () => ({
    rangeTotals: {},
    modelLeaders: [],
    rangeInsights: {},
  }),
}));
// The sections themselves are covered by their own tests; stub them to isolate the page layout.
vi.mock('../components/DashboardFilter', () => ({ DashboardFilter: () => <div data-testid="filter" /> }));
vi.mock('../components/OverviewCards', () => ({ OverviewCards: () => <div data-testid="overview-cards" /> }));
vi.mock('../components/TopModels', () => ({ TopModels: () => <div data-testid="top-models" /> }));
vi.mock('../components/Insights', () => ({ Insights: () => <div data-testid="insights" /> }));
vi.mock('../components/TimeSeriesCharts', () => ({ TimeSeriesCharts: () => <div data-testid="time-series" /> }));
vi.mock('../components/UsageCharts', () => ({ UsageCharts: () => <div data-testid="usage-charts" /> }));
vi.mock('../components/ToolUsageCharts', () => ({ ToolUsageCharts: () => <div data-testid="tool-usage-charts" /> }));
vi.mock('../components/CacheHeatmaps', () => ({ CacheHeatmaps: () => <div data-testid="cache-heatmaps" /> }));

import { DashboardPage } from '../DashboardPage';

describe('DashboardPage deferred rendering', () => {
  it('defers only the below-the-fold tool usage and cache heatmap sections', () => {
    render(<DashboardPage />);

    const toolSection = screen.getByTestId('dashboard-tool-usage-section');
    expect(toolSection).toHaveClass('deferred-section');
    expect(toolSection).toContainElement(screen.getByTestId('tool-usage-charts'));

    const cacheSection = screen.getByTestId('dashboard-cache-heatmaps-section');
    expect(cacheSection).toHaveClass('deferred-section');
    expect(cacheSection).toContainElement(screen.getByTestId('cache-heatmaps'));

    // Above-the-fold content must render normally, outside any deferred container.
    for (const id of ['overview-cards', 'top-models', 'insights', 'time-series', 'usage-charts']) {
      expect(screen.getByTestId(id).closest('.deferred-section')).toBeNull();
    }
  });
});
