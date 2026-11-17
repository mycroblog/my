// SPDX-License-Identifier: AGPL-3.0-only

// The prompt to turn Web Push on, over the notifications (ADR-0086 §18–§21): when it is offered, each of its conditions
// false alone, and what Turn on and Not now do, on a browser stubbed whole: its notifications, and its push manager
// behind a Service Worker. What the browser's own parts answer is the platform's (web/platform/src/push.test.ts), and
// that a subscription reaches a real server and a push comes back is the E2E test's (web/e2e/specs/web-push.spec.ts).

import { expect, onTestFinished, test } from "bun:test";
import { instanceQuery, rememberPushPromptClosed, viewerQuery } from "@my/app-data";
import { fakeHost, type FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { fakeData, renderFeature } from "../test/feature";
import { loadNotifications, Notifications } from "./notifications";

/** What the stubbed browser does: what the site's notifications are, and how subscribing answers. */
type PushBrowser = {
  /** `Notification.permission` before any question; `default` by default. */
  permission?: NotificationPermission;
  /** What the question answers; `granted` by default. */
  answer?: NotificationPermission;
  /** How the push manager subscribes: at once (`ok`, by default), or after a first failure (`fails-once`). */
  subscribe?: "ok" | "fails-once";
  /** The browser has no Push API. */
  noPushApi?: boolean;
};

/**
 * Gives the test a browser with notifications, a Service Worker and (unless `noPushApi`) the Push API, none of which
 * a simulation has, until the test ends; and returns what it saw: how many times the site asked, and subscribed.
 */
function pushBrowser({
  permission = "default",
  answer = "granted",
  subscribe = "ok",
  noPushApi = false,
}: PushBrowser): { asked: number; subscribed: number } {
  const seen = { asked: 0, subscribed: 0 };
  let now = permission;
  let failures = subscribe === "fails-once" ? 1 : 0;
  const endpoint = "https://push.example/s/test";
  const made = {
    endpoint,
    expirationTime: null,
    options: { userVisibleOnly: true, applicationServerKey: null },
    getKey: (name: string) =>
      name === "p256dh" ? new Uint8Array([4, 1, 2]).buffer : name === "auth" ? new Uint8Array([9, 9]).buffer : null,
    toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh: "BAEC", auth: "CQk" } }),
    unsubscribe: () => Promise.resolve(true),
  };
  let current: typeof made | null = null;
  class Registration {
    readonly active = {};
    readonly pushManager = {
      getSubscription: () => Promise.resolve(current),
      subscribe: () => {
        seen.subscribed += 1;
        if (failures > 0) {
          failures -= 1;
          return Promise.reject(new DOMException("The push service is not available", "AbortError"));
        }
        current = made;
        return Promise.resolve(current);
      },
    };
    showNotification() {
      return Promise.resolve();
    }
  }
  const registration = new Registration();
  const globals = {
    Notification: {
      get permission() {
        return now;
      },
      requestPermission: () => {
        seen.asked += 1;
        now = answer;
        return Promise.resolve(answer);
      },
    },
    ServiceWorkerRegistration: Registration,
    ...(noPushApi
      ? {}
      : {
          PushManager: function PushManager() {
            return undefined;
          },
        }),
  };
  Object.assign(globalThis, globals);
  // The browser's navigator, reached as a stub reaches it: only @my/platform reads the platform's globals in the app.
  const browser = Reflect.get(globalThis, "navigator") as object;
  Object.defineProperty(browser, "serviceWorker", {
    configurable: true,
    value: {
      ready: Promise.resolve(registration),
      controller: {},
      getRegistration: () => Promise.resolve(registration),
      register: () => Promise.resolve(registration),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    },
  });
  onTestFinished(() => {
    for (const name of Object.keys(globals)) Reflect.deleteProperty(globalThis, name);
    Reflect.deleteProperty(browser, "serviceWorker");
  });
  return seen;
}

