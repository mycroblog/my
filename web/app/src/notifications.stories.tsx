// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { rememberPushPromptClosed } from "@my/app-data";
import { createFakeApi, fakeOrigin } from "@my/e2e/fake";
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

// The prompt's conditions (ADR-0086 §18), each false alone, and the rest of its flow (§19), in en: a browser stubbed
// whole, its notifications and its push manager behind a Service Worker, and the device's record of who closed the
// prompt, which every story starts and ends without.

/** The device's key of the accounts that closed the prompt (ADR-0086 §21). */
const closedKey = "my:push-prompt-closed";

/** What the stubbed browser does: what the site's notifications are, and how subscribing answers. */
type PushBrowser = {
  /** Notification.permission before any question; `default` by default. */
  permission?: NotificationPermission;
  /** What the question answers; `granted` by default. */
  answer?: NotificationPermission;
  /** How the push manager subscribes: at once (`ok`, by default), or after a first failure (`fails-once`). */
  subscribe?: "ok" | "fails-once";
  /** The browser has no Push API. */
  noPushApi?: boolean;
  /** The accounts that closed the prompt on this device before, by username. */
  closedBy?: readonly string[];
};

/** The ID of the scenario's person of `username`, as the fake server answers it. */
async function accountID(username: string): Promise<string> {
  const me = await createFakeApi({ viewer: username }).serve(
    new Request(`${fakeOrigin}/api/v1/accounts/verify_credentials`),
  );
  const { id } = (await me.json()) as { id: string };
  return id;
}

/** A subscription as the browser holds one, made with the application key `key`. */
function subscriptionOf(key: BufferSource | string | null | undefined) {
  const endpoint = "https://push.example/s/story";
  const applicationServerKey =
    key === null || key === undefined || typeof key === "string"
      ? null
      : key instanceof ArrayBuffer
        ? key
        : new Uint8Array(key.buffer, key.byteOffset, key.byteLength).slice().buffer;
  return {
    endpoint,
    expirationTime: null,
    options: { userVisibleOnly: true, applicationServerKey },
    getKey: (name: string) =>
      name === "p256dh" ? new Uint8Array([4, 1, 2]).buffer : name === "auth" ? new Uint8Array([9, 9]).buffer : null,
    toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh: "BAEC", auth: "CQk" } }),
    unsubscribe: () => Promise.resolve(true),
  };
}

/** The stubbed browser of a story, and what it saw: how many times the site asked, and subscribed. */
function pushBrowser({
  permission = "default",
  answer = "granted",
  subscribe = "ok",
  noPushApi = false,
  closedBy = [],
}: PushBrowser) {
  const seen = { asked: 0, subscribed: 0 };
  const beforeEach = async () => {
    seen.asked = 0;
    seen.subscribed = 0;
    localStorage.removeItem(closedKey);
    for (const username of closedBy) rememberPushPromptClosed(await accountID(username), true);
    const notifications = Reflect.get(window, "Notification") as object;
    const notificationsWere = Object.getOwnPropertyDescriptors(notifications);
    let now = permission;
    Object.defineProperty(notifications, "permission", { configurable: true, get: () => now });
    Object.defineProperty(notifications, "requestPermission", {
      configurable: true,
      value: () => {
        seen.asked += 1;
        now = answer;
        return Promise.resolve(answer);
      },
    });
    let current: ReturnType<typeof subscriptionOf> | null = null;
    let failures = subscribe === "fails-once" ? 1 : 0;
    const pushManager = {
      getSubscription: () => Promise.resolve(current),
      permissionState: () => Promise.resolve(now === "granted" ? "granted" : now === "denied" ? "denied" : "prompt"),
      subscribe: (options?: PushSubscriptionOptionsInit) => {
        seen.subscribed += 1;
        if (failures > 0) {
          failures -= 1;
          return Promise.reject(new DOMException("The push service is not available", "AbortError"));
        }
        current = subscriptionOf(options?.applicationServerKey);
        return Promise.resolve(current);
      },
    };
    const registration = {
      scope: `${location.origin}/`,
      active: {},
      pushManager,
      showNotification: () => Promise.resolve(),
      getNotifications: () => Promise.resolve([]),
    };
    // The page's Service Worker container, reached as the stub of Notification above is: only @my/platform reads the
    // platform's globals in the app.
    const container = Reflect.get(window, "navigator").serviceWorker;
    Object.defineProperty(container, "ready", { configurable: true, get: () => Promise.resolve(registration) });
    Object.defineProperty(container, "getRegistration", {
      configurable: true,
      value: () => Promise.resolve(registration),
    });
    Object.defineProperty(container, "getRegistrations", {
      configurable: true,
      value: () => Promise.resolve([registration]),
    });
    const pushApi = Object.getOwnPropertyDescriptor(window, "PushManager");
    if (noPushApi) Reflect.deleteProperty(window, "PushManager");
    return () => {
      localStorage.removeItem(closedKey);
      for (const name of ["permission", "requestPermission"] as const) {
        const was = notificationsWere[name];
        if (was === undefined) Reflect.deleteProperty(notifications, name);
        else Object.defineProperty(notifications, name, was);
      }
      for (const name of ["ready", "getRegistration", "getRegistrations"]) Reflect.deleteProperty(container, name);
      if (pushApi !== undefined) Object.defineProperty(window, "PushManager", pushApi);
    };
  };
  return { beforeEach, seen };
}

