import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

import { api } from '@/lib/api';
import { EditRedemptionPage } from '../EditRedemptionPage';

/** generateCodes submits the create form so the page shows two freshly generated codes. */
async function generateCodes() {
  (api.post as any).mockResolvedValue({ data: { success: true, data: ['code-1', 'code-2'] } });
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/redemptions/add']}>
      <Routes>
        <Route path="/redemptions/add" element={<EditRedemptionPage />} />
      </Routes>
    </MemoryRouter>
  );
  await user.type(screen.getByPlaceholderText('Enter redemption name'), 'batch');
  await user.click(screen.getByRole('button', { name: 'Create Redemption' }));
  await screen.findByRole('button', { name: 'Copy all' });
  return user;
}

describe('EditRedemptionPage copy codes', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
    (api.post as any).mockReset();
  });

  it('copies the generated codes through the shared clipboard helper and announces success', async () => {
    mocks.copy.mockResolvedValue(undefined);
    const user = await generateCodes();

    await user.click(screen.getByRole('button', { name: 'Copy all' }));

    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument();
    expect(mocks.copy).toHaveBeenCalledExactlyOnceWith('code-1\ncode-2');
    expect(screen.getByRole('status')).toHaveTextContent('Copied');
  });

  it('shows and announces a failed copy instead of staying silent', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const user = await generateCodes();

    await user.click(screen.getByRole('button', { name: 'Copy all' }));

    expect(await screen.findByRole('button', { name: 'Copy failed' })).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Copy failed');
  });
});
