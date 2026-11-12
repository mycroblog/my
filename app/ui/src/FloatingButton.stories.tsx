// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FloatingButton } from "./FloatingButton";
import { PlusIcon } from "./Icons";

const meta = {
  title: "Compose/FloatingButton",
  component: FloatingButton,
  args: { label: "New post", icon: <PlusIcon />, onPress: fn() },
} satisfies Meta<typeof FloatingButton>;

export default meta;
type Story = StoryObj<typeof meta>;

/** With its words. */
export const Labelled: Story = {
  play: async ({ canvasElement, args }) => {
    const button = within(canvasElement).getByRole("button", { name: "New post" });
    await userEvent.click(button);
    await expect(args.onPress).toHaveBeenCalled();
    // Back to the resting state.
    await userEvent.unhover(button);
    button.blur();
  },
};

/** Only the icon: named by `aria-label`, with the same words as a tooltip. */
export const IconOnly: Story = { args: { iconOnly: true } };
