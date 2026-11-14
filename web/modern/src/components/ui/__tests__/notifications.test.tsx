import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

import {
  DEFAULT_NOTIFICATION_DURATION_MS,
  ERROR_NOTIFICATION_DURATION_MS,
  NotificationsProvider,
  useNotifications,
  type NotificationsContextValue,
} from '../notifications';

let api: NotificationsContextValue;

/** CaptureNotifications stores the provider's notify/dismiss functions for the test body. */
function CaptureNotifications() {
  api = useNotifications();
  return null;
}

/** renderProvider mounts the notifications provider with a capture probe and returns its API. */
function renderProvider() {
  render(
    <NotificationsProvider>
      <CaptureNotifications />
    </NotificationsProvider>
  );
  return api;
}

/** politeAnnouncer returns the always-mounted polite live region. */
const politeAnnouncer = () => screen.getByTestId('notifications-announcer-polite');

/** assertiveAnnouncer returns the always-mounted assertive live region used for errors. */
const assertiveAnnouncer = () => screen.getByTestId('notifications-announcer-assertive');

/** toastFor returns the visible toast element that shows the given message. */
function toastFor(message: string) {
  const region = screen.getByRole('region', { name: 'Notifications' });
  const match = Array.from(region.querySelectorAll<HTMLElement>('[data-notification-id]')).find((el) =>
    el.textContent?.includes(message)
  );
  expect(match).toBeDefined();
  return match as HTMLElement;
}

