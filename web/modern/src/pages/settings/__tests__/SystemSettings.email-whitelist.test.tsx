import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { SystemSettings } from '../SystemSettings';

describe('SystemSettings: EmailDomainWhitelist multi-tag', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders existing domains as removable badges and saves comma-joined string', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [
          { key: 'EmailDomainRestrictionEnabled', value: 'true' },
          { key: 'EmailDomainWhitelist', value: 'gmail.com,example.com' },
        ],
      },
    });

    const putMock = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } });

    const user = userEvent.setup();

    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1));

    // Existing chips render
    expect(await screen.findByText('gmail.com')).toBeInTheDocument();
    expect(screen.getByText('example.com')).toBeInTheDocument();

    const input = screen.getByLabelText('New email domain') as HTMLInputElement;
    const addButton = screen.getByRole('button', { name: /^add$/i });

    // Add a valid new domain
    await user.type(input, 'foo.bar');
    await user.click(addButton);
    expect(await screen.findByText('foo.bar')).toBeInTheDocument();
    expect(input.value).toBe('');

    // Try to add an invalid domain — should not be added
    await user.type(input, 'invalid');
    await user.click(addButton);
    expect(screen.queryByText('invalid')).not.toBeInTheDocument();

    // Clear input then try a duplicate (gmail.com)
    await user.clear(input);
    await user.type(input, 'gmail.com');
    await user.click(addButton);
    // Still only one badge for gmail.com
    expect(screen.getAllByText('gmail.com')).toHaveLength(1);

    // Remove example.com via its remove button
    const removeBtn = screen.getByRole('button', { name: /remove example\.com/i });
    await user.click(removeBtn);
    await waitFor(() => expect(screen.queryByText('example.com')).not.toBeInTheDocument());

    // Click Save (the button inside the EmailDomainWhitelist card)
    const saveButtons = screen.getAllByRole('button', { name: /^save$/i });
    // Save the first one (the email whitelist save is the only actionable Save in this group)
    // Find the one that lives in the same card as the input
    const whitelistCard = input.closest('.border.rounded-lg') as HTMLElement;
    const saveBtn = Array.from(whitelistCard.querySelectorAll('button')).find((b) => /save/i.test(b.textContent || ''));
    expect(saveBtn).toBeTruthy();
    await user.click(saveBtn as HTMLButtonElement);

    await waitFor(() =>
      expect(putMock).toHaveBeenCalledWith('/api/option/', {
        key: 'EmailDomainWhitelist',
        value: 'gmail.com,foo.bar',
      })
    );
    expect(saveButtons.length).toBeGreaterThan(0);
  });
});
