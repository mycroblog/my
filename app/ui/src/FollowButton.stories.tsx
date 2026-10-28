// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent } from "storybook/test";
import { FollowButton, type FollowStatus } from "./FollowButton";

const meta = {
  title: "Actions/FollowButton",
  component: FollowButton,
  args: { status: "none", label: "Follow" },
} satisfies Meta<typeof FollowButton>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Not followed: the one main action, filled. */
export const NotFollowed: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Follow" })).toBeVisible();
  },
};

/** Every status: followed and requested turn quiet, each with its own icon; loading keeps the button's place. */
export const Statuses: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <FollowButton status="none" label="Follow" />
      <FollowButton status="following" label="Following" />
      <FollowButton status="requested" label="Requested" />
      <FollowButton status="blocking" label="Blocked" />
      <FollowButton status="loading" label="" />
      <FollowButton status="none" label="Follow" size="sm" />
      <FollowButton status="loading" label="" size="sm" />
    </div>
  ),
};

function Toggle() {
  const [status, setStatus] = useState<FollowStatus>("none");
  return (
    <FollowButton
      status={status}
      label={status === "none" ? "Follow" : "Following"}
      onPress={() => {
        setStatus(status === "none" ? "following" : "none");
      }}
    />
  );
}

/** Pressing it changes its look in place, and the focus stays on it. */
export const Pressed: Story = {
  render: () => <Toggle />,
  play: async ({ canvas }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Follow" }));
    const following = canvas.getByRole("button", { name: "Following" });
    await expect(following).toHaveFocus();
    await expect(following).toHaveAttribute("data-follow-status", "following");
  },
};
