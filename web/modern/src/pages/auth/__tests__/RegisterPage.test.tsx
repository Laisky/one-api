import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { RegisterPage } from '../RegisterPage';
import { vi } from 'vitest';
import { api } from '@/lib/api';

vi.mock('@/lib/api');

// Mock localStorage used by the page to read cached system status
const mockLocalStorage = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
};
Object.defineProperty(window, 'localStorage', { value: mockLocalStorage, configurable: true });

const mockApiGet = vi.mocked(api.get);

describe('RegisterPage (modern)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockReset();
    mockApiGet.mockResolvedValue({
      data: {
        success: true,
        data: { turnstile_check: false },
      },
    } as any);
    // Provide minimal system status cache to avoid optional UI branches
    mockLocalStorage.getItem.mockReturnValue(JSON.stringify({ system_name: 'Test API', github_oauth: false }));
  });

  it('prefills affiliate code (aff) from URL into aff_code field', async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/register', search: '?aff=ABCD' }]}>
        <RegisterPage />
      </MemoryRouter>
    );

    // Locate the input via its placeholder as labels are not programmatically associated
    const affInput = await screen.findByPlaceholderText(/Enter invitation code/i);
    expect((affInput as HTMLInputElement).value).toBe('ABCD');
  });

  it('turns off text correction on identifier fields and caps the username at the backend limit', async () => {
    render(
      <MemoryRouter initialEntries={['/register']}>
        <RegisterPage />
      </MemoryRouter>
    );

    const username = await screen.findByLabelText('Username');
    expect(username).toHaveAttribute('autocomplete', 'username');
    expect(username).toHaveAttribute('maxlength', '30');

    // Verification codes are compared case-sensitively by the backend.
    const code = screen.getByLabelText('Verification Code');
    expect(code).toHaveAttribute('autocomplete', 'one-time-code');

    const aff = screen.getByLabelText('Invitation/Affiliate Code (Optional)');
    for (const input of [username, code, aff]) {
      expect(input).toHaveAttribute('autocapitalize', 'none');
      expect(input).toHaveAttribute('autocorrect', 'off');
      expect(input).toHaveAttribute('spellcheck', 'false');
    }
  });
});
