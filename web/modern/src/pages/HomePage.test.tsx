import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { HomePage } from "./HomePage";

// Mock api client
vi.mock("@/lib/api", () => {
  return {
    api: {
      get: vi.fn(),
    },
    default: { get: vi.fn() },
  };
});

// Import the mocked api after vi.mock so TypeScript can reference it without dynamic import
import { api } from "@/lib/api";

// Simple localStorage mock helpers
const setLocalStorage = (key: string, value: string) => {
  window.localStorage.setItem(key, value);
};
const clearLocalStorage = () => {
  window.localStorage.clear();
};

describe("HomePage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    clearLocalStorage();
  });

  it("renders iframe when content is a URL", async () => {
    (api.get as any).mockResolvedValue({
      data: { success: true, data: "https://example.com" },
    });
    render(<HomePage />);
    await waitFor(() =>
      expect(api.get).toHaveBeenCalledWith("/api/home_page_content")
    );
    const iframe = await screen.findByTitle("Home");
    expect(iframe).toBeInTheDocument();
  });

  it("renders HTML content when provided", async () => {
    (api.get as any).mockResolvedValue({
      data: { success: true, data: "<h2>Hi</h2>" },
    });
    render(<HomePage />);
    await waitFor(() => screen.getByText("Hi"));
    expect(screen.getByText("Hi")).toBeInTheDocument();
  });

  it("shows minimal empty state when no content configured", async () => {
    (api.get as any).mockResolvedValue({ data: { success: true, data: "" } });
    render(<HomePage />);
    // Wait for API call
    await waitFor(() => expect(api.get).toHaveBeenCalled());
    // Empty state container exists
    const empty = await screen.findByTestId("home-empty");
    expect(empty).toBeInTheDocument();
  });
});
