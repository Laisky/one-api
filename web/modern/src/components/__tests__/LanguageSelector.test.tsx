import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/i18n', () => ({ changeAppLanguage: vi.fn() }));

import { LanguageSelector } from '../LanguageSelector';

describe('LanguageSelector', () => {
  afterEach(() => {
    cleanup();
  });

  it('marks each native language name with its own lang and exposes the active language as checked', async () => {
    const user = userEvent.setup();
    render(<LanguageSelector />);

    const trigger = screen.getByRole('button', { name: 'Select language' });
    expect(trigger).toHaveClass('touch-target');
    await user.click(trigger);

    const options = await screen.findAllByRole('menuitemradio');
    expect(options).toHaveLength(5);

    const expected: Record<string, string> = { English: 'en', 简体中文: 'zh', Français: 'fr', Español: 'es', 日本語: 'ja' };
    for (const [label, code] of Object.entries(expected)) {
      expect(screen.getByText(label)).toHaveAttribute('lang', code);
    }

    expect(screen.getByRole('menuitemradio', { name: 'English' })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByRole('menuitemradio', { name: 'Français' })).toHaveAttribute('aria-checked', 'false');
  });
});
