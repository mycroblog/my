// SPDX-License-Identifier: AGPL-3.0-only

// The search field as the reader uses it (#3126): a searchbox named by its label, which hands on each word as it is
// written; Clear at its end empties it, hands on "" and leaves the focus in it; Escape empties it too; and while
// something is looked up afar it is busy, with no Clear. The words are placeholders.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { SearchField } from "./search-field";

const meta = {
  title: "Search field states",
  args: { onChange: fn() },
  render: (args) => (
    <SearchField
      id="search"
      label="Search"
      hint="People by name or handle, and hashtags"
      clearLabel="Clear"
      defaultValue={args.defaultValue}
      busy={args.busy}
      onChange={args.onChange}
    />
  ),
} satisfies Meta<{ onChange: (value: string) => void; defaultValue?: string; busy?: boolean }>;
export default meta;

type Story = StoryObj<typeof meta>;

/** The searchbox, and the story's canvas. */
function field(context: StoryContext) {
  const canvas = within(context.canvasElement);
  return { canvas, box: canvas.getByRole("searchbox", { name: "Search" }) };
}

/** Each word written is handed on as the field's words. */
export const HandsOnTheWords: Story = {
  play: async (context) => {
    const { box } = field(context);
    await userEvent.type(box, "sam");
    await waitFor(() => expect(context.args.onChange).toHaveBeenLastCalledWith("sam"));
    await expect(box).toHaveValue("sam");
  },
};

/** Clear empties the field, hands on "", and leaves the focus in the field. */
export const ClearEmpties: Story = {
  args: { defaultValue: "sam" },
  play: async (context) => {
    const { canvas, box } = field(context);
    await expect(box).toHaveValue("sam");
    await userEvent.click(canvas.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(box).toHaveValue(""));
    await expect(context.args.onChange).toHaveBeenLastCalledWith("");
    await expect(box).toHaveFocus();
  },
};

/** Escape in the field empties it and hands on "". */
export const EscapeEmpties: Story = {
  args: { defaultValue: "sam" },
  play: async (context) => {
    const { box } = field(context);
    await userEvent.click(box);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(box).toHaveValue(""));
    await expect(context.args.onChange).toHaveBeenLastCalledWith("");
  },
};

/** While an account is looked up afar the field is busy, and no Clear stands at its end. */
export const BusyWhileLookingUp: Story = {
  args: { defaultValue: "@kiri@far.example", busy: true },
  play: async (context) => {
    const { canvas, box } = field(context);
    await waitFor(() => expect(box.closest('[aria-busy="true"]')).not.toBeNull());
    await expect(canvas.queryByRole("button", { name: "Clear" })).toBeNull();
  },
};

/** Not busy, the same words show Clear and nothing busy. */
export const NotBusy: Story = {
  args: { defaultValue: "@kiri@far.example" },
  play: async (context) => {
    const { canvas, box } = field(context);
    await expect(canvas.getByRole("button", { name: "Clear" })).toBeVisible();
    await expect(box.closest('[aria-busy="true"]')).toBeNull();
  },
};
