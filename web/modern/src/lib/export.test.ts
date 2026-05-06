import { describe, expect, it, vi } from 'vitest';

import { buildCsv, fetchAllPaginatedResults, mapWithConcurrency } from './export';

describe('fetchAllPaginatedResults', () => {
  it('aggregates every page until the reported total is reached', async () => {
    const requestPage = vi
      .fn()
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ id: 1 }, { id: 2 }],
          total: 5,
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ id: 3 }, { id: 4 }],
          total: 5,
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [{ id: 5 }],
          total: 5,
        },
      });

    const rows = await fetchAllPaginatedResults(requestPage, '/api/log/', new URLSearchParams([['type', '2']]), 2);

    expect(rows).toEqual([{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }, { id: 5 }]);
    expect(requestPage).toHaveBeenCalledTimes(3);
    expect(requestPage).toHaveBeenNthCalledWith(1, '/api/log/?type=2&p=0&size=2');
    expect(requestPage).toHaveBeenNthCalledWith(2, '/api/log/?type=2&p=1&size=2');
    expect(requestPage).toHaveBeenNthCalledWith(3, '/api/log/?type=2&p=2&size=2');
  });

  it('surfaces backend export failures', async () => {
    const requestPage = vi.fn().mockResolvedValue({
      data: {
        success: false,
        message: 'export failed',
      },
    });

    await expect(fetchAllPaginatedResults(requestPage, '/api/log/self', new URLSearchParams())).rejects.toThrow('export failed');
  });

  it('fetches remaining pages in bounded parallel batches after the first page', async () => {
    let resolvePageOne: ((value: { data: { success: boolean; data: Array<{ id: number }>; total: number } }) => void) | undefined;
    let resolvePageTwo: ((value: { data: { success: boolean; data: Array<{ id: number }>; total: number } }) => void) | undefined;

    const requestPage = vi.fn().mockImplementation((url: string) => {
      if (url.includes('p=0')) {
        return Promise.resolve({
          data: {
            success: true,
            data: [{ id: 1 }, { id: 2 }],
            total: 6,
          },
        });
      }

      if (url.includes('p=1')) {
        return new Promise((resolve) => {
          resolvePageOne = resolve;
        });
      }

      if (url.includes('p=2')) {
        return new Promise((resolve) => {
          resolvePageTwo = resolve;
        });
      }

      return Promise.reject(new Error(`unexpected url: ${url}`));
    });

    const rowsPromise = fetchAllPaginatedResults(requestPage, '/api/log/', new URLSearchParams([['type', '2']]), 2, 2);

    await vi.waitFor(() => {
      expect(requestPage).toHaveBeenCalledTimes(3);
    });
    expect(requestPage).toHaveBeenNthCalledWith(1, '/api/log/?type=2&p=0&size=2');
    expect(requestPage).toHaveBeenNthCalledWith(2, '/api/log/?type=2&p=1&size=2');
    expect(requestPage).toHaveBeenNthCalledWith(3, '/api/log/?type=2&p=2&size=2');

    resolvePageOne?.({
      data: {
        success: true,
        data: [{ id: 3 }, { id: 4 }],
        total: 6,
      },
    });
    resolvePageTwo?.({
      data: {
        success: true,
        data: [{ id: 5 }, { id: 6 }],
        total: 6,
      },
    });

    await expect(rowsPromise).resolves.toEqual([{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }, { id: 5 }, { id: 6 }]);
  });

  it('quotes CSV fields so modal metadata and trace payloads remain intact', () => {
    const csv = buildCsv([
      ['Title', 'Metadata', 'Tracing'],
      ['alpha,beta', { value: 'hello', lines: ['first', 'second'] }, 'line 1\nline 2'],
    ]);

    expect(csv).toBe(
      '"Title","Metadata","Tracing"\n"alpha,beta","{""value"":""hello"",""lines"":[""first"",""second""]}","line 1\nline 2"'
    );
  });
});

describe('mapWithConcurrency', () => {
  it('preserves input order while limiting active tasks', async () => {
    let activeTasks = 0;
    let maxActiveTasks = 0;

    const results = await mapWithConcurrency(
      [1, 2, 3, 4, 5],
      async (value) => {
        activeTasks += 1;
        maxActiveTasks = Math.max(maxActiveTasks, activeTasks);
        await Promise.resolve();
        activeTasks -= 1;
        return value * 10;
      },
      2
    );

    expect(results).toEqual([10, 20, 30, 40, 50]);
    expect(maxActiveTasks).toBeLessThanOrEqual(2);
  });
});
