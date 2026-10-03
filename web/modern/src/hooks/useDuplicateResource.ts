import { useEffect, useRef, useState } from 'react';

import { useNotifications } from '@/components/ui/notifications';

/** DuplicateResourceOptions supplies the resource-specific request and localized feedback to the shared interaction. */
interface DuplicateResourceOptions {
  create: (ref: string) => Promise<string | undefined>;
  onSuccess: () => void | Promise<void>;
  successMessage: (name?: string) => string;
  failureMessage: string;
  refreshFailureMessage: string;
}

/** getDuplicateErrorMessage extracts a textual API error without logging the request or its credentials. */
export function getDuplicateErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { response?: { data?: { message?: unknown } } } | null)?.response?.data?.message;
  if (typeof message === 'string' && message) return message;
  return error instanceof Error && error.message ? error.message : fallback;
}

/** useDuplicateResource shares per-row locking, notifications, and post-create refresh semantics across resource pages. */
export function useDuplicateResource({
  create,
  onSuccess,
  successMessage,
  failureMessage,
  refreshFailureMessage,
}: DuplicateResourceOptions) {
  const { notify } = useNotifications();
  const onSuccessRef = useRef(onSuccess);
  useEffect(() => {
    onSuccessRef.current = onSuccess;
  }, [onSuccess]);
  const inFlight = useRef(new Set<string>());
  const [pending, setPending] = useState<Set<string>>(new Set());

  /** duplicate creates one copy of the supplied reference, suppressing re-entry until the current list has refreshed. */
  const duplicate = async (ref: string | number): Promise<void> => {
    const key = String(ref);
    if (!key || inFlight.current.has(key)) return;
    // A ref, rather than state alone, also protects clicks in the same React render.
    inFlight.current.add(key);
    setPending(new Set(inFlight.current));
    try {
      let name: string | undefined;
      try {
        name = await create(key);
      } catch (error) {
        notify({ type: 'error', message: getDuplicateErrorMessage(error, failureMessage) });
        return;
      }
      notify({ type: 'success', message: successMessage(name) });
      // Never label a known successful mutation as failed or retry the POST on a refresh error.
      try {
        await onSuccessRef.current();
      } catch {
        notify({ type: 'error', message: refreshFailureMessage });
      }
    } finally {
      inFlight.current.delete(key);
      setPending(new Set(inFlight.current));
    }
  };

  return { duplicate, pending };
}
