// SPDX-License-Identifier: AGPL-3.0-only
// DialogActions stories: the buttons at the end of a dialog's form, and long labels that wrap onto a second line.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { Dialog } from "./Dialog";
import { DialogActions } from "./DialogActions";
import { Stack } from "./Stack";
import { InlineError } from "./Text";

const meta = {
  title: "Overlays/DialogActions",
  component: DialogActions,
  args: {
    children: (
      <>
        <Button>Cancel</Button>
        <Button variant="primary">Save</Button>
      </>
    ),
  },
} satisfies Meta<typeof DialogActions>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Cancel first, the action last, at the inline end. */
export const Default: Story = {};

/** Labels too long for one row wrap, still at the inline end. */
export const LongLabels: Story = {
  args: {
    children: (
      <>
        <Button>Keep editing the announcement</Button>
        <Button variant="primary">Publish the announcement to everyone now</Button>
      </>
    ),
  },
};

/**
 * In a dialog's form, under the fields and the error of the last attempt.
 */
export const InDialog: Story = {
  render: () => (
    <Dialog title="Edit the role" closeLabel="Close" defaultOpen isDismissable={false}>
      {({ close }) => (
        <Stack
          as="form"
          gap="4"
          onSubmit={(event) => {
            event.preventDefault();
          }}
        >
          <InlineError alert>Couldn&apos;t save the role: the key is already in use.</InlineError>
          <DialogActions>
            <Button onPress={close}>Cancel</Button>
            <Button type="submit" variant="primary">
              Save
            </Button>
          </DialogActions>
        </Stack>
      )}
    </Dialog>
  ),
  play: async ({ canvas }) => {
    const dialog = await canvas.findByRole("dialog", { name: "Edit the role" });
    await waitFor(() => expect(dialog).toBeVisible());
  },
};
