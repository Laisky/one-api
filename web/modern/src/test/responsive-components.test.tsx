/**
 * Basic test to validate responsive components can be imported and rendered
 * This is a simple smoke test to ensure our responsive system is working
 */

import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import { BrowserRouter } from "react-router-dom";
import {
  ResponsiveContainer,
  ResponsivePageContainer,
} from "@/components/ui/responsive-container";
import { AdaptiveGrid } from "@/components/ui/adaptive-grid";
import { MobileDrawer } from "@/components/ui/mobile-drawer";
import { ResponsiveForm } from "@/components/ui/responsive-form";

// Mock the responsive hook for testing
vi.mock("@/hooks/useResponsive", () => ({
  useResponsive: () => ({
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isLarge: false,
    currentBreakpoint: "desktop",
    width: 1024,
    height: 768,
  }),
}));

const TestWrapper = ({ children }: { children: React.ReactNode }) => (
  <BrowserRouter>{children}</BrowserRouter>
);

describe("Responsive Components", () => {
  it("should render ResponsiveContainer without crashing", () => {
    const { container } = render(
      <TestWrapper>
        <ResponsiveContainer>
          <div>Test content</div>
        </ResponsiveContainer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it("should render ResponsivePageContainer without crashing", () => {
    const { container } = render(
      <TestWrapper>
        <ResponsivePageContainer title="Test Page">
          <div>Test content</div>
        </ResponsivePageContainer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it("should render AdaptiveGrid without crashing", () => {
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

  it("should render MobileDrawer without crashing", () => {
    const { container } = render(
      <TestWrapper>
        <MobileDrawer isOpen={false} onClose={() => {}}>
          <div>Drawer content</div>
        </MobileDrawer>
      </TestWrapper>
    );
    expect(container).toBeTruthy();
  });

  it("should render ResponsiveForm without crashing", () => {
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

import { useResponsive } from "@/hooks/useResponsive";
import { EnhancedDataTable } from "@/components/ui/enhanced-data-table";

describe("Responsive Hooks", () => {
  it("should provide responsive state", () => {
    const state = useResponsive();

    expect(state).toHaveProperty("isMobile");
    expect(state).toHaveProperty("isTablet");
    expect(state).toHaveProperty("isDesktop");
    expect(state).toHaveProperty("isLarge");
    expect(state).toHaveProperty("currentBreakpoint");
    expect(state).toHaveProperty("width");
    expect(state).toHaveProperty("height");
  });
});

describe("Enhanced Data Table", () => {
  it("should handle responsive props", () => {
    const columns = [
      { header: "Name", accessorKey: "name" },
      { header: "Email", accessorKey: "email" },
    ];

    const data = [
      { name: "John Doe", email: "john@example.com" },
      { name: "Jane Smith", email: "jane@example.com" },
    ];

    const { container } = render(
      <TestWrapper>
        <EnhancedDataTable
          columns={columns}
          data={data}
          mobileCardLayout={true}
          hideColumnsOnMobile={["email"]}
          compactMode={false}
        />
      </TestWrapper>
    );

    expect(container).toBeTruthy();
  });
});
