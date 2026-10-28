// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Radio, RadioGroup } from "./RadioGroup";

const meta = {
  title: "Forms/RadioGroup",
  component: RadioGroup,
  args: {
    label: "Visibility",
    defaultValue: "public",
    children: (
      <>
        <Radio value="public">Public</Radio>
        <Radio value="unlisted">Unlisted</Radio>
        <Radio value="followers">Followers only</Radio>
        <Radio value="direct">Mentioned people only</Radio>
      </>
    ),
  },
} satisfies Meta<typeof RadioGroup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Vertical: Story = {};

export const Horizontal: Story = { args: { orientation: "horizontal" } };

export const WithDescriptions: Story = {
  args: {
    description: "Who can see this post.",
    children: (
      <>
        <Radio value="public" description="Anyone, on any server.">
          Public
        </Radio>
        <Radio value="local" description="People on this server and your followers.">
          Local
        </Radio>
        <Radio value="direct" description="Only the people you mention." isDisabled>
          Mentioned people only
        </Radio>
      </>
    ),
  },
};

export const Invalid: Story = {
  args: { defaultValue: null, isInvalid: true, errorMessage: "Choose who can see this post." },
};

export const Disabled: Story = { args: { isDisabled: true } };
