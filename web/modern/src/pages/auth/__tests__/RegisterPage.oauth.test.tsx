import { api } from '@/lib/api';
import * as oauth from '@/lib/oauth';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { vi } from 'vitest';
import { RegisterPage } from '../RegisterPage';

vi.mock('@/lib/api');
vi.mock('@/lib/oauth', async () => {
  const actual = await vi.importActual<typeof import('@/lib/oauth')>('@/lib/oauth');
  return {
    ...actual,
    getOAuthState: vi.fn(),
  };
});

const mockApiGet = vi.mocked(api.get);
const mockGetOAuthState = vi.mocked(oauth.getOAuthState);

const mockLocalStorage = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
};
Object.defineProperty(window, 'localStorage', { value: mockLocalStorage, configurable: true });

describe('RegisterPage OAuth state handling', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockReset();
    mockGetOAuthState.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: {
          turnstile_check: false,
          github_oauth: true,
          github_client_id: 'github-client',
        },
      },
    } as any);
    mockLocalStorage.getItem.mockReturnValue(
      JSON.stringify({
        system_name: 'Test API',
        github_oauth: true,
        github_client_id: 'github-client',
      })
    );
  });

  it('shows the localized OAuth state error instead of launching GitHub OAuth when state creation fails', async () => {
    mockGetOAuthState.mockRejectedValueOnce(new Error(''));

    render(
      <MemoryRouter initialEntries={['/register']}>
        <RegisterPage />
      </MemoryRouter>
    );

    fireEvent.click(await screen.findByRole('button', { name: 'GitHub' }));

    await waitFor(() => {
      expect(screen.getByText('Unable to start OAuth. Please try again.')).toBeInTheDocument();
    });
    expect(mockGetOAuthState).toHaveBeenCalledTimes(1);
  });
});
