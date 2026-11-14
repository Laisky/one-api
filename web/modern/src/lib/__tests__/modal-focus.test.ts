import { afterEach, describe, expect, it } from 'vitest';
import { acquireModalFocus } from '../modal-focus';

afterEach(() => {
  document.body.replaceChildren();
});

describe('shared modal focus ownership', () => {
  it.each([true, false])('restores the opener chain through either close order: %s', (lowerFirst) => {
    const opener = document.createElement('button');
    const lower = document.createElement('div');
    const lowerButton = document.createElement('button');
    lower.append(lowerButton);
    const upper = document.createElement('div');
    const upperButton = document.createElement('button');
    upper.append(upperButton);
    document.body.append(opener, lower, upper);
    const first = acquireModalFocus(lower, opener);
    const second = acquireModalFocus(upper, lowerButton);
    upperButton.focus();
    expect(lower).toHaveAttribute('inert');
    if (lowerFirst) {
      first.release();
      first.release();
      first.restore();
      lower.remove();
      expect(document.activeElement).toBe(upperButton);
      second.release();
      second.restore();
    } else {
      second.release();
      second.restore();
      expect(lower).not.toHaveAttribute('inert');
      expect(document.activeElement).toBe(lowerButton);
      first.release();
      first.restore();
    }
    expect(document.activeElement).toBe(opener);
  });

  it('focuses a surviving layer when simultaneous opens share an external opener', () => {
    const opener = document.createElement('button');
    const lower = document.createElement('div');
    const lowerButton = document.createElement('button');
    lower.append(lowerButton);
    const upper = document.createElement('div');
    document.body.append(opener, lower, upper);
    const first = acquireModalFocus(lower, opener);
    const second = acquireModalFocus(upper, opener);
    second.release();
    second.restore();
    expect(document.activeElement).toBe(lowerButton);
    first.release();
    first.restore();
    expect(document.activeElement).toBe(opener);
  });

  it('preserves prior inert state and does not steal focus from a newer layer', () => {
    const first = document.createElement('div');
    const second = document.createElement('div');
    const opener = document.createElement('button');
    document.body.append(opener, first, second);
    first.setAttribute('inert', 'external');
    const old = acquireModalFocus(first, opener);
    old.release();
    const latest = acquireModalFocus(second, opener);
    const target = document.createElement('button');
    second.append(target);
    target.focus();
    old.restore();
    expect(document.activeElement).toBe(target);
    latest.release();
    expect(first).toHaveAttribute('inert', 'external');
  });
});
