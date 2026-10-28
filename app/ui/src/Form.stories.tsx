// SPDX-License-Identifier: AGPL-3.0-only
// Form stories: fields laid out as a stack, and an error from the server shown on its field through `validationErrors`.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn } from "storybook/test";
import { Button } from "./Button";
import { Form } from "./Form";
import { TextField } from "./TextField";

const meta = {
  title: "Forms/Form",
  component: Form,
  args: {
    "aria-label": "New role",
    gap: "4",
    align: "start",
    width: "sm",
    onSubmit: fn((event: { preventDefault: () => void }) => {
      event.preventDefault();
    }),
    children: (
      <>
        <TextField name="key" label="Key" isRequired />
        <TextField
          name="name"
          label="Name of a role whose name is much longer than the room it has"
          description="Shown on the badge."
        />
        <Button type="submit" variant="primary">
          Create
        </Button>
      </>
    ),
  },
} satisfies Meta<typeof Form>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Fields one above another with the gap of fields, at most `max-w-sm` wide. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("form", { name: "New role" })).toBeVisible();
  },
};

/** An error the server returned for a field, shown on that field. */
export const ServerError: Story = {
  args: { validationErrors: { key: "A role with this key already exists." } },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("textbox", { name: "Key" })).toHaveAttribute("aria-invalid", "true");
    await expect(canvas.getByText("A role with this key already exists.")).toBeVisible();
  },
};
