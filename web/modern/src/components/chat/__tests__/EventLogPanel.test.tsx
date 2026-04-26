import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import type { EventLogEntry } from '@/types/realtime';
import { EventLogPanel } from '../EventLogPanel';

const buildEvent = (overrides: Partial<EventLogEntry> = {}): EventLogEntry => ({
  id: overrides.id ?? 'evt-1',
  direction: overrides.direction ?? 'out',
  type: overrides.type ?? 'response.output_text.delta',
  payload: overrides.payload ?? { hello: 'world' },
  timestamp: overrides.timestamp ?? new Date('2024-01-01T00:00:00Z'),
  transport: overrides.transport ?? 'sse',
});

describe('EventLogPanel', () => {
  it('renders empty state text when no events are present', () => {
    render(
      <EventLogPanel
        events={[]}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.getByText('Events')).toBeInTheDocument();
  });

  it('renders the count badge reflecting events length', () => {
    const events = [
      buildEvent({ id: 'a' }),
      buildEvent({ id: 'b' }),
      buildEvent({ id: 'c' }),
    ];

    render(
      <EventLogPanel
        events={events}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('shows direction labels for outgoing and incoming events', () => {
    const events = [
      buildEvent({ id: 'out-evt', direction: 'out' }),
      buildEvent({ id: 'in-evt', direction: 'in' }),
    ];

    render(
      <EventLogPanel
        events={events}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.getByText('Sent')).toBeInTheDocument();
    expect(screen.getByText('Received')).toBeInTheDocument();
  });

  it('renders uppercase transport pills for ws and sse', () => {
    const events = [
      buildEvent({ id: 'ws-evt', transport: 'ws' }),
      buildEvent({ id: 'sse-evt', transport: 'sse' }),
    ];

    render(
      <EventLogPanel
        events={events}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.getByText('WS')).toBeInTheDocument();
    expect(screen.getByText('SSE')).toBeInTheDocument();
  });

  it('renders the event type label as-is', () => {
    render(
      <EventLogPanel
        events={[buildEvent({ id: 'evt', type: 'response.output_text.delta' })]}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.getByText('response.output_text.delta')).toBeInTheDocument();
  });

  it('renders expanded payload JSON when the event id is in expandedEvents', () => {
    render(
      <EventLogPanel
        events={[buildEvent({ id: 'abc', payload: { hello: 'world' } })]}
        expandedEvents={new Set(['abc'])}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    const pre = document.querySelector('pre');
    expect(pre).not.toBeNull();
    expect(pre!.textContent).toContain('hello');
    expect(pre!.textContent).toContain('world');
  });

  it('calls onToggleExpand with the event id when the row is clicked', () => {
    const onToggleExpand = vi.fn();
    render(
      <EventLogPanel
        events={[buildEvent({ id: 'abc' })]}
        expandedEvents={new Set()}
        onToggleExpand={onToggleExpand}
        onClear={vi.fn()}
      />
    );

    const row = screen.getByText('response.output_text.delta').closest('button');
    expect(row).not.toBeNull();
    fireEvent.click(row!);

    expect(onToggleExpand).toHaveBeenCalledWith('abc');
  });

  it('calls onClear when the Clear button is clicked', () => {
    const onClear = vi.fn();
    render(
      <EventLogPanel
        events={[buildEvent({ id: 'abc' })]}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={onClear}
      />
    );

    const clearButton = screen.getByRole('button', { name: /Clear/i });
    fireEvent.click(clearButton);

    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it('hides the Clear button when there are no events', () => {
    render(
      <EventLogPanel
        events={[]}
        expandedEvents={new Set()}
        onToggleExpand={vi.fn()}
        onClear={vi.fn()}
      />
    );

    expect(screen.queryByRole('button', { name: /Clear/i })).toBeNull();
  });
});
