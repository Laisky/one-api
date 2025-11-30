import { api } from "@/lib/api";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useChannelForm } from "./useChannelForm";

// Mocks
vi.mock("@/components/ui/notifications", () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock("@/lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}));

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({ id: "1" }), // Default to edit mode
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockApiGet = vi.mocked(api.get);

describe("useChannelForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should not cause infinite loop when loading channel", async () => {
    // Mock responses
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith("/api/channel/1")) {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              id: 1,
              type: 1,
              name: "Test Channel",
              models: "gpt-3.5-turbo",
              group: "default",
              config: "{}",
              tooling: "{}",
            },
          },
        });
      }
      if (url.startsWith("/api/models")) {
        return Promise.resolve({
          data: {
            success: true,
            data: { 1: ["gpt-3.5-turbo"] },
          },
        });
      }
      if (url.startsWith("/api/option/")) {
        return Promise.resolve({
          data: {
            success: true,
            data: [{ key: "AvailableGroups", value: "vip" }],
          },
        });
      }
      if (url.startsWith("/api/channel/default-pricing")) {
        return Promise.resolve({
          data: {
            success: true,
            data: { model_configs: "{}", tooling: "{}" },
          },
        });
      }
      if (url.startsWith("/api/channel/metadata")) {
        return Promise.resolve({
          data: {
            success: true,
            data: { default_base_url: "https://api.openai.com" },
          },
        });
      }
      return Promise.resolve({ data: { success: false } });
    });

    const { result } = renderHook(() => useChannelForm());

    // Wait for loading to finish
    await waitFor(
      () => {
        expect(result.current.loading).toBe(false);
      },
      { timeout: 3000 }
    );

    // Check call counts
    // loadChannel should be called once
    const channelCalls = mockApiGet.mock.calls.filter((call) =>
      call[0].startsWith("/api/channel/1")
    ).length;
    expect(channelCalls).toBe(1);

    // loadModelsCatalog should be called once
    const modelsCalls = mockApiGet.mock.calls.filter((call) =>
      call[0].startsWith("/api/models")
    ).length;
    expect(modelsCalls).toBe(1);

    // loadGroups should be called once
    const groupsCalls = mockApiGet.mock.calls.filter((call) =>
      call[0].startsWith("/api/option/")
    ).length;
    expect(groupsCalls).toBe(1);

    // default pricing might be called multiple times but should settle
    const pricingCalls = mockApiGet.mock.calls.filter((call) =>
      call[0].startsWith("/api/channel/default-pricing")
    ).length;
    expect(pricingCalls).toBeGreaterThanOrEqual(1);
    expect(pricingCalls).toBeLessThan(5); // Arbitrary limit, but definitely not infinite
  });
});
