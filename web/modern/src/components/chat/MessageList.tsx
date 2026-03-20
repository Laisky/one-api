import { MessageItem } from '@/components/chat/MessageItem';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Message } from '@/lib/utils';
import { Bot } from 'lucide-react';
import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

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
  onEditMessage?: (messageIndex: number, newContent: string) => void;
  onDeleteMessage?: (messageIndex: number) => void;
}

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
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Scroll to bottom when new messages arrive - use ScrollArea's viewport for proper containment
  const scrollToBottom = useCallback(() => {
    if (scrollAreaRef.current) {
      const viewport = scrollAreaRef.current.querySelector('[data-radix-scroll-area-viewport]');
      if (viewport) {
        // Use requestAnimationFrame to scroll after content is rendered
        requestAnimationFrame(() => {
          viewport.scrollTop = viewport.scrollHeight;
        });
      }
    }
  }, []);

  // Focus mode: Auto-scroll to bottom continuously while streaming
  useEffect(() => {
    if (!focusModeEnabled || !isStreaming) return;
    scrollToBottom();
  }, [messages, focusModeEnabled, isStreaming, scrollToBottom]);

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
            }`}
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
        <div ref={messagesEndRef} />
      </div>
    </ScrollArea>
  );
}
