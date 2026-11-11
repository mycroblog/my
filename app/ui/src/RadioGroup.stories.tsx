// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
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

/** The native input lies over the circle, so that a press on the radio itself reaches it (src/ToggleInput.ts). */
export const InputOverCircle: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas, userEvent }) => {
    const input = canvas.getByRole("radio", { name: "Unlisted" });
    const circle = input.closest("label")?.querySelector(":scope > span:not(:has(input))")?.getBoundingClientRect();
    if (circle === undefined) throw new Error("the radio has no circle");
    await expect(document.elementFromPoint(circle.x + circle.width / 2, circle.y + circle.height / 2)).toBe(input);
    await userEvent.click(input);
    await expect(input).toBeChecked();
  },
};
