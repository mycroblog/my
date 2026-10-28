// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Sparkline } from "./Sparkline";

const meta = {
  title: "Data/Sparkline",
  component: Sparkline,
  args: { values: [4, 6, 5, 9, 7, 12, 10] },
  // A row of a list: the number in text beside its line.
  render: (args) => (
    <p className="flex items-center gap-3 rounded-lg bg-surface-raised p-4">
      <span className="flex-1">Deliveries queued</span>
      <Sparkline {...args} />
      <span className="tabular-nums">10</span>
    </p>
  ),
} satisfies Meta<typeof Sparkline>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The line is decorative: the row reads as its name and its number. */
export const Default: Story = {
  play: async ({ canvasElement }) => {
    const line = canvasElement.querySelector("svg");
    await expect(line?.closest("[aria-hidden='true']")).not.toBeNull();
  },
};

/** The three series colors, and a gap where a period has no value. */
export const Series: Story = {
  render: () => (
    <div className="flex flex-col gap-2 rounded-lg bg-surface-raised p-4">
      <Sparkline values={[4, 6, 5, 9, 7, 12, 10]} series={1} />
      <Sparkline values={[9, 8, null, 6, 7, 3, 2]} series={2} />
      <Sparkline values={[1, 1, 1, 1, 1, 1, 1]} series={3} />
    </div>
  ),
};

/** Forced colors draw the line in CanvasText and the newest point in Highlight; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
};

/** Without a value the box stays empty and keeps its room in the row. */
export const Empty: Story = {
  args: { values: [] },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("svg")).toBeNull();
  },
};

/** A single value is a dot in the middle. */
export const OnePoint: Story = {
  args: { values: [10] },
};
