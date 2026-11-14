import { afterEach, describe, expect, it, vi } from 'vitest';

import { mapInChunks, yieldToMain } from '../yield';

type SchedulerHost = { scheduler?: unknown };

const host = globalThis as SchedulerHost;

describe('yieldToMain', () => {
  afterEach(() => {
    delete host.scheduler;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('uses scheduler.yield when the Scheduler API is available', async () => {
    const schedulerYield = vi.fn().mockResolvedValue(undefined);
    host.scheduler = { yield: schedulerYield };
    const timeoutSpy = vi.spyOn(globalThis, 'setTimeout');

    await yieldToMain();

    expect(schedulerYield).toHaveBeenCalledTimes(1);
    expect(timeoutSpy).not.toHaveBeenCalled();
  });

  it('falls back to a zero-delay setTimeout when scheduler is missing', async () => {
    expect(host.scheduler).toBeUndefined();
    vi.useFakeTimers();

    let resolved = false;
    const pending = yieldToMain().then(() => {
      resolved = true;
    });

    await Promise.resolve();
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(0);
    await pending;
    expect(resolved).toBe(true);
  });

  it('falls back to setTimeout when scheduler exists without a yield method', async () => {
    host.scheduler = { postTask: vi.fn() };
    const timeoutSpy = vi.spyOn(globalThis, 'setTimeout');

    await yieldToMain();

    expect(timeoutSpy).toHaveBeenCalledWith(expect.any(Function), 0);
  });
});

describe('mapInChunks', () => {
  it('maps in order without yielding while inside the time budget', async () => {
    const yieldFn = vi.fn().mockResolvedValue(undefined);

    const result = await mapInChunks([1, 2, 3], (value, index) => `${index}:${value * 2}`, { budgetMs: 60_000, yieldFn });

    expect(result).toEqual(['0:2', '1:4', '2:6']);
    expect(yieldFn).not.toHaveBeenCalled();
  });

  it('yields between items once the budget is exhausted, never after the final item', async () => {
    const order: string[] = [];
    const yieldFn = vi.fn(async () => {
      order.push('yield');
    });

    const result = await mapInChunks(
      ['a', 'b', 'c'],
      (value) => {
        order.push(value);
        return value.toUpperCase();
      },
      { budgetMs: 0, yieldFn }
    );

    expect(result).toEqual(['A', 'B', 'C']);
    expect(order).toEqual(['a', 'yield', 'b', 'yield', 'c']);
  });

  it('returns an empty array for empty input', async () => {
    const yieldFn = vi.fn().mockResolvedValue(undefined);

    await expect(mapInChunks([], (value) => value, { budgetMs: 0, yieldFn })).resolves.toEqual([]);
    expect(yieldFn).not.toHaveBeenCalled();
  });
});
