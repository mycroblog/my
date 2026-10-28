// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent } from "storybook/test";
import { ActionCount } from "./ActionCount";
import { IconToggleButton } from "./IconButton";
import { StarIcon } from "./Icons";

const meta = {
  title: "Display/ActionCount",
  component: ActionCount,
  args: { value: 12 },
} satisfies Meta<typeof ActionCount>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** A large count is compact in the locale of the page. */
export const Compact: Story = { args: { value: 12_345 } };

/** Beside a toggle of a post's action bar: the count changes, and slides in, when the person turns it on. */
export const InAToggle: Story = {
  render: function Render(args) {
    const [on, setOn] = useState(false);
    return (
      <IconToggleButton label="Favourite" tone="warning" isSelected={on} onChange={setOn}>
        <StarIcon filled={on} />
        <ActionCount value={args.value + (on ? 1 : 0)} />
      </IconToggleButton>
    );
  },
  play: async ({ canvas }) => {
    const button = canvas.getByRole("button", { name: "Favourite" });
    await userEvent.click(button);
    await expect(button).toHaveTextContent("13");
  },
};
