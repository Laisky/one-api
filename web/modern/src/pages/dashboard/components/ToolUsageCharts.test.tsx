import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ToolUsageCharts } from './ToolUsageCharts';

describe('ToolUsageCharts', () => {
  it('renders the tool dashboard sections', () => {
    render(
      <ToolUsageCharts
        toolStackedData={[
          { date: '2026-04-28', web_search: 3, file_read: 2 },
          { date: '2026-04-29', web_search: 1, file_read: 4 },
        ]}
        toolKeys={['web_search', 'file_read']}
        toolUserStackedData={[
          { date: '2026-04-28', alice: 3, bob: 1 },
          { date: '2026-04-29', alice: 2, bob: 2 },
        ]}
        toolUserKeys={['alice', 'bob']}
        toolTokenStackedData={[
          { date: '2026-04-28', 'alpha(alice)': 2, 'beta(bob)': 1 },
          { date: '2026-04-29', 'alpha(alice)': 1, 'beta(bob)': 3 },
        ]}
        toolTokenKeys={['alpha(alice)', 'beta(bob)']}
        toolStatisticsMetric="requests"
        setToolStatisticsMetric={() => {}}
      />
    );

    expect(screen.getByText('Tool Usage')).toBeInTheDocument();
    expect(screen.getByText('Tool Usage by User')).toBeInTheDocument();
    expect(screen.getByText('Tool Usage by Token')).toBeInTheDocument();
    expect(screen.getAllByText('Metric: Requests').length).toBeGreaterThanOrEqual(2);
  });
});
