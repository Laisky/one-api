import { act, fireEvent, render, screen, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/dialog', () => {
  const Dialog = ({ children }: { children: ReactNode }) => <>{children}</>;
  const DialogContent = ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  );
  const DialogHeader = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogTitle = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogDescription = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription };
});

vi.mock('@/components/ui/copy-button', () => ({
  CopyButton: () => <button type="button">copy</button>,
}));

import { ModelPricingModal, type ModelDisplayData } from './ModelPricingModal';

// Mirrors the DeepSeek V4 schedule: off-peak wraps midnight, peak is same-day.
const modelData: ModelDisplayData = {
  input_price: 0.14,
  output_price: 0.28,
  time_windows: [
    {
      name: 'deepseek-offpeak',
      timezone: 'Asia/Shanghai',
      ranges: [
        { start: '18:00', end: '09:00' },
        { start: '12:00', end: '14:00' },
      ],
      overlay: { input_price: 0.22, output_price: 0.66 },
    },
    {
      name: 'deepseek-peak',
      timezone: 'Asia/Shanghai',
      ranges: [
        { start: '09:00', end: '12:00' },
        { start: '14:00', end: '18:00' },
      ],
      overlay: { input_price: 0.44, output_price: 1.32 },
    },
  ],
  // Deliberately stale: the browser clock must win over the server snapshot.
  active_time_window: 'deepseek-peak',
};

function activeWindowLabel(): string | null {
  const card = document.querySelector('[aria-current="true"]');
  return card ? (card.textContent ?? '') : null;
}

function renderModal() {
  return render(<ModelPricingModal open onOpenChange={() => {}} modelName="deepseek-v4-flash" data={modelData} channelName="deepseek" />);
}

describe('ModelPricingModal time-of-day highlight', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('highlights the window matching the browser clock, not the server snapshot', () => {
    // 2026-08-19T23:30Z is 07:30 in Asia/Shanghai: off-peak.
    vi.setSystemTime(new Date('2026-08-19T23:30:00Z'));
    renderModal();

    expect(screen.getByText('Active now')).toBeInTheDocument();
    expect(activeWindowLabel()).toContain('deepseek-offpeak');
    expect(activeWindowLabel()).not.toContain('deepseek-peak');
  });

  it('moves the highlight when wall-clock time crosses a window boundary', () => {
    vi.setSystemTime(new Date('2026-08-20T00:59:30Z'));
    renderModal();
    expect(activeWindowLabel()).toContain('deepseek-offpeak');

    // 2026-08-20T01:00Z is 09:00 in Asia/Shanghai: peak starts.
    act(() => {
      vi.setSystemTime(new Date('2026-08-20T01:00:01Z'));
      vi.advanceTimersByTime(1000);
    });

    expect(activeWindowLabel()).toContain('deepseek-peak');
    expect(activeWindowLabel()).not.toContain('deepseek-offpeak');
  });

  it('drops the highlight when no window is active', () => {
    // Before the schedule takes effect no window matches.
    const scheduled: ModelDisplayData = {
      ...modelData,
      time_windows: modelData.time_windows?.map((window) => ({ ...window, date_from: '2026-08-21' })),
    };
    vi.setSystemTime(new Date('2026-08-19T23:30:00Z'));
    render(<ModelPricingModal open onOpenChange={() => {}} modelName="deepseek-v4-flash" data={scheduled} channelName="deepseek" />);

    expect(screen.queryByText('Active now')).not.toBeInTheDocument();
    expect(activeWindowLabel()).toBeNull();
  });

  it('clears its polling timer on unmount so switching models cannot leak timers', () => {
    vi.setSystemTime(new Date('2026-08-19T23:30:00Z'));
    const idle = vi.getTimerCount();

    const first = renderModal();
    expect(vi.getTimerCount()).toBe(idle + 1);
    first.unmount();
    expect(vi.getTimerCount()).toBe(idle);

    // A second mount starts exactly one timer again.
    const second = renderModal();
    expect(vi.getTimerCount()).toBe(idle + 1);
    second.unmount();
    expect(vi.getTimerCount()).toBe(idle);
  });
});