/** The device's key of the accounts that closed the prompt (ADR-0086 §21). */
const closedKey = "my:push-prompt-closed";

/** The server that sends Web Push to a reader (yuki) with no subscription. */
const sends: FakeOptions = { push: true };

/**
 * The notifications of yuki, drawn on a server of `options` in a browser of `browser`, on a device where the people of
 * `closedBy` closed the prompt before. The reader and the instance are read first, as the app's shell reads them
 * before any screen, so what the prompt decides on is there when the screen is drawn.
 */
async function notifications(browser: PushBrowser = {}, options = sends, closedBy: readonly string[] = []) {
  localStorage.removeItem(closedKey);
  onTestFinished(() => {
    localStorage.removeItem(closedKey);
  });
  const seen = pushBrowser(browser);
  const made = fakeData(options);
  for (const username of closedBy) rememberPushPromptClosed(made.fake.accountId(username), true);
  await made.data.queryClient.query(viewerQuery(made.data));
  await made.data.queryClient.query(instanceQuery(made.data));
  await loadNotifications(made.data);
  // Inside the main landmark, where the app's shell places every screen: the screen finds its tabs there.
  const user = renderFeature(
    made.data,
    <main>
      <Notifications instance={fakeHost} />
    </main>,
  );
  return { ...made, user, seen };
}

/** The prompt, a region named by its text, if it shows. */
const prompt = () => screen.queryByRole("region", { name: /^Get your notifications on this device/ });

/** The prompt, which the test fails without. */
function shownPrompt(): HTMLElement {
  const shown = prompt();
  if (shown === null) throw new Error("test: the prompt does not show");
  return shown;
}

/** The names of the prompt's buttons, in order. */
const buttonsOf = (shown: HTMLElement) =>
  within(shown)
    .getAllByRole("button")
    .map((button) => button.textContent);

/** The buttons of the prompt anywhere on the screen. */
const anyButton = () => screen.queryAllByRole("button", { name: /^(?:Turn on|Not now)$/ });

/** The line that push is on, in place of the prompt, once it shows. */
const pushIsOn = () => screen.findByText("Notifications are on for this device.");

/** Presses the prompt's button named `name`. */
const press = (user: Awaited<ReturnType<typeof notifications>>["user"], name: "Turn on" | "Not now") =>
  user.click(within(shownPrompt()).getByRole("button", { name }));

