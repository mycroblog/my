// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel. Live options (`notification-dismiss`) set how a notification is dismissed: by a button of its own at
// its line's end, or from a menu there.

import { DismissLookProvider, type DismissLook } from "@my/app-features";
import { Options } from "@my/devtools/options";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const at = "/notifications";

/** The app in each way a notification is dismissed. */
function Variants({ app }: { app: () => ReactNode }) {
  const as = (look: DismissLook) => <DismissLookProvider value={look}>{app()}</DismissLookProvider>;
  return <Options id="notification-dismiss" variants={{ button: as("button"), menu: as("menu") }} />;
}

const meta = {
  title: "Notifications",
  ...screenMeta,
  render: (args: unknown, context: StoryContext) => <Variants app={() => screenMeta.render(args, context)} />,
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
 * A notification of every kind the server sends: each line leads to what it tells of (a post, an account, an event, an
 * announcement, a room), the server's notices with their own glyphs; the Mentions tab holds the new posts of accounts
 * the reader is notified of.
 */
export const EveryKind: Story = {
  parameters: screen({ at, server: { everyKind: true } }, "notifications"),
};
export const EveryKindJapanese: Story = {
  parameters: screen({ at, server: { everyKind: true } }, "notifications"),
  globals: { locale: "ja" },
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
    const dismiss = within(first).queryByRole("button", { name: "Dismiss notification" });
    if (dismiss !== null) await userEvent.click(dismiss);
    else {
      await userEvent.click(within(first).getByRole("button", { name: "Notification menu" }));
      await userEvent.click(await within(document.body).findByRole("menuitem", { name: "Dismiss notification" }));
    }
    await waitFor(async () => {
      await expect(name === null ? null : context.canvasElement.ownerDocument.getElementById(name)).toBeNull();
    });
  },
};
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