/** The prompt's name in en: its text. */
const promptName = /^Get your notifications on this device/;

/** The prompt in en, if it shows. */
const promptIn = (context: StoryContext) => within(context.canvasElement).queryByRole("region", { name: promptName });

/** The prompt in en, once it shows. */
const shownPrompt = (context: StoryContext) => within(context.canvasElement).findByRole("region", { name: promptName });

/** The names of the prompt's buttons, in order. */
const buttonsOf = (prompt: HTMLElement) =>
  within(prompt)
    .getAllByRole("button")
    .map((b) => b.textContent);

/**
 * Once the screen shows its notifications and its tabs, there is no prompt, nor its buttons anywhere. The prompt shows
 * with the screen when it shows at all (PushOfferedWithScreen), so the screen at rest decides.
 */
async function noPrompt(context: StoryContext) {
  await screenMeta.play(context);
  const canvas = within(context.canvasElement);
  await expect(canvas.getAllByRole("tab").length).toBeGreaterThan(0);
  await expect(promptIn(context)).toBeNull();
  await expect(canvas.queryByRole("button", { name: "Turn on" })).toBeNull();
  await expect(canvas.queryByRole("button", { name: "Not now" })).toBeNull();
}

/** Presses the prompt's button named `name`, in en. */
async function press(context: StoryContext, name: string) {
  await userEvent.click(within(await shownPrompt(context)).getByRole("button", { name }));
}

/** The line that push is on, in place of the prompt. */
const pushIsOn = async (context: StoryContext) =>
  within(context.canvasElement).findByText("Notifications are on for this device.");

const offered = pushBrowser({});
/**
 * Offered with the screen, over the tabs, with Turn on and Not now; the browser is not asked until Turn on is pressed.
 */
export const PushOfferedWithScreen: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: offered.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    const prompt = promptIn(context);
    if (prompt === null) throw new Error("the prompt does not show with the screen");
    await expect(buttonsOf(prompt)).toEqual(["Turn on", "Not now"]);
    const [tab] = within(context.canvasElement).getAllByRole("tab");
    if (tab === undefined) throw new Error("no tab");
    await expect(prompt.compareDocumentPosition(tab) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await expect(offered.seen).toEqual({ asked: 0, subscribed: 0 });
  },
};
/** A server that sends no Web Push (no `push_server_key`): no prompt. */
export const PushNotSent: Story = {
  parameters: screen({ at }, "notifications"),
  globals: { locale: "en" },
  beforeEach: pushBrowser({}).beforeEach,
  play: noPrompt,
};
/** A browser without the Push API: no prompt. */
export const PushNotInBrowser: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: pushBrowser({ noPushApi: true }).beforeEach,
  play: noPrompt,
};
/** The site's notifications blocked before the screen opens: no prompt. */
export const PushBlockedBefore: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: pushBrowser({ permission: "denied" }).beforeEach,
  play: noPrompt,
};
/** A reader whose grant has a subscription already: no prompt. */
export const PushAlreadyOn: Story = {
  parameters: screen({ at, server: { push: true, pushState: "subscribed" } }, "notifications"),
  globals: { locale: "en" },
  beforeEach: pushBrowser({}).beforeEach,
  play: noPrompt,
};
/** The read of the subscription fails, so whether there is one is not known: no prompt. */
export const PushSubscriptionUnread: Story = {
  parameters: screen({ at, server: { push: true, pushState: "subscription-fails" } }, "notifications"),
  globals: { locale: "en" },
  beforeEach: pushBrowser({}).beforeEach,
  play: noPrompt,
};
/** The read of the reader's notification settings fails, so the kinds to push are not known: no prompt. */
export const PushSettingsUnread: Story = {
  parameters: screen({ at, server: { push: true, pushState: "settings-fail" } }, "notifications"),
  globals: { locale: "en" },
  beforeEach: pushBrowser({}).beforeEach,
  play: noPrompt,
};
/** The reader (yuki) closed the prompt on this device before: no prompt. */
export const PushClosedBefore: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: pushBrowser({ closedBy: ["yuki"] }).beforeEach,
  play: noPrompt,
};
/** Another account closed the prompt on this device: the reader still gets it. */
export const PushClosedByAnother: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: pushBrowser({ closedBy: ["hana"] }).beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await expect(await shownPrompt(context)).toBeVisible();
  },
};

