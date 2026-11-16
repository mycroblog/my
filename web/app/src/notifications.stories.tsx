// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
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
// The prompt to turn Web Push on (ADR-0086 §18–§19), on a server that sends it, for a reader with no subscription who
// has not closed it. The browser's notifications are stubbed, as a headless one blocks them: a site never asked yet,
// whose question then closes without a choice, or is refused.

/** The browser's notifications as a site never asked yet sees them, the question answering `answer`. */
const notificationsAsk = (answer: NotificationPermission) => () => {
  const notifications = Reflect.get(window, "Notification") as object;
  let permission: NotificationPermission = "default";
  const before = Object.getOwnPropertyDescriptors(notifications);
  Object.defineProperty(notifications, "permission", { configurable: true, get: () => permission });
  Object.defineProperty(notifications, "requestPermission", {
    configurable: true,
    value: () => {
      permission = answer;
      return Promise.resolve(answer);
    },
  });
  return () => {
    for (const name of ["permission", "requestPermission"] as const) {
      const was = before[name];
      if (was === undefined) Reflect.deleteProperty(notifications, name);
      else Object.defineProperty(notifications, name, was);
    }
  };
};

const push = screen({ at, server: { push: true } }, "notifications");

/** The prompt's buttons, Turn on first while it is there, in any locale. */
async function promptButtons(context: StoryContext) {
  const prompt = await within(context.canvasElement).findByRole("region", { name: /./ });
  return within(prompt).getAllByRole("button");
}

/** Presses Turn on in the prompt. */
async function turnOn(context: StoryContext) {
  await screenMeta.play(context);
  const [first] = await promptButtons(context);
  if (first === undefined) throw new Error("the prompt has no button");
  await userEvent.click(first);
}

/** Offered over the tabs, with Turn on and Not now. */
export const PushOffered: Story = { parameters: push, beforeEach: notificationsAsk("default") };
/** The browser's question closed without a choice: the prompt says so, and stays to be pressed again. */
export const PushQuestionClosed: Story = {
  parameters: push,
  beforeEach: notificationsAsk("default"),
  play: async (context) => {
    await turnOn(context);
    await within(context.canvasElement).findByRole("alert");
    await expect(await promptButtons(context)).toHaveLength(2);
  },
};
/** The site's notifications refused: the prompt says so, and keeps only Not now. */
export const PushBlocked: Story = {
  parameters: push,
  beforeEach: notificationsAsk("denied"),
  play: async (context) => {
    await turnOn(context);
    await within(context.canvasElement).findByRole("alert");
    await expect(await promptButtons(context)).toHaveLength(1);
  },
};
/**
 * The Mentions tab over a notification of every kind from people: only the mention, the quote and the new post of a
 * subscribed account stay, each drawn as its post.
 */
export const Mentions: Story = {
  parameters: screen({ at, server: { everyKind: "people" } }, "notifications"),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const canvas = within(context.canvasElement);
    await userEvent.click(canvas.getByRole("tab", { name: "Mentions" }));
    await waitFor(async () => {
      await expect(within(canvas.getByRole("feed", { name: "Notifications" })).getAllByRole("article")).toHaveLength(3);
    });
    // Newest first: sam's mention, mio's quote, and hana's new post, each its sentence over the post.
    const [mention, quote, post] = within(canvas.getByRole("feed", { name: "Notifications" })).getAllByRole("article");
    await expect(mention).toHaveTextContent(/^⁨?Sam Rivera⁩? mentioned you⁨?Sam Rivera/);
    await expect(quote).toHaveTextContent(/^⁨?Mio 澪⁩? quoted your post⁨?Mio 澪/);
    await expect(post).toHaveTextContent(/^⁨?Hana 花⁩? posted⁨?Hana 花/);
  },
};
/**
 * A dismissal the server fails: the notification comes back with a notice and Try again, which dismisses it for good.
 */
export const DismissFailed: Story = {
  parameters: screen({ at, server: { dismissFails: true } }, "notifications"),
  globals: { locale: "en" },
  play: async (context) => {
    await screenMeta.play(context);
    const canvas = within(context.canvasElement);
    const feed = canvas.getByRole("feed", { name: "Notifications" });
    const first = within(feed).getAllByRole("article")[0];
    if (first === undefined) throw new Error("no notification");
    // The line comes back as a new element: it is found again by what it says.
    const said = first.textContent;
    const shown = () =>
      within(feed)
        .queryAllByRole("article")
        .filter((a) => a.textContent === said);
    await userEvent.click(within(first).getByRole("button", { name: "Dismiss notification" }));
    // The notice shows at the bottom of the page.
    const page = within(context.canvasElement.ownerDocument.body);
    await expect(await page.findByText("The notification could not be dismissed.")).toBeVisible();
    await waitFor(async () => {
      await expect(shown()).toHaveLength(1);
    });
    await userEvent.click(page.getByRole("button", { name: "Try again" }));
    await waitFor(async () => {
      await expect(shown()).toHaveLength(0);
    });
  },
};
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };
