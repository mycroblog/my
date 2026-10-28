// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { SeverityMark } from "./SeverityMark";

const meta = {
  title: "Display/SeverityMark",
  component: SeverityMark,
  args: { level: 5, max: 10, label: "Severity 5 of 10" },
} satisfies Meta<typeof SeverityMark>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Severity 5 of 10")).toBeInTheDocument();
  },
};

/** The lightest, a middle and the heaviest step; the heaviest cannot be undone. */
export const Scale: Story = {
  render: () => (
    <div className="flex flex-col items-start gap-3">
      <SeverityMark level={1} max={10} label="Severity 1 of 10" />
      <SeverityMark level={5} max={10} label="Severity 5 of 10" />
      <SeverityMark level={10} max={10} label="Severity 10 of 10" irreversibleLabel="Cannot be undone" />
    </div>
  ),
};
