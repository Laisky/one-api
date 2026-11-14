/**
 * DEFAULT_YIELD_BUDGET_MS is how long chunked work may run before it yields to the main thread.
 * 50ms is the long-task boundary: anything longer delays input handling and hurts INP.
 */
export const DEFAULT_YIELD_BUDGET_MS = 50;

/** YieldingScheduler is the subset of the Prioritized Task Scheduling API that yieldToMain relies on. */
interface YieldingScheduler {
  yield: () => Promise<void>;
}

/** ChunkedWorkOptions tunes how mapInChunks slices its work; the defaults suit production use. */
export interface ChunkedWorkOptions {
  /** budgetMs is the time each slice may run before yielding; it defaults to DEFAULT_YIELD_BUDGET_MS. */
  budgetMs?: number;
  /** yieldFn replaces yieldToMain, which lets tests observe or control each yield point. */
  yieldFn?: () => Promise<void>;
}

/**
 * getYieldingScheduler feature-detects scheduler.yield().
 *
 * @returns the global scheduler when it exposes a callable yield(), otherwise null.
 */
function getYieldingScheduler(): YieldingScheduler | null {
  const candidate = (globalThis as { scheduler?: Partial<YieldingScheduler> }).scheduler;
  if (candidate && typeof candidate.yield === 'function') {
    return candidate as YieldingScheduler;
  }
  return null;
}

/**
 * yieldToMain pauses the current async task so the browser can handle pending input and paint.
 * It prefers scheduler.yield(), whose continuation runs ahead of other queued tasks, and falls back
 * to a zero-delay setTimeout in browsers without the Scheduler API (for example Safari).
 *
 * @returns a promise that resolves once control has returned to the caller.
 */
export function yieldToMain(): Promise<void> {
  const scheduler = getYieldingScheduler();
  if (scheduler) {
    return scheduler.yield();
  }
  return new Promise<void>((resolve) => {
    setTimeout(resolve, 0);
  });
}

/**
 * mapInChunks maps items like Array.prototype.map but yields to the main thread whenever the
 * current slice has run past its time budget, so large inputs never form a single long task.
 *
 * @param items - the input values, processed in order.
 * @param mapper - the synchronous transform applied to each item and its index.
 * @param options - optional budget and yield overrides.
 * @returns a promise resolving to the mapped values in input order.
 */
export async function mapInChunks<T, R>(
  items: readonly T[],
  mapper: (item: T, index: number) => R,
  options: ChunkedWorkOptions = {}
): Promise<R[]> {
  const budgetMs = options.budgetMs ?? DEFAULT_YIELD_BUDGET_MS;
  const yieldFn = options.yieldFn ?? yieldToMain;
  const results = new Array<R>(items.length);
  let deadline = performance.now() + budgetMs;

  for (let index = 0; index < items.length; index += 1) {
    // Check before each item after the first, so no trailing yield follows the final item.
    if (index > 0 && performance.now() >= deadline) {
      await yieldFn();
      deadline = performance.now() + budgetMs;
    }
    results[index] = mapper(items[index], index);
  }

  return results;
}
