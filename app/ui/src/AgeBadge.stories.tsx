// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AgeBadge } from "./AgeBadge";

const meta = {
  title: "Display/AgeBadge",
  component: AgeBadge,
  args: { label: "Waiting 2 hr.", dateTime: "2026-10-16T07:00:00Z", title: "Friday, 16 October 2026 at 07:00" },
} satisfies Meta<typeof AgeBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Waiting 2 hr.")).toBeVisible();
  },
};

/** The three levels: the words say the wait, the tone rises with it. */
export const Levels: Story = {
  render: (args) => (
    <div className="flex flex-wrap items-center gap-2">
      <AgeBadge {...args} label="Waiting 2 hr." level="fresh" />
      <AgeBadge {...args} label="Waiting 1 day" level="aging" />
      <AgeBadge {...args} label="Waiting 4 days" level="overdue" />
    </div>
  ),
};
