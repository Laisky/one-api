import React from 'react';
import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import katex from 'katex';
import { MarkdownRenderer } from './markdown';

afterEach(cleanup);

// Dependency regressions exercise the application renderer and KaTeX trust defaults.
describe('Markdown dependency compatibility and renderer trust', () => {
  it('renders inline and display expressions through the application component', () => {
    const { container } = render(<MarkdownRenderer compact={false} content={'Inline $a^2+b^2=c^2$.\n\n$$\n\\frac{1}{2}+\\sqrt{x}\n$$'} />);
    expect(container.querySelectorAll('.katex')).toHaveLength(2);
    expect(container.querySelectorAll('.katex-display')).toHaveLength(1);
    expect(container.querySelectorAll('math')).toHaveLength(2);
    expect(container.querySelector('.prose')).toHaveClass('dark:prose-invert');
    expect(container.querySelector('.katex-error')).toBeNull();
  });

  it('keeps invalid math visible rather than dropping the message', () => {
    const { container } = render(<MarkdownRenderer compact={false} content={'Invalid $\\unknowncommand{x}$'} />);
    expect(container.querySelector('.katex')).not.toBeNull();
    expect(container.textContent).toContain('unknowncommand');
  });

  it('preserves explicit untrusted rendering and supported trusted links', () => {
    const expression = String.raw`\href{javascript:alert(1)}{x}`;
    expect(katex.renderToString(expression, { trust: false, throwOnError: false })).not.toContain('href=');
    expect(katex.renderToString(String.raw`\href{https://example.test}{x}`, { trust: true })).toContain('href="https://example.test"');
  });

  it('ignores inherited trust and restores the original prototype after the regression', () => {
    const descriptor = Object.getOwnPropertyDescriptor(Object.prototype, 'trust');
    try {
      Object.defineProperty(Object.prototype, 'trust', { value: true, configurable: true, writable: true });
      const output = katex.renderToString(String.raw`\href{javascript:alert(1)}{x}`, { throwOnError: false });
      expect(output).not.toContain('href=');
      expect(output).toContain('class="katex"');
    } finally {
      if (descriptor) Object.defineProperty(Object.prototype, 'trust', descriptor);
      else Reflect.deleteProperty(Object.prototype, 'trust');
    }
  });
});
