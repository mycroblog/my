// SPDX-License-Identifier: AGPL-3.0-only
// Web Push in the preferences through the whole app (web-client/README.md T4, FR-206): MSW plays the server, and a stand-in
// for the browser's Push API (jsdom has none) plays the Service Worker registration's PushManager and the
// Notifications API.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import type { PushSubscriptionRequest } from "./api/push";
import { pushServerKey } from "./test/handlers";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";

function keyBytes(text: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(text.replaceAll("-", "+").replaceAll("_", "/")), (c) => c.charCodeAt(0));
}

/** A browser subscription to `endpoint` made with `key`. */
function browserSubscription(endpoint: string, key: string) {
  return {
    endpoint,
    options: { applicationServerKey: keyBytes(key).buffer, userVisibleOnly: true },
    toJSON: () => ({ endpoint, keys: { p256dh: "browser-p256dh", auth: "browser-auth" } }),
    unsubscribe: vi.fn(() => Promise.resolve(true)),
  };
}

/** Plays a browser with the Push API whose user answers the permission prompt with `answer`. */
function pushBrowser({
  permission = "default",
  answer = "granted",
}: { permission?: NotificationPermission; answer?: NotificationPermission } = {}) {
  let current: ReturnType<typeof browserSubscription> | null = null;
  const pushManager = {
    getSubscription: vi.fn(() => Promise.resolve(current)),
    subscribe: vi.fn((options: PushSubscriptionOptionsInit) => {
      current = browserSubscription("https://push.example.net/send/1", pushServerKey);
      expect(options.userVisibleOnly).toBe(true);
      expect(new Uint8Array(options.applicationServerKey as ArrayBuffer)).toEqual(keyBytes(pushServerKey));
      return Promise.resolve(current);
    }),
  };
  const notifications = { permission, requestPermission: vi.fn(() => Promise.resolve(answer)) };
  vi.stubGlobal("PushManager", function PushManager() {});
  vi.stubGlobal("Notification", notifications);
  Object.defineProperty(navigator, "serviceWorker", {
    configurable: true,
    value: { ready: Promise.resolve({ pushManager }) },
  });
  return {
    pushManager,
    notifications,
    get current() {
      return current;
    },
    /** Starts with a subscription that the browser made with `key`. */
    subscribed(key: string) {
      current = browserSubscription("https://push.example.net/send/old", key);
      return current;
    },
  };
}

/** The server: an instance that sends Web Push, and the grant's subscription with the requests it received. */
function pushServer(initial: Schemas["PushSubscription"] | null = null) {
  let saved = initial;
  const puts: PushSubscriptionRequest[] = [];
  let deletes = 0;
  server.use(
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, push_server_key: pushServerKey })),
    http.get(`${apiBase}/push/subscription`, ({ request }) =>
      withViewer(request, () =>
        saved === null
          ? HttpResponse.json(
              { type: "about:blank", title: "Not Found", status: 404 },
              { status: 404, headers: { "Content-Type": "application/problem+json" } },
            )
          : HttpResponse.json(saved),
      ),
    ),
    http.put(`${apiBase}/push/subscription`, async ({ request }) => {
      const body = (await request.clone().json()) as PushSubscriptionRequest;
      puts.push(body);
      saved = {
        id: "01900000-0000-7000-8000-0000000000f1",
        endpoint: body.endpoint,
        alerts: body.alerts,
        server_key: pushServerKey,
      };
      return withViewer(request, () => HttpResponse.json(saved));
    }),
    http.delete(`${apiBase}/push/subscription`, ({ request }) =>
      withViewer(request, () => {
        deletes++;
        saved = null;
        return new HttpResponse(null, { status: 204 });
      }),
    ),
  );
  return {
    puts,
    get deletes() {
      return deletes;
    },
  };
}

/** Every kind that a user without moderator permissions chooses from, in the order of the checkboxes. */
const everyUserType = [
  "mention",
  "follow",
  "follow_request",
  "follow_accepted",
  "boost",
  "favourite",
  "reaction",
  "quote",
  "service_closure",
  "account_moved",
  "moderation_notice",
  "appeal_decided",
  "report_resolved",
  "relationships_severed",
  "custom_emoji_request_decided",
  "supporter_payment_failed",
  "supporter_subscription_ended",
  "supporter_plan_retired",
  "supporter_service_closure",
];

beforeEach(() => {
  server.use(signedInHandler);
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "serviceWorker");
});

async function pushSection() {
  return screen.findByRole("region", { name: "Push notifications" });
}

