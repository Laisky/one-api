import { fireEvent, render } from '@testing-library/react';
import type { ComponentProps } from 'react';
import { describe, expect, it, vi } from 'vitest';

import type { Message } from '@/lib/utils';

vi.mock('@/components/chat/MessageItem', () => ({
  /** MessageItem renders a lightweight marker so list scrolling can be tested in isolation. */
  MessageItem: ({ message }: { message: Message }) => <div data-testid="message">{String(message.content)}</div>,
}));

import { MessageList } from '../MessageList';

/** buildMessages returns count alternating user/assistant messages, optionally overriding the last content. */
function buildMessages(count: number, lastContent?: string): Message[] {
  return Array.from({ length: count }, (_, index) => ({
    role: index % 2 === 0 ? 'user' : 'assistant',
    content: index === count - 1 && lastContent !== undefined ? lastContent : `message ${index}`,
    timestamp: index,
  }));
}

/** baseProps returns MessageList props with inert callbacks. */
function baseProps(overrides: Partial<ComponentProps<typeof MessageList>> = {}): ComponentProps<typeof MessageList> {
  return {
    messages: [],
    isStreaming: false,
    showReasoningContent: false,
    expandedReasonings: {},
    onToggleReasoning: vi.fn(),
    focusModeEnabled: false,
    ...overrides,
  };
}

/** mockViewportLayout gives the jsdom viewport a fixed layout and a writable scrollTop. */
function mockViewportLayout(viewport: HTMLElement, { scrollHeight, clientHeight, scrollTop }: Record<string, number>) {
  Object.defineProperty(viewport, 'scrollHeight', { configurable: true, value: scrollHeight });
  Object.defineProperty(viewport, 'clientHeight', { configurable: true, value: clientHeight });
  Object.defineProperty(viewport, 'scrollTop', { configurable: true, writable: true, value: scrollTop });
}

/** getViewport returns the Radix scroll viewport rendered by MessageList. */
function getViewport(container: HTMLElement) {
  const viewport = container.querySelector<HTMLElement>('[data-radix-scroll-area-viewport]');
  expect(viewport).not.toBeNull();
  return viewport!;
}

describe('MessageList scrolling', () => {
  it('opens a restored conversation at its latest message once', () => {
    const { container, rerender } = render(<MessageList {...baseProps()} />);
    const viewport = getViewport(container);
    mockViewportLayout(viewport, { scrollHeight: 2000, clientHeight: 400, scrollTop: 0 });

    // Persistence restores messages in an effect after the first render.
    rerender(<MessageList {...baseProps({ messages: buildMessages(5) })} />);
    expect(viewport.scrollTop).toBe(2000);

    // Later changes outside streaming do not move a reader who scrolled up.
    viewport.scrollTop = 100;
    fireEvent.scroll(viewport);
    rerender(<MessageList {...baseProps({ messages: buildMessages(6) })} />);
    expect(viewport.scrollTop).toBe(100);
  });

  it('follows streamed output in focus mode while the reader is near the bottom', () => {
    const messages = buildMessages(4, 'partial');
    const { container, rerender } = render(<MessageList {...baseProps({ messages, focusModeEnabled: true, isStreaming: true })} />);
    const viewport = getViewport(container);
    mockViewportLayout(viewport, { scrollHeight: 2000, clientHeight: 400, scrollTop: 1560 });
    fireEvent.scroll(viewport);

    Object.defineProperty(viewport, 'scrollHeight', { configurable: true, value: 2100 });
    rerender(<MessageList {...baseProps({ messages: buildMessages(4, 'partial and more'), focusModeEnabled: true, isStreaming: true })} />);

    expect(viewport.scrollTop).toBe(2100);
  });

  it('does not yank a reader who scrolled up while focus mode streams', async () => {
    const messages = buildMessages(4, 'partial');
    const { container, rerender } = render(<MessageList {...baseProps({ messages, focusModeEnabled: true, isStreaming: true })} />);
    const viewport = getViewport(container);
    mockViewportLayout(viewport, { scrollHeight: 2000, clientHeight: 400, scrollTop: 500 });
    fireEvent.scroll(viewport);

    rerender(<MessageList {...baseProps({ messages: buildMessages(4, 'partial and more'), focusModeEnabled: true, isStreaming: true })} />);
    // Wait a frame so a deferred (requestAnimationFrame) scroll would also be caught.
    await new Promise((resolve) => requestAnimationFrame(resolve));

    expect(viewport.scrollTop).toBe(500);
  });
});

describe('MessageList deferred rendering', () => {
  it('defers rendering work only for messages older than the last three', () => {
    const { getAllByTestId } = render(<MessageList {...baseProps({ messages: buildMessages(5) })} />);
    const wrappers = getAllByTestId('message').map((item) => item.parentElement!);

    expect(wrappers.map((wrapper) => wrapper.classList.contains('chat-message-deferred'))).toEqual([true, true, false, false, false]);
  });

  it('renders short conversations without deferral', () => {
    const { getAllByTestId } = render(<MessageList {...baseProps({ messages: buildMessages(3) })} />);

    for (const item of getAllByTestId('message')) {
      expect(item.parentElement).not.toHaveClass('chat-message-deferred');
    }
  });
});
