import { useState } from 'react';
import { useTranslation } from 'react-i18next';

/** ChatStreamingStatusProps carries whether a response is currently streaming. */
interface ChatStreamingStatusProps {
  isStreaming: boolean;
}

/**
 * ChatStreamingStatus is an always-mounted, visually hidden polite status region
 * that announces when a response starts generating and when it completes. The
 * streamed message content itself is deliberately not a live region, so screen
 * readers hear two short updates instead of every token.
 *
 * @param props - Whether a response is currently streaming.
 * @returns The status region element.
 */
export function ChatStreamingStatus({ isStreaming }: ChatStreamingStatusProps) {
  const { t } = useTranslation();
  const [hasStreamed, setHasStreamed] = useState(isStreaming);

  // Adjust state during render: remember that a response has started so its end can be announced.
  if (isStreaming && !hasStreamed) {
    setHasStreamed(true);
  }

  const message = isStreaming ? t('playground.chat.generating') : hasStreamed ? t('playground.chat.response_complete') : '';

  return (
    <div role="status" className="sr-only">
      {message}
    </div>
  );
}
