import { useCallback, useEffect, useRef, useState } from 'react';

import { api } from '@/lib/api';

const STORAGE_KEY = 'notice_seen_content';

/**
 * useNotice fetches `/api/notice` once on mount and exposes the new notice
 * content if it differs from the most recently dismissed value persisted in
 * localStorage. Calling `dismiss` records the current content so it will not
 * be returned on subsequent mounts until the server publishes a different
 * notice.
 */
export interface UseNoticeResult {
  notice: { content: string; dismiss: () => void } | null;
}

export function useNotice(): UseNoticeResult {
  const [content, setContent] = useState<string | null>(null);
  // Track the latest fetched content so dismiss can store the exact string.
  const contentRef = useRef<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const fetchNotice = async () => {
      try {
        const res = await api.get('/api/notice');
        const { success, data } = res.data || {};
        if (cancelled) return;
        if (success && typeof data === 'string') {
          const trimmed = data.trim();
          const seen = (typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null) ?? '';
          if (trimmed && trimmed !== seen) {
            contentRef.current = data;
            setContent(data);
          }
        }
      } catch (err) {
        // Notice is non-critical; swallow errors to avoid spamming users.
        console.error('Error loading notice:', err);
      }
    };

    fetchNotice();

    return () => {
      cancelled = true;
    };
  }, []);

  const dismiss = useCallback(() => {
    const value = contentRef.current;
    if (typeof window !== 'undefined' && value !== null) {
      try {
        window.localStorage.setItem(STORAGE_KEY, value);
      } catch (err) {
        console.error('Error persisting dismissed notice:', err);
      }
    }
    setContent(null);
  }, []);

  if (content === null) {
    return { notice: null };
  }

  return { notice: { content, dismiss } };
}

export default useNotice;
