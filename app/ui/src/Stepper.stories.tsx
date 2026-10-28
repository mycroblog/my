// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Stepper } from "./Stepper";

const meta = {
  title: "Display/Stepper",
  component: Stepper,
  args: { current: 2, total: 3, label: "Step 2 of 3" },
} satisfies Meta<typeof Stepper>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Step 2 of 3")).toBeVisible();
  },
};

export const First: Story = { args: { current: 1, total: 3, label: "Step 1 of 3" } };

export const Last: Story = { args: { current: 4, total: 4, label: "Step 4 of 4" } };
