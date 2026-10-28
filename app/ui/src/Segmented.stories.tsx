// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { Key } from "react-aria-components";
import { expect, fn } from "storybook/test";
import { Segmented, SegmentedItem, type SegmentedProps } from "./Segmented";

const layouts = [
  { id: "lg", label: "lg · 3 columns" },
  { id: "md", label: "md · 2 columns" },
  { id: "sm", label: "sm · 1 column" },
] as const;

/** A switcher that keeps its own value, as an app controls it with `value` / `onChange`. */
function Layouts({ onChange, ...props }: Omit<SegmentedProps, "children" | "value">) {
  const [value, setValue] = useState<Key>("lg");
  return (
    <Segmented
      {...props}
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange?.(next);
      }}
    >
      {layouts.map(({ id, label }) => (
        <SegmentedItem key={id} id={id}>
          {label}
        </SegmentedItem>
      ))}
    </Segmented>
  );
}

const meta = {
  title: "Navigation/Segmented",
  component: Layouts,
  args: { "aria-label": "Layout", onChange: fn() },
} satisfies Meta<typeof Layouts>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("radiogroup", { name: "Layout" })).toBeVisible();
    await expect(canvas.getByRole("radio", { name: "lg · 3 columns" })).toBeChecked();
  },
};

/** Tab enters the group on the selected segment, the arrow keys select the next and the previous in the writing direction, and Tab leaves. */
export const Keyboard: Story = {
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.tab();
    await expect(canvas.getByRole("radio", { name: "lg · 3 columns" })).toHaveFocus();
    // The next segment lies in the writing direction: to the left in right-to-left text.
    await userEvent.keyboard(document.documentElement.dir === "rtl" ? "{ArrowLeft}" : "{ArrowRight}");
    const md = canvas.getByRole("radio", { name: "md · 2 columns" });
    await expect(md).toHaveFocus();
    await userEvent.keyboard(" ");
    await expect(md).toBeChecked();
    await expect(args.onChange).toHaveBeenLastCalledWith("md");
    await expect(canvas.getByRole("radio", { name: "lg · 3 columns" })).not.toBeChecked();
    // Pressing the selected segment again does not clear the choice.
    await userEvent.keyboard(" ");
    await expect(md).toBeChecked();
    await userEvent.tab();
    await expect(md).not.toHaveFocus();
  },
};

export const WithPointer: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("radio", { name: "sm · 1 column" }));
    await expect(canvas.getByRole("radio", { name: "sm · 1 column" })).toBeChecked();
  },
};

export const Disabled: Story = {
  args: { isDisabled: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("radio", { name: "md · 2 columns" })).toBeDisabled();
  },
};

/** Forced colors fill the selected segment with Highlight and its text with HighlightText; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
};
