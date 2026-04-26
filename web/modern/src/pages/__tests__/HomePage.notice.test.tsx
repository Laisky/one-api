import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
  default: { get: vi.fn() },
}));

import { api } from '@/lib/api';
import { HomePage } from '../HomePage';

const mockedGet = api.get as unknown as ReturnType<typeof vi.fn>;

const setupGet = (handlers: Record<string, any>) => {
  mockedGet.mockImplementation((url: string) => {
    for (const key of Object.keys(handlers)) {
      if (url.startsWith(key)) {
        return Promise.resolve(handlers[key]);
      }
    }
    return Promise.resolve({ data: { success: false } });
  });
};

describe('HomePage notice integration', () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockedGet.mockReset();
  });

  it('renders notice banner with markdown content and dismisses on close click', async () => {
    setupGet({
      '/api/home_page_content': { data: { success: true, data: '' } },
      '/api/notice': { data: { success: true, data: '# Important' } },
    });

    render(<HomePage />);

    const banner = await screen.findByTestId('home-notice');
    expect(banner).toBeInTheDocument();
    expect(banner.textContent).toContain('Important');

    const dismissBtn = screen.getByTestId('home-notice-dismiss');
    fireEvent.click(dismissBtn);

    await waitFor(() => {
      expect(screen.queryByTestId('home-notice')).not.toBeInTheDocument();
    });
    expect(window.localStorage.getItem('notice_seen_content')).toBe('# Important');
  });

  it('does not render banner when notice is empty', async () => {
    setupGet({
      '/api/home_page_content': { data: { success: true, data: '' } },
      '/api/notice': { data: { success: true, data: '' } },
    });

    render(<HomePage />);

    await waitFor(() => {
      expect(mockedGet).toHaveBeenCalled();
    });
    expect(screen.queryByTestId('home-notice')).not.toBeInTheDocument();
  });
});
