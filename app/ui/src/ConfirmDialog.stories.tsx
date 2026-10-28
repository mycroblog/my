// SPDX-License-Identifier: AGPL-3.0-only
// ConfirmDialog stories. The play functions are the alert dialog's behaviour tests (`scripts/task.sh ui:test`): focus moves
// to the first safe control on open (cancel, or the first field), Esc closes it, focus returns to the trigger,
// and confirming waits for the fields to be valid.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { Button } from "./Button";
import { ConfirmDialog, type ConfirmDialogProps } from "./ConfirmDialog";
import { DialogTrigger } from "./Dialog";
import { DurationSelect } from "./DurationSelect";
import { ReasonField } from "./ReasonField";

// Stories may spell out English; apps pass react-intl messages instead (web-client/README.md A9).
const formatRemaining = (remaining: number) =>
  remaining < 0 ? `${String(-remaining)} characters too many` : `${String(remaining)} characters left`;
const day = 86_400;
const formatDuration = (seconds: number) => (seconds === day ? "1 day" : `${String(seconds / day)} days`);

const meta = {
  title: "Overlays/ConfirmDialog",
  component: ConfirmDialog,
  // The modal covers the story frame, so give it room.
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Suspend example.com?",
    description:
      "Deliveries to and from example.com stop and follows between its accounts and yours end. Lifting the suspension does not bring the follows back.",
    confirmLabel: "Suspend",
    cancelLabel: "Cancel",
    onConfirm: fn(),
  },
  render: (args) => (
    <DialogTrigger>
      <Button variant="danger">Suspend</Button>
      <ConfirmDialog {...args} />
    </DialogTrigger>
  ),
} satisfies Meta<typeof ConfirmDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Focus starts on Cancel, never on the destructive button; Esc closes the dialog and focus returns to the trigger. */
export const FocusAndEscape: Story = {
  play: async ({ args, canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Suspend" });
    await userEvent.click(trigger);
    const dialog = await canvas.findByRole("alertdialog", { name: "Suspend example.com?" });
    await expect(dialog).toHaveAccessibleDescription(/Deliveries to and from example.com stop/);
    await waitFor(() => expect(canvas.getByRole("button", { name: "Cancel" })).toHaveFocus());

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("alertdialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    await expect(args.onConfirm).not.toHaveBeenCalled();
  },
};

/** Cancel closes the dialog and returns focus too; the confirm button runs the action. */
export const CancelAndConfirm: Story = {
  args: {
    onConfirm: fn((close: () => void) => {
      close();
    }),
  },
  play: async ({ args, canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Suspend" });
    await userEvent.click(trigger);
    await userEvent.click(await canvas.findByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(canvas.queryByRole("alertdialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    await userEvent.click(trigger);
    const dialog = await canvas.findByRole("alertdialog");
    // The trigger and the confirm button share the name; the one in the dialog is the second.
    const confirm = canvas.getAllByRole("button", { name: "Suspend" }).find((button) => dialog.contains(button));
    if (confirm === undefined) throw new Error("no confirm button");
    await userEvent.click(confirm);
    await expect(args.onConfirm).toHaveBeenCalledOnce();
    await waitFor(() => expect(canvas.queryByRole("alertdialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

function SuspendWithFields({ open = false, ...args }: ConfirmDialogProps & { open?: boolean }) {
  const [reason, setReason] = useState("");
  const [duration, setDuration] = useState<number | null>(7 * day);
  return (
    <DialogTrigger defaultOpen={open}>
      <Button variant="danger">Suspend</Button>
      <ConfirmDialog {...args}>
        <ReasonField
          label="Reason"
          description="Shown to the account's owner."
          isRequired
          value={reason}
          onChange={setReason}
          characterLimit={5000}
          formatRemaining={formatRemaining}
          errorMessage="Give a reason."
        />
        <DurationSelect
          label="Duration"
          durations={[day, 7 * day, 30 * day]}
          formatDuration={formatDuration}
          indefiniteLabel="Indefinitely"
          value={duration}
          onChange={setDuration}
        />
      </ConfirmDialog>
    </DialogTrigger>
  );
}

/** With fields, focus starts on the first one; an empty required reason keeps the action from running. */
export const WithFields: Story = {
  args: { title: "Suspend @alice?", description: "Alice can no longer sign in. You can lift the suspension later." },
  render: (args) => <SuspendWithFields {...args} />,
  play: async ({ args, canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Suspend" }));
    const dialog = await canvas.findByRole("alertdialog", { name: "Suspend @alice?" });
    const reason = canvas.getByRole("textbox", { name: "Reason" });
    await waitFor(() => expect(reason).toHaveFocus());

    const confirm = canvas.getAllByRole("button", { name: "Suspend" }).find((button) => dialog.contains(button));
    if (confirm === undefined) throw new Error("no confirm button");
    await userEvent.click(confirm);
    await expect(args.onConfirm).not.toHaveBeenCalled();
    await expect(reason).toBeInvalid();

    await userEvent.type(reason, "Spam");
    await userEvent.click(confirm);
    await expect(args.onConfirm).toHaveBeenCalledOnce();
  },
};

/** The fields open, for the screenshots and axe of the filled-in dialog. */
export const WithFieldsOpen: Story = {
  args: { title: "Suspend @alice?", description: "Alice can no longer sign in. You can lift the suspension later." },
  render: (args) => <SuspendWithFields {...args} open />,
};

export const Open: Story = {
  render: (args) => (
    <DialogTrigger defaultOpen>
      <Button variant="danger">Suspend</Button>
      <ConfirmDialog {...args} />
    </DialogTrigger>
  ),
};

/** While the action runs, the confirm button waits. */
export const Pending: Story = { ...Open, args: { isPending: true } };

/** A failed action stays in the dialog, so that it can be tried again. */
export const Failed: Story = {
  ...Open,
  args: { errorMessage: "Could not suspend example.com: the server did not answer. Try again." },
  play: async ({ canvas }) => {
    await expect(await canvas.findByRole("alert")).toHaveTextContent("Could not suspend example.com");
  },
};

/** A primary confirmation, for actions that are not destructive. */
export const Primary: Story = {
  ...Open,
  args: {
    tone: "primary",
    title: "Resume deliveries to example.com?",
    description: "Queued activities are sent again.",
    confirmLabel: "Resume",
  },
};
