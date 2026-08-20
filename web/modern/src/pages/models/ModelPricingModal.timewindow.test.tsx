import { act, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/dialog', () => {
  const Dialog = ({ children }: { children: ReactNode }) => <>{children}</>;
  const DialogContent = ({ children, className }: { children: ReactNode; className?: string }) => <div className={className}>{children}</div>;
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
