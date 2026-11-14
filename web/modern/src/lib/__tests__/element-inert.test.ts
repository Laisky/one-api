import { describe, expect, it } from 'vitest';
import { acquireElementInert } from '../element-inert';

describe('acquireElementInert', () => {
  it.each([true, false])('retains ownership through either release order and repeated release: %s', (firstCloses) => {
    const element = document.createElement('div');
    const first = acquireElementInert(element);
    const second = acquireElementInert(element);
    const release = firstCloses ? first : second;
    release();
    release();
    expect(element).toHaveAttribute('inert');
    (firstCloses ? second : first)();
    expect(element).not.toHaveAttribute('inert');
    const next = acquireElementInert(element);
    expect(element).toHaveAttribute('inert');
    next();
    expect(element).not.toHaveAttribute('inert');
  });

  it('restores the exact pre-existing attribute and isolates different elements', () => {
    const first = document.createElement('div');
    const second = document.createElement('div');
    first.setAttribute('inert', 'prior');
    const releaseFirst = acquireElementInert(first);
    const releaseSecond = acquireElementInert(second);
    releaseFirst();
    expect(first).toHaveAttribute('inert', 'prior');
    expect(second).toHaveAttribute('inert');
    releaseSecond();
    expect(second).not.toHaveAttribute('inert');
  });
});
