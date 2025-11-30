import { useCallback, useEffect, useRef, useState } from "react";

/**
 * ClipboardManagerToken describes the minimal token fields required to manage clipboard feedback.
 */
export interface ClipboardManagerToken {
  id: number;
  key: string;
}

/**
 * ClipboardManagerResult exposes clipboard feedback state and handlers for the token table.
 */
export interface ClipboardManagerResult {
  copiedTokens: Record<number, boolean>;
  manualCopyToken: ClipboardManagerToken | null;
  handleCopySuccess: (tokenId: number) => void;
  handleCopyFailure: (token: ClipboardManagerToken) => void;
  clearManualCopyToken: () => void;
}

/**
 * useClipboardManager centralizes clipboard success animations and manual fallback handling.
 */
export function useClipboardManager(): ClipboardManagerResult {
  const [copiedTokens, setCopiedTokens] = useState<Record<number, boolean>>({});
  const [manualCopyToken, setManualCopyToken] =
    useState<ClipboardManagerToken | null>(null);
  const resetTimersRef = useRef<Record<number, ReturnType<typeof setTimeout>>>(
    {}
  );

  const handleCopySuccess = useCallback((tokenId: number) => {
    setCopiedTokens((prev) => ({
      ...prev,
      [tokenId]: true,
    }));

    if (resetTimersRef.current[tokenId]) {
      clearTimeout(resetTimersRef.current[tokenId]);
    }

    resetTimersRef.current[tokenId] = setTimeout(() => {
      setCopiedTokens((prevState) => {
        const nextState = { ...prevState };
        delete nextState[tokenId];
        return nextState;
      });
      delete resetTimersRef.current[tokenId];
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
