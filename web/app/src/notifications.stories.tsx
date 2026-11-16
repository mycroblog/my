// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

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
/** Dismissing a notification takes it out of the list at once. */
export const Dismissed: Story = {
  parameters: screen({ at }, "notifications"),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const feed = within(context.canvasElement).getByRole("feed", { name: "Notifications" });
    const first = within(feed).getAllByRole("article")[0];
    if (first === undefined) throw new Error("no notification");
    const name = first.getAttribute("aria-labelledby");
    await userEvent.click(within(first).getByRole("button", { name: "Dismiss notification" }));
    await waitFor(async () => {
      await expect(name === null ? null : context.canvasElement.ownerDocument.getElementById(name)).toBeNull();
    });
  },
};
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