describe("push notifications", () => {
  test("are not offered when the server sends no Web Push", async () => {
    pushBrowser();
    await renderApp({ path: "/settings/preferences" });

    expect(await screen.findByRole("heading", { level: 1, name: "Preferences" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Push notifications" })).not.toBeInTheDocument();
  });

  test("say why a browser without the Push API cannot have them", async () => {
    pushServer();
    await renderApp({ path: "/settings/preferences" });

    const section = await pushSection();
    expect(within(section).getByText(/This browser cannot receive push notifications/)).toBeInTheDocument();
    expect(within(section).queryByRole("switch")).not.toBeInTheDocument();
  });

  test("say how to allow them when the browser blocks notifications for the site", async () => {
    pushBrowser({ permission: "denied" });
    pushServer();
    await renderApp({ path: "/settings/preferences" });

    const section = await pushSection();
    expect(within(section).getByText(/Notifications are blocked for this site/)).toBeInTheDocument();
    expect(within(section).queryByRole("switch")).not.toBeInTheDocument();
  });

  test("turning them on asks for the permission, subscribes with the server's key and registers every kind", async () => {
    const browser = pushBrowser();
    const api = pushServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const section = await pushSection();
    const toggle = within(section).getByRole("switch", { name: "Notify me on this device" });
    expect(toggle).not.toBeChecked();
    expect(within(section).queryByRole("group", { name: "Notify me of" })).not.toBeInTheDocument();

    await user.click(toggle);

    await waitFor(() => {
      expect(api.puts).toHaveLength(1);
    });
    expect(browser.notifications.requestPermission).toHaveBeenCalledOnce();
    expect(browser.pushManager.subscribe).toHaveBeenCalledOnce();
    expect(api.puts[0]).toEqual({
      endpoint: "https://push.example.net/send/1",
      keys: { p256dh: "browser-p256dh", auth: "browser-auth" },
      alerts: everyUserType,
    });
    await waitFor(() => {
      expect(toggle).toBeChecked();
    });
    const kinds = within(section).getByRole("group", { name: "Notify me of" });
    expect(within(kinds).getAllByRole("checkbox")).toHaveLength(12);
    expect(within(kinds).getByRole("checkbox", { name: "Boosts" })).toBeChecked();
  });

  test("unchecking a kind registers the subscription without it", async () => {
    const browser = pushBrowser({ permission: "granted" });
    browser.subscribed(pushServerKey);
    const api = pushServer({
      id: "01900000-0000-7000-8000-0000000000f1",
      endpoint: "https://push.example.net/send/old",
      alerts: ["mention", "boost"],
      server_key: pushServerKey,
    });
    const { user } = await renderApp({ path: "/settings/preferences" });
    const section = await pushSection();
    expect(within(section).getByRole("switch", { name: "Notify me on this device" })).toBeChecked();
    const boosts = within(section).getByRole("checkbox", { name: "Boosts" });
    expect(boosts).toBeChecked();
    expect(within(section).getByRole("checkbox", { name: "Favourites" })).not.toBeChecked();

    await user.click(boosts);

    await waitFor(() => {
      expect(api.puts).toEqual([
        {
          endpoint: "https://push.example.net/send/old",
          keys: { p256dh: "browser-p256dh", auth: "browser-auth" },
          alerts: ["mention"],
        },
      ]);
    });
    expect(browser.notifications.requestPermission).not.toHaveBeenCalled();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(boosts).not.toBeChecked();
    });
  });

  test("a subscription made with another key is replaced", async () => {
    const browser = pushBrowser({ permission: "granted" });
    const old = browser.subscribed(
      "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
    );
    const api = pushServer();
    const { user } = await renderApp({ path: "/settings/preferences" });

    await user.click(within(await pushSection()).getByRole("switch", { name: "Notify me on this device" }));

    await waitFor(() => {
      expect(api.puts).toHaveLength(1);
    });
    expect(old.unsubscribe).toHaveBeenCalledOnce();
    expect(api.puts[0]?.endpoint).toBe("https://push.example.net/send/1");
  });

  test("turning them off deletes the server's subscription and the browser's", async () => {
    const browser = pushBrowser({ permission: "granted" });
    const current = browser.subscribed(pushServerKey);
    const api = pushServer({
      id: "01900000-0000-7000-8000-0000000000f1",
      endpoint: "https://push.example.net/send/old",
      alerts: ["mention"],
      server_key: pushServerKey,
    });
    const { user } = await renderApp({ path: "/settings/preferences" });
    const toggle = within(await pushSection()).getByRole("switch", { name: "Notify me on this device" });

    await user.click(toggle);

    await waitFor(() => {
      expect(api.deletes).toBe(1);
    });
    await waitFor(() => {
      expect(toggle).not.toBeChecked();
    });
    expect(current.unsubscribe).toHaveBeenCalledOnce();
  });

  test("stay off when the user does not allow notifications", async () => {
    const browser = pushBrowser({ answer: "denied" });
    const api = pushServer();
    const { user } = await renderApp({ path: "/settings/preferences" });
    const section = await pushSection();
    const toggle = within(section).getByRole("switch", { name: "Notify me on this device" });

    await user.click(toggle);

    expect(await within(section).findByRole("alert")).toHaveTextContent(
      "Push notifications are off because the browser did not allow notifications.",
    );
    expect(toggle).not.toBeChecked();
    expect(browser.pushManager.subscribe).not.toHaveBeenCalled();
    expect(api.puts).toEqual([]);
  });
});
