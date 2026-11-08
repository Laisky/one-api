import { act, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Trans, useTranslation } from 'react-i18next';
import { DomMutationBoundary as TranslationBoundary, translateText as replaceTranslationText } from '@/test/dom-translation';

import i18n, { changeAppLanguage } from '../index';

vi.unmock('react-i18next');

/** EditDescription renders the actual message-edit description with the application's translation configuration. */
function EditDescription() {
  const { t } = useTranslation();
  return (
    <p data-testid="description" aria-label={t('playground.edit.title', { role: 'User' })}>
      <Trans i18nKey="playground.edit.description">
        Make changes to the message content. You can use <kbd>Ctrl+Enter</kbd> to save or <kbd>Esc</kbd> to cancel.
      </Trans>
    </p>
  );
}

/** expectedDescription returns the active locale's message-edit description without its component placeholders. */
function expectedDescription(): string {
  return i18n.t('playground.edit.description').replace(/<\/?\d+>/g, '');
}

beforeEach(async () => {
  await act(async () => {
    await changeAppLanguage('en');
  });
});

describe('real Trans with application locale changes', () => {
  it('updates the mixed description and shortcut elements when the application language changes', async () => {
    const onCaughtError = vi.fn();
    render(
      <TranslationBoundary>
        <EditDescription />
      </TranslationBoundary>,
      { onCaughtError }
    );
    const description = screen.getByTestId('description');
    expect(description.querySelectorAll('kbd')).toHaveLength(2);
    expect(description.textContent === expectedDescription()).toBe(true);

    await act(async () => {
      await changeAppLanguage('fr');
    });

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(description.textContent === expectedDescription()).toBe(true);
    expect(description.querySelectorAll('kbd')).toHaveLength(2);
  });

  it('updates translated mixed text and shortcut labels without keeping detached text stale', async () => {
    const onCaughtError = vi.fn();
    const { unmount } = render(
      <TranslationBoundary>
        <EditDescription />
      </TranslationBoundary>,
      { onCaughtError }
    );
    const description = screen.getByTestId('description');
    expect(replaceTranslationText(description)).toBe(5);

    await act(async () => {
      await changeAppLanguage('fr');
    });

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(description.textContent === expectedDescription()).toBe(true);
    expect(description.querySelectorAll('kbd')).toHaveLength(2);
    unmount();
    expect(onCaughtError.mock.calls.length).toBe(0);
  });
});
