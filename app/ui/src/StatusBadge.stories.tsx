// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AboutIcon, CheckIcon, MicrophoneOffIcon, PrivateVisibilityIcon, WarningIcon } from "./Icons";
import { StatusBadge, VisibilityMark } from "./StatusBadge";

const meta = {
  title: "Display/StatusBadge",
  component: StatusBadge,
  args: { icon: <CheckIcon />, label: "Connected", tone: "success" },
} satisfies Meta<typeof StatusBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByText("Connected")).toBeVisible();
  },
};

/** Every tone, each with its own icon: the icon and the label say the state, the tone only adds to them. */
export const Tones: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <StatusBadge icon={<PrivateVisibilityIcon />} label="Private" tone="neutral" />
      <StatusBadge icon={<AboutIcon />} label="Open" tone="accent" />
      <StatusBadge icon={<CheckIcon />} label="Connected" tone="success" />
      <StatusBadge icon={<MicrophoneOffIcon />} label="Muted" tone="warning" />
      <StatusBadge icon={<WarningIcon filled />} label="Suspended" tone="danger" />
    </div>
  ),
};

/** The visibility of a post, one icon per level (ADR-0017), as PostCard shows it under the time. */
export const Visibilities: Story = {
  render: () => (
    <div className="flex flex-col items-start gap-2">
      <VisibilityMark visibility="public" label="Public" />
      <VisibilityMark visibility="local" label="This server" />
      <VisibilityMark visibility="unlisted" label="Unlisted" />
      <VisibilityMark visibility="private" label="Followers" />
      <VisibilityMark visibility="direct" label="Mentioned people" />
    </div>
  ),
};
