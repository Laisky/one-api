import { fireEvent, render, screen, within } from '@testing-library/react';
import type { ComponentType, ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { translateText } from '@/test/dom-translation';
import { UsageCharts } from './UsageCharts';
import { ToolUsageCharts } from './ToolUsageCharts';

vi.mock('recharts', async () => {
  const { useState } = await import('react');
  /** Container renders chart children while omitting jsdom's unavailable chart layout. */
  function Container({ children }: { children: ReactNode }) {
    return <div>{children}</div>;
  }
  /** Decoration returns no DOM for axes and series unrelated to the custom tooltip content. */
  function Decoration() {
    return null;
  }
  /** Tooltip preserves Recharts' content component identity while pointer movement updates its active value. */
  function Tooltip({ content: Content }: { content: ComponentType<{ active: boolean; payload: unknown[]; label: string }> }) {
    const [value, setValue] = useState(5);
    return (
      <section aria-label="Chart tooltip" onMouseMove={() => setValue(12)}>
        <Content active payload={[{ name: 'series', value, color: '#000' }]} label="Example day" />
      </section>
    );
  }
  return {
    Bar: Decoration,
    BarChart: Container,
    CartesianGrid: Decoration,
    Legend: Decoration,
    ResponsiveContainer: Container,
    Tooltip,
    XAxis: Decoration,
    YAxis: Decoration,
  };
});

const charts = [
  [
    'usage',
    <UsageCharts
      modelStackedData={[]}
      modelKeys={[]}
      userStackedData={[]}
      userKeys={[]}
      tokenStackedData={[]}
      tokenKeys={[]}
      statisticsMetric="requests"
      setStatisticsMetric={vi.fn()}
    />,
  ],
  [
    'tool usage',
    <ToolUsageCharts
      toolStackedData={[]}
      toolKeys={[]}
      toolUserStackedData={[]}
      toolUserKeys={[]}
      toolTokenStackedData={[]}
      toolTokenKeys={[]}
      toolStatisticsMetric="requests"
      setToolStatisticsMetric={vi.fn()}
    />,
  ],
] as const;

describe('usage chart tooltips with browser-translated values', () => {
  it.each(charts)('updates the reused %s tooltip series value when the pointer moves', (_label, chart) => {
    render(chart);
    const tooltip = screen.getAllByRole('region', { name: 'Chart tooltip' })[0];
    const value = within(tooltip).getByText('series: 5');
    expect(translateText(value)).toBeGreaterThan(0);

    fireEvent.mouseMove(tooltip);

    expect(value).toHaveTextContent('series: 12');
    expect(value).not.toHaveTextContent('series: 5');
  });
});
