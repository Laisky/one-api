import { type ComponentProps } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { ChatInterface } from '../ChatInterface';

vi.mock('@/hooks/useResponsive', () => ({
  /** useResponsive returns mobile breakpoints so the icon-only Send/Stop controls render. */
  useResponsive: () => ({ isMobile: true, isTablet: false }),
  /** useIsTouchDevice returns true for the mobile layout. */
  useIsTouchDevice: () => true,
}));

/** buildProps returns ChatInterface props for a ready conversation with spy callbacks. */
function buildProps(overrides: Partial<ComponentProps<typeof ChatInterface>> = {}): ComponentProps<typeof ChatInterface> {
  return {
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
    ...overrides,
  };
}

describe('ChatInterface accessible names on mobile', () => {
  it('labels the message input and names the icon-only controls', async () => {
    const user = userEvent.setup();
    const props = buildProps();
    const { rerender } = render(<ChatInterface {...props} />);

    expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue('Example prompt');

    await user.click(screen.getByRole('button', { name: 'Model & Parameters' }));
    expect(props.onMobileMenuToggle).toHaveBeenCalledOnce();

    await user.click(screen.getByRole('button', { name: 'Send' }));
    expect(props.onSendMessage).toHaveBeenCalledExactlyOnceWith('Example prompt', []);

    rerender(<ChatInterface {...props} isStreaming />);
    await user.click(screen.getByRole('button', { name: 'Stop' }));
    expect(props.onStopGeneration).toHaveBeenCalledOnce();
  });
});

describe('ChatInterface streaming announcements', () => {
  it('keeps one polite status region mounted and announces generation start and completion', () => {
    const props = buildProps();
    const { rerender } = render(<ChatInterface {...props} />);
    const status = screen.getByRole('status');
    expect(status).toBeEmptyDOMElement();

    rerender(<ChatInterface {...props} isStreaming />);
    expect(screen.getByRole('status')).toBe(status);
    expect(status).toHaveTextContent('Generating...');

    rerender(<ChatInterface {...props} />);
    expect(screen.getByRole('status')).toBe(status);
    expect(status).toHaveTextContent('Response complete');
  });
});
