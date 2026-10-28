// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { ProgressMeter } from "./ProgressMeter";

const meta = {
  title: "Data/ProgressMeter",
  component: ProgressMeter,
  args: { kind: "progress", label: "Deliveries sent", value: 1234, maxValue: 5000, valueText: "1,234 / 5,000 servers" },
  render: (args) => (
    <div className="rounded-lg bg-surface-raised p-4">
      <ProgressMeter {...args} />
    </div>
  ),
} satisfies Meta<typeof ProgressMeter>;

export default meta;
type Story = StoryObj<typeof meta>;

/** A progress bar named by its label, with its value in words as `aria-valuetext`. */
export const Default: Story = {
  play: async ({ canvas }) => {
    const bar = canvas.getByRole("progressbar", { name: "Deliveries sent" });
    await expect(bar).toHaveAttribute("aria-valuenow", "1234");
    await expect(bar).toHaveAttribute("aria-valuetext", "1,234 / 5,000 servers");
  },
};

/** Meters at each level; the words say the level too. */
export const Meters: Story = {
  render: () => (
    <div className="flex flex-col gap-4 rounded-lg bg-surface-raised p-4">
      <ProgressMeter kind="meter" label="Storage used" value={2.1} maxValue={5} valueText="2.1 / 5 GB" />
      <ProgressMeter
        kind="meter"
        label="Media cache"
        value={4.2}
        maxValue={5}
        valueText="4.2 / 5 GB, almost full"
        tone="warning"
      />
      <ProgressMeter kind="meter" label="Database" value={5} maxValue={5} valueText="5 / 5 GB, full" tone="danger" />
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("meter", { name: "Media cache" })).toHaveAttribute(
      "aria-valuetext",
      "4.2 / 5 GB, almost full",
    );
  },
};

/** A meter whose value has a pending part, drawn lighter at the end of the fill; the words say it too. */
export const Pending: Story = {
  args: {
    kind: "meter",
    label: "Custom emoji used",
    value: 4,
    maxValue: 10,
    pending: 2,
    valueText: "4 of 10, 2 of them pending",
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("meter", { name: "Custom emoji used" })).toHaveAttribute(
      "aria-valuetext",
      "4 of 10, 2 of them pending",
    );
  },
};

/** Forced colors draw the fill in Highlight inside a CanvasText border; captured so. */
export const ForcedColors: Story = {
  tags: ["forced-colors"],
};

/** Forced colors draw the pending part in GrayText after the Highlight fill; captured so. */
export const PendingForcedColors: Story = {
  ...Pending,
  tags: ["forced-colors"],
};

/** Nothing done yet: an empty track. */
export const Empty: Story = {
  args: { value: 0, valueText: "0 / 5,000 servers" },
};

/** A task of one step, done. */
export const OnePoint: Story = {
  args: { value: 1, maxValue: 1, valueText: "1 / 1 server" },
};
