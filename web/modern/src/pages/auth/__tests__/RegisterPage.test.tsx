import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { RegisterPage } from '../RegisterPage'
import { vi } from 'vitest'

// Mock localStorage used by the page to read cached system status
const mockLocalStorage = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
}
Object.defineProperty(window, 'localStorage', { value: mockLocalStorage })

describe('RegisterPage (modern)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Provide minimal system status cache to avoid optional UI branches
    mockLocalStorage.getItem.mockReturnValue(
      JSON.stringify({ system_name: 'Test API', github_oauth: false })
    )
  })

  it('prefills affiliate code (aff) from URL into aff_code field', async () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/register', search: '?aff=ABCD' }]}>
        <RegisterPage />
      </MemoryRouter>
    )

    // Locate the input via its placeholder as labels are not programmatically associated
    const affInput = await screen.findByPlaceholderText(/Enter invitation code/i)
    expect((affInput as HTMLInputElement).value).toBe('ABCD')
  })
})
