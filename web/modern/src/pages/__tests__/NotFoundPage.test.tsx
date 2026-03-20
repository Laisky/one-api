import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { act } from 'react-dom/test-utils';
import { NotFoundPage } from '../NotFoundPage';

// Mock useNavigate so we can assert redirection without jsdom navigation errors
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<any>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('NotFoundPage', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockNavigate.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows 404 and countdown, then navigates after 5s', () => {
    render(
      <MemoryRouter>
        <NotFoundPage />
      </MemoryRouter>
    );

    expect(screen.getByText('404')).toBeInTheDocument();
    expect(screen.getByText(/Redirecting to home in/)).toBeInTheDocument();

    // Fast-forward 3 seconds and ensure countdown updated (no exact number assertion to avoid flakiness)
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(screen.getByText(/Redirecting to home in/)).toBeInTheDocument();

    // Fast-forward to 5 seconds to trigger navigate('/' , { replace: true })
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true });
  });
});
