/**
 * Basic test to validate responsive components can be imported and rendered
 * This is a simple smoke test to ensure our responsive system is working
 */

import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { ResponsiveContainer, ResponsivePageContainer } from '@/components/ui/responsive-container';
import { AdaptiveGrid } from '@/components/ui/adaptive-grid';
import { MobileDrawer } from '@/components/ui/mobile-drawer';
import { ResponsiveForm } from '@/components/ui/responsive-form';

// Mock the responsive hook for testing
vi.mock('@/hooks/useResponsive', () => ({
  useResponsive: () => ({
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isLarge: false,
    currentBreakpoint: 'desktop',
    width: 1024,
    height: 768,
  }),
}));

const TestWrapper = ({ children }: { children: React.ReactNode }) => <BrowserRouter>{children}</BrowserRouter>;

describe('Responsive Components', () => {
  it('should render ResponsiveContainer without crashing', () => {
    const { container } = render(
      <TestWrapper>
        <ResponsiveContainer>
          <div>Test content</div>
        </ResponsiveContainer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it('should render ResponsivePageContainer without crashing', () => {
    const { container } = render(
      <TestWrapper>
        <ResponsivePageContainer title="Test Page">
          <div>Test content</div>
        </ResponsivePageContainer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it('should render AdaptiveGrid without crashing', () => {
    const { container } = render(
      <TestWrapper>
        <AdaptiveGrid>
          <div>Grid item 1</div>
          <div>Grid item 2</div>
        </AdaptiveGrid>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it('should render MobileDrawer without crashing', () => {
    const { container } = render(
      <TestWrapper>
        <MobileDrawer isOpen={false} onClose={() => {}}>
          <div>Drawer content</div>
        </MobileDrawer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it('labels an open MobileDrawer with a unique title id, focuses inside it, and leaves body overflow alone', () => {
    const onClose = vi.fn();
    render(
      <TestWrapper>
        <MobileDrawer isOpen onClose={onClose} title="First drawer" id="first-drawer">
          <div>First content</div>
        </MobileDrawer>
        <MobileDrawer isOpen onClose={() => {}} title="Second drawer">
          <div>Second content</div>
        </MobileDrawer>
      </TestWrapper>
    );

    const first = screen.getByRole('dialog', { name: 'First drawer' });
    const second = screen.getByRole('dialog', { name: 'Second drawer' });
    expect(first).toHaveAttribute('id', 'first-drawer');
    expect(first).toHaveAttribute('aria-modal', 'true');
    expect(first.getAttribute('aria-labelledby')).not.toBe(second.getAttribute('aria-labelledby'));
    expect(document.querySelector('#drawer-title')).toBeNull();
    // Scroll chaining is contained with overscroll-behavior instead of locking <body>.
    expect(document.body.style.overflow).toBe('');
    expect(first.className).toContain('overscroll-contain');
    expect(first.className).toContain('motion-reduce:transition-none');

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('restores focus to the opener and clears inert on the app root when a MobileDrawer closes', () => {
    const appRoot = document.createElement('div');
    appRoot.id = 'root';
    document.body.appendChild(appRoot);
    const opener = document.createElement('button');
    opener.textContent = 'Open drawer';
    appRoot.appendChild(opener);
    opener.focus();

    const { rerender } = render(
      <TestWrapper>
        <MobileDrawer isOpen onClose={() => {}} title="Menu">
          <a href="/x">Link</a>
        </MobileDrawer>
      </TestWrapper>
    );

    const drawer = screen.getByRole('dialog', { name: 'Menu' });
    expect(appRoot).toHaveAttribute('inert');
    expect(drawer).toContainElement(document.activeElement as HTMLElement);

    rerender(
      <TestWrapper>
        <MobileDrawer isOpen={false} onClose={() => {}} title="Menu">
          <a href="/x">Link</a>
        </MobileDrawer>
      </TestWrapper>
    );

    expect(screen.queryByRole('dialog', { name: 'Menu' })).toBeNull();
    expect(appRoot).not.toHaveAttribute('inert');
    expect(document.activeElement).toBe(opener);
    appRoot.remove();
  });

  it('should render ResponsiveForm without crashing', () => {
    const { container } = render(
      <TestWrapper>
        <ResponsiveForm>
          <div>Form content</div>
        </ResponsiveForm>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });
});

import { useResponsive } from '@/hooks/useResponsive';
import { EnhancedDataTable } from '@/components/ui/enhanced-data-table';

describe('Responsive Hooks', () => {
  it('should provide responsive state', () => {
    const state = useResponsive();

    expect(state).toHaveProperty('isMobile');
    expect(state).toHaveProperty('isTablet');
    expect(state).toHaveProperty('isDesktop');
    expect(state).toHaveProperty('isLarge');
    expect(state).toHaveProperty('currentBreakpoint');
    expect(state).toHaveProperty('width');
    expect(state).toHaveProperty('height');
  });
});

describe('Enhanced Data Table', () => {
  it('should handle responsive props', () => {
    const columns = [
      { header: 'Name', accessorKey: 'name' },
      { header: 'Email', accessorKey: 'email' },
    ];

    const data = [
      { name: 'John Doe', email: 'john@example.com' },
      { name: 'Jane Smith', email: 'jane@example.com' },
    ];

    const { container } = render(
      <TestWrapper>
        <EnhancedDataTable columns={columns} data={data} mobileCardLayout={true} hideColumnsOnMobile={['email']} compactMode={false} />
      </TestWrapper>
    );

    expect(container).toBeTruthy();
  });

  it('should hide floating row actions while hovering interactive row controls', async () => {
    const columns = [
      { header: 'Name', accessorKey: 'name' },
      {
        id: 'actions',
        header: 'Actions',
        cell: () => <button type="button">Inline action</button>,
      },
    ];

    const data = [{ name: 'John Doe' }];

    render(
      <TestWrapper>
        <EnhancedDataTable columns={columns} data={data} floatingRowActions={() => <button type="button">Floating row action</button>} />
      </TestWrapper>
    );

    const row = screen.getByText('John Doe').closest('tr');
    expect(row).toBeTruthy();

    fireEvent.mouseOver(row as HTMLTableRowElement, { clientX: 32, clientY: 48 });
    await waitFor(() => {
      expect(screen.getByText('Floating row action')).toBeInTheDocument();
    });

    fireEvent.mouseMove(screen.getByText('Inline action'), { clientX: 40, clientY: 48 });
    await waitFor(() => {
      expect(screen.queryByText('Floating row action')).not.toBeInTheDocument();
    });

    fireEvent.mouseMove(screen.getByText('John Doe'), { clientX: 56, clientY: 48 });
    expect(screen.getByText('Floating row action')).toBeInTheDocument();
  });
});
