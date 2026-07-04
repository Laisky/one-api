import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * ClipboardManagerToken describes the minimal token fields required to manage clipboard feedback.
 */
export interface ClipboardManagerToken {
  ref: string;
  key: string;
}

/**
 * ClipboardManagerResult exposes clipboard feedback state and handlers for the token table.
 */
export interface ClipboardManagerResult {
  copiedTokens: Record<string, boolean>;
  manualCopyToken: ClipboardManagerToken | null;
  handleCopySuccess: (tokenRef: string) => void;
  handleCopyFailure: (token: ClipboardManagerToken) => void;
  clearManualCopyToken: () => void;
}

/**
 * useClipboardManager centralizes clipboard success animations and manual fallback handling.
 */
export function useClipboardManager(): ClipboardManagerResult {
  const [copiedTokens, setCopiedTokens] = useState<Record<string, boolean>>({});
  const [manualCopyToken, setManualCopyToken] = useState<ClipboardManagerToken | null>(null);
  const resetTimersRef = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  const handleCopySuccess = useCallback((tokenRef: string) => {
    setCopiedTokens((prev) => ({
      ...prev,
      [tokenRef]: true,
    }));

    if (resetTimersRef.current[tokenRef]) {
      clearTimeout(resetTimersRef.current[tokenRef]);
    }

    resetTimersRef.current[tokenRef] = setTimeout(() => {
      setCopiedTokens((prevState) => {
        const nextState = { ...prevState };
        delete nextState[tokenRef];
        return nextState;
      });
      delete resetTimersRef.current[tokenRef];
    }, 3000);
  }, []);

  const handleCopyFailure = useCallback((token: ClipboardManagerToken) => {
    setManualCopyToken(token);
  }, []);

  const clearManualCopyToken = useCallback(() => {
    setManualCopyToken(null);
  }, []);

  useEffect(() => {
    return () => {
      Object.values(resetTimersRef.current).forEach((timeoutId) => {
        clearTimeout(timeoutId);
      });
    };
  }, []);

  return {
    copiedTokens,
    manualCopyToken,
    handleCopySuccess,
    handleCopyFailure,
    clearManualCopyToken,
  };
}