const turnedOn = pushBrowser({});
/** Turned on: the browser asked, then subscribed; the prompt gives way to a line that push is on. */
export const PushTurnedOn: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: turnedOn.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Turn on");
    await expect(await pushIsOn(context)).toBeVisible();
    const canvas = within(context.canvasElement);
    await expect(canvas.queryByRole("button", { name: "Turn on" })).toBeNull();
    await expect(canvas.queryByRole("button", { name: "Not now" })).toBeNull();
    await expect(turnedOn.seen).toEqual({ asked: 1, subscribed: 1 });
  },
};
const allowedBefore = pushBrowser({ permission: "granted" });
/** The site's notifications allowed before: Turn on subscribes without asking again. */
export const PushAllowedBefore: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: allowedBefore.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Turn on");
    await expect(await pushIsOn(context)).toBeVisible();
    await expect(allowedBefore.seen).toEqual({ asked: 0, subscribed: 1 });
  },
};
/** Subscribing fails: the prompt says so and stays, and Turn on, pressed again, turns it on. */
export const PushFailed: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: pushBrowser({ subscribe: "fails-once" }).beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Turn on");
    const prompt = await shownPrompt(context);
    await expect(await within(prompt).findByRole("alert")).toHaveTextContent(
      "Notifications could not be turned on. Reload or update the app, then try again.",
    );
    await expect(buttonsOf(prompt)).toEqual(["Turn on", "Not now"]);
    await press(context, "Turn on");
    await expect(await pushIsOn(context)).toBeVisible();
  },
};
const questionClosed = pushBrowser({ answer: "default" });
/** The question closed without a choice: the prompt says why, Turn on stays, and nothing subscribes. */
export const PushQuestionClosedSays: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: questionClosed.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Turn on");
    const prompt = await shownPrompt(context);
    await expect(await within(prompt).findByRole("alert")).toHaveTextContent(
      "The browser's question closed without an answer. Choose Turn on to ask again.",
    );
    await expect(buttonsOf(prompt)).toEqual(["Turn on", "Not now"]);
    await expect(questionClosed.seen).toEqual({ asked: 1, subscribed: 0 });
  },
};
const refused = pushBrowser({ answer: "denied" });
/** Refused: the prompt says why and keeps only Not now; nothing subscribes. */
export const PushRefusedSays: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: refused.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Turn on");
    const prompt = await shownPrompt(context);
    await expect(await within(prompt).findByRole("alert")).toHaveTextContent(
      "Your browser blocks notifications from this site. Allow them in the browser's settings.",
    );
    await expect(buttonsOf(prompt)).toEqual(["Not now"]);
    await expect(refused.seen).toEqual({ asked: 1, subscribed: 0 });
  },
};
const notNow = pushBrowser({});
/**
 * Not now: the prompt goes, focus moves to the filter tabs, nothing is asked or subscribed, and the device keeps that
 * this account closed it.
 */
export const PushNotNow: Story = {
  parameters: push,
  globals: { locale: "en" },
  beforeEach: notNow.beforeEach,
  play: async (context) => {
    await screenMeta.play(context);
    await press(context, "Not now");
    await waitFor(async () => {
      await expect(promptIn(context)).toBeNull();
    });
    const tabs = within(context.canvasElement).getAllByRole("tab");
    await expect(tabs).toContain(context.canvasElement.ownerDocument.activeElement);
    await expect(notNow.seen).toEqual({ asked: 0, subscribed: 0 });
    await expect(JSON.parse(localStorage.getItem(closedKey) ?? "[]")).toEqual([await accountID("yuki")]);
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
