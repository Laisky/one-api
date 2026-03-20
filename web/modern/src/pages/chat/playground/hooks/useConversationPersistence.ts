import { useCallback, useEffect, useState } from 'react';

import { STORAGE_KEYS } from '@/lib/storage';
import { clearStorage, generateUUIDv4, loadFromStorage, type Message, saveToStorage } from '@/lib/utils';

interface ConversationPersistenceOptions {
  username?: string;
}

interface ConversationPersistenceResult {
  messages: Message[];
  setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
  conversationId: string;
  conversationCreated: number;
  conversationCreatedBy: string;
  clearConversation: () => void;
}

const buildDefaultConversation = (username?: string) => ({
  id: generateUUIDv4(),
  timestamp: Date.now(),
  createdBy: username || 'unknown',
  messages: [] as Message[],
});

export const useConversationPersistence = (options: ConversationPersistenceOptions): ConversationPersistenceResult => {
  const { username } = options;
  const [messages, setMessages] = useState<Message[]>([]);
  const [conversationId, setConversationId] = useState('');
  const [conversationCreated, setConversationCreated] = useState(0);
  const [conversationCreatedBy, setConversationCreatedBy] = useState('');

  useEffect(() => {
    const saved = loadFromStorage(STORAGE_KEYS.CONVERSATION, null);
    if (saved && typeof saved === 'object' && Array.isArray(saved.messages)) {
      setMessages(saved.messages);
      setConversationId(saved.id || generateUUIDv4());
      setConversationCreated(saved.timestamp || Date.now());
      setConversationCreatedBy(saved.createdBy || username || 'unknown');
      return;
    }

    const fresh = buildDefaultConversation(username);
    setMessages(fresh.messages);
    setConversationId(fresh.id);
    setConversationCreated(fresh.timestamp);
    setConversationCreatedBy(fresh.createdBy);
  }, [username]);

  useEffect(() => {
    if (!conversationId) {
      return;
    }

    const payload = {
      id: conversationId,
      timestamp: conversationCreated,
      createdBy: conversationCreatedBy,
      messages,
    };

    saveToStorage(STORAGE_KEYS.CONVERSATION, payload);
  }, [messages, conversationId, conversationCreated, conversationCreatedBy]);

  const clearConversation = useCallback(() => {
    const fresh = buildDefaultConversation(username);
    setMessages(fresh.messages);
    setConversationId(fresh.id);
    setConversationCreated(fresh.timestamp);
    setConversationCreatedBy(fresh.createdBy);
    clearStorage(STORAGE_KEYS.CONVERSATION);
  }, [username]);

  return {
    messages,
    setMessages,
    conversationId,
    conversationCreated,
    conversationCreatedBy,
    clearConversation,
  };
};
