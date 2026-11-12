import { Check, Copy, X } from 'lucide-react';
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { cn, copyToClipboard } from '@/lib/utils';

type NotificationType = 'success' | 'error' | 'info' | 'warning';

/** DEFAULT_NOTIFICATION_DURATION_MS is the auto-dismiss delay for success, info, and warning toasts. */
export const DEFAULT_NOTIFICATION_DURATION_MS = 3000;

/** ERROR_NOTIFICATION_DURATION_MS keeps error toasts on screen long enough to read and act on. */
export const ERROR_NOTIFICATION_DURATION_MS = 8000;

/** COPY_STATUS_RESET_MS is how long the copy confirmation or failure stays visible on a toast. */
const COPY_STATUS_RESET_MS = 2000;

export interface NotificationOptions {
  id?: string;
  title?: string;
  message: string;
  type?: NotificationType;
  /** durationMs defaults to 3000ms, or 8000ms for error toasts. */
  durationMs?: number;
}

interface Notification extends Required<Omit<NotificationOptions, 'durationMs'>> {
  durationMs: number;
  /** revision changes on every notify call so a re-used id restarts its toast and announcement. */
  revision: number;
}

export interface NotificationsContextValue {
  notify: (opts: NotificationOptions) => string;
  dismiss: (id: string) => void;
}

/** DismissTimer tracks a pausable auto-dismiss countdown for one toast. */
interface DismissTimer {
  handle: ReturnType<typeof setTimeout> | null;
  remainingMs: number;
  startedAt: number;
}

/** CopyStatus records the outcome of the most recent copy action for one toast. */
interface CopyStatus {
  id: string;
  state: 'copied' | 'failed';
  key: string;
}

const NotificationsContext = createContext<NotificationsContextValue | null>(null);

/**
 * useNotifications returns the notify and dismiss functions of the nearest NotificationsProvider.
 *
 * @returns The notifications context value.
 * @throws Error when called outside a NotificationsProvider.
 */
export function useNotifications(): NotificationsContextValue {
  const ctx = useContext(NotificationsContext);
  if (!ctx) throw new Error('useNotifications must be used within NotificationsProvider');
  return ctx;
}

/**
 * getDefaultNotificationDuration returns the auto-dismiss delay used when a caller passes no durationMs.
 * Errors stay longer because they usually need the user to read and react to them.
 *
 * @param type - The notification type.
 * @returns The default duration in milliseconds.
 */
function getDefaultNotificationDuration(type: NotificationType): number {
  return type === 'error' ? ERROR_NOTIFICATION_DURATION_MS : DEFAULT_NOTIFICATION_DURATION_MS;
}

/**
 * getNotificationText returns the visible text of a toast, used for both copying and announcements.
 *
 * @param notification - The toast whose title and message are combined.
 * @returns The title and message joined with a colon, or only the message when there is no title.
 */
function getNotificationText(notification: Pick<Notification, 'title' | 'message'>): string {
  return notification.title ? `${notification.title}: ${notification.message}` : notification.message;
}

/** genId returns a random identifier for a toast or an announcement entry. */
function genId() {
  // Use crypto.randomUUID() if available for secure ID generation
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }

  // Fallback to crypto.getRandomValues() for secure random bytes
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const array = new Uint8Array(16);
    crypto.getRandomValues(array);
    return Array.from(array, (byte) => byte.toString(36))
      .join('')
      .slice(0, 11);
  }

  // Final fallback to Math.random() (should rarely be needed in modern browsers)
  return Math.random().toString(36).slice(2);
}

/**
 * NotificationsProvider owns the toast queue and its pausable auto-dismiss timers, and renders the
 * always-present notifications region into document.body so it stays outside inert app content.
 *
 * @param props.children - The application subtree that may call useNotifications.
 * @returns The provider wrapping its children plus the portaled notifications region.
 */