/** advance moves fake time forward inside act so timer-driven state updates are committed. */
function advance(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

describe('NotificationsProvider', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mocks.copy.mockReset();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('mounts the region and both announcers before any toast so later messages are announced', () => {
    renderProvider();

    const region = screen.getByRole('region', { name: 'Notifications' });
    expect(region.parentElement).toBe(document.body);
    expect(politeAnnouncer()).toHaveAttribute('aria-live', 'polite');
    expect(assertiveAnnouncer()).toHaveAttribute('aria-live', 'assertive');
    expect(politeAnnouncer()).toBeEmptyDOMElement();
    expect(assertiveAnnouncer()).toBeEmptyDOMElement();
  });

  it('writes only the toast text into the polite announcer and keeps aria-live off the toast', () => {
    renderProvider();

    act(() => {
      api.notify({ title: 'Saved', message: 'Token updated', type: 'success' });
    });

    expect(politeAnnouncer()).toHaveTextContent('Saved: Token updated');
    expect(politeAnnouncer()).not.toHaveTextContent('Dismiss');
    expect(politeAnnouncer()).not.toHaveTextContent('Copy');
    expect(assertiveAnnouncer()).toBeEmptyDOMElement();

    const toast = toastFor('Token updated');
    expect(toast).not.toHaveAttribute('aria-live');
    expect(toast.querySelector('[aria-live]')).toBeNull();
  });

  it('announces errors assertively and keeps them for the longer default duration', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'Request failed', type: 'error' });
    });

    expect(assertiveAnnouncer()).toHaveTextContent('Request failed');
    expect(politeAnnouncer()).toBeEmptyDOMElement();

    advance(DEFAULT_NOTIFICATION_DURATION_MS);
    expect(screen.getAllByText('Request failed').length).toBeGreaterThan(0);

    advance(ERROR_NOTIFICATION_DURATION_MS - DEFAULT_NOTIFICATION_DURATION_MS - 1);
    expect(screen.getAllByText('Request failed').length).toBeGreaterThan(0);

    advance(1);
    expect(screen.queryAllByText('Request failed')).toHaveLength(0);
    expect(assertiveAnnouncer()).toBeEmptyDOMElement();
  });

  it('dismisses non-error toasts after the default duration and honors an explicit duration', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'Copied', type: 'info' });
      api.notify({ message: 'Custom error', type: 'error', durationMs: 1000 });
    });

    advance(999);
    expect(screen.getAllByText('Custom error').length).toBeGreaterThan(0);
    advance(1);
    expect(screen.queryAllByText('Custom error')).toHaveLength(0);

    advance(DEFAULT_NOTIFICATION_DURATION_MS - 1000 - 1);
    expect(screen.getAllByText('Copied').length).toBeGreaterThan(0);
    advance(1);
    expect(screen.queryAllByText('Copied')).toHaveLength(0);
  });

  it('pauses dismissal while hovered and resumes with the remaining time', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'Hover me', type: 'info' });
    });
    const toast = toastFor('Hover me');

    advance(1000);
    fireEvent.mouseEnter(toast);
    advance(10_000);
    expect(screen.getAllByText('Hover me').length).toBeGreaterThan(0);

    fireEvent.mouseLeave(toast);
    advance(DEFAULT_NOTIFICATION_DURATION_MS - 1000 - 1);
    expect(screen.getAllByText('Hover me').length).toBeGreaterThan(0);
    advance(1);
    expect(screen.queryAllByText('Hover me')).toHaveLength(0);
  });

  it('pauses dismissal while focus is inside the toast and resumes once focus leaves it', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'Focus me', type: 'warning' });
    });
    const toast = toastFor('Focus me');
    const dismiss = within(toast).getByRole('button', { name: 'Dismiss' });
    const copy = within(toast).getByRole('button', { name: 'Copy notification' });

    act(() => {
      dismiss.focus();
    });
    advance(10_000);
    expect(screen.getAllByText('Focus me').length).toBeGreaterThan(0);

    // Moving focus between controls inside the same toast keeps it paused.
    act(() => {
      copy.focus();
    });
    advance(10_000);
    expect(screen.getAllByText('Focus me').length).toBeGreaterThan(0);

    act(() => {
      copy.blur();
    });
    advance(DEFAULT_NOTIFICATION_DURATION_MS - 1);
    expect(screen.getAllByText('Focus me').length).toBeGreaterThan(0);
    advance(1);
    expect(screen.queryAllByText('Focus me')).toHaveLength(0);
  });

  it('gives toast buttons at least a 24px target and a keyboard focus style', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'Targets', type: 'info' });
    });

    for (const button of within(toastFor('Targets')).getAllByRole('button')) {
      expect(button).toHaveAttribute('type', 'button');
      expect(button.className).toContain('min-h-6');
      expect(button.className).toContain('min-w-6');
      expect(button.className).toContain('focus-visible:outline-2');
    }
  });

  it('copies the toast text through the shared clipboard helper and announces success', async () => {
    mocks.copy.mockResolvedValue(undefined);
    renderProvider();

    act(() => {
      api.notify({ title: 'Key', message: 'sk-visible', type: 'success' });
    });
    const toast = toastFor('sk-visible');

    await act(async () => {
      fireEvent.click(within(toast).getByRole('button', { name: 'Copy notification' }));
      await Promise.resolve();
    });

    expect(mocks.copy).toHaveBeenCalledWith('Key: sk-visible');
    expect(politeAnnouncer()).toHaveTextContent('Copied!');
    expect(within(toast).queryByText('Copy failed')).toBeNull();
  });

  it('shows and announces a failure when copying is rejected', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderProvider();

    act(() => {
      api.notify({ message: 'Copy me', type: 'info' });
    });
    const toast = toastFor('Copy me');

    await act(async () => {
      fireEvent.click(within(toast).getByRole('button', { name: 'Copy notification' }));
      await Promise.resolve();
    });

    expect(within(toast).getByText('Copy failed')).toBeInTheDocument();
    expect(politeAnnouncer()).toHaveTextContent('Copy failed');
    expect(consoleError).toHaveBeenCalledWith('Failed to copy notification: denied');
  });

  it('replaces a toast that reuses an id instead of rendering a duplicate', () => {
    renderProvider();

    act(() => {
      api.notify({ id: 'sync', message: 'Syncing', type: 'info' });
      api.notify({ id: 'sync', message: 'Synced', type: 'success' });
    });

    expect(document.querySelectorAll('[data-notification-id="sync"]')).toHaveLength(1);
    expect(screen.queryAllByText('Syncing')).toHaveLength(0);
    expect(politeAnnouncer()).toHaveTextContent('Synced');
  });

  it('moves focus to the remaining toast when a focused toast is dismissed', () => {
    renderProvider();

    act(() => {
      api.notify({ message: 'First toast', type: 'info' });
      api.notify({ message: 'Second toast', type: 'info' });
    });
    const first = toastFor('First toast');
    const second = toastFor('Second toast');
    const firstDismiss = within(first).getByRole('button', { name: 'Dismiss' });

    act(() => {
      firstDismiss.focus();
    });
    fireEvent.click(firstDismiss);

    expect(screen.queryAllByText('First toast')).toHaveLength(0);
    expect(document.activeElement).toBe(within(second).getByRole('button', { name: 'Dismiss' }));
  });
});
