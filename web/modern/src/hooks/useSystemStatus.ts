import { useState, useEffect, useCallback, useRef } from "react";
import { loadSystemStatus, type SystemStatus } from "@/lib/utils";

const getInitialSystemStatus = (): SystemStatus => {
  try {
    const cached = localStorage.getItem("status");
    if (cached) {
      const parsed = JSON.parse(cached);
      return parsed as SystemStatus;
    }
  } catch (error) {
    console.error("Failed to parse system status from storage:", error);
  }
  return {} as SystemStatus;
};

export interface UseSystemStatusResult {
  systemStatus: SystemStatus;
  isSystemStatusLoading: boolean;
  refreshSystemStatus: () => Promise<SystemStatus | null>;
}

export const useSystemStatus = (): UseSystemStatusResult => {
  const initialStatusRef = useRef<SystemStatus | null>(null);
  if (initialStatusRef.current === null) {
    initialStatusRef.current = getInitialSystemStatus();
  }

  const [systemStatus, setSystemStatus] = useState<SystemStatus>(
    initialStatusRef.current
  );
  const [isSystemStatusLoading, setIsSystemStatusLoading] = useState<boolean>(
    () => Object.keys(initialStatusRef.current || {}).length === 0
  );
  const isMountedRef = useRef(true);

  useEffect(() => {
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  const refreshSystemStatus = useCallback(async () => {
    if (!isMountedRef.current) {
      return null;
    }
    setIsSystemStatusLoading(true);
    try {
      const status = await loadSystemStatus();
      if (status && isMountedRef.current) {
        setSystemStatus(status);
      }
      return status;
    } finally {
      if (isMountedRef.current) {
        setIsSystemStatusLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    refreshSystemStatus();
  }, [refreshSystemStatus]);

  return { systemStatus, isSystemStatusLoading, refreshSystemStatus };
};
