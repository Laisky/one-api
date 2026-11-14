import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { LazyMarkdownRenderer } from '../lazy-markdown';

// React.lazy resolves its module once per test file, so the fallback can only be observed on the first
// render in this file; keep the fallback test first.
describe('LazyMarkdownRenderer', () => {
  it('shows the raw source while the renderer loads, then the formatted Markdown', async () => {
    render(<LazyMarkdownRenderer content={'# Title\n\nSome **bold** text'} compact={false} className="custom-class" />);

    const fallback = screen.getByText(/# Title/);
    expect(fallback).toHaveClass('whitespace-pre-wrap');
    expect(fallback).toHaveClass('custom-class');
    expect(fallback.textContent).toBe('# Title\n\nSome **bold** text');

    const heading = await screen.findByRole('heading', { level: 1, name: 'Title' }, { timeout: 10_000 });
    expect(heading).toBeInTheDocument();
    expect(screen.getByText('bold').tagName).toBe('STRONG');
    expect(screen.queryByText(/# Title/)).not.toBeInTheDocument();
  });

  it('forwards className and compact to the real renderer once loaded', async () => {
    const { container } = render(<LazyMarkdownRenderer content={'line one\nline two'} compact={true} className="forwarded" />);

    await screen.findByText(/line one/, { selector: 'p' }, { timeout: 10_000 });
    expect(container.querySelector('.prose.forwarded')).not.toBeNull();
    // compact=true turns single newlines into hard breaks.
    expect(container.querySelector('p br')).not.toBeNull();
  });
});
