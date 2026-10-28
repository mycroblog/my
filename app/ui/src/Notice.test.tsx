// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AnnouncerProvider } from "./Announcer";
import { Button } from "./Button";
import { ChangeNotice } from "./ChangeNotice";
import { useNotice } from "./Notice";
import { ToastProvider } from "./Toast";

function Actions() {
  const notice = useNotice();
  return (
    <>
      <Button
        onPress={() => {
          notice.inPlace("Saved");
        }}
      >
        Save
      </Button>
      <Button
        onPress={() => {
          notice.inPlace("The room ended.", "assertive");
        }}
      >
        End
      </Button>
      <Button
        onPress={() => {
          notice.toast({ message: "Link copied", tone: "success", timeout: null });
        }}
      >
        Copy
      </Button>
    </>
  );
}

function renderActions() {
  return render(
    <AnnouncerProvider>
      <ToastProvider regionLabel="Notifications" dismissLabel="Dismiss">
        <Actions />
      </ToastProvider>
    </AnnouncerProvider>,
  );
}

describe("useNotice", () => {
  it("reads a result that shows in place out once, politely or assertively", async () => {
    const user = userEvent.setup();
    renderActions();
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByRole("status")).toHaveTextContent("Saved");
    await user.click(screen.getByRole("button", { name: "End" }));
    expect(screen.getByRole("alert")).toHaveTextContent("The room ended.");
  });

  it("shows a toast, which reads it out", async () => {
    const user = userEvent.setup();
    renderActions();
    await user.click(screen.getByRole("button", { name: "Copy" }));
    expect(screen.getByRole("region", { name: "Notifications" })).toHaveTextContent("Link copied");
    expect(screen.getByRole("status")).toHaveTextContent("Link copied");
  });
});

describe("ChangeNotice", () => {
  it("shows the change with a dismiss button, and is no live region", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();
    render(
      <ChangeNotice dismissLabel="Dismiss" onDismiss={onDismiss}>
        The host turned your microphone off.
      </ChangeNotice>,
    );
    expect(screen.getByText("The host turned your microphone off.")).toBeVisible();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
