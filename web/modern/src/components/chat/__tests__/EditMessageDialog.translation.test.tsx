import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import { EditMessageDialog } from '../EditMessageDialog';

/** getStatistic returns the complete statistic label even when its count and suffix use separate text elements. */
function getStatistic(label: string): HTMLElement {
  return screen.getByText((_value, element) => element?.tagName === 'SPAN' && element.textContent === label);
}

describe('EditMessageDialog with browser-translated statistics', () => {
  it.each([
    { label: 'character', initial: '5 chars' },
    { label: 'word', initial: '1 words' },
  ])('refreshes the translated $label count while preserving saved text and image attachments', async ({ initial }) => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    const onClose = vi.fn();
    const onSave = vi.fn();
    const attachment = { type: 'image_url', image_url: { url: 'https://example.test/attachment.png' } };
    const originalContent = [{ type: 'text', text: 'Hello' }, attachment];
    const { unmount } = render(
      <DomMutationBoundary>
        <EditMessageDialog
          isOpen
          onClose={onClose}
          onSave={onSave}
          currentContent="Hello"
          originalContent={originalContent}
          messageRole="user"
        />
      </DomMutationBoundary>,
      { onCaughtError }
    );

    const chars = getStatistic('5 chars');
    const words = getStatistic('1 words');
    expect(translateText(getStatistic(initial))).toBe(2);
    const editor = screen.getByRole('textbox', { name: 'Enter User message content...' });
    await user.clear(editor);
    await user.type(editor, 'Hello world');

    expect(chars).toHaveTextContent(/^11\s/);
    expect(words).toHaveTextContent(/^2\s/);
    expect(editor).toHaveValue('Hello world');
    expect(onCaughtError).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Save Changes' }));

    expect(onSave).toHaveBeenCalledExactlyOnceWith([{ type: 'text', text: 'Hello world' }, attachment]);
    expect(onClose).toHaveBeenCalledOnce();
    expect(originalContent).toEqual([{ type: 'text', text: 'Hello' }, attachment]);
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });
});
