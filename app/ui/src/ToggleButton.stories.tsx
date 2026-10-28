// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn } from "storybook/test";
import { StarIcon } from "./Icons";
import { ToggleButton, type ToggleButtonProps } from "./ToggleButton";

/** A toggle with its own state, as an app controls it with `isSelected` / `onChange`. */
function Controlled({
  defaultSelected = false,
  count,
  ...props
}: Omit<ToggleButtonProps, "children"> & { count?: number }) {
  const [isSelected, setIsSelected] = useState(defaultSelected);
  return (
    <ToggleButton
      {...props}
      isSelected={isSelected}
      onChange={(next) => {
        setIsSelected(next);
        props.onChange?.(next);
      }}
    >
      <StarIcon filled={isSelected} />
      {count !== undefined && <span aria-hidden="true">{count + (isSelected ? 1 : 0)}</span>}
    </ToggleButton>
  );
}

const meta = {
  title: "Forms/ToggleButton",
  component: Controlled,
  args: { "aria-label": "Favourite", onChange: fn() },
} satisfies Meta<typeof Controlled>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Pressing it flips `aria-pressed`; the name stays the same. */
export const Default: Story = {
  play: async ({ canvas, userEvent, args }) => {
    const button = canvas.getByRole("button", { name: "Favourite" });
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(button);
    await expect(button).toHaveAttribute("aria-pressed", "true");
    await expect(args.onChange).toHaveBeenCalledWith(true);
    // The screenshot follows the play function: leave no hover state behind, which would make it vary.
    await userEvent.unhover(button);
    await userEvent.keyboard(" ");
    await expect(button).toHaveAttribute("aria-pressed", "false");
  },
};

export const Selected: Story = { args: { defaultSelected: true } };

/** Every tone, off and on, with a count, and disabled. */
export const Matrix: Story = {
  render: (args) => (
    <div className="flex flex-col gap-3">
      {(["accent", "success", "warning"] as const).map((tone) => (
        <div key={tone} className="flex flex-wrap items-center gap-2">
          <Controlled {...args} aria-label={`${tone} off`} tone={tone} count={3} />
          <Controlled {...args} aria-label={`${tone} on`} tone={tone} count={3} defaultSelected />
          <Controlled {...args} aria-label={`${tone} disabled`} tone={tone} isDisabled />
        </div>
      ))}
    </div>
  ),
};
