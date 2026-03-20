import { ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment';
import { useNotifications } from '@/components/ui/notifications';
import { Message } from '@/lib/utils';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

interface UsePlaygroundActionsProps {
  messages: Message[];
  setMessages: (messages: Message[]) => void;
  setCurrentMessage: (message: string) => void;
  sendMessage: (message: string, images?: ImageAttachmentType[]) => Promise<void>;
  regenerateMessage: (messages: Message[]) => Promise<void>;
  isStreaming: boolean;
  setExpandedReasonings: (expanded: Record<number, boolean> | ((prev: Record<number, boolean>) => Record<number, boolean>)) => void;
  setExportDialogOpen: (open: boolean) => void;
}

export const usePlaygroundActions = ({
  messages,
  setMessages,
  setCurrentMessage,
  sendMessage,
  regenerateMessage,
  isStreaming,
  setExpandedReasonings,
  setExportDialogOpen,
}: UsePlaygroundActionsProps) => {
  const { t } = useTranslation();
  const { notify } = useNotifications();

  const exportConversation = useCallback(() => {
    setExportDialogOpen(true);
  }, [setExportDialogOpen]);

  const toggleReasoning = useCallback(
    (messageIndex: number) => {
      setExpandedReasonings((prev) => ({
        ...prev,
        [messageIndex]: !prev[messageIndex],
      }));
    },
    [setExpandedReasonings]
  );

  const handleCurrentMessageChange = useCallback(
    (value: string) => {
      setCurrentMessage(value);
    },
    [setCurrentMessage]
  );

  const handleSendMessage = useCallback(
    async (message: string, images?: ImageAttachmentType[]) => {
      if (message.trim() || (images && images.length > 0)) {
        setCurrentMessage('');
        await sendMessage(message, images);
      }
    },
    [sendMessage, setCurrentMessage]
  );

  const handleCopyMessage = useCallback(
    async (messageIndex: number, content: string) => {
      try {
        await navigator.clipboard.writeText(content);
        notify({
          title: t('playground.notifications.copied_title'),
          message: t('playground.notifications.copied_message'),
          type: 'success',
        });
      } catch (error) {
        notify({
          title: t('playground.notifications.copy_failed_title'),
          message: t('playground.notifications.copy_failed_message'),
          type: 'error',
        });
      }
    },
    [notify, t]
  );

  const handleRegenerateMessage = useCallback(
    async (messageIndex: number) => {
      if (messageIndex < 1 || isStreaming) return;

      const targetMessage = messages[messageIndex];
      if (targetMessage.role !== 'assistant') return;

      let userMessageIndex = -1;
      for (let i = messageIndex - 1; i >= 0; i--) {
        if (messages[i].role === 'user') {
          userMessageIndex = i;
          break;
        }
      }

      if (userMessageIndex === -1) return;

      const newMessages = messages.slice(0, userMessageIndex + 1);
      setMessages(newMessages);

      await regenerateMessage(newMessages);
    },
    [messages, isStreaming, regenerateMessage, setMessages]
  );

  const handleEditMessage = useCallback(
    (messageIndex: number, newContent: string | any[]) => {
      const updatedMessages = [...messages];
      updatedMessages[messageIndex] = {
        ...updatedMessages[messageIndex],
        content: newContent,
        timestamp: Date.now(),
      };
      setMessages(updatedMessages);

      notify({
        title: t('playground.notifications.message_edited_title'),
        message: t('playground.notifications.message_edited_message'),
        type: 'success',
      });
    },
    [messages, setMessages, notify, t]
  );

  const handleDeleteMessage = useCallback(
    (messageIndex: number) => {
      const updatedMessages = messages.filter((_, index) => index !== messageIndex);
      setMessages(updatedMessages);

      notify({
        title: t('playground.notifications.message_deleted_title'),
        message: t('playground.notifications.message_deleted_message'),
        type: 'success',
      });
    },
    [messages, setMessages, notify, t]
  );

  return {
    exportConversation,
    toggleReasoning,
    handleCurrentMessageChange,
    handleSendMessage,
    handleCopyMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleDeleteMessage,
  };
};