describe('ModelPricingModal historical pricing', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const allDay = {
    timezone: 'UTC',
    ranges: [{ start: '00:00', end: '00:00' }],
    overlay: { input_price: 0.22, output_price: 0.66 },
  };

  function renderHistory(windows: NonNullable<ModelDisplayData['time_windows']>) {
    return render(
      <ModelPricingModal
        open
        onOpenChange={() => {}}
        modelName="history-fixture"
        data={{ ...modelData, time_windows: windows }}
        channelName="fixture"
      />
    );
  }

  it('collapses only expired valid rules, preserving original indices and all details', () => {
    vi.setSystemTime(new Date('2026-08-20T00:00:00Z'));
    const windows = [
      { ...allDay, date_to: '2026-08-20' },
      { ...allDay },
      { ...allDay, date_to: '2026-08-19' },
      { ...allDay, name: 'upcoming', date_from: '2026-08-21' },
      { ...allDay, name: 'invalid-date', date_to: '2026-02-30' },
      { ...allDay, name: 'unresolved-zone', timezone: 'Not/AZone', date_to: '2026-08-19' },
    ];
    const original = JSON.stringify(windows);
    renderHistory(windows);
    const summary = screen.getByText(/Historical pricing/);
    const history = summary.closest('details')!;
    expect(history).not.toHaveAttribute('open');
    expect(within(history).getByText('Window 1')).toBeInTheDocument();
    expect(within(history).getByText('Window 3')).toBeInTheDocument();
    expect(
      within(history).getByText('Window 1').compareDocumentPosition(within(history).getByText('Window 3')) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
    expect(activeWindowLabel()).toContain('Window 2');
    for (const name of ['Window 2', 'upcoming', 'invalid-date', 'unresolved-zone']) {
      expect(screen.getByText(name).closest('details')).toBeNull();
    }
    fireEvent.click(summary);
    expect(history).toHaveAttribute('open');
    expect(within(history).getAllByText(/2026-08-20|2026-08-19/)).toHaveLength(2);
    expect(JSON.stringify(windows)).toBe(original);
  });

  it('refreshes historical membership at local midnight even when active index stays unchanged', () => {
    vi.setSystemTime(new Date('2026-08-19T15:59:59Z'));
    renderHistory([
      { ...allDay, name: 'always-first' },
      { ...allDay, name: 'expires-at-shanghai-midnight', timezone: 'Asia/Shanghai', date_to: '2026-08-20' },
    ]);
    expect(activeWindowLabel()).toContain('always-first');
    expect(screen.queryByText(/Historical pricing/)).not.toBeInTheDocument();
    act(() => {
      vi.setSystemTime(new Date('2026-08-19T16:00:00Z'));
      vi.advanceTimersByTime(1000);
    });
    expect(activeWindowLabel()).toContain('always-first');
    expect(screen.getByText('expires-at-shanghai-midnight').closest('details')).not.toHaveAttribute('open');
    expect(screen.getByText(/Historical pricing/)).toBeInTheDocument();
  });

  it('switches to a shorter model list without rendering stale historical indices or leaking timers', () => {
    vi.setSystemTime(new Date('2026-08-20T00:00:00Z'));
    const idle = vi.getTimerCount();
    const view = renderHistory([
      { ...allDay, name: 'old-current' },
      { ...allDay, name: 'old-upcoming', date_from: '2026-08-21' },
      { ...allDay, name: 'old-history', date_to: '2026-08-20' },
    ]);
    expect(vi.getTimerCount()).toBe(idle + 1);
    view.rerender(
      <ModelPricingModal
        open
        onOpenChange={() => {}}
        modelName="shorter-model"
        data={{ ...modelData, time_windows: [{ ...allDay, name: 'new-current' }] }}
        channelName="fixture"
      />
    );
    expect(screen.queryByText('old-history')).not.toBeInTheDocument();
    expect(screen.queryByText(/Historical pricing/)).not.toBeInTheDocument();
    expect(activeWindowLabel()).toContain('new-current');
    expect(vi.getTimerCount()).toBe(idle + 1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(idle);
  });

  it('reclassifies same-length replacement schedules without changing source priority', () => {
    vi.setSystemTime(new Date('2026-08-20T00:00:00Z'));
    const view = renderHistory([
      { ...allDay, name: 'old-history', date_to: '2026-08-20' },
      { ...allDay, name: 'old-current' },
    ]);
    view.rerender(
      <ModelPricingModal
        open
        onOpenChange={() => {}}
        modelName="replacement"
        data={{
          ...modelData,
          time_windows: [
            { ...allDay, name: 'new-first' },
            { ...allDay, name: 'new-history', date_to: '2026-08-20' },
          ],
        }}
        channelName="fixture"
      />
    );
    expect(screen.getByText('new-first').closest('details')).toBeNull();
    expect(screen.getByText('new-history').closest('details')).not.toHaveAttribute('open');
    expect(activeWindowLabel()).toContain('new-first');
  });

  it('keeps UTC-default and upcoming rules in their normal group until exclusive expiry', () => {
    vi.setSystemTime(new Date('2026-08-19T16:00:00Z'));
    renderHistory([
      { ...allDay, name: 'utc-not-expired', timezone: undefined, date_to: '2026-08-20' },
      { ...allDay, name: 'future', date_from: '2026-08-21' },
    ]);
    expect(screen.queryByText(/Historical pricing/)).not.toBeInTheDocument();
    expect(screen.getByText('utc-not-expired').closest('details')).toBeNull();
    expect(screen.getByText('future').closest('details')).toBeNull();
  });
});
