import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { NotificationsProvider } from "@/components/ui/notifications";
import { api } from "@/lib/api";
import { SystemSettings } from "./SystemSettings";

describe("SystemSettings", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders sensitive options and allows updating secret values", async () => {
    vi.spyOn(api, "get").mockResolvedValue({
      data: {
        success: true,
        data: [
          { key: "SMTPServer", value: "smtp.example.com" },
          { key: "SMTPPort", value: "587" },
          { key: "SMTPAccount", value: "mailer@example.com" },
          { key: "SMTPFrom", value: "noreply@example.com" },
        ],
      },
    });

    const putMock = vi
      .spyOn(api, "put")
      .mockResolvedValue({ data: { success: true } });

    const user = userEvent.setup();

    render(
      <NotificationsProvider>
        <SystemSettings />
      </NotificationsProvider>
    );

    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1));

    expect(await screen.findByText("SMTPAccount")).toBeInTheDocument();
    expect(screen.getByText("SMTPToken")).toBeInTheDocument();

    const input = screen.getByLabelText("SMTPToken value") as HTMLInputElement;

    await user.type(input, "super-secret-token");

    const saveButton = input.parentElement?.querySelector("button");
    expect(saveButton).toBeTruthy();

    await user.click(saveButton as HTMLButtonElement);

    await waitFor(() =>
      expect(putMock).toHaveBeenCalledWith("/api/option/", {
        key: "SMTPToken",
        value: "super-secret-token",
      })
    );

    expect(input.value).toBe("");
  });
});
