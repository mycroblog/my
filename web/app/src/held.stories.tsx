// SPDX-License-Identifier: AGPL-3.0-only

// The notifications the reader's notification policy holds back (ADR-0158 §7), the screen the row over the
// notifications opens: kiri (a boost and a favourite) and kobo (a follow), each with how many and Dismiss beside
// Accept (`held`); each state every screen is shown in (app.samples.tsx); kiri's held-back notifications on their own
// screen, its failure and its request gone; and the notifications with the row over them. The answer in place of a
// row's buttons, right after Accept or Dismiss, is the row's own state (the design system's Follow requests).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const at = "/notifications/requests";
const server = { held: true };

const meta = {
  title: "Held-back notifications",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, server, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, server, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, server, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, server, state: "long" }) };
/** One account's held-back notifications, as the list draws them, under the account and its answer. */
export const OneAccount: Story = { parameters: screen({ at: "held:kiri@far.example", server }) };
/** One account's screen when its request could not be read: the failure, with Try again. */
export const OneAccountFailed: Story = {
  parameters: screen({ at: "held:kiri@far.example", server, state: "error" }),
};
/** One account's screen once the server lists no request of it (answered elsewhere): it says it holds none. */
export const OneAccountGone: Story = { parameters: screen({ at: "held:sam", server }) };
/** The notifications, with the row of the held-back ones over them. */
export const InNotifications: Story = { parameters: screen({ at: "/notifications", server }) };
