import { afterEach, describe, expect, it, vi } from 'vitest';
import { acquirePageScrollLock } from '@/lib/page-scroll-lock';

afterEach(() => {
  vi.restoreAllMocks();
  document.body.removeAttribute('style');
});

describe('acquirePageScrollLock', () => {
  it('preserves body styles and the visible page position, restoring them on the final release', () => {
    document.body.style.position = 'relative';
    document.body.style.paddingRight = '12px';
    document.body.style.color = 'red';
    vi.spyOn(window, 'scrollY', 'get').mockReturnValueOnce(240).mockReturnValue(0);
    vi.spyOn(window, 'scrollX', 'get').mockReturnValue(0);
    vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(1000);
    vi.spyOn(document.documentElement, 'clientWidth', 'get').mockReturnValue(980);
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
    const first = acquirePageScrollLock();
    const second = acquirePageScrollLock();

    expect(document.body.style.position).toBe('fixed');
    expect(document.body.style.top).toBe('-240px');
    expect(document.body.style.paddingRight).toBe('32px');
    first();
    first(); // Each owner may release only once.
    expect(document.body.style.position).toBe('fixed');
    second();
    expect(document.body.style.position).toBe('relative');
    expect(document.body.style.top).toBe('');
    expect(document.body.style.paddingRight).toBe('12px');
    expect(document.body.style.color).toBe('red');
    expect(scrollTo).toHaveBeenCalledWith(0, 240);
  });

  it('reacquires after a completed lifetime without retaining previous offsets', () => {
    const release = acquirePageScrollLock();
    release();
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(500);
    const releaseAgain = acquirePageScrollLock();
    expect(document.body.style.top).toBe('-500px');
    releaseAgain();
    expect(document.body.style.position).toBe('');
  });
});
