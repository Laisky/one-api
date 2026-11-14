import type { SystemStatus } from './utils';

/** systemStatusState owns cached status and request generations without importing the API or auth store. */
export const systemStatusState: {
  cache: { data: SystemStatus; serialized: string; fetchedAt: number } | null;
  request: Promise<SystemStatus | null> | null;
  generation: number;
} = { cache: null, request: null, generation: 0 };

const clearListeners = new Set<() => void>();

/** invalidateSystemStatusCache detaches pending requests and invalidates all earlier status results. */
export const invalidateSystemStatusCache = (): void => {
  systemStatusState.cache = null;
  systemStatusState.request = null;
  systemStatusState.generation += 1;
};

/** subscribeSystemStatusClear registers a status-clear listener and returns its unsubscribe function. */
export const subscribeSystemStatusClear = (listener: () => void): (() => void) => {
  clearListeners.add(listener);
  return () => { clearListeners.delete(listener); };
};

/** clearSystemStatus clears the shared lifecycle and every persisted status field on logout. */
export const clearSystemStatus = (): void => {
  invalidateSystemStatusCache();
  for (const key of ['status', 'system_name', 'chat_link', 'logo', 'footer_html', 'quota_per_unit', 'display_in_currency']) {
    localStorage.removeItem(key);
  }
  for (const listener of clearListeners) listener();
};
