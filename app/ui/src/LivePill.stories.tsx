// SPDX-License-Identifier: AGPL-3.0-only
// LivePill stories: the pill of an open room, in English and in a longer word.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { LivePill } from "./LivePill";

const meta = {
  title: "Voice/LivePill",
  component: LivePill,
  args: { label: "Live" },
} satisfies Meta<typeof LivePill>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Live")).toBeVisible();
  },
};

/** While the user's call reconnects, the pill is a dashed outline. */
export const Interrupted: Story = {
  args: { interrupted: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Live")).toHaveAttribute("data-interrupted");
  },
};

/** The pill is as wide as its word. */
export const LongLabel: Story = {
  args: { label: "En direct" },
};
