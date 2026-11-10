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
});