test("the prompt is offered with the screen, over the tabs, with Turn on and Not now, and the browser is asked nothing yet", async () => {
  const { seen } = await notifications();

  // Read at once: the prompt shows with the screen when it shows at all, which the tests of no prompt rely on.
  const offered = shownPrompt();
  expect(buttonsOf(offered)).toEqual(["Turn on", "Not now"]);
  const [tab] = screen.getAllByRole("tab");
  if (tab === undefined) throw new Error("test: the screen shows no tab");
  expect(offered.compareDocumentPosition(tab) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(seen).toEqual({ asked: 0, subscribed: 0 });
});

// Each condition of the prompt, false alone.

const never: [string, PushBrowser, FakeOptions, readonly string[]][] = [
  ["on a server that sends no Web Push", {}, {}, []],
  ["in a browser without the Push API", { noPushApi: true }, sends, []],
  ["where the site's notifications were blocked before", { permission: "denied" }, sends, []],
  ["to a reader whose grant has a subscription already", {}, { push: true, pushState: "subscribed" }, []],
  ["when the read of the subscription failed", {}, { push: true, pushState: "subscription-fails" }, []],
  ["when the read of the notification settings failed", {}, { push: true, pushState: "settings-fail" }, []],
  ["to a reader who closed it on this device before", {}, sends, ["yuki"]],
];

for (const [when, browser, options, closedBy] of never) {
  test(`the prompt is not offered ${when}`, async () => {
    const { seen } = await notifications(browser, options, closedBy);

    expect(screen.getAllByRole("tab").length).toBeGreaterThan(0);
    expect(prompt()).toBeNull();
    expect(anyButton()).toHaveLength(0);
    // Nor once everything the screen asked on being drawn has been answered.
    await screen.findByRole("feed", { name: "Notifications" });
    await new Promise((answered) => setTimeout(answered, 0));
    expect(prompt()).toBeNull();
    expect(anyButton()).toHaveLength(0);
    expect(seen).toEqual({ asked: 0, subscribed: 0 });
  });
}

test("the prompt is offered to a reader on a device where another account closed it", async () => {
  await notifications({}, sends, ["hana"]);

  expect(buttonsOf(shownPrompt())).toEqual(["Turn on", "Not now"]);
});

test("Turn on asks the browser, subscribes, and a line that push is on takes the prompt's place", async () => {
  const { user, seen, requests } = await notifications();

  await press(user, "Turn on");

  expect(await pushIsOn()).toBeDefined();
  expect(prompt()).toBeNull();
  expect(anyButton()).toHaveLength(0);
  expect(seen).toEqual({ asked: 1, subscribed: 1 });
  expect(requests).toContain("PUT /push/subscription");
});

test("where the site's notifications were allowed before, Turn on subscribes without asking again", async () => {
  const { user, seen } = await notifications({ permission: "granted" });

  await press(user, "Turn on");

  expect(await pushIsOn()).toBeDefined();
  expect(seen).toEqual({ asked: 0, subscribed: 1 });
});

test("when subscribing fails, the prompt says so and stays, and Turn on pressed again turns it on", async () => {
  const { user } = await notifications({ subscribe: "fails-once" });

  await press(user, "Turn on");

  const failure = await within(shownPrompt()).findByRole("alert");
  expect(failure.textContent).toBe("Notifications could not be turned on. Reload or update the app, then try again.");
  expect(buttonsOf(shownPrompt())).toEqual(["Turn on", "Not now"]);

  await press(user, "Turn on");
  expect(await pushIsOn()).toBeDefined();
});

test("when the browser's question closes without a choice, the prompt says why, Turn on stays, and nothing subscribes", async () => {
  const { user, seen, requests } = await notifications({ answer: "default" });

  await press(user, "Turn on");

  const why = await within(shownPrompt()).findByRole("alert");
  expect(why.textContent).toBe("The browser's question closed without an answer. Choose Turn on to ask again.");
  expect(buttonsOf(shownPrompt())).toEqual(["Turn on", "Not now"]);
  expect(seen).toEqual({ asked: 1, subscribed: 0 });
  expect(requests).not.toContain("PUT /push/subscription");
});

test("when the reader refuses the browser's question, the prompt says why and keeps only Not now, and nothing subscribes", async () => {
  const { user, seen, requests } = await notifications({ answer: "denied" });

  await press(user, "Turn on");

  const why = await within(shownPrompt()).findByRole("alert");
  expect(why.textContent).toBe(
    "Your browser blocks notifications from this site. Allow them in the browser's settings.",
  );
  expect(buttonsOf(shownPrompt())).toEqual(["Not now"]);
  expect(seen).toEqual({ asked: 1, subscribed: 0 });
  expect(requests).not.toContain("PUT /push/subscription");
});

test("Not now takes the prompt away, moves the focus to the tabs, asks and subscribes nothing, and the device keeps that this account closed it", async () => {
  const { fake, user, seen } = await notifications();

  await press(user, "Not now");

  await waitFor(() => {
    if (prompt() !== null) throw new Error("test: the prompt still shows");
  });
  const focused = document.activeElement;
  expect(screen.getAllByRole("tab").filter((tab) => tab === focused)).toHaveLength(1);
  expect(seen).toEqual({ asked: 0, subscribed: 0 });
  expect(JSON.parse(localStorage.getItem(closedKey) ?? "[]")).toEqual([fake.accountId("yuki")]);
});
