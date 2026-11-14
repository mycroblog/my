// SPDX-License-Identifier: AGPL-3.0-only

// The icon button of a post's actions: Default, driven by its props in the Controls, and a story per state: with a
// count, with a count the button rounds, toggled on, and hovered and focused from the keyboard (shown at rest).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconButton } from "./icon-button";
import { icons, type IconName } from "./icons";
import { Preview, type PreviewState } from "./preview";

type Args = { icon: IconName; label: string; count: number; pressed: boolean; shown: "none" | PreviewState };

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Icon button",
  tags: ["themes"],
  args: { icon: "favourite", label: "Favourite", count: 0, pressed: false, shown: "none" },
  argTypes: {
    icon: { control: "select", options: Object.keys(icons) },
    label: { control: "text" },
    count: { control: "number", description: "How many have done it; zero shows no count" },
    pressed: { control: "boolean", description: "A toggle's state" },
    shown: {
      control: "select",
      options: ["none", "hover", "pressed", "focus-visible"],
      description: "A state shown at rest, as the pointer or the keyboard would put it",
    },
  },
  render: ({ icon, label, count, pressed, shown }) => (
    <Preview state={shown === "none" ? undefined : shown}>
      <IconButton icon={icon} label={label} count={count} pressed={pressed} />
    </Preview>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const WithCount: Story = { args: { icon: "reply", label: "Reply", count: 2 } };
export const LargeCount: Story = { args: { count: 12_840 } };
export const Pressed: Story = { args: { icon: "boost", label: "Boost", count: 1_234, pressed: true } };
export const Hovered: Story = { args: { count: 5, shown: "hover" } };
export const FocusVisible: Story = { args: { count: 5, shown: "focus-visible" } };
