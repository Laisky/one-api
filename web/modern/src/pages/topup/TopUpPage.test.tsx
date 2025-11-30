import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/stores/auth";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TopUpPage } from "./TopUpPage";

vi.mock("@/lib/api", () => {
  const get = vi.fn();
  const post = vi.fn();
  return {
    api: {
      get,
      post,
      defaults: { withCredentials: true },
      interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
    },
  };
});

describe("TopUpPage", () => {
  beforeEach(() => {
    // Reset store
    useAuthStore.setState({
      user: {
        id: 1,
        username: "testuser",
        role: 1,
        status: 1,
        quota: 1000,
        used_quota: 0,
        group: "default",
      } as any,
      token: "token",
      isAuthenticated: true,
      login: vi.fn() as any,
      logout: vi.fn() as any,
      updateUser: vi.fn() as any,
    });

    // Clear and set localStorage defaults used by the page
    localStorage.clear();
    localStorage.setItem("quota_per_unit", "500000");
    localStorage.setItem("display_in_currency", "true");

    // Mock system status with a payment link
    localStorage.setItem(
      "status",
      JSON.stringify({ top_up_link: "https://pay.example.com" })
    );

    // Reset API mocks
    (api.get as any).mockReset();
    (api.post as any).mockReset();
    (api.get as any).mockResolvedValue({
      data: {
        success: true,
        data: { id: 1, username: "testuser", quota: 1000 },
      },
    });
    (api.post as any).mockResolvedValue({ data: { success: true, data: 500 } });
  });

  it("renders and redeems a code", async () => {
    render(<TopUpPage />);

    // Field should be present
    const input = await screen.findByPlaceholderText(
      /enter your redemption code/i
    );

    // Type code and submit
    fireEvent.change(input, { target: { value: "ABC-123" } });
    const redeemBtn = screen.getByRole("button", { name: /redeem code/i });
    fireEvent.click(redeemBtn);

    await waitFor(() => {
      expect(api.post).toHaveBeenCalledWith("/api/user/topup", {
        key: "ABC-123",
      });
    });

    // Success message should appear
    await screen.findByText(/successfully redeemed/i);
  });

  it("loads user quota on mount", async () => {
    render(<TopUpPage />);

    await waitFor(() => {
      expect(api.get).toHaveBeenCalledWith("/api/user/self");
    });

    // Shows current balance text
    await screen.findByText(/current balance/i);
  });
});
