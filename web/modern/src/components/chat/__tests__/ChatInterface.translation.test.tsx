import { type ComponentProps } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DomMutationBoundary as ChatErrorBoundary, translateText } from '@/test/dom-translation';

import { ChatInterface } from '../ChatInterface';

vi.mock('@/hooks/useResponsive', () => ({
  /** useResponsive returns desktop breakpoints so the real fragment-based Send label renders. */
  useResponsive: () => ({ isMobile: false, isTablet: false }),
  /** useIsTouchDevice returns false for desktop interaction tests. */
  useIsTouchDevice: () => false,
}));

describe('ChatInterface with browser-translated controls', () => {
  it('starts and stops generation when the desktop Send fragment and Stop text have been replaced', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    const props: ComponentProps<typeof ChatInterface> = {
      messages: [],
      onClearConversation: vi.fn(),
      onExportConversation: vi.fn(),
      currentMessage: 'Example prompt',
      onCurrentMessageChange: vi.fn(),
      onSendMessage: vi.fn(),
      isStreaming: false,
      onStopGeneration: vi.fn(),
      selectedModel: 'model-a',
      selectedToken: 'example-token',
      supportsVision: false,
      attachedImages: [],
      onAttachedImagesChange: vi.fn(),
      showPreview: false,
      onPreviewChange: vi.fn(),
      onMobileMenuToggle: vi.fn(),
      showReasoningContent: false,
      expandedReasonings: {},
      onToggleReasoning: vi.fn(),
      focusModeEnabled: false,
      onFocusModeChange: vi.fn(),
    };
    const { rerender, unmount } = render(
      <ChatErrorBoundary>
        <ChatInterface {...props} />
      </ChatErrorBoundary>,
      { onCaughtError }
    );
    const send = screen.getByRole('button', { name: 'Send' });
    expect(translateText(send)).toBe(1);
    await user.click(send);
    expect(props.onSendMessage).toHaveBeenCalledExactlyOnceWith('Example prompt', []);
    expect(props.onAttachedImagesChange).toHaveBeenCalledExactlyOnceWith([]);

    rerender(
      <ChatErrorBoundary>
        <ChatInterface {...props} isStreaming />
      </ChatErrorBoundary>
    );

    expect(onCaughtError).not.toHaveBeenCalled();
    const stop = screen.getByRole('button', { name: 'Stop' });
    expect(translateText(stop)).toBe(1);
    await user.click(stop);
    expect(props.onStopGeneration).toHaveBeenCalledOnce();

    rerender(
      <ChatErrorBoundary>
        <ChatInterface {...props} />
      </ChatErrorBoundary>
    );

    expect(onCaughtError).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: /Stop/ })).not.toBeInTheDocument();
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });
});
