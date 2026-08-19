import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
  default: { get: vi.fn() },
}));

import { api } from '@/lib/api';
import { NoticeBanner } from '../NoticeBanner';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;

describe('NoticeBanner', () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockedGet.mockReset();
  });

  it('renders notice banner with markdown content and dismisses on close click', async () => {
    mockedGet.mockResolvedValue({ data: { success: true, data: '# Important' } });

    render(<NoticeBanner />);

    const banner = await screen.findByTestId('global-notice');
    expect(banner).toBeInTheDocument();
    expect(banner.textContent).toContain('Important');

    const dismissBtn = screen.getByTestId('global-notice-dismiss');
    fireEvent.click(dismissBtn);

    await waitFor(() => {
      expect(screen.queryByTestId('global-notice')).not.toBeInTheDocument();
    });
    expect(window.localStorage.getItem('notice_seen_content')).toBe('# Important');
  });

  it('does not render banner when notice is empty', async () => {
    mockedGet.mockResolvedValue({ data: { success: true, data: '' } });

    render(<NoticeBanner />);

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalledWith('/api/notice');
    });
    expect(screen.queryByTestId('global-notice')).not.toBeInTheDocument();
  });

  it('does not render banner when current notice was previously dismissed', async () => {
    window.localStorage.setItem('notice_seen_content', '# Old News');
    mockedGet.mockResolvedValue({ data: { success: true, data: '# Old News' } });

    render(<NoticeBanner />);

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalledWith('/api/notice');
    });
    expect(screen.queryByTestId('global-notice')).not.toBeInTheDocument();
  });
});
