// SPDX-License-Identifier: AGPL-3.0-only

// Notifications, the entry of the settings (FR-210, ADR-0086 §8), on the fake server as yuki reads it, with the rows a
// member of a server gets. As read on a device with Web Push and the notification sound on, on a server that sends
// email, where the tables have every channel; with Web Push off, and where it cannot be turned on, for each reason
// the switch gives; on a server without email, and on one that stopped sending email to the reader's address; with a
// kind turned off in the app, whose other cells are off and say why; as an administrator reads it, with the
// moderators' kinds; and while it loads, and when it could not be read. The browser is stubbed (push.samples.ts). What
// a cell says of a change (saving, saved, not saved), and the table in a frame of the minimum width, are the design
// system's own stories (Channel table).

import { memberNotificationRows, moderatorNotificationRows } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { pushBrowser } from "./push.samples";

const at = "/settings/notifications";

/** A server that sends Web Push and email, to a member. */
const server = { push: true, email: true, settings: { notificationRows: memberNotificationRows } } as const;

/** A device that lets the site notify, with the notification sound on for three kinds. */
const sounding = pushBrowser({ permission: "granted", sounding: ["mention", "follow", "follow_request"] });

// A story's parameters are laid over the meta's, member by member: what only some stories have (a subscription) is
// each story's own.
const meta = {
  title: "Settings notifications",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * /settings/notifications on a device with Web Push on, for mentions alone, and the notification sound on for three
 * kinds: every channel is a column, and the first row of Push and of Sound is half on.
 */
export const Entry: Story = {
  parameters: screen({ at, server: { ...server, pushState: "subscribed" } }),
  beforeEach: sounding,
};
/** With Web Push off on a device that can have it, and the sound off: the tables have the app and email alone. */
export const PushOff: Story = {
  parameters: screen({ at, server }),
  beforeEach: pushBrowser({ permission: "granted" }),
};
/** On a server that sends no Web Push: the switch is off and disabled, and says so. */
export const PushNoServer: Story = {
  parameters: screen({ at, server: { ...server, push: false } }),
  beforeEach: pushBrowser(),
};
/** In a browser without Web Push: the switch is off and disabled, and says so. */
export const PushNoBrowser: Story = {
  parameters: screen({ at, server }),
  beforeEach: pushBrowser({ push: "none" }),
};
/** In a tab of Safari on an iPhone: the switch is off and disabled, and says to add the site to the Home Screen. */
export const PushNeedsInstall: Story = {
  parameters: screen({ at, server }),
  beforeEach: pushBrowser({ push: "install" }),
};
/** In a browser that blocks the site's notifications: the switch is off and disabled, and says where to allow them. */
export const PushBlocked: Story = {
  parameters: screen({ at, server }),
  beforeEach: pushBrowser({ permission: "denied" }),
};
/** On a server that sends no email: the tables have no Email column. */
export const NoEmail: Story = {
  parameters: screen({ at, server: { ...server, email: false, pushState: "subscribed" } }),
  beforeEach: sounding,
};
/** On a server that stopped sending email to the reader's address after one bounced: the screen says why, first. */
export const EmailStopped: Story = {
  parameters: screen({
    at,
    server: {
      ...server,
      preferences: { email_suppression: { kind: "hard_bounce", state_changed_at: "2026-11-02T09:30:00Z" } },
    },
  }),
  beforeEach: pushBrowser({ permission: "granted" }),
};
/**
 * The same on a server that sent as many verification emails as it sends for now: pressed, the action says how long
 * the server asks the reader to wait.
 */
export const EmailStoppedLimited: Story = {
  parameters: screen({
    at,
    server: {
      ...server,
      verificationEmail: "limited",
      preferences: { email_suppression: { kind: "hard_bounce", state_changed_at: "2026-11-02T09:30:00Z" } },
    },
  }),
  beforeEach: pushBrowser({ permission: "granted" }),
};
/** With favourites turned off in the app: its other cells are off and disabled, and the row says why. */
export const KindOffInTheApp: Story = {
  parameters: screen({
    at,
    server: {
      ...server,
      pushState: "subscribed",
      settings: {
        notificationRows: memberNotificationRows.map((row) =>
          row.group === "favourite" ? { ...row, in_app: false, email: true } : row,
        ),
      },
    },
  }),
  beforeEach: sounding,
};
/** As the administrator reads it: the moderators' kinds in a table of their own, between the other two. */
export const ForModerators: Story = {
  parameters: screen({
    at,
    viewer: "hana",
    server: { ...server, settings: { notificationRows: [...memberNotificationRows, ...moderatorNotificationRows] } },
  }),
  beforeEach: pushBrowser({ permission: "granted" }),
};
/** While what the screen shows is on its way: its head, and that it loads. */
export const Loading: Story = { parameters: screen({ at, state: "loading", server }) };
/** When it could not be read: it says so, with the way to try again. */
export const Failed: Story = { parameters: screen({ at, state: "error", server }) };
