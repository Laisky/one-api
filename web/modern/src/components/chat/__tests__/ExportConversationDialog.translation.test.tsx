import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DomMutationBoundary as ExportErrorBoundary, translateText } from '@/test/dom-translation';

import type { Message } from '@/lib/utils';
import { ExportConversationDialog } from '../ExportConversationDialog';

const messages: Message[] = [{ role: 'user', content: 'Example message', timestamp: 1_700_000_000_000 }];

/** translatedSummary replaces the export summary's text nodes with translator elements and returns its container. */
function translatedSummary(): HTMLElement {
  const summary = screen.getByText(/Ready to export/).closest('div');
  expect(summary).not.toBeNull();
  expect(translateText(summary!)).toBe(2);
  return summary!;
}

describe('ExportConversationDialog with browser-translated metadata', () => {
  it('removes a selected model after an asynchronous entitlement update without crashing', () => {
    const onCaughtError = vi.fn();
    const onClose = vi.fn();
    const { rerender, unmount } = render(
      <ExportErrorBoundary>
        <ExportConversationDialog isOpen onClose={onClose} messages={messages} selectedModel="model-a" />
      </ExportErrorBoundary>,
      { onCaughtError }
    );
    const summary = translatedSummary();

    // Model discovery can clear a persisted model after the export dialog opens.
    rerender(
      <ExportErrorBoundary>
        <ExportConversationDialog isOpen onClose={onClose} messages={messages} selectedModel="" />
      </ExportErrorBoundary>
    );

    expect(onCaughtError).not.toHaveBeenCalled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(summary).toHaveTextContent('Ready to export 1 message');
    expect(summary).not.toHaveTextContent('model-a');
    expect(onClose).not.toHaveBeenCalled();
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('updates the message count and model instead of leaving translated metadata stale', () => {
    const onClose = vi.fn();
    const { rerender } = render(<ExportConversationDialog isOpen onClose={onClose} messages={messages} selectedModel="model-a" />);
    const summary = translatedSummary();

    rerender(<ExportConversationDialog isOpen onClose={onClose} messages={[...messages, ...messages]} selectedModel="model-b" />);

    expect(summary).toHaveTextContent('Ready to export 2 messages from model-b');
    expect(summary).not.toHaveTextContent('model-a');
    expect(onClose).not.toHaveBeenCalled();
  });
});
