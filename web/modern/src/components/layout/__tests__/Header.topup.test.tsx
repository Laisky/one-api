import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Header } from '../Header';

describe('Header mobile overflow prevention', () => {
  it('renders header with no horizontal overflow and truncates brand text', () => {
    // Ensure a deterministic brand name so the query is unique
    localStorage.setItem('status', JSON.stringify({ system_name: 'One API Test Brand' }));
    render(
      <MemoryRouter initialEntries={['/']}>
        <Header />
      </MemoryRouter>
    );

    // Header root should use full width sizing
    const header = screen.getByRole('banner');
    expect(header.className).toContain('w-full');

    // Brand text should truncate on small screens to avoid pushing layout
    const brand = screen.getByRole('link', { name: 'One API Test Brand' });
    expect(brand.className).toContain('truncate');
  });
});
