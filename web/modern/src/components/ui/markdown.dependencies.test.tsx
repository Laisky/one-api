import React from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import katex from 'katex';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

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

// Compact chat rendering must add soft line breaks to prose without rewriting code or math.
describe('Markdown compact line breaks', () => {
  const content = ['First line', 'second line', '', '```python', 'def greet():', '    return "hi"', '```'].join('\n');

  it('keeps fenced code free of trailing spaces in compact mode', () => {
    const { container } = render(<MarkdownRenderer content={content} />);
    const code = container.querySelector('pre code');
    expect(code).not.toBeNull();
    expect(code!.textContent).toContain('def greet():\n    return "hi"');
    expect(code!.textContent).not.toMatch(/ +\n/);
  });

  it('copies fenced code without trailing spaces', async () => {
    mocks.copy.mockResolvedValue(undefined);
    render(<MarkdownRenderer content={content} />);

    fireEvent.click(screen.getByRole('button', { name: 'Copy to clipboard' }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(mocks.copy).toHaveBeenCalledOnce();
    const copied = mocks.copy.mock.calls[0][0];
    expect(copied).toContain('def greet():\n    return "hi"');
    expect(copied).not.toMatch(/ +\n/);
  });

  it('still turns single newlines in compact paragraphs into line breaks', () => {
    const { container } = render(<MarkdownRenderer content={content} />);
    const paragraph = container.querySelector('p');
    expect(paragraph).not.toBeNull();
    expect(paragraph!.querySelectorAll('br')).toHaveLength(1);
    expect(paragraph!.textContent).toMatch(/^First line\s*second line$/);
  });

  it('keeps single newlines soft when compact mode is disabled', () => {
    const { container } = render(<MarkdownRenderer compact={false} content={'First line\nsecond line'} />);
    expect(container.querySelector('p br')).toBeNull();
  });

  it('keeps display math intact in compact mode', () => {
    const { container } = render(<MarkdownRenderer content={'Before\n\n$$\n\\frac{1}{2}\n$$'} />);
    expect(container.querySelectorAll('.katex-display')).toHaveLength(1);
    expect(container.querySelector('.katex-error')).toBeNull();
  });
});
