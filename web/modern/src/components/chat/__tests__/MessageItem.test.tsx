import { act, cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ComponentProps } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  copy: vi.fn<(text: string) => Promise<void>>(),
  notify: vi.fn(),
}));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

vi.mock('@/components/ui/notifications', () => ({
  /** useNotifications exposes the notify spy so toast feedback can be asserted. */
  useNotifications: () => ({ notify: mocks.notify, dismiss: vi.fn() }),
}));

import { MessageItem } from '../MessageItem';

/** renderMessage mounts one MessageItem with default props that callers can override. */
function renderMessage(overrides: Partial<ComponentProps<typeof MessageItem>> = {}) {
  const props: ComponentProps<typeof MessageItem> = {
    message: { role: 'user', content: 'Copy me please', timestamp: 0 },
    messageIndex: 0,
    isStreaming: false,
    isLastMessage: true,
    showReasoningContent: false,
    expandedReasonings: {},
    onToggleReasoning: vi.fn(),
    ...overrides,
  };
  return render(<MessageItem {...props} />);
}

/** copyThroughMenu opens the user message menu and activates its copy item. */
async function copyThroughMenu() {
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'User message options' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Copy Message' }));
  await act(async () => {
    await Promise.resolve();
  });
}

describe('MessageItem', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
    mocks.notify.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it('routes the fallback copy through the shared clipboard helper and reports success', async () => {
    mocks.copy.mockResolvedValue(undefined);
    renderMessage();

    await copyThroughMenu();

    expect(mocks.copy).toHaveBeenCalledExactlyOnceWith('Copy me please');
    expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'success' }));
  });

  it('reports a rejected fallback copy as an error toast', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    renderMessage();

    await copyThroughMenu();

    expect(mocks.notify).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ type: 'error', title: 'Copy failed' }));
  });

  it('keeps message actions reachable without hover on touch devices and while focused or open', () => {
    renderMessage();
    const wrapper = screen.getByRole('button', { name: 'User message options' }).parentElement!;

    expect(wrapper.className).toContain('no-touch:opacity-0');
    expect(wrapper.className).toContain('no-touch:group-hover:opacity-100');
    expect(wrapper.className).toContain('no-touch:group-focus-within:opacity-100');
    expect(wrapper.className).toContain('no-touch:has-[[data-state=open]]:opacity-100');
    expect(wrapper.className.split(/\s+/)).not.toContain('opacity-0');
  });

  it('lazy-loads attached images without inline styles', () => {
    renderMessage({
      message: {
        role: 'user',
        content: [
          { type: 'text', text: 'See image' },
          { type: 'image_url', image_url: { url: 'data:image/png;base64,AAAA' } },
        ],
        timestamp: 0,
      },
    });

    const image = screen.getByRole('img', { name: 'Attached image' });
    expect(image).toHaveAttribute('loading', 'lazy');
    expect(image).toHaveAttribute('decoding', 'async');
    expect(image).not.toHaveAttribute('style');
  });

  it('hides the decorative streaming spinner from assistive technology', () => {
    const { container } = renderMessage({
      message: { role: 'assistant', content: '', model: 'model-a', timestamp: 0 },
      isStreaming: true,
      isLastMessage: true,
    });

    const spinner = container.querySelector('svg.animate-spin');
    expect(spinner).not.toBeNull();
    expect(spinner).toHaveAttribute('aria-hidden', 'true');
  });
});
