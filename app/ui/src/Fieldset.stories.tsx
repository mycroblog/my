// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Checkbox } from "./Checkbox";
import { Fieldset } from "./Fieldset";
import { TextField } from "./TextField";

const meta = {
  title: "Forms/Fieldset",
  component: Fieldset,
  args: {
    legend: "Poll",
    children: (
      <>
        <TextField label="Choice 1" />
        <TextField label="Choice 2" />
        <Checkbox>Allow several choices</Checkbox>
      </>
    ),
  },
} satisfies Meta<typeof Fieldset>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The group is named by its legend. */
export const Default: Story = {
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("group", { name: "Poll" })).toBeInTheDocument();
  },
};

/** A long legend wraps inside the frame. */
export const LongLegend: Story = {
  args: { legend: "A poll whose caption is much longer than the room the frame leaves for it on a narrow screen" },
};
