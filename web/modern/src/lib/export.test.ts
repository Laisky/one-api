import { describe, expect, it, vi } from 'vitest';

import { fetchAllPaginatedResults } from './export';

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
});
