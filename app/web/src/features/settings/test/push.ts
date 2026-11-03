// SPDX-License-Identifier: AGPL-3.0-only
// Web Push in the tests of the settings feature (FR-206): a stand-in for the browser's Push API, which jsdom does not
// have, playing the Service Worker registration's PushManager and the Notifications API, and MSW playing the server.
import { http, HttpResponse } from "msw";
import { expect, vi } from "vitest";
import { instanceFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";
import { server } from "../../../test/msw/server";
import type { PushSubscriptionRequest } from "../api/push";
import { pushServerKey } from "./handlers";

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
export function pushBrowser({
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
    /** Starts with a subscription that the browser made with `key`. */
    subscribed(key: string) {
      current = browserSubscription("https://push.example.net/send/old", key);
      return current;
    },
  };
}

/** The server: an instance that sends Web Push, and the grant's subscription with the requests it received. */
export function pushServer(initial: Schemas["PushSubscription"] | null = null) {
  let saved = initial;
  const puts: PushSubscriptionRequest[] = [];
  let deletes = 0;
  let reads = 0;
  server.use(
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, push_server_key: pushServerKey })),
    http.get(`${apiBase}/push/subscription`, ({ request }) =>
      withViewer(request, () => {
        reads++;
        return saved === null
          ? HttpResponse.json(
              { type: "about:blank", title: "Not Found", status: 404 },
              { status: 404, headers: { "Content-Type": "application/problem+json" } },
            )
          : HttpResponse.json(saved);
      }),
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
    get reads() {
      return reads;
    },
  };
}
