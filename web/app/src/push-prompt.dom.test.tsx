// SPDX-License-Identifier: AGPL-3.0-only

// The prompt to turn Web Push on over the notifications (FR-206), closed with Not now: the device keeps the choice, so
// the app opened again in the same browser does not offer it, and nothing subscribes. The browser is one that can
// receive pushes and was never asked; that a push arrives once it is turned on is an E2E test
// (web/e2e/specs/web-push.spec.ts). The reader is Yuki.

import { expect, onTestFinished, test } from "bun:test";
import { cleanup, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { fakeApp } from "./test/server";

/** The device's record of the accounts that closed the prompt. */
const closedKey = "my:push-prompt-closed";

/**
 * Makes the browser one with a Service Worker, the Push API and notifications the site never asked for, until the test
 * ends, on a device that keeps no record of the prompt; returns how many times the site asked and subscribed.
 */
function browserWithPush() {
  const seen = { asked: 0, subscribed: 0 };
  const registration = {
    scope: `${location.origin}/`,
    active: {},
    pushManager: {
      getSubscription: () => Promise.resolve(null),
      permissionState: () => Promise.resolve("prompt"),
      subscribe: () => {
        seen.subscribed += 1;
        return Promise.reject(new DOMException("The test's browser subscribes nothing", "AbortError"));
      },
    },
    showNotification: () => Promise.resolve(),
    getNotifications: () => Promise.resolve([]),
  };
  const globals = {
    PushManager: function PushManager() {
      return undefined;
    },
    ServiceWorkerRegistration: function ServiceWorkerRegistration() {
      return undefined;
    },
    Notification: {
      permission: "default",
      requestPermission: () => {
        seen.asked += 1;
        return Promise.resolve("default");
      },
    },
  };
  const names = Object.keys(globals) as (keyof typeof globals)[];
  const before = names.map((name) => Object.getOwnPropertyDescriptor(globalThis, name));
  // The browser itself, reached as a stub reaches it: in the app only @my/platform reads the platform's globals.
  const browser = Reflect.get(globalThis, "navigator") as object;
  const workers = Object.getOwnPropertyDescriptor(browser, "serviceWorker");
  for (const name of names) Object.defineProperty(globalThis, name, { configurable: true, value: globals[name] });
  Object.defineProperty(browser, "serviceWorker", {
    configurable: true,
    value: {
      ready: Promise.resolve(registration),
      controller: {},
      getRegistration: () => Promise.resolve(registration),
      getRegistrations: () => Promise.resolve([registration]),
      register: () => Promise.resolve(registration),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    },
  });
  localStorage.removeItem(closedKey);
  onTestFinished(() => {
    localStorage.removeItem(closedKey);
    names.forEach((name, i) => {
      const was = before[i];
      if (was === undefined) Reflect.deleteProperty(globalThis, name);
      else Object.defineProperty(globalThis, name, was);
    });
    if (workers === undefined) Reflect.deleteProperty(browser, "serviceWorker");
    else Object.defineProperty(browser, "serviceWorker", workers);
  });
  return seen;
}

/** The prompts over the notifications: one while Web Push is offered, none otherwise. */
const prompts = () => main().queryAllByRole("region", { name: /^Get your notifications on this device/ });

/** Opens the notifications on a server that sends Web Push, as a browser opens the app, and waits for their list. */
async function opened() {
  const on = fakeApp({ viewer: "yuki", push: true });
  const subscribed = on.fake.script("PUT /push/subscription");
  const app = await renderApp("/notifications", { on });
  expect(await main().findByRole("feed", { name: "Notifications" })).toBeDefined();
  expect(main().getAllByRole("tab").length).toBeGreaterThan(0);
  return { app, subscribed };
}

test("a reader who chose Not now is not offered Web Push again when the app opens again in this browser", async () => {
  const seen = browserWithPush();
  // The prompt shows with the notifications when it shows at all: the list in place decides, here and below.
  const first = await opened();
  expect(prompts()).toHaveLength(1);

  await first.app.user.click(within(prompts()[0] ?? document.body).getByRole("button", { name: "Not now" }));
  await waitFor(() => {
    if (prompts().length > 0) throw new Error("test: the prompt is still there");
  });

  // The app opened again, on a server that knows the reader as the first did.
  cleanup();
  const again = await opened();
  expect(prompts()).toHaveLength(0);
  expect(main().queryAllByRole("button", { name: "Turn on" })).toHaveLength(0);
  expect(seen).toEqual({ asked: 0, subscribed: 0 });
  expect([...first.subscribed.seen, ...again.subscribed.seen]).toHaveLength(0);
});
