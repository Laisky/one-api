import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import { ThinkingBubble } from '../ThinkingBubble';

describe('ThinkingBubble with browser-translated status text', () => {
  it('updates streaming and completed labels while preserving reasoning content and toggle callbacks', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    const onToggle = vi.fn();
    const content = 'Consider the available options.';
    const { rerender, unmount } = render(
      <DomMutationBoundary>
        <ThinkingBubble content={content} isStreaming isExpanded={false} onToggle={onToggle} />
      </DomMutationBoundary>,
      { onCaughtError }
    );

    const toggle = screen.getByRole('button', { name: /Reasoning/ });
    const streaming = screen.getByText('Reasoning...').closest('span');
    expect(streaming).not.toBeNull();
    expect(translateText(streaming!)).toBe(1);
    await user.click(toggle);
    expect(onToggle).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'click' }));

    rerender(
      <DomMutationBoundary>
        <ThinkingBubble content={content} isStreaming={false} isExpanded onToggle={onToggle} />
      </DomMutationBoundary>
    );

    expect(onCaughtError).not.toHaveBeenCalled();
    expect(toggle).toHaveTextContent('Reasoning Process');
    expect(toggle).not.toHaveTextContent('Reasoning...');
    expect(toggle).toHaveTextContent(`${content.length} chars`);
    expect(screen.getByText(content)).toBeVisible();
    const completed = screen.getByText('Reasoning Process').closest('span');
    expect(completed).not.toBeNull();
    expect(translateText(completed!)).toBe(1);

    rerender(
      <DomMutationBoundary>
        <ThinkingBubble content={content} isStreaming isExpanded={false} onToggle={onToggle} />
      </DomMutationBoundary>
    );

    expect(toggle).toHaveTextContent('Reasoning...');
    expect(toggle).not.toHaveTextContent('Reasoning Process');
    expect(screen.queryByText(content)).not.toBeInTheDocument();
    expect(onToggle).toHaveBeenCalledOnce();
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('exposes the toggle as a disclosure button that controls the reasoning region', () => {
    const content = 'Consider the available options.';
    const { container, rerender } = render(<ThinkingBubble content={content} isStreaming isExpanded={false} onToggle={vi.fn()} />);

    const toggle = screen.getByRole('button', { name: /Reasoning/ });
    expect(toggle).toHaveAttribute('type', 'button');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    const region = document.getElementById(toggle.getAttribute('aria-controls') ?? '');
    expect(region).not.toBeNull();
    expect(region).not.toBeVisible();

    const dots = container.querySelectorAll('.animate-pulse.rounded-full');
    expect(dots).toHaveLength(3);
    expect(dots[0].parentElement).toHaveAttribute('aria-hidden', 'true');
    expect(container.querySelector('[style]')).toBeNull();

    rerender(<ThinkingBubble content={content} isStreaming={false} isExpanded onToggle={vi.fn()} />);

    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(region).toBeVisible();
    expect(region).toHaveTextContent(content);
  });
});
