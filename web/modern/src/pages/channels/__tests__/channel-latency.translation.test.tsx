import { render, screen } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';

import { translateText } from '@/test/dom-translation';
import { createChannelColumns, type Channel } from '../channels-page-columns';

const columns = createChannelColumns({
  t: ((key: string) => key) as TFunction,
  navigate: vi.fn(),
  refreshingBalanceIds: new Set(),
  renderChannelTypeBadge: vi.fn(),
  renderStatusBadge: vi.fn(),
  onPriorityUpdate: vi.fn(),
  onBalanceRefresh: vi.fn(),
  onTestingModelUpdate: vi.fn(),
  onDuplicate: vi.fn(),
  onManage: vi.fn(),
});

/** LatencyCell renders the real response-time column for a channel with the supplied probe result. */
function LatencyCell({ responseTime }: { responseTime: number }) {
  const column = columns.find((entry) => 'accessorKey' in entry && entry.accessorKey === 'response_time');
  if (!column || typeof column.cell !== 'function') throw new Error('Response-time column is unavailable');
  const channel: Channel = { id: 7, name: 'Example channel', type: 1, status: 1, created_time: 0, response_time: responseTime };
  return column.cell({ row: { original: channel } } as Parameters<typeof column.cell>[0]);
}

describe('channel latency with browser-translated text', () => {
  it('refreshes the displayed latency after a subsequent probe updates the same channel', () => {
    const { rerender } = render(<LatencyCell responseTime={120} />);
    const latency = screen.getByText('120ms');
    expect(translateText(latency)).toBeGreaterThan(0);

    rerender(<LatencyCell responseTime={345} />);

    expect(latency).toHaveTextContent('345ms');
    expect(latency).not.toHaveTextContent('120');
    expect(latency).toHaveClass('text-success');
  });
});
