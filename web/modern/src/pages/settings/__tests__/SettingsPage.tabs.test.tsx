import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { useAuthStore } from '@/lib/stores/auth';

let isMobile = false;

vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile, isTablet: false, isDesktop: !isMobile, isLarge: false }),
}));
vi.mock('../PersonalSettings', () => ({ PersonalSettings: () => <div>Personal panel</div> }));
vi.mock('../OperationSettings', () => ({ OperationSettings: () => <div>Operation panel</div> }));
vi.mock('../SystemSettings', () => ({ SystemSettings: () => <div>System panel</div> }));
vi.mock('../OtherSettings', () => ({ OtherSettings: () => <div>Other panel</div> }));

import { SettingsPage } from '../SettingsPage';

/** renderSettings mounts the settings page on a /settings/:tab route. */
function renderSettings(path = '/settings/personal') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/settings/:tab" element={<SettingsPage />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('SettingsPage tabs', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: { id: 1, username: 'root', role: 100 } as any });
  });

  afterEach(() => {
    cleanup();
    isMobile = false;
  });

  for (const mobile of [false, true]) {
    it(`keeps every tab on one horizontally scrolling row (${mobile ? 'mobile' : 'desktop'})`, () => {
      isMobile = mobile;
      renderSettings();

      const tablist = screen.getByRole('tablist');
      expect(tablist).toHaveAttribute('aria-orientation', 'horizontal');
      expect(tablist).toHaveClass('flex', 'overflow-x-auto', 'justify-start');
      expect(tablist.className).not.toMatch(/grid|flex-col/);

      const tabs = within(tablist).getAllByRole('tab');
      expect(tabs).toHaveLength(4);
      for (const tab of tabs) expect(tab).toHaveClass('flex-none');
    });
  }

  it('names the tablist and moves between tabs with the horizontal arrow keys', async () => {
    const user = userEvent.setup();
    renderSettings();

    const tablist = screen.getByRole('tablist');
    expect(tablist).toHaveAccessibleName();

    await user.click(screen.getByRole('tab', { name: 'Personal' }));
    await user.keyboard('{ArrowRight}');
    expect(screen.getByRole('tab', { name: 'Operation' })).toHaveFocus();
  });
});
