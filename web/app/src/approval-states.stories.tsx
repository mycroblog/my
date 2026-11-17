// SPDX-License-Identifier: AGPL-3.0-only

// Asking for the author's approval to reply, boost and quote (ADR-0155, #3113), on the fake server's posts that approve
// by hand (`approvals`): what waits for the reader's approval, counted over the notifications; and the lines over the
// reader's reply and boost that wait or were not approved, which no one else sees.
//
// The fake's reader is yuki: marco asks to reply to and boost yuki-asks and mio to quote it; layla-asks admits replies
// and boosts of the people it mentions at once and of everyone else by approval, and holds yuki's reply that waits,
// yuki's reply that was not approved, and yuki's boost that waits. What a press opens over these screens is the part's
// own: the boost's menu that says it will wait (the design system's Menu), the composer of a reply that waits and its
// choice of who may reply, boost and quote (Compose), an answer in a request's place (the design system's Follow
// requests).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Approval states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/notifications", server: { approvals: true } }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Over the notifications, a row counts the three requests and opens their screen. */
export const Counted: Story = {};

const laylaAsks = screen({ at: "layla-asks", server: { approvals: true } });
const laylaAsksToSam = screen({ at: "layla-asks", viewer: "sam", server: { approvals: true } });

/** Under layla-asks, the reader's waiting boost and replies each carry their line. */
export const OwnLines: Story = { parameters: laylaAsks };

/** Someone else reads layla-asks: neither yuki's reply that waits nor the one declined, and no line. */
export const OthersSee: Story = { parameters: laylaAsksToSam };
