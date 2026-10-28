// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Sparkline } from "./Sparkline";
import { StatTile } from "./StatTile";

const meta = {
  title: "Data/StatTile",
  component: StatTile,
  args: {
    label: "Open reports",
    value: "12",
    change: { direction: "up", text: "+3", period: "from yesterday", tone: "negative" },
  },
  render: (args) => (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-3">
      <StatTile {...args} />
    </div>
  ),
} satisfies Meta<typeof StatTile>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The change shows by its arrow and its text, never by color alone. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Open reports")).toBeVisible();
    await expect(canvas.getByText("+3")).toBeVisible();
    await expect(canvas.getByText("from yesterday")).toBeVisible();
  },
};

/** A row of tiles: a fall, states that need attention, no change, and a tile with a sparkline. */
export const Summary: Story = {
  render: () => (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-3">
      <StatTile
        label="Failed jobs"
        value="4"
        status="danger"
        statusLabel="Failing"
        change={{ direction: "up", text: "+4", period: "in the last hour", tone: "negative" }}
      />
      <StatTile
        label="Deliveries queued"
        value="1,204"
        status="warning"
        statusLabel="Slow"
        change={{ direction: "down", text: "−18%", period: "from an hour ago", tone: "positive" }}
      >
        <Sparkline values={[1800, 1650, 1500, 1460, 1300, 1204]} />
      </StatTile>
      <StatTile
        label="Costs this month"
        value="$56.78"
        change={{ direction: "flat", text: "±0%", period: "from last month" }}
      />
      <StatTile label="Accounts" value="1,234,567" />
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Failing")).toBeVisible();
    await expect(canvas.getByText("Slow")).toBeVisible();
  },
};

/** Forced colors keep the arrow, the text and an outline around the state; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
  args: { status: "warning", statusLabel: "Needs review" },
};

/** Without a change from the period before (no data yet), the tile has its name and value only. */
export const Empty: Story = {
  render: () => (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-3">
      <StatTile label="Open reports" value="0" />
    </div>
  ),
};

/** The first period: a value and no comparison. */
export const OnePoint: Story = {
  args: { label: "Posts today", value: "1", change: { direction: "flat", text: "New", period: "first day" } },
};
