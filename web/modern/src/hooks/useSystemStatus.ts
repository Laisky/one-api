import { systemStatusState, subscribeSystemStatusClear } from '@/lib/system-status-cache';
import { useState, useEffect, useCallback, useRef } from 'react';
import { loadSystemStatus, type LoadSystemStatusOptions, type SystemStatus } from '@/lib/utils';

/** getInitialSystemStatus returns the status cached in localStorage, or an empty object when none is usable. */
const getInitialSystemStatus = (): SystemStatus => {
  try {
    const cached = localStorage.getItem('status');
    if (cached) {
      const parsed = JSON.parse(cached);
      return parsed as SystemStatus;
    }
  } catch (error) {
    console.error(`Failed to parse system status from storage: ${error instanceof Error ? error.message : String(error)}`);
  }
  return {} as SystemStatus;
};

/** UseSystemStatusResult is the public surface of useSystemStatus. */
export interface UseSystemStatusResult {
  /** systemStatus is the latest known status, seeded from localStorage on first render. */
  systemStatus: SystemStatus;
  /** isSystemStatusLoading is true while a load started by this hook is pending. */
  isSystemStatusLoading: boolean;
  /** refreshSystemStatus forces a fresh /api/status request, bypassing the shared short-lived cache. */
  refreshSystemStatus: () => Promise<SystemStatus | null>;
}

/**
 * useSystemStatus exposes the system status to a component. On mount it joins the app-wide shared request
 * (or reuses a response that is a few seconds old), so Header, Footer and the auth pages no longer each
 * trigger their own /api/status call; refreshSystemStatus is the explicit path that always asks the server.
 *
 * @returns the current status, its loading flag, and a forced refresh function.
 */
export const useSystemStatus = (): UseSystemStatusResult => {
  const initialStatusRef = useRef<SystemStatus | null>(null);
  if (initialStatusRef.current === null) {
    initialStatusRef.current = getInitialSystemStatus();
  }

  const [systemStatus, setSystemStatus] = useState<SystemStatus>(initialStatusRef.current);
  const [isSystemStatusLoading, setIsSystemStatusLoading] = useState<boolean>(
    () => Object.keys(initialStatusRef.current || {}).length === 0
  );
  const isMountedRef = useRef(true);
  const loadSequenceRef = useRef(0);

  useEffect(() => {
    // Re-arm on every mount: StrictMode's simulated unmount would otherwise leave the flag false forever.
    isMountedRef.current = true;
    const unsubscribe = subscribeSystemStatusClear(() => {
      loadSequenceRef.current += 1;
      setSystemStatus({});
      setIsSystemStatusLoading(false);
    });
    return () => {
      isMountedRef.current = false;
      loadSequenceRef.current += 1;
      unsubscribe();
    };
  }, []);

  const runLoad = useCallback(async (options: LoadSystemStatusOptions) => {
    if (!isMountedRef.current) {
      return null;
    }
    const sequence = ++loadSequenceRef.current;
    setIsSystemStatusLoading(true);
    const request = loadSystemStatus(options);
    const generation = systemStatusState.generation;
    try {
      const status = await request;
      if (sequence !== loadSequenceRef.current || generation !== systemStatusState.generation) return null;
      if (status && isMountedRef.current) {
        setSystemStatus(status);
      }
      return status;
    } finally {
      if (isMountedRef.current && sequence === loadSequenceRef.current) {
        setIsSystemStatusLoading(false);
      }
    }
  }, []);

  const refreshSystemStatus = useCallback(() => runLoad({ force: true }), [runLoad]);

  useEffect(() => {
    void runLoad({});
  }, [runLoad]);

  return { systemStatus, isSystemStatusLoading, refreshSystemStatus };
};
