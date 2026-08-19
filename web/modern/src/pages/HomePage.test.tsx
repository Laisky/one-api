import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { HomePage } from './HomePage';

// Mock api client
vi.mock('@/lib/api', () => {
  return {
    api: {
      get: vi.fn(),
    },
    default: { get: vi.fn() },
  };
});

// Import the mocked api after vi.mock so TypeScript can reference it without dynamic import
import { api } from '@/lib/api';

// Simple localStorage mock helpers
const setLocalStorage = (key: string, value: string) => {
  window.localStorage.setItem(key, value);
};
const clearLocalStorage = () => {
  window.localStorage.clear();
};

// Helper: route api.get mock by URL so /api/notice doesn't echo home content
const mockApiGet = (homeContent: any, noticeContent: any = '') => {
  (api.get as any).mockImplementation((url: string) => {
    if (url.startsWith('/api/notice')) {
      return Promise.resolve({ data: { success: true, data: noticeContent } });
    }
    return Promise.resolve({ data: { success: true, data: homeContent } });
  });
};

describe('HomePage', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    clearLocalStorage();
  });

  it('renders iframe when content is a URL', async () => {
    mockApiGet('https://example.com');
    render(<HomePage />);
    await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/home_page_content'));
    const iframe = await screen.findByTitle('Home');
    expect(iframe).toBeInTheDocument();
  });

  it('renders HTML content when provided', async () => {
    mockApiGet('<h2>Hi</h2>');
    render(<HomePage />);
    await waitFor(() => screen.getByText('Hi'));
    expect(screen.getByText('Hi')).toBeInTheDocument();
  });

  it('shows minimal empty state when no content configured', async () => {
    mockApiGet('');
    render(<HomePage />);
    // Wait for API call
    await waitFor(() => expect(api.get).toHaveBeenCalled());
    // Empty state container exists
    const empty = await screen.findByTestId('home-empty');
    expect(empty).toBeInTheDocument();
  });
});
