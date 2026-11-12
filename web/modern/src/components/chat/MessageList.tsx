import { MessageItem } from '@/components/chat/MessageItem';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Message } from '@/lib/utils';
import { Bot } from 'lucide-react';
import { useCallback, useEffect, useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

/** STICK_TO_BOTTOM_THRESHOLD_PX is how close to the bottom the user must be for streaming to keep following. */
const STICK_TO_BOTTOM_THRESHOLD_PX = 64;

/** RECENT_MESSAGES_RENDERED_EAGERLY is how many trailing messages always render; older ones defer off-screen work. */
const RECENT_MESSAGES_RENDERED_EAGERLY = 3;

interface MessageListProps {
  messages: Message[];
  isStreaming: boolean;
  showReasoningContent: boolean;
  expandedReasonings: Record<number, boolean>;
  onToggleReasoning: (messageIndex: number) => void;
  focusModeEnabled: boolean;
  // Message actions
  onCopyMessage?: (messageIndex: number, content: string) => void;
  onRegenerateMessage?: (messageIndex: number) => void;
  onEditMessage?: (messageIndex: number, newContent: string | any[]) => void;
  onDeleteMessage?: (messageIndex: number) => void;
}

/**
 * MessageList renders the conversation inside a scroll area. A restored or newly
 * loaded conversation opens at its latest message, focus mode follows streamed
 * output only while the user stays near the bottom, and older messages skip
 * off-screen rendering work via the chat-message-deferred class.
 *
 * @param props - Messages, streaming and reasoning state, focus mode flag, and message action callbacks.
 * @returns The scrollable message list.
 */
export function MessageList({
  messages,
  isStreaming,
  showReasoningContent,
  expandedReasonings,
  onToggleReasoning,
  focusModeEnabled,
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
}: MessageListProps) {
  const { t } = useTranslation();
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  // Whether the user was within the threshold of the bottom at their last scroll.
  const pinnedToBottomRef = useRef(true);
  // Whether the current non-empty conversation already received its initial scroll.
  const initialScrollDoneRef = useRef(false);

  /** getViewport returns the Radix scroll viewport element, or null before it mounts. */
  const getViewport = useCallback(
    () => scrollAreaRef.current?.querySelector<HTMLElement>('[data-radix-scroll-area-viewport]') ?? null,
    []
  );

  /** scrollToBottom jumps the viewport to its end instantly, without moving the page. */
  const scrollToBottom = useCallback(() => {
    const viewport = getViewport();
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight;
    }
  }, [getViewport]);

  // Track whether the user is near the bottom so streaming never yanks someone who scrolled up to read.
  useEffect(() => {
    const viewport = getViewport();
    if (!viewport) return undefined;

    const handleScroll = () => {
      const distanceFromBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
      pinnedToBottomRef.current = distanceFromBottom <= STICK_TO_BOTTOM_THRESHOLD_PX;
    };

    viewport.addEventListener('scroll', handleScroll, { passive: true });
    return () => viewport.removeEventListener('scroll', handleScroll);
  }, [getViewport]);

  // Restored conversations arrive after mount; open them at the latest message once, before paint.
  useLayoutEffect(() => {
    if (messages.length === 0) {
      initialScrollDoneRef.current = false;
      pinnedToBottomRef.current = true;
      return;
    }
    if (initialScrollDoneRef.current) return;
    initialScrollDoneRef.current = true;
    pinnedToBottomRef.current = true;
    scrollToBottom();
  }, [messages.length, scrollToBottom]);

  // Focus mode follows streamed output, but only while the user stays near the bottom.
  useLayoutEffect(() => {
    if (!focusModeEnabled || !isStreaming || !pinnedToBottomRef.current) return;
    scrollToBottom();
  }, [messages, focusModeEnabled, isStreaming, scrollToBottom]);

  const deferredBefore = messages.length - RECENT_MESSAGES_RENDERED_EAGERLY;

  return (
    <ScrollArea ref={scrollAreaRef} className="h-full p-4">
      <div className="space-y-4">
        {messages.length === 0 && (
          <div className="text-center text-muted-foreground py-12">
            <Bot className="h-16 w-16 mx-auto mb-6 opacity-50" />
            <p className="text-lg font-medium">{t('playground.chat.empty_state.title')}</p>
            <p className="text-sm mt-2">{t('playground.chat.empty_state.description')}</p>
          </div>
        )}

        {messages.map((message, index) => (
          <div
            key={index}
            className={`${
              message.role === 'user'
                ? 'flex justify-end'
                : message.role === 'error'
                  ? 'space-y-2'
                  : message.role === 'assistant'
                    ? 'space-y-2'
                    : 'flex justify-start'
            }${index < deferredBefore ? ' chat-message-deferred' : ''}`}
          >
            <MessageItem
              message={message}
              messageIndex={index}
              isStreaming={isStreaming}
              isLastMessage={index === messages.length - 1}
              showReasoningContent={showReasoningContent}
              expandedReasonings={expandedReasonings}
              onToggleReasoning={onToggleReasoning}
              onCopyMessage={onCopyMessage}
              onRegenerateMessage={onRegenerateMessage}
              onEditMessage={onEditMessage}
              onDeleteMessage={onDeleteMessage}
            />
          </div>
        ))}
      </div>
    </ScrollArea>
  );
}
