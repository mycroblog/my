// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Switch } from "./Switch";

const meta = {
  title: "Forms/Switch",
  component: Switch,
  args: { children: "Autoplay animated images" },
} satisfies Meta<typeof Switch>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Off: Story = {};
export const On: Story = { args: { defaultSelected: true } };
export const WithDescription: Story = {
  args: { description: "GIFs and animated avatars play without a click.", defaultSelected: true },
};

export const Disabled: Story = {
  render: (args) => (
    <div className="flex flex-col gap-2">
      <Switch {...args} isDisabled />
      <Switch {...args} isDisabled defaultSelected />
    </div>
  ),
};
