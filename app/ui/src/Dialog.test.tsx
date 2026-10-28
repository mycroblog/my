// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, waitFor } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Button } from "./Button";
import { Dialog, DialogTrigger, Sheet } from "./Dialog";

/** The focus trap holds focus somewhere inside the dialog; which element it is does not matter here. */
function expectFocusInside(dialog: HTMLElement) {
  // eslint-disable-next-line testing-library/no-node-access -- the focused element is what the focus trap is about
  expect(dialog).toContainElement(document.activeElement as HTMLElement);
}

/** The scrim behind the panel: the overlay that holds the modal that holds the dialog. */
function scrimOf(dialog: HTMLElement): HTMLElement {
  // eslint-disable-next-line testing-library/no-node-access -- the scrim has no role; it is the dialog's grandparent
  const scrim = dialog.parentElement?.parentElement;
  if (scrim == null) throw new Error("no scrim");
  return scrim;
}

function EditProfile({ sheet = false, isDismissable = true }: { sheet?: boolean; isDismissable?: boolean }) {
  const Modal = sheet ? Sheet : Dialog;
  return (
    <DialogTrigger>
      <Button>Edit profile</Button>
      <Modal title="Edit profile" closeLabel="Close" isDismissable={isDismissable}>
        {({ close }) => (
          <>
            <Button onPress={close}>Cancel</Button>
            <Button onPress={close}>Save</Button>
          </>
        )}
      </Modal>
    </DialogTrigger>
  );
}

describe.each([
  ["Dialog", false],
  ["Sheet", true],
])("%s", (_, sheet) => {
  it("moves focus in, keeps Tab inside, closes on Esc and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    render(<EditProfile sheet={sheet} />);
    const trigger = screen.getByRole("button", { name: "Edit profile" });
    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "Edit profile" });
    await waitFor(() => {
      expectFocusInside(dialog);
    });
    for (let i = 0; i < 4; i++) {
      await user.tab();
      expectFocusInside(dialog);
    }
    // Four presses of Tab come round to the × again (×, Cancel, Save, ×), which shows its name as a tooltip (G12).
    // Esc closes the innermost layer first, the tooltip, and then the dialog. The modal hides what is outside it from
    // assistive technology, the tooltip's portal among it; the × keeps its own name.
    expect(screen.getByRole("button", { name: "Close" })).toHaveFocus();
    expect(await screen.findByRole("tooltip", { hidden: true })).toHaveTextContent("Close");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("tooltip", { hidden: true })).not.toBeInTheDocument();
    });
    expect(dialog).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
  });

  it("closes on a press on the scrim and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    render(<EditProfile sheet={sheet} />);
    const trigger = screen.getByRole("button", { name: "Edit profile" });
    await user.click(trigger);
    await user.click(scrimOf(await screen.findByRole("dialog", { name: "Edit profile" })));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
  });

  it("stays open on a press on the scrim when it is not dismissable", async () => {
    const user = userEvent.setup();
    render(<EditProfile sheet={sheet} isDismissable={false} />);
    await user.click(screen.getByRole("button", { name: "Edit profile" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit profile" });
    await user.click(scrimOf(dialog));
    expect(dialog).toBeInTheDocument();
  });

  it("returns focus to the trigger after the × button closes it", async () => {
    const user = userEvent.setup();
    render(<EditProfile sheet={sheet} />);
    const trigger = screen.getByRole("button", { name: "Edit profile" });
    await user.click(trigger);
    await user.click(await screen.findByRole("button", { name: "Close" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
  });
});
