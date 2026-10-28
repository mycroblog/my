// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { BreakdownBar } from "./BreakdownBar";

const meta = {
  title: "Data/BreakdownBar",
  component: BreakdownBar,
  args: {
    label: "Costs in October",
    items: [
      { id: "hosting", name: "Hosting", value: 30, valueText: "$30.00" },
      { id: "storage", name: "Storage", value: 12, valueText: "$12.00" },
      { id: "email", name: "Email", value: 6, valueText: "$6.00" },
      { id: "other", name: "Other", value: 2, valueText: "$2.00" },
    ],
  },
  render: (args) => (
    <div className="rounded-lg bg-surface-raised p-4">
      <BreakdownBar {...args} />
    </div>
  ),
} satisfies Meta<typeof BreakdownBar>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The legend says each part's name, value and share in text; the bar only draws them. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("list", { name: "Costs in October" })).toBeVisible();
    await expect(canvas.getAllByRole("listitem")).toHaveLength(4);
    await expect(canvas.getByText("$12.00")).toBeVisible();
  },
};

/** Forced colors keep the borders between the segments; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
};

/** Without an amount, the bar is an empty track and every share is 0%. */
export const Empty: Story = {
  args: {
    items: [
      { id: "hosting", name: "Hosting", value: 0, valueText: "$0.00" },
      { id: "storage", name: "Storage", value: 0, valueText: "$0.00" },
    ],
  },
};

/** A single part fills the bar. */
export const OnePoint: Story = {
  args: { items: [{ id: "hosting", name: "Hosting", value: 30, valueText: "$30.00" }] },
};