export const NotificationsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [items, setItems] = useState<Notification[]>([]);
  const timers = useRef(new Map<string, DismissTimer>());
  const revisionRef = useRef(0);

  const clearTimer = useCallback((id: string) => {
    const timer = timers.current.get(id);
    if (timer?.handle) clearTimeout(timer.handle);
    timers.current.delete(id);
  }, []);

  const dismiss = useCallback(
    (id: string) => {
      setItems((prev) => prev.filter((n) => n.id !== id));
      clearTimer(id);
    },
    [clearTimer]
  );

  /** pause stops a toast's countdown and remembers the time left; it is a no-op when already paused. */
  const pause = useCallback((id: string) => {
    const timer = timers.current.get(id);
    if (!timer || timer.handle === null) return;
    clearTimeout(timer.handle);
    timer.handle = null;
    timer.remainingMs = Math.max(0, timer.remainingMs - (Date.now() - timer.startedAt));
  }, []);

  /** resume restarts a paused toast's countdown with the time it had left; it is a no-op when running. */
  const resume = useCallback(
    (id: string) => {
      const timer = timers.current.get(id);
      if (!timer || timer.handle !== null) return;
      timer.startedAt = Date.now();
      timer.handle = setTimeout(() => dismiss(id), timer.remainingMs);
    },
    [dismiss]
  );

  const notify = useCallback(
    (opts: NotificationOptions) => {
      const id = opts.id || genId();
      const type = opts.type ?? 'info';
      revisionRef.current += 1;
      const n: Notification = {
        id,
        title: opts.title ?? '',
        message: opts.message,
        type,
        durationMs: opts.durationMs ?? getDefaultNotificationDuration(type),
        revision: revisionRef.current,
      };
      // Re-using an id replaces the existing toast instead of rendering duplicate keys.
      clearTimer(id);
      setItems((prev) => [...prev.filter((item) => item.id !== id), n]);
      timers.current.set(id, {
        handle: setTimeout(() => dismiss(id), n.durationMs),
        remainingMs: n.durationMs,
        startedAt: Date.now(),
      });
      return id;
    },
    [clearTimer, dismiss]
  );

  useEffect(() => {
    const activeTimers = timers.current;
    return () => {
      activeTimers.forEach((timer) => {
        if (timer.handle) clearTimeout(timer.handle);
      });
      activeTimers.clear();
    };
  }, []);

  const value = useMemo(() => ({ notify, dismiss }), [notify, dismiss]);
  const viewport = <NotificationsViewport items={items} onClose={dismiss} onPause={pause} onResume={resume} />;

  return (
    <NotificationsContext.Provider value={value}>
      {children}
      {typeof document === 'undefined' ? viewport : createPortal(viewport, document.body)}
    </NotificationsContext.Provider>
  );
};

/**
 * NotificationsViewport renders the persistent notifications region: two always-present screen reader
 * announcers (polite, and assertive for errors) and the visible toast stack.
 *
 * @param props.items - The toasts to show, oldest first.
 * @param props.onClose - Dismisses a toast by id.
 * @param props.onPause - Pauses a toast's auto-dismiss countdown while it is hovered or focused.
 * @param props.onResume - Resumes a toast's countdown with its remaining time.
 * @returns The notifications region element.
 */
export const NotificationsViewport: React.FC<{
  items: Notification[];
  onClose: (id: string) => void;
  onPause?: (id: string) => void;
  onResume?: (id: string) => void;
}> = ({ items, onClose, onPause, onResume }) => {
  const { t } = useTranslation();
  const [copyStatus, setCopyStatus] = useState<CopyStatus | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!copyStatus) return;
    const handle = setTimeout(() => setCopyStatus(null), COPY_STATUS_RESET_MS);
    return () => clearTimeout(handle);
  }, [copyStatus]);

  /** handleCopy copies a toast's visible text and reports success or failure to every user. */
  const handleCopy = async (event: React.MouseEvent, notification: Notification) => {
    event.preventDefault();
    event.stopPropagation();
    try {
      await copyToClipboard(getNotificationText(notification));
      setCopyStatus({ id: notification.id, state: 'copied', key: genId() });
    } catch (error) {
      console.error(`Failed to copy notification: ${error instanceof Error ? error.message : String(error)}`);
      setCopyStatus({ id: notification.id, state: 'failed', key: genId() });
    }
  };

  /** handleRegionFocus remembers where focus came from so dismissing the last toast can return it there. */
  const handleRegionFocus = (event: React.FocusEvent<HTMLDivElement>) => {
    const from = event.relatedTarget;
    if (from instanceof HTMLElement && !event.currentTarget.contains(from)) {
      returnFocusRef.current = from;
    }
  };

  /** handleClose dismisses a toast and, when it held focus, moves focus to a neighbouring toast or back. */
  const handleClose = (event: React.MouseEvent<HTMLButtonElement>, id: string) => {
    const toast = event.currentTarget.closest<HTMLElement>('[data-notification-id]');
    if (toast && toast.contains(document.activeElement)) {
      const neighbour = (toast.nextElementSibling ?? toast.previousElementSibling) as HTMLElement | null;
      const nextTarget = neighbour?.matches('[data-notification-id]')
        ? neighbour.querySelector<HTMLElement>('[data-notification-dismiss]')
        : null;
      const fallback = returnFocusRef.current?.isConnected ? returnFocusRef.current : null;
      (nextTarget ?? fallback)?.focus({ preventScroll: true });
    }
    onClose(id);
  };

  const copyAnnouncement = copyStatus ? t(copyStatus.state === 'copied' ? 'common.copied' : 'common.copy_failed') : '';

  return (
    <div
      className="fixed right-3 top-3 z-[1000] flex w-[90vw] max-w-sm flex-col gap-2 md:right-6 md:top-6"
      role="region"
      aria-label={t('notifications.region', 'Notifications')}
      onFocus={handleRegionFocus}
    >
      {/* Always-mounted announcers: only their newly inserted text is spoken, and Radix modal dialogs
          keep pre-existing [aria-live] nodes exposed, so toasts raised from dialogs are still heard. */}
      <div className="sr-only" aria-live="polite" data-testid="notifications-announcer-polite">
        {items
          .filter((n) => n.type !== 'error')
          .map((n) => (
            <p key={`${n.id}-${n.revision}`}>{getNotificationText(n)}</p>
          ))}
        {copyStatus && <p key={copyStatus.key}>{copyAnnouncement}</p>}
      </div>
      <div className="sr-only" aria-live="assertive" data-testid="notifications-announcer-assertive">
        {items
          .filter((n) => n.type === 'error')
          .map((n) => (
            <p key={`${n.id}-${n.revision}`}>{getNotificationText(n)}</p>
          ))}
      </div>
      {items.map((n) => (
        <NotificationToast
          key={`${n.id}-${n.revision}`}
          notification={n}
          copyState={copyStatus?.id === n.id ? copyStatus.state : null}
          copyStatusText={copyStatus?.id === n.id ? copyAnnouncement : ''}
          onCopy={handleCopy}
          onClose={handleClose}
          onPause={onPause}
          onResume={onResume}
        />
      ))}
    </div>
  );
};

