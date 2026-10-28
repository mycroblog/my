// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Checkbox } from "./Checkbox";

const meta = {
  title: "Forms/Checkbox",
  component: Checkbox,
  args: { children: "Mark media as sensitive" },
} satisfies Meta<typeof Checkbox>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Unchecked: Story = {};
export const Checked: Story = { args: { defaultSelected: true } };
export const Indeterminate: Story = { args: { isIndeterminate: true, children: "Select all" } };
export const WithDescription: Story = { args: { description: "Media is hidden behind a warning until opened." } };

export const Invalid: Story = {
  args: { children: "I accept the server rules", isInvalid: true, errorMessage: "Accept the rules to continue." },
};

export const InvalidChecked: Story = {
  args: { children: "I accept the server rules", isInvalid: true, defaultSelected: true },
};

export const Disabled: Story = {
  render: (args) => (
    <div className="flex flex-col gap-2">
      <Checkbox {...args} isDisabled />
      <Checkbox {...args} isDisabled defaultSelected />
    </div>
  ),
};
