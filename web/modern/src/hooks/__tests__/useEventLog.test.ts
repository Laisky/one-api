import { renderHook, act } from '@testing-library/react';
import { describe, it, expect } from 'vitest';

import { useEventLog } from '../useEventLog';

describe('useEventLog', () => {
  it('starts with empty events and expandedEvents', () => {
    const { result } = renderHook(() => useEventLog());
    expect(result.current.events).toEqual([]);
    expect(result.current.expandedEvents).toBeInstanceOf(Set);
    expect(result.current.expandedEvents.size).toBe(0);
  });

  it('appends an event with generated id and timestamp', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.addEvent({ direction: 'out', type: 'request', payload: { foo: 1 }, transport: 'sse' });
    });

    expect(result.current.events).toHaveLength(1);
    const entry = result.current.events[0];
    expect(entry.direction).toBe('out');
    expect(entry.type).toBe('request');
    expect(entry.payload).toEqual({ foo: 1 });
    expect(entry.transport).toBe('sse');
    expect(typeof entry.id).toBe('string');
    expect(entry.id.length).toBeGreaterThan(0);
    expect(entry.timestamp).toBeInstanceOf(Date);
  });

  it('generates unique ids for repeated addEvent calls', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.addEvent({ direction: 'out', type: 'request', payload: { foo: 1 }, transport: 'sse' });
      result.current.addEvent({ direction: 'out', type: 'request', payload: { foo: 1 }, transport: 'sse' });
    });

    expect(result.current.events).toHaveLength(2);
    expect(result.current.events[0].id).not.toBe(result.current.events[1].id);
  });

  it('keeps addEvent reference stable across rerenders', () => {
    const { result, rerender } = renderHook(() => useEventLog());
    const before = result.current.addEvent;
    rerender();
    const after = result.current.addEvent;
    expect(after).toBe(before);
  });

  it('toggleExpand adds an id then removes it on second call', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.toggleExpand('id-1');
    });
    expect(result.current.expandedEvents.has('id-1')).toBe(true);
    expect(result.current.expandedEvents.size).toBe(1);

    act(() => {
      result.current.toggleExpand('id-1');
    });
    expect(result.current.expandedEvents.has('id-1')).toBe(false);
    expect(result.current.expandedEvents.size).toBe(0);
  });

  it('toggleExpand tracks each id independently', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.toggleExpand('id-1');
      result.current.toggleExpand('id-2');
    });
    expect(result.current.expandedEvents.has('id-1')).toBe(true);
    expect(result.current.expandedEvents.has('id-2')).toBe(true);

    act(() => {
      result.current.toggleExpand('id-1');
    });
    expect(result.current.expandedEvents.has('id-1')).toBe(false);
    expect(result.current.expandedEvents.has('id-2')).toBe(true);
  });

  it('clearEvents resets events and expandedEvents', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.addEvent({ direction: 'out', type: 'request', payload: { a: 1 }, transport: 'sse' });
      result.current.addEvent({ direction: 'in', type: 'response', payload: { b: 2 }, transport: 'sse' });
      result.current.toggleExpand('id-a');
      result.current.toggleExpand('id-b');
    });

    expect(result.current.events.length).toBeGreaterThan(0);
    expect(result.current.expandedEvents.size).toBeGreaterThan(0);

    act(() => {
      result.current.clearEvents();
    });

    expect(result.current.events).toEqual([]);
    expect(result.current.expandedEvents.size).toBe(0);
  });

  it('preserves order and retains all events across multiple addEvent calls', () => {
    const { result } = renderHook(() => useEventLog());

    act(() => {
      result.current.addEvent({ direction: 'out', type: 'request', payload: { n: 1 }, transport: 'sse' });
      result.current.addEvent({ direction: 'in', type: 'response', payload: { n: 2 }, transport: 'ws' });
      result.current.addEvent({ direction: 'out', type: 'cancel', payload: { n: 3 }, transport: 'sse' });
    });

    expect(result.current.events).toHaveLength(3);
    expect(result.current.events.map((e) => e.type)).toEqual(['request', 'response', 'cancel']);
    expect(result.current.events.map((e) => (e.payload as { n: number }).n)).toEqual([1, 2, 3]);
  });
});
