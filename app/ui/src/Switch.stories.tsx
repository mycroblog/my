// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
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

/** The native input lies over the track, so that a press on the switch itself reaches it (src/ToggleInput.ts). */
export const InputOverTrack: Story = {
  tags: ["no-screenshot"],
  play: async ({ canvas, userEvent }) => {
    const input = canvas.getByRole("switch");
    const track = input.closest("label")?.querySelector(":scope > span:not(:has(input))")?.getBoundingClientRect();
    if (track === undefined) throw new Error("the switch has no track");
    await expect(document.elementFromPoint(track.x + track.width / 2, track.y + track.height / 2)).toBe(input);
    await userEvent.click(input);
    await expect(input).toBeChecked();
  },
};
