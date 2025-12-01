import { useState, useCallback, useEffect, useRef } from "react";

/**
 * Storage keys for persistent UI settings.
 * All keys are prefixed with 'oneapi_' to avoid conflicts with other applications.
 */
export const STORAGE_KEYS = {
  // Global page size shared across all tables
  PAGE_SIZE: "oneapi_page_size",
} as const;

export type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS];

/**
 * usePersistentState - A hook for managing state that persists to localStorage.
 *
 * This hook provides the same API as useState but automatically saves and restores
 * the value from localStorage. It's useful for UI preferences like page size,
 * sort order, and other settings that should persist across page reloads.
 *
 * @param key - The localStorage key to use for persistence
 * @param defaultValue - The default value if no persisted value exists
 * @returns A tuple of [value, setValue] similar to useState
 */
export function usePersistentState<T>(
  key: string,
  defaultValue: T
): [T, (value: T | ((prev: T) => T)) => void] {
  // Use a ref to track if we've initialized from localStorage
  const initialized = useRef(false);

  // Initialize state from localStorage or use default
  const [value, setValueInternal] = useState<T>(() => {
    // Only access localStorage on the client side
    if (typeof window === "undefined") {
      return defaultValue;
    }

    try {
      const stored = localStorage.getItem(key);
      if (stored !== null) {
        const parsed = JSON.parse(stored);
        initialized.current = true;
        return parsed;
      }
    } catch (error) {
      console.warn(`Failed to parse localStorage key "${key}":`, error);
    }

    return defaultValue;
  });

  // Persist value to localStorage whenever it changes
  useEffect(() => {
    if (typeof window === "undefined") return;

    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch (error) {
      console.warn(`Failed to save to localStorage key "${key}":`, error);
    }
  }, [key, value]);

  // Wrapper to handle both direct values and updater functions
  const setValue = useCallback(
    (newValue: T | ((prev: T) => T)) => {
      setValueInternal((prev) => {
        const resolved =
          typeof newValue === "function"
            ? (newValue as (prev: T) => T)(prev)
            : newValue;
        return resolved;
      });
    },
    [setValueInternal]
  );

  return [value, setValue];
}

/**
 * usePageSize - A specialized hook for managing page size with persistence.
 *
 * This hook handles the common pattern of page size management with validation
 * and fallback values. It ensures the page size is always a valid number
 * within acceptable bounds.
 *
 * @param storageKey - The storage key for this specific page
 * @param defaultSize - The default page size (default: 10)
 * @param validSizes - Array of valid page size options (for validation)
 * @returns A tuple of [pageSize, setPageSize]
 */
export function usePageSize(
  storageKey: string,
  defaultSize: number = 10,
  validSizes: number[] = [10, 20, 30, 50, 100]
): [number, (size: number) => void] {
  const [pageSize, setPageSizeInternal] = usePersistentState<number>(
    storageKey,
    defaultSize
  );

  // Validate and sanitize the page size
  const setPageSize = useCallback(
    (size: number) => {
      // Ensure size is a valid number and within bounds
      const sanitized = Number.isFinite(size) ? Math.max(1, Math.min(size, 1000)) : defaultSize;

      // If we have valid sizes defined, snap to the nearest valid size
      if (validSizes.length > 0) {
        // Check if the size is in valid sizes
        if (validSizes.includes(sanitized)) {
          setPageSizeInternal(sanitized);
        } else {
          // Find the nearest valid size
          const nearest = validSizes.reduce((prev, curr) =>
            Math.abs(curr - sanitized) < Math.abs(prev - sanitized) ? curr : prev
          );
          setPageSizeInternal(nearest);
        }
      } else {
        setPageSizeInternal(sanitized);
      }
    },
    [defaultSize, validSizes, setPageSizeInternal]
  );

  return [pageSize, setPageSize];
}

/**
 * getStoredPageSize - Utility function to read a stored page size synchronously.
 *
 * Useful for initializing page size in components that need the value immediately
 * without using the hook pattern.
 *
 * @param key - The storage key to read
 * @param defaultValue - The default value if not found
 * @returns The stored page size or default value
 */
export function getStoredPageSize(key: string, defaultValue: number = 10): number {
  if (typeof window === "undefined") return defaultValue;

  try {
    const stored = localStorage.getItem(key);
    if (stored !== null) {
      const parsed = Number(JSON.parse(stored));
      if (Number.isFinite(parsed) && parsed > 0) {
        return parsed;
      }
    }
  } catch (error) {
    console.warn(`Failed to read localStorage key "${key}":`, error);
  }

  return defaultValue;
}
