// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, waitFor, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./Button";
import { ConfirmDialog } from "./ConfirmDialog";
import { DialogTrigger } from "./Dialog";
import { DurationSelect } from "./DurationSelect";
import { ReasonField } from "./ReasonField";
import { StatusMessage } from "./StatusMessage";

const formatRemaining = (remaining: number) => `${String(remaining)} left`;
const day = 86_400;

function Suspend({ onConfirm, withFields = false }: { onConfirm: (close: () => void) => void; withFields?: boolean }) {
  const [reason, setReason] = useState("");
  return (
    <DialogTrigger>
      <Button>Suspend example.com</Button>
      <ConfirmDialog
        title="Suspend example.com?"
        description="Deliveries stop."
        confirmLabel="Suspend"
        cancelLabel="Cancel"
        onConfirm={onConfirm}
      >
        {withFields ? (
          <>
            <ReasonField
              label="Reason"
              isRequired
              value={reason}
              onChange={setReason}
              characterLimit={100}
              formatRemaining={formatRemaining}
            />
            <DurationSelect
              label="Duration"
              durations={[day]}
              formatDuration={() => "1 day"}
              indefiniteLabel="Indefinitely"
              defaultValue={null}
            />
          </>
        ) : undefined}
      </ConfirmDialog>
    </DialogTrigger>
  );
}

describe("ConfirmDialog", () => {
  it("is an alertdialog named by its question and described by its consequence, focused on cancel", async () => {
    const user = userEvent.setup();
    render(<Suspend onConfirm={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Suspend example.com" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Suspend example.com?" });
    expect(dialog).toHaveAccessibleDescription("Deliveries stop.");
    await waitFor(() => {
      expect(within(dialog).getByRole("button", { name: "Cancel" })).toHaveFocus();
    });
  });

  it("closes on a press on the scrim without confirming", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<Suspend onConfirm={onConfirm} />);
    await user.click(screen.getByRole("button", { name: "Suspend example.com" }));
    // eslint-disable-next-line testing-library/no-node-access -- the scrim has no role; it is the dialog's grandparent
    const scrim = (await screen.findByRole("alertdialog")).parentElement?.parentElement;
    if (scrim == null) throw new Error("no scrim");
    await user.click(scrim);
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("stays open on a press on the scrim when it has fields, which the press would lose", async () => {
    const user = userEvent.setup();
    render(<Suspend onConfirm={vi.fn()} withFields />);
    await user.click(screen.getByRole("button", { name: "Suspend example.com" }));
    const dialog = await screen.findByRole("alertdialog");
    // eslint-disable-next-line testing-library/no-node-access -- the scrim has no role; it is the dialog's grandparent
    const scrim = dialog.parentElement?.parentElement;
    if (scrim == null) throw new Error("no scrim");
    await user.click(scrim);
    expect(dialog).toBeInTheDocument();
  });

  it("closes on Esc without confirming and returns focus to the trigger", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<Suspend onConfirm={onConfirm} />);
    const trigger = screen.getByRole("button", { name: "Suspend example.com" });
    await user.click(trigger);
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("runs the action, which closes the dialog", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn((close: () => void) => {
      close();
    });
    render(<Suspend onConfirm={onConfirm} />);
    const trigger = screen.getByRole("button", { name: "Suspend example.com" });
    await user.click(trigger);
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Suspend" }));
    expect(onConfirm).toHaveBeenCalledOnce();
    await waitFor(() => {
      expect(trigger).toHaveFocus();
    });
  });

  it("focuses the first field and waits for a required reason", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<Suspend onConfirm={onConfirm} withFields />);
    await user.click(screen.getByRole("button", { name: "Suspend example.com" }));
    const dialog = await screen.findByRole("alertdialog");
    const reason = within(dialog).getByRole("textbox", { name: "Reason" });
    await waitFor(() => {
      expect(reason).toHaveFocus();
    });
    await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
    expect(onConfirm).not.toHaveBeenCalled();
    await user.type(reason, "Spam");
    await user.click(within(dialog).getByRole("button", { name: "Suspend" }));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("shows a failure in the dialog as an alert", async () => {
    render(
      <ConfirmDialog
        defaultOpen
        title="Suspend?"
        description="Deliveries stop."
        confirmLabel="Suspend"
        cancelLabel="Cancel"
        onConfirm={vi.fn()}
        errorMessage="Could not suspend."
      />,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not suspend.");
  });
});

describe("StatusMessage", () => {
  it("is an empty status region until it has a message", () => {
    const { rerender } = render(<StatusMessage>{null}</StatusMessage>);
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    rerender(<StatusMessage>Suspended.</StatusMessage>);
    expect(screen.getByRole("status")).toHaveTextContent("Suspended.");
  });
});
