// SPDX-License-Identifier: AGPL-3.0-only
// Dialog stories. The play functions are the dialog's behaviour tests (they run in Chromium with
// `scripts/task.sh ui:test`): focus moves in on open, Tab stays inside, Esc, the close buttons and the scrim close it,
// and focus returns to the trigger.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { Dialog, type DialogProps, DialogTrigger } from "./Dialog";

const meta = {
  title: "Overlays/Dialog",
  component: Dialog,
  // The modal covers the story frame, so give it room.
  decorators: [
    (Story) => (
      <div className="min-h-136">
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Edit profile",
    closeLabel: "Close",
    children: ({ close }) => (
      <div className="flex flex-col gap-4">
        <p className="text-fg-muted">Your display name and bio are shown on your profile and next to your posts.</p>
        <div className="flex flex-wrap justify-end gap-2">
          <Button onPress={close}>Cancel</Button>
          <Button variant="primary" onPress={close}>
            Save
          </Button>
        </div>
      </div>
    ),
  },
  render: (args) => (
    <DialogTrigger>
      <Button>Edit profile</Button>
      <Dialog {...args} />
    </DialogTrigger>
  ),
} satisfies Meta<typeof Dialog>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Opens with the trigger, traps Tab, closes on Esc and returns focus to the trigger. */
export const FocusTrapAndReturn: Story = {
  play: async ({ canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Edit profile" });
    await userEvent.click(trigger);
    const dialog = await canvas.findByRole("dialog", { name: "Edit profile" });
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement));

    // Three focusable elements (×, Cancel, Save): Tab and Shift+Tab wrap around inside the dialog.
    for (let i = 0; i < 4; i++) {
      await userEvent.tab();
      await expect(dialog).toContainElement(document.activeElement as HTMLElement);
    }
    await userEvent.tab({ shift: true });
    await expect(dialog).toContainElement(document.activeElement as HTMLElement);

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

/** The × button and the actions close the dialog and return focus too. */
export const CloseButtonReturnsFocus: Story = {
  play: async ({ canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Edit profile" });
    await userEvent.click(trigger);
    await userEvent.click(await canvas.findByRole("button", { name: "Close" }));
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    await userEvent.keyboard("{Enter}");
    await userEvent.click(await canvas.findByRole("button", { name: "Save" }));
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};

function renderOpen(args: DialogProps) {
  return (
    <DialogTrigger defaultOpen>
      <Button>Edit profile</Button>
      <Dialog {...args} />
    </DialogTrigger>
  );
}

export const Open: Story = { render: renderOpen };

export const Small: Story = { ...Open, args: { size: "sm", title: "Sign out?" } };

export const Large: Story = { ...Open, args: { size: "lg" } };

/** A confirmation without a × button: Cancel answers it, and Esc and a press on the scrim cancel too. */
export const AlertDialog: Story = {
  args: {
    role: "alertdialog",
    title: "Delete this post?",
    children: ({ close }) => (
      <div className="flex flex-col gap-4">
        <p>This cannot be undone. Replies to the post stay, without it.</p>
        <div className="flex flex-wrap justify-end gap-2">
          <Button onPress={close}>Cancel</Button>
          <Button variant="danger" onPress={close}>
            Delete
          </Button>
        </div>
      </div>
    ),
  },
  // Leaves out the meta's closeLabel.
  render: ({ title, children }) => (
    <DialogTrigger defaultOpen>
      <Button>Delete</Button>
      <Dialog role="alertdialog" closeLabel={null} title={title}>
        {children}
      </Dialog>
    </DialogTrigger>
  ),
  play: async ({ canvas }) => {
    const dialog = await canvas.findByRole("alertdialog", { name: "Delete this post?" });
    await waitFor(() => expect(dialog).toBeVisible());
    await expect(canvas.queryByRole("button", { name: "Close" })).not.toBeInTheDocument();
  },
};

/** Long content scrolls inside the panel; the header stays put. */
export const LongContent: Story = {
  args: {
    title: "Terms of service",
    children: ({ close }) => (
      <div className="flex flex-col gap-3">
        {Array.from({ length: 12 }, (_, i) => `Clause ${String(i + 1)}`).map((clause) => (
          <p key={clause}>
            {clause}: the instance is run by volunteers, who may remove content that breaks its rules and may stop the
            service at any time.
          </p>
        ))}
        <div className="flex justify-end">
          <Button variant="primary" onPress={close}>
            Accept
          </Button>
        </div>
      </div>
    ),
  },
  render: renderOpen,
};

/** A press on the scrim closes the dialog (isDismissable is on by default). */
export const Dismissable: Story = {
  play: async ({ canvas, userEvent }) => {
    const trigger = canvas.getByRole("button", { name: "Edit profile" });
    await userEvent.click(trigger);
    const dialog = await canvas.findByRole("dialog");
    // The scrim is the panel's grandparent (overlay > modal > dialog).
    const scrim = dialog.parentElement?.parentElement;
    if (scrim == null) throw new Error("no scrim");
    await userEvent.pointer({ keys: "[MouseLeft]", target: scrim, coords: { clientX: 4, clientY: 4 } });
    await waitFor(() => expect(canvas.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  },
};