/** NotificationToastProps describes one visible toast and the handlers it forwards to the viewport. */
interface NotificationToastProps {
  notification: Notification;
  copyState: CopyStatus['state'] | null;
  copyStatusText: string;
  onCopy: (event: React.MouseEvent, notification: Notification) => void;
  onClose: (event: React.MouseEvent<HTMLButtonElement>, id: string) => void;
  onPause?: (id: string) => void;
  onResume?: (id: string) => void;
}

/**
 * NotificationToast renders one toast whose auto-dismiss pauses while the pointer is over it or focus is
 * inside it, and resumes with the remaining time once both have left.
 *
 * @param props - See NotificationToastProps.
 * @returns The toast element.
 */
function NotificationToast({ notification: n, copyState, copyStatusText, onCopy, onClose, onPause, onResume }: NotificationToastProps) {
  const { t } = useTranslation();
  const hoveredRef = useRef(false);
  const focusedRef = useRef(false);

  /** syncTimer pauses the countdown while the toast is hovered or focused and resumes it otherwise. */
  const syncTimer = () => {
    if (hoveredRef.current || focusedRef.current) onPause?.(n.id);
    else onResume?.(n.id);
  };

  /** setProgressDuration feeds the toast duration to the CSS progress animation without an inline style prop. */
  const setProgressDuration = (element: HTMLDivElement | null) => {
    element?.style.setProperty('--toast-duration', `${n.durationMs}ms`);
  };

  const buttonClass =
    'touch-target inline-flex min-h-6 min-w-6 items-center justify-center rounded p-1 text-current/70 hover:text-current focus-visible:text-current focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-current';

  return (
    <div
      data-notification-id={n.id}
      className={cn(
        'group relative w-full rounded-md border px-4 py-3 shadow-sm transition',
        n.type === 'success' && 'border-success-border bg-success-muted text-success-foreground',
        n.type === 'error' && 'border-destructive/30 bg-destructive/5 text-destructive',
        n.type === 'warning' && 'border-warning-border bg-warning-muted text-warning-foreground',
        n.type === 'info' && 'border-info-border bg-info-muted text-info-foreground'
      )}
      onMouseEnter={() => {
        hoveredRef.current = true;
        syncTimer();
      }}
      onMouseLeave={() => {
        hoveredRef.current = false;
        syncTimer();
      }}
      onFocus={() => {
        focusedRef.current = true;
        syncTimer();
      }}
      onBlur={(event) => {
        if (event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget)) return;
        focusedRef.current = false;
        syncTimer();
      }}
    >
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          {n.title && <div className="font-medium leading-5">{n.title}</div>}
          <div className="text-sm leading-5">{n.message}</div>
          {copyState === 'failed' && <div className="mt-1 text-xs font-medium leading-4">{copyStatusText}</div>}
        </div>
        <div className="flex shrink-0 gap-1">
          <button
            type="button"
            onClick={(e) => onCopy(e, n)}
            className={buttonClass}
            aria-label={t('notifications.copy', 'Copy notification')}
            title={copyState === 'copied' ? copyStatusText : t('notifications.copy_content', 'Copy notification content')}
          >
            {copyState === 'copied' ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : <Copy className="h-3.5 w-3.5" aria-hidden="true" />}
          </button>
          <button
            type="button"
            data-notification-dismiss=""
            onClick={(e) => onClose(e, n.id)}
            className={buttonClass}
            aria-label={t('notifications.dismiss', 'Dismiss')}
            title={t('notifications.dismiss_content', 'Dismiss notification')}
          >
            <X className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
      </div>
      <div className="absolute inset-x-0 -bottom-[1px] motion-reduce:hidden" aria-hidden="true">
        <div
          ref={setProgressDuration}
          className={cn(
            'h-0.5 w-full origin-left',
            // Longhand animation properties keep the paused play-state from being reset by a shorthand.
            'motion-safe:[animation-name:shrink] [animation-duration:var(--toast-duration)] [animation-timing-function:linear] [animation-fill-mode:forwards]',
            'group-hover:[animation-play-state:paused] group-focus-within:[animation-play-state:paused]',
            n.type === 'success' && 'bg-success',
            n.type === 'error' && 'bg-destructive',
            n.type === 'warning' && 'bg-warning',
            n.type === 'info' && 'bg-info'
          )}
        />
      </div>
    </div>
  );
}
