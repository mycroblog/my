// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Avatar } from "./Avatar";
import { NotificationKindMark, type NotificationKind } from "./NotificationKindMark";

const meta = {
  title: "Display/NotificationKindMark",
  component: NotificationKindMark,
  args: { kind: "favourite" },
} satisfies Meta<typeof NotificationKindMark>;

export default meta;
type Story = StoryObj<typeof meta>;

const kinds: readonly NotificationKind[] = [
  "favourite",
  "boost",
  "follow",
  "mention",
  "status",
  "reaction",
  "quote",
  "poll",
  "voice",
  "staff",
  "notice",
];

/** Every kind on the corner of an avatar: each has its own shape, which its color only repeats. */
export const OnAvatars: Story = {
  render: () => (
    <div className="flex flex-wrap gap-4">
      {kinds.map((kind) => (
        <NotificationKindMark key={kind} kind={kind}>
          <Avatar name="Alice" seed={kind} decorative />
        </NotificationKindMark>
      ))}
    </div>
  ),
  play: async ({ canvasElement }) => {
    // Decorative: the row's sentence says the kind in words.
    for (const mark of canvasElement.querySelectorAll("[data-notification-kind] > [aria-hidden]"))
      await expect(mark).toHaveAttribute("aria-hidden", "true");
  },
};

/** A row without an actor (a notice of the server): the mark stands in the avatar's place. */
export const Alone: Story = { args: { kind: "staff" } };
