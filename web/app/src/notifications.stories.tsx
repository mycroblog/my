// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { pushBrowser } from "./push.samples";

const at = "/notifications";

const meta = {
  title: "Notifications",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at }, "notifications") },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
export const Empty: Story = { parameters: screen({ at, state: "empty" }, "notifications") };
export const Loading: Story = { parameters: screen({ at, state: "loading" }, "notifications") };
export const Failed: Story = { parameters: screen({ at, state: "error" }, "notifications") };
export const LongText: Story = { parameters: screen({ at, state: "long" }, "notifications") };
/**
 * With replies, boosts and quotes of the reader's post waiting for the reader's approval: the row over the
 * notifications counts them and opens their screen; their notifications are plain lines.
 */
export const WaitingForApproval: Story = { parameters: screen({ at, server: { approvals: true } }, "notifications") };
/**
 * A notification of every kind from people (a follow, a mention, a reaction, a request, an event's invitation, …):
 * each line leads to what it tells of, a post, an account, a room or an event.
 */
export const FromPeople: Story = { parameters: screen({ at, server: { everyKind: "people" } }, "notifications") };
/**
 * The server's notices to the reader (an announcement, the moderators', the supporter plan's, the service's closure, a
 * scheduled post that failed, an event's changes, …), each by its own glyph.
 */
export const FromServer: Story = { parameters: screen({ at, server: { everyKind: "notices" } }, "notifications") };
/** The notices to the server's moderators, each by its own glyph. */
export const ToModerators: Story = {
  parameters: screen({ at, server: { everyKind: "moderators" } }, "notifications"),
};
// The prompt to turn Web Push on (ADR-0086 §18–§19), on a server that sends it, for a reader with no subscription who
// has not closed it, in a browser that was never asked (push.samples.ts stubs it, and the device's record of who
// closed the prompt). What the prompt says after Turn on is the prompt's own (the design system's Inline prompt).

/** Offered over the tabs, with Turn on and Not now. */
export const PushOffered: Story = {
  parameters: screen({ at, server: { push: true } }, "notifications"),
  beforeEach: pushBrowser(),
};

/**
 * The Mentions tab, which the address opens the screen on, over a notification of every kind from people: only the
 * mention, the quote and the new post of a subscribed account stay, each drawn as its post.
 */
export const Mentions: Story = {
  parameters: screen({ at: `${at}?tab=mentions`, server: { everyKind: "people" } }, "notifications"),
};
/** The link of a notification the reader no longer has: the notifications, with a line that it is gone. */
export const GoneByItsLink: Story = {
  parameters: screen({ at: "/notifications/0190a5d0-0000-7000-8000-000000000999" }, "notifications"),
};
