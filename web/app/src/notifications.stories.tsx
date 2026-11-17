// SPDX-License-Identifier: AGPL-3.0-only

// The notifications, a story per state every screen is shown in (app.samples.tsx), fed new notifications from the
// Stream panel.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

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
// has not closed it. The browser is stubbed, as a headless one blocks notifications: its notifications, a site never
// asked yet, and its push manager behind a Service Worker; and the device's record of who closed the prompt, which
// every story starts and ends without. What the prompt says after Turn on has no prop of the screen: a play presses it.

/** The device's key of the accounts that closed the prompt (ADR-0086 §21). */
const closedKey = "my:push-prompt-closed";

/** What the stubbed browser does: what its question answers, and how subscribing answers. */
type PushBrowser = {
  /** What the question answers; `granted` by default. */
  answer?: NotificationPermission;
  /** How the push manager subscribes: at once (`ok`, by default), or after a first failure (`fails-once`). */
  subscribe?: "ok" | "fails-once";
};

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

/** Stubs the browser of a story before it renders, and returns what puts the browser back. */
const pushBrowser =
  ({ answer = "granted", subscribe = "ok" }: PushBrowser = {}) =>
  () => {
    localStorage.removeItem(closedKey);
    const notifications = Reflect.get(window, "Notification") as object;
    const notificationsWere = Object.getOwnPropertyDescriptors(notifications);
    let now: NotificationPermission = "default";
    Object.defineProperty(notifications, "permission", { configurable: true, get: () => now });
    Object.defineProperty(notifications, "requestPermission", {
      configurable: true,
      value: () => {
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
    return () => {
      localStorage.removeItem(closedKey);
      for (const name of ["permission", "requestPermission"] as const) {
        const was = notificationsWere[name];
        if (was === undefined) Reflect.deleteProperty(notifications, name);
        else Object.defineProperty(notifications, name, was);
      }
      for (const name of ["ready", "getRegistration", "getRegistrations"]) Reflect.deleteProperty(container, name);
    };
  };

const push = screen({ at, server: { push: true } }, "notifications");

/** Presses Turn on, the first button of the prompt in any locale, once the screen shows. */
async function turnOn(context: StoryContext) {
  await screenMeta.play(context);
  const prompt = await within(context.canvasElement).findByRole("region", { name: /./ });
  const [first] = within(prompt).getAllByRole("button");
  if (first === undefined) throw new Error("the prompt has no button");
  await userEvent.click(first);
}

/** Presses Turn on, and waits until the prompt says why push is not on. */
async function turnOnInVain(context: StoryContext) {
  await turnOn(context);
  await within(context.canvasElement).findByRole("alert");
}

/** Offered over the tabs, with Turn on and Not now. */
export const PushOffered: Story = { parameters: push, beforeEach: pushBrowser() };
/** Turned on: a line that push is on takes the prompt's place. */
export const PushTurnedOn: Story = {
  parameters: push,
  beforeEach: pushBrowser(),
  play: async (context) => {
    await turnOn(context);
    await waitFor(() => {
      if (within(context.canvasElement).queryByRole("region", { name: /./ }) !== null)
        throw new Error("the prompt still shows");
    });
  },
};
/** The browser's question closed without a choice: the prompt says so, and stays to be pressed again. */
export const PushQuestionClosed: Story = {
  parameters: push,
  beforeEach: pushBrowser({ answer: "default" }),
  play: turnOnInVain,
};
/** The site's notifications refused: the prompt says so, and keeps only Not now. */
export const PushBlocked: Story = {
  parameters: push,
  beforeEach: pushBrowser({ answer: "denied" }),
  play: turnOnInVain,
};
/** Subscribing fails: the prompt says so and stays, with Turn on to press again. */
export const PushFailed: Story = {
  parameters: push,
  beforeEach: pushBrowser({ subscribe: "fails-once" }),
  play: turnOnInVain,
};
/**
 * The Mentions tab over a notification of every kind from people: only the mention, the quote and the new post of a
 * subscribed account stay, each drawn as its post. The screen has no prop for its tab: a play presses it.
 */
export const Mentions: Story = {
  parameters: screen({ at, server: { everyKind: "people" } }, "notifications"),
  play: async (context) => {
    await screenMeta.play(context);
    const canvas = within(context.canvasElement);
    // Mentions is the second of the two tabs, in any locale.
    const [, mentions] = canvas.getAllByRole("tab");
    if (mentions === undefined) throw new Error("the screen has no second tab");
    await userEvent.click(mentions);
    await waitFor(() => {
      if (mentions.getAttribute("aria-selected") !== "true" || canvas.getByRole("feed").ariaBusy === "true")
        throw new Error("the mentions do not show yet");
    });
  },
};
/**
 * A dismissal the server fails: the notification comes back with a notice and Try again at the bottom of the page.
 * The notice follows a press: a play dismisses the first notification.
 */
export const DismissFailed: Story = {
  parameters: screen({ at, server: { dismissFails: true } }, "notifications"),
  play: async (context) => {
    await screenMeta.play(context);
    // Dismiss is the first button of a notification, in any locale.
    const [first] = within(context.canvasElement).getAllByRole("article");
    const [dismiss] = first === undefined ? [] : within(first).getAllByRole("button");
    if (dismiss === undefined) throw new Error("no notification to dismiss");
    await userEvent.click(dismiss);
    // The notice, with its Try again.
    const page = within(context.canvasElement.ownerDocument.body);
    await waitFor(() => {
      if (!page.getAllByRole("status").some((status) => within(status).queryByRole("button") !== null))
        throw new Error("the notice does not show yet");
    });
  },
};
/** The link of a notification the reader no longer has: the notifications, with a line that it is gone. */
export const GoneByItsLink: Story = {
  parameters: screen({ at: "/notifications/0190a5d0-0000-7000-8000-000000000999" }, "notifications"),
};
